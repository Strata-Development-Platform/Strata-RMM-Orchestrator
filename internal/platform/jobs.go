package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type jobRequest struct {
	Type           string                 `json:"type"`
	DeviceIDs      []string               `json:"device_ids"`
	Payload        map[string]interface{} `json:"payload"`
	Priority       int                    `json:"priority"`
	MaxRetries     int                    `json:"max_retries"`
	IdempotencyKey string                 `json:"idempotency_key,omitempty"`
	ScheduleAt     string                 `json:"schedule_at,omitempty"`
}

func (s *APIServer) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	clientID := r.URL.Query().Get("client_id")
	if mspID == "" || clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and client_id required"})
		return
	}

	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}
	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	var req jobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Type == "" || len(req.DeviceIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type and device_ids required"})
		return
	}

	if req.MaxRetries <= 0 {
		req.MaxRetries = 3
	}
	if req.MaxRetries > 10 {
		req.MaxRetries = 10
	}

	db := s.requestDB(r)
	deviceIDs := append([]string(nil), req.DeviceIDs...)
	sort.Strings(deviceIDs)
	fingerprintJSON, err := json.Marshal(struct {
		Type       string                 `json:"type"`
		ClientID   string                 `json:"client_id"`
		DeviceIDs  []string               `json:"device_ids"`
		Payload    map[string]interface{} `json:"payload"`
		Priority   int                    `json:"priority"`
		MaxRetries int                    `json:"max_retries"`
		ScheduleAt string                 `json:"schedule_at"`
	}{req.Type, clientID, deviceIDs, req.Payload, req.Priority, req.MaxRetries, req.ScheduleAt})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid job payload"})
		return
	}
	requestHash := fmt.Sprintf("%x", sha256.Sum256(fingerprintJSON))

	// Check tenant-scoped idempotency and reject key reuse with a different request.
	if req.IdempotencyKey != "" {
		var existingID string
		var existingStatus string
		var existingHash string
		err := db.QueryRowContext(r.Context(), `
			SELECT id, status, COALESCE(request_hash, '')
			FROM jobs WHERE msp_id = $1 AND idempotency_key = $2
		`, mspID, req.IdempotencyKey).Scan(&existingID, &existingStatus, &existingHash)
		if err == nil {
			if existingHash != requestHash {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency key already used for a different request"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"job_id": existingID, "status": existingStatus, "duplicate": true,
			})
			return
		}
	}

	id := uuid.New().String()
	correlationID := uuid.New().String()
	payloadJSON, err := json.Marshal(req.Payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid job payload"})
		return
	}

	scheduledFor := time.Now()
	if req.ScheduleAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ScheduleAt); err == nil {
			scheduledFor = t
		}
	}
	expiresAt := scheduledFor.Add(72 * time.Hour)

	_, err = db.ExecContext(r.Context(), `
		INSERT INTO jobs (id, msp_id, client_id, created_by, type, status, priority,
		                  payload, idempotency_key, max_retries, max_devices, expires_at,
		                  correlation_id, scheduled_for, request_hash)
		VALUES ($1, $2, $3, $4, $5, 'queued', $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, id, mspID, clientID, "api", req.Type, req.Priority,
		payloadJSON, nullIfEmpty(req.IdempotencyKey), req.MaxRetries, len(req.DeviceIDs),
		expiresAt, correlationID, scheduledFor, requestHash)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Create targets and outbox entries
	for _, deviceID := range req.DeviceIDs {
		var agentID string
		if err := db.QueryRowContext(r.Context(), `
			SELECT agent_id::text
			FROM devices
			WHERE id::text = $1 AND msp_id = $2 AND client_id = $3
			  AND agent_id IS NOT NULL AND status <> 'disabled'
		`, deviceID, mspID, clientID).Scan(&agentID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "one or more devices are unavailable or outside the authorized client"})
			return
		}
		targetID := uuid.New().String()
		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO job_targets (id, job_id, device_id, agent_id, msp_id, status)
			VALUES ($1, $2, $3, $4, $5, 'queued')
		`, targetID, id, deviceID, agentID, mspID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating job target"})
			return
		}

		// Write outbox entry for each target
		outboxPayload, err := json.Marshal(map[string]interface{}{
			"job_id":         id,
			"target_id":      targetID,
			"device_id":      deviceID,
			"agent_id":       agentID,
			"msp_id":         mspID,
			"correlation_id": correlationID,
			"attempt":        1,
			"issued_at":      time.Now().UTC().Format(time.RFC3339),
			"expires_at":     expiresAt.UTC().Format(time.RFC3339),
			"type":           req.Type,
			"payload":        req.Payload,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encoding job target"})
			return
		}
		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
			VALUES (gen_random_uuid(), $1, $2, 'job.dispatch', $3, $4)
		`, mspID, id, outboxPayload, scheduledFor); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating durable dispatch event"})
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"job_id":     id,
		"status":     "queued",
		"type":       req.Type,
		"device_ids": req.DeviceIDs,
	})
}

func (s *APIServer) handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")

	var mspID, clientID, jobType, status string
	var createdAt, expiresAt time.Time
	db := s.requestDB(r)
	err := db.QueryRowContext(r.Context(), `
		SELECT msp_id, client_id, type, status, created_at, expires_at
		FROM jobs WHERE id = $1
	`, jobID).Scan(&mspID, &clientID, &jobType, &status, &createdAt, &expiresAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := db.QueryContext(r.Context(), `
		SELECT device_id, status, COALESCE(error_message, ''), started_at, completed_at, attempt, exit_code
		FROM job_targets WHERE job_id = $1 ORDER BY created_at ASC
	`, jobID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var targets []map[string]interface{}
	for rows.Next() {
		var deviceID, targetStatus, errMsg string
		var startedAt, completedAt *time.Time
		var attempt, exitCode int
		if err := rows.Scan(&deviceID, &targetStatus, &errMsg, &startedAt, &completedAt, &attempt, &exitCode); err != nil {
			continue
		}
		targets = append(targets, map[string]interface{}{
			"device_id": deviceID, "status": targetStatus,
			"error_message": errMsg, "attempt": attempt, "exit_code": exitCode,
			"started_at":   nullTimeStr(startedAt),
			"completed_at": nullTimeStr(completedAt),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": jobID, "msp_id": mspID, "client_id": clientID,
		"type": jobType, "status": status,
		"created_at": createdAt.UTC().Format(time.RFC3339),
		"targets":    targets,
	})
}

func (s *APIServer) handleListJobs(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	clientID := r.URL.Query().Get("client_id")
	statusFilter := r.URL.Query().Get("status")
	typeFilter := r.URL.Query().Get("type")

	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	query := `SELECT j.id, j.type, j.status, j.priority, j.max_devices,
	                  j.completed_count, j.failed_count, j.created_at,
	                  COALESCE(j.correlation_id, '')
	           FROM jobs j WHERE j.msp_id = $1`
	args := []interface{}{mspID}
	argIdx := 2

	if clientID != "" {
		query += fmt.Sprintf(" AND j.client_id = $%d", argIdx)
		args = append(args, clientID)
		argIdx++
	}
	if statusFilter != "" {
		query += fmt.Sprintf(" AND j.status = $%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}
	if typeFilter != "" {
		query += fmt.Sprintf(" AND j.type = $%d", argIdx)
		args = append(args, typeFilter)
	}
	query += " ORDER BY j.created_at DESC LIMIT 100"

	rows, err := s.requestDB(r).QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = rows.Close() }()

	var jobs []map[string]interface{}
	for rows.Next() {
		var id, jobType, status, correlationID string
		var priority, maxDev, compCount, failCount int
		var createdAt time.Time
		if err := rows.Scan(&id, &jobType, &status, &priority, &maxDev, &compCount, &failCount, &createdAt, &correlationID); err != nil {
			continue
		}
		jobs = append(jobs, map[string]interface{}{
			"id": id, "type": jobType, "status": status, "priority": priority,
			"max_devices": maxDev, "completed_count": compCount, "failed_count": failCount,
			"correlation_id": correlationID,
			"created_at":     createdAt.UTC().Format(time.RFC3339),
		})
	}
	if jobs == nil {
		jobs = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
}

func (s *APIServer) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")

	var mspID, status string
	db := s.requestDB(r)
	err := db.QueryRowContext(r.Context(), `SELECT msp_id, status FROM jobs WHERE id = $1`, jobID).Scan(&mspID, &status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	if err := TransitionJob(status, "cancelled"); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	res, err := db.ExecContext(r.Context(), `
		UPDATE jobs SET status = 'cancelled', completed_at = NOW(), cancelled_at=NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'queued', 'dispatched', 'running')
	`, jobID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cancelling job"})
		return
	}
	affected, err := res.RowsAffected()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "checking cancellation"})
		return
	}
	if affected == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job cannot be cancelled in current state"})
		return
	}

	rows, err := db.QueryContext(r.Context(), `
		UPDATE job_targets SET status = 'cancelled', completed_at=NOW(), lease_owner=NULL, lease_expires=NULL
		WHERE job_id = $1 AND status IN ('pending', 'queued', 'dispatched', 'running')
		RETURNING id::text, COALESCE(agent_id,'')
	`, jobID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cancelling job targets"})
		return
	}
	type cancelledTarget struct{ id, agentID string }
	var targets []cancelledTarget
	for rows.Next() {
		var target cancelledTarget
		if err := rows.Scan(&target.id, &target.agentID); err != nil {
			_ = rows.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reading cancelled targets"})
			return
		}
		targets = append(targets, target)
	}
	rowsErr := rows.Err()
	if err := rows.Close(); err != nil || rowsErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reading cancelled targets"})
		return
	}
	for _, target := range targets {
		if target.agentID == "" || s.nats == nil {
			continue
		}
		payload, err := json.Marshal(map[string]string{"job_id": jobID, "target_id": target.id})
		if err != nil {
			continue
		}
		subject := fmt.Sprintf("tenant.%s.cmd.%s.cancel", mspID, target.agentID)
		if err := s.nats.Publish(subject, payload); err != nil {
			s.logger.Warn("publish job cancellation", zap.String("job_id", jobID), zap.String("target_id", target.id), zap.Error(err))
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *APIServer) handleRetryJobTargets(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	targetID := r.URL.Query().Get("target_id")

	var mspID, status string
	db := s.requestDB(r)
	err := db.QueryRowContext(r.Context(), `SELECT msp_id, status FROM jobs WHERE id = $1`, jobID).Scan(&mspID, &status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	if targetID != "" {
		_, err = db.ExecContext(r.Context(), `
			UPDATE job_targets SET status = 'queued', retry_count = retry_count + 1, next_retry_at = NOW() + INTERVAL '30 seconds'
			WHERE id = $1 AND job_id = $2 AND status IN ('failed', 'expired')
		`, targetID, jobID)
	} else {
		_, err = db.ExecContext(r.Context(), `
			UPDATE job_targets SET status = 'queued', retry_count = retry_count + 1, next_retry_at = NOW() + INTERVAL '30 seconds'
			WHERE job_id = $1 AND status IN ('failed', 'expired') AND retry_count < (SELECT max_retries FROM jobs WHERE id = $1)
		`, jobID)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scheduling retry"})
		return
	}
	if _, err := db.ExecContext(r.Context(), `UPDATE jobs SET status = 'queued', updated_at = NOW() WHERE id = $1`, jobID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "updating job"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "retry_scheduled"})
}

func (s *APIServer) handleListDeviceJobs(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	db := s.requestDB(r)
	var clientID string
	if err := db.QueryRowContext(r.Context(), `SELECT client_id::text FROM devices WHERE id::text = $1`, deviceID).Scan(&clientID); err != nil {
		writeAuthorizationDenied(w)
		return
	}
	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	rows, err := db.QueryContext(r.Context(), `
		SELECT j.id, j.type, j.status, jt.status as target_status, jt.attempt,
		       jt.exit_code, jt.completed_at, j.created_at
		FROM job_targets jt JOIN jobs j ON jt.job_id = j.id
		WHERE jt.device_id = $1
		ORDER BY jt.created_at DESC LIMIT 50
	`, deviceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = rows.Close() }()

	var jobs []map[string]interface{}
	for rows.Next() {
		var id, jobType, jobStatus, targetStatus string
		var attempt, exitCode int
		var completedAt, createdAt *time.Time
		if err := rows.Scan(&id, &jobType, &jobStatus, &targetStatus, &attempt, &exitCode, &completedAt, &createdAt); err != nil {
			continue
		}
		jobs = append(jobs, map[string]interface{}{
			"job_id": id, "type": jobType, "job_status": jobStatus,
			"target_status": targetStatus, "attempt": attempt, "exit_code": exitCode,
			"completed_at": nullTimeStr(completedAt), "created_at": nullTimeStr(createdAt),
		})
	}
	if jobs == nil {
		jobs = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
}

func (s *APIServer) handleListJobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	db := s.requestDB(r)
	var mspID string
	if err := db.QueryRowContext(r.Context(), `SELECT msp_id::text FROM jobs WHERE id = $1`, jobID).Scan(&mspID); err != nil {
		writeAuthorizationDenied(w)
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := db.QueryContext(r.Context(), `
		SELECT id, event_type, schema_version, payload::text, created_at, published_at IS NOT NULL as is_published
		FROM job_outbox WHERE aggregate_id = $1 ORDER BY created_at ASC
	`, jobID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = rows.Close() }()

	var events []map[string]interface{}
	for rows.Next() {
		var id, eventType, payloadStr string
		var schemaVersion int
		var createdAt time.Time
		var isPublished bool
		if err := rows.Scan(&id, &eventType, &schemaVersion, &payloadStr, &createdAt, &isPublished); err != nil {
			continue
		}
		events = append(events, map[string]interface{}{
			"id": id, "event_type": eventType, "schema_version": schemaVersion,
			"created_at": createdAt.UTC().Format(time.RFC3339), "published": isPublished,
		})
	}
	if events == nil {
		events = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"events": events})
}

func nullTimeStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

type ScheduleOrchestrator struct {
	nc     *nats.Conn
	db     *sql.DB
	logger *zap.Logger
}

func NewScheduleOrchestrator(nc *nats.Conn, db *sql.DB, logger *zap.Logger) *ScheduleOrchestrator {
	return &ScheduleOrchestrator{nc: nc, db: db, logger: logger}
}

func (so *ScheduleOrchestrator) ExecuteSchedule(scheduleID string) error {
	db := so.db

	var schedule struct {
		ID             string          `json:"id"`
		TenantID       string          `json:"tenant_id"`
		ScriptID       string          `json:"script_id"`
		ScheduleType   string          `json:"schedule_type"`
		ScheduleParams json.RawMessage `json:"schedule_params"`
		TargetDevices  json.RawMessage `json:"target_devices"`
		MaxRetries     int             `json:"max_retries"`
		RetryInterval  int             `json:"retry_interval"`
		NextRunAt      time.Time       `json:"next_run_at"`
	}

	var params, devices []byte
	err := db.QueryRowContext(context.Background(), `
		SELECT id, tenant_id, script_id, schedule_type, schedule_params, target_devices,
		       max_retries, retry_interval, next_run_at
		FROM schedules WHERE id = $1 AND status = 'active'
	`, scheduleID).Scan(&schedule.ID, &schedule.TenantID, &schedule.ScriptID,
		&schedule.ScheduleType, &params, &devices, &schedule.MaxRetries,
		&schedule.RetryInterval, &schedule.NextRunAt)
	if err != nil {
		return fmt.Errorf("schedule not found or not active: %w", err)
	}

	if params != nil && len(params) > 0 {
		schedule.ScheduleParams = params
	}
	if devices != nil && len(devices) > 0 {
		schedule.TargetDevices = devices
	}

	var deviceIDs []string
	if err := json.Unmarshal(devices, &deviceIDs); err != nil {
		return fmt.Errorf("failed to parse device list: %w", err)
	}

	if len(deviceIDs) == 0 {
		return fmt.Errorf("no devices to execute schedule on")
	}

	var script struct {
		Name     string `json:"name"`
		Language string `json:"language"`
		Content  string `json:"content"`
		Timeout  int    `json:"timeout_sec"`
	}

	err = db.QueryRowContext(context.Background(), `
		SELECT name, language, content, timeout_sec FROM scripts WHERE id = $1
	`, schedule.ScriptID).Scan(&script.Name, &script.Language, &script.Content, &script.Timeout)
	if err != nil {
		return fmt.Errorf("script not found: %w", err)
	}

	for _, deviceID := range deviceIDs {
		execID := uuid.New().String()

		_, err := db.ExecContext(context.Background(), `
			INSERT INTO schedule_device_executions (id, schedule_id, device_id, status)
			VALUES ($1, $2, $3, 'pending')
			ON CONFLICT (schedule_id, device_id) DO UPDATE SET status = 'pending',
			           retry_count = 0, started_at = NULL, completed_at = NULL
		`, execID, schedule.ID, deviceID)
		if err != nil {
			so.logger.Warn("create schedule execution", zap.Error(err))
			continue
		}

		cmdPayload, _ := json.Marshal(map[string]interface{}{
			"type":         "script_exec",
			"execution_id": execID,
			"schedule_id":  schedule.ID,
			"device_id":    deviceID,
			"language":     script.Language,
			"content":      script.Content,
			"parameters":   schedule.ScheduleParams,
			"timeout":      script.Timeout,
			"max_retries":  schedule.MaxRetries,
			"retry_count":  0,
		})

		subject := fmt.Sprintf("tenant.%s.cmd.%s", schedule.TenantID, deviceID)
		if err := so.nc.Publish(subject, cmdPayload); err != nil {
			so.logger.Warn("publish schedule command", zap.Error(err))
			db.ExecContext(context.Background(), `
				UPDATE schedule_device_executions SET status = 'failed', stderr = 'NATS publish failed'
				WHERE id = $1
			`, execID)
		}
	}

	_, err = db.ExecContext(context.Background(), `
		UPDATE schedules SET last_run_at = NOW(), status = 'running'
		WHERE id = $1
	`, schedule.ID)
	if err != nil {
		so.logger.Warn("update schedule status", zap.Error(err))
	}

	return nil
}

func (so *ScheduleOrchestrator) ProcessScheduleDeviceResult(result map[string]interface{}) error {
	execID, ok := result["execution_id"].(string)
	if !ok || execID == "" {
		return fmt.Errorf("invalid execution_id")
	}

	_, _ = result["schedule_id"].(string)
	deviceID, _ := result["device_id"].(string)
	status, _ := result["status"].(string)
	stdout, _ := result["stdout"].(string)
	stderr, _ := result["stderr"].(string)
	exitCode, _ := result["exit_code"].(int)
	durationMs, _ := result["duration_ms"].(int64)

	now := time.Now()
	_, err := so.db.ExecContext(context.Background(), `
		UPDATE schedule_device_executions SET status = $1, stdout = $2, stderr = $3,
		           exit_code = $4, duration_ms = $5, completed_at = $6,
		           last_retry_at = CASE WHEN $7 THEN NOW() ELSE last_retry_at END
		WHERE id = $8
	`, status, stdout, stderr, exitCode, durationMs, now,
		status == "failed", execID)
	if err != nil {
		return fmt.Errorf("update execution: %w", err)
	}

	var scheduleID2 string
	err = so.db.QueryRowContext(context.Background(), `
		SELECT schedule_id FROM schedule_device_executions WHERE id = $1
	`, execID).Scan(&scheduleID2)
	if err != nil {
		return fmt.Errorf("get schedule id: %w", err)
	}

	var schedStatus string
	err = so.db.QueryRowContext(context.Background(), `
		SELECT status FROM schedules WHERE id = $1
	`, scheduleID2).Scan(&schedStatus)
	if err != nil {
		return fmt.Errorf("get schedule status: %w", err)
	}

	if schedStatus != "running" && schedStatus != "active" {
		return nil
	}

	var maxRetries, retryCount int
	var execStatus string
	err = so.db.QueryRowContext(context.Background(), `
		SELECT status, retry_count FROM schedule_device_executions WHERE id = $1
	`, execID).Scan(&execStatus, &retryCount)
	if err != nil {
		return fmt.Errorf("get execution status: %w", err)
	}

	if execStatus == "failed" && retryCount < maxRetries {
		_, err = so.db.ExecContext(context.Background(), `
			UPDATE schedule_device_executions SET status = 'pending', next_retry_at = NOW() + INTERVAL '30 seconds'
			WHERE id = $1
		`, execID)
		if err != nil {
			so.logger.Warn("schedule retry", zap.Error(err))
		}
		return nil
	}

	if execStatus == "failed" && retryCount >= maxRetries {
		so.logger.Info("schedule device execution max retries reached",
			zap.String("exec_id", execID), zap.String("device_id", deviceID),
			zap.String("schedule_id", scheduleID2), zap.Int("retry_count", retryCount))
	}

	if execStatus == "completed" {
		allCompleted, err := so.checkScheduleCompletion(scheduleID2)
		if err != nil {
			so.logger.Warn("check schedule completion", zap.Error(err))
		} else if allCompleted {
			so.logger.Info("schedule completed successfully",
				zap.String("schedule_id", scheduleID2))
		}
	}

	return nil
}

func (so *ScheduleOrchestrator) checkScheduleCompletion(scheduleID string) (bool, error) {
	var total, completed, failed int
	err := so.db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) as total,
		       SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed,
		       SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
		FROM schedule_device_executions WHERE schedule_id = $1
	`, scheduleID).Scan(&total, &completed, &failed)
	if err != nil {
		return false, err
	}

	if completed+failed == total && total > 0 {
		so.db.ExecContext(context.Background(), `
			UPDATE schedules SET status = 'completed', completed_at = NOW()
			WHERE id = $1
		`, scheduleID)
		return true, nil
	}

	return false, nil
}

func (so *ScheduleOrchestrator) ResumeSchedule(scheduleID string) error {
	var status string
	err := so.db.QueryRowContext(context.Background(), `
		SELECT status FROM schedules WHERE id = $1
	`, scheduleID).Scan(&status)
	if err != nil {
		return fmt.Errorf("schedule not found: %w", err)
	}

	if status != "paused" {
		return fmt.Errorf("schedule must be paused to resume")
	}

	_, err = so.db.ExecContext(context.Background(), `
		UPDATE schedules SET status = 'active', started_at = NOW()
		WHERE id = $1
	`, scheduleID)
	if err != nil {
		return fmt.Errorf("update schedule status: %w", err)
	}

	_, err = so.db.ExecContext(context.Background(), `
		UPDATE schedule_device_executions SET status = 'pending'
		WHERE schedule_id = $1 AND status = 'paused'
	`, scheduleID)
	if err != nil {
		return fmt.Errorf("resume device executions: %w", err)
	}

	return nil
}

func (so *ScheduleOrchestrator) PauseSchedule(scheduleID string) error {
	var status string
	err := so.db.QueryRowContext(context.Background(), `
		SELECT status FROM schedules WHERE id = $1
	`, scheduleID).Scan(&status)
	if err != nil {
		return fmt.Errorf("schedule not found: %w", err)
	}

	if status != "active" {
		return fmt.Errorf("schedule must be active to pause")
	}

	_, err = so.db.ExecContext(context.Background(), `
		UPDATE schedules SET status = 'paused', updated_at = NOW()
		WHERE id = $1
	`, scheduleID)
	if err != nil {
		return fmt.Errorf("update schedule status: %w", err)
	}

	_, err = so.db.ExecContext(context.Background(), `
		UPDATE schedule_device_executions SET status = 'paused'
		WHERE schedule_id = $1 AND status IN ('pending', 'running')
	`, scheduleID)
	if err != nil {
		return fmt.Errorf("pause device executions: %w", err)
	}

	return nil
}
