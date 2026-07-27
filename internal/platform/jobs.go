package platform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
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

	// Check idempotency
	if req.IdempotencyKey != "" {
		var existingID string
		var existingStatus string
		err := s.db.DB().QueryRowContext(r.Context(), `
			SELECT id, status FROM jobs WHERE msp_id = $1 AND idempotency_key = $2
		`, mspID, req.IdempotencyKey).Scan(&existingID, &existingStatus)
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"job_id": existingID, "status": existingStatus, "duplicate": true,
			})
			return
		}
	}

	id := uuid.New().String()
	correlationID := uuid.New().String()
	payloadJSON, _ := json.Marshal(req.Payload)

	scheduledFor := time.Now()
	if req.ScheduleAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ScheduleAt); err == nil {
			scheduledFor = t
		}
	}
	expiresAt := scheduledFor.Add(72 * time.Hour)

	db := s.requestDB(r)
	_, err := db.ExecContext(r.Context(), `
		INSERT INTO jobs (id, msp_id, client_id, created_by, type, status, priority,
		                  payload, idempotency_key, max_retries, max_devices, expires_at,
		                  correlation_id, scheduled_for)
		VALUES ($1, $2, $3, $4, $5, 'queued', $6, $7, $8, $9, $10, $11, $12, $13)
	`, id, mspID, clientID, "api", req.Type, req.Priority,
		payloadJSON, nullIfEmpty(req.IdempotencyKey), req.MaxRetries, len(req.DeviceIDs),
		expiresAt, correlationID, scheduledFor)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Create targets and outbox entries
	for _, deviceID := range req.DeviceIDs {
		targetID := uuid.New().String()
		db.ExecContext(r.Context(), `
			INSERT INTO job_targets (id, job_id, device_id, msp_id, status)
			VALUES ($1, $2, $3, $4, 'queued')
		`, targetID, id, deviceID, mspID)

		// Write outbox entry for each target
		outboxPayload, _ := json.Marshal(map[string]interface{}{
			"job_id":    id,
			"target_id": targetID,
			"device_id": deviceID,
			"type":      req.Type,
			"payload":   req.Payload,
		})
		db.ExecContext(r.Context(), `
			INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
			VALUES (gen_random_uuid(), $1, $2, 'job.dispatch', $3, $4)
		`, mspID, id, outboxPayload, scheduledFor)
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
			"started_at": nullTimeStr(startedAt),
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
		argIdx++
	}
	query += " ORDER BY j.created_at DESC LIMIT 100"

	rows, err := s.db.DB().QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

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
			"created_at": createdAt.UTC().Format(time.RFC3339),
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
	err := s.db.DB().QueryRowContext(r.Context(), `SELECT msp_id, status FROM jobs WHERE id = $1`, jobID).Scan(&mspID, &status)
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

	res, err := s.db.DB().ExecContext(r.Context(), `
		UPDATE jobs SET status = 'cancelled', completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'queued', 'dispatched')
	`, jobID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job cannot be cancelled in current state"})
		return
	}

	s.db.DB().ExecContext(r.Context(), `
		UPDATE job_targets SET status = 'cancelled' WHERE job_id = $1 AND status IN ('pending', 'queued', 'dispatched')
	`, jobID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *APIServer) handleRetryJobTargets(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	targetID := r.URL.Query().Get("target_id")

	var mspID, status string
	err := s.db.DB().QueryRowContext(r.Context(), `SELECT msp_id, status FROM jobs WHERE id = $1`, jobID).Scan(&mspID, &status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	if targetID != "" {
		s.db.DB().ExecContext(r.Context(), `
			UPDATE job_targets SET status = 'queued', retry_count = retry_count + 1, next_retry_at = NOW() + INTERVAL '30 seconds'
			WHERE id = $1 AND job_id = $2 AND status IN ('failed', 'expired')
		`, targetID, jobID)
	} else {
		s.db.DB().ExecContext(r.Context(), `
			UPDATE job_targets SET status = 'queued', retry_count = retry_count + 1, next_retry_at = NOW() + INTERVAL '30 seconds'
			WHERE job_id = $1 AND status IN ('failed', 'expired') AND retry_count < (SELECT max_retries FROM jobs WHERE id = $1)
		`, jobID)
	}

	s.db.DB().ExecContext(r.Context(), `UPDATE jobs SET status = 'queued', updated_at = NOW() WHERE id = $1`, jobID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "retry_scheduled"})
}

func (s *APIServer) handleListDeviceJobs(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")

	rows, err := s.db.DB().QueryContext(r.Context(), `
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
	defer rows.Close()

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

	rows, err := s.db.DB().QueryContext(r.Context(), `
		SELECT id, event_type, schema_version, payload::text, created_at, published_at IS NOT NULL as is_published
		FROM job_outbox WHERE aggregate_id = $1 ORDER BY created_at ASC
	`, jobID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

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
