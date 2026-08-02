package platform

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Schedule struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	ScriptID       string          `json:"script_id"`
	ScriptName     string          `json:"script_name,omitempty"`
	ScheduleType   string          `json:"schedule_type"`
	ScheduleParams json.RawMessage `json:"schedule_params,omitempty"`
	TargetDevices  json.RawMessage `json:"target_devices,omitempty"`
	MaxRetries     int             `json:"max_retries"`
	RetryInterval  int             `json:"retry_interval"`
	Status         string          `json:"status"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	NextRunAt      *time.Time      `json:"next_run_at,omitempty"`
	LastRunAt      *time.Time      `json:"last_run_at,omitempty"`
	CreatedBy      *string         `json:"created_by,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type ScriptScheduleDeviceExecution struct {
	ID          string     `json:"id"`
	ScheduleID  string     `json:"schedule_id"`
	DeviceID    string     `json:"device_id"`
	Status      string     `json:"status"`
	RetryCount  int        `json:"retry_count"`
	Stdout      string     `json:"stdout,omitempty"`
	Stderr      string     `json:"stderr,omitempty"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	DurationMs  *int64     `json:"duration_ms,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
	LastRetryAt *time.Time `json:"last_retry_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (s *APIServer) handleCreateScriptSchedule(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	var req struct {
		Name           string          `json:"name"`
		Description    string          `json:"description"`
		ScriptID       string          `json:"script_id"`
		ScheduleType   string          `json:"schedule_type"`
		ScheduleParams json.RawMessage `json:"schedule_params"`
		DeviceIDs      []string        `json:"device_ids"`
		MaxRetries     int             `json:"max_retries"`
		RetryInterval  int             `json:"retry_interval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.ScriptID == "" || req.ScheduleType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, script_id, and schedule_type required"})
		return
	}

	validSchedules := map[string]bool{"now": true, "hourly": true, "daily": true, "weekly": true, "monthly": true}
	if !validSchedules[req.ScheduleType] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schedule_type: must be now, hourly, daily, weekly, or monthly"})
		return
	}

	if req.MaxRetries <= 0 {
		req.MaxRetries = 3
	}
	if req.MaxRetries > 10 {
		req.MaxRetries = 10
	}
	if req.RetryInterval <= 0 {
		req.RetryInterval = 60
	}

	if req.DeviceIDs == nil {
		req.DeviceIDs = []string{}
	}

	var scriptName string
	var scriptTenantID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT name, tenant_id FROM scripts WHERE id = $1
	`, req.ScriptID).Scan(&scriptName, &scriptTenantID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "script not found"})
		return
	}
	if scriptTenantID != tenantID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "script not found"})
		return
	}

	params := req.ScheduleParams
	if params == nil {
		params = json.RawMessage("null")
	}

	devicesJSON, err := json.Marshal(req.DeviceIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode device list"})
		return
	}

	var scheduleID string
	var createdAt, updatedAt time.Time
	err = s.requestDB(r).QueryRowContext(r.Context(), `
		INSERT INTO schedules (tenant_id, name, description, script_id, schedule_type, schedule_params, 
		                       target_devices, max_retries, retry_interval, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active')
		RETURNING id, created_at, updated_at
	`, tenantID, req.Name, req.Description, req.ScriptID, req.ScheduleType, params, devicesJSON,
		req.MaxRetries, req.RetryInterval).Scan(&scheduleID, &createdAt, &updatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	nextRunAt := time.Now()
	if req.ScheduleType != "now" {
		nextRunAt = calculateNextRun(req.ScheduleType, params)
	}

	_, err = s.requestDB(r).ExecContext(r.Context(), `
		UPDATE schedules SET next_run_at = $1 WHERE id = $2
	`, nextRunAt, scheduleID)
	if err != nil {
		s.logger.Warn("update schedule next run", zap.Error(err))
	}

	for _, deviceID := range req.DeviceIDs {
		_, err := s.requestDB(r).ExecContext(r.Context(), `
			INSERT INTO schedule_device_executions (schedule_id, device_id, status, retry_count)
			VALUES ($1, $2, 'pending', 0)
		`, scheduleID, deviceID)
		if err != nil {
			s.logger.Warn("create schedule device execution", zap.Error(err))
		}
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            scheduleID,
		"name":          req.Name,
		"script_id":     req.ScriptID,
		"script_name":   scriptName,
		"schedule_type": req.ScheduleType,
		"device_count":  len(req.DeviceIDs),
		"max_retries":   req.MaxRetries,
		"status":        "active",
		"next_run_at":   nextRunAt,
		"created_at":    createdAt,
	})
}

func (s *APIServer) handleListScriptSchedules(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	statusFilter := r.URL.Query().Get("status")

	query := `
		SELECT id, name, description, script_id, schedule_type, schedule_params, target_devices,
		       max_retries, retry_interval, status, started_at, completed_at, next_run_at,
		       last_run_at, created_by, created_at, updated_at
		FROM schedules WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}
	argIdx := 2

	if statusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}

	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := s.requestDB(r).QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var schedules []map[string]interface{}
	for rows.Next() {
		var id, name, desc, scriptID, schedType, status string
		var params, devices []byte
		var maxRetries, retryInterval int
		var startedAt, completedAt, nextRunAt, lastRunAt sql.NullTime
		var createdBy sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &desc, &scriptID, &schedType, &params, &devices,
			&maxRetries, &retryInterval, &status, &startedAt, &completedAt, &nextRunAt,
			&lastRunAt, &createdBy, &createdAt, &updatedAt); err != nil {
			continue
		}

		s := map[string]interface{}{
			"id":             id,
			"name":           name,
			"description":    desc,
			"script_id":      scriptID,
			"schedule_type":  schedType,
			"max_retries":    maxRetries,
			"retry_interval": retryInterval,
			"status":         status,
			"created_at":     createdAt,
			"updated_at":     updatedAt,
		}
		if createdBy.Valid {
			s["created_by"] = createdBy.String
		}
		if params != nil && len(params) > 0 {
			s["schedule_params"] = params
		}
		if devices != nil && len(devices) > 0 {
			s["target_devices"] = devices
		}
		if startedAt.Valid {
			s["started_at"] = startedAt.Time
		}
		if completedAt.Valid {
			s["completed_at"] = completedAt.Time
		}
		if nextRunAt.Valid {
			s["next_run_at"] = nextRunAt.Time
		}
		if lastRunAt.Valid {
			s["last_run_at"] = lastRunAt.Time
		}
		schedules = append(schedules, s)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"schedules": schedules})
}

func (s *APIServer) handleGetScriptSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")

	var schedule Schedule
	var createdBy sql.NullString
	var params, devices []byte
	var startedAt, completedAt, nextRunAt, lastRunAt sql.NullTime

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, tenant_id, name, description, script_id, schedule_type, schedule_params, target_devices,
		       max_retries, retry_interval, status, started_at, completed_at, next_run_at,
		       last_run_at, created_by, created_at, updated_at
		FROM schedules WHERE id = $1
	`, scheduleID).Scan(
		&schedule.ID, &schedule.TenantID, &schedule.Name, &schedule.Description,
		&schedule.ScriptID, &schedule.ScheduleType, &params, &devices,
		&schedule.MaxRetries, &schedule.RetryInterval, &schedule.Status,
		&startedAt, &completedAt, &nextRunAt, &lastRunAt, &createdBy,
		&schedule.CreatedAt, &schedule.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}
	if createdBy.Valid {
		schedule.CreatedBy = &createdBy.String
	}
	if params != nil && len(params) > 0 {
		schedule.ScheduleParams = params
	}
	if devices != nil && len(devices) > 0 {
		schedule.TargetDevices = devices
	}
	if startedAt.Valid {
		schedule.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		schedule.CompletedAt = &completedAt.Time
	}
	if nextRunAt.Valid {
		schedule.NextRunAt = &nextRunAt.Time
	}
	if lastRunAt.Valid {
		schedule.LastRunAt = &lastRunAt.Time
	}

	var scriptName string
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT name FROM scripts WHERE id = $1
	`, schedule.ScriptID).Scan(&scriptName); err == nil {
		schedule.ScriptName = scriptName
	}

	writeJSON(w, http.StatusOK, schedule)
}

func (s *APIServer) handleUpdateScriptSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")
	var req struct {
		Name           string          `json:"name"`
		Description    string          `json:"description"`
		DeviceIDs      []string        `json:"device_ids"`
		MaxRetries     *int            `json:"max_retries"`
		RetryInterval  *int            `json:"retry_interval"`
		Status         string          `json:"status"`
		ScheduleParams json.RawMessage `json:"schedule_params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	var currentStatus string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT status FROM schedules WHERE id = $1
	`, scheduleID).Scan(&currentStatus)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}

	if req.Status != "" && currentStatus != req.Status {
		validStatus := map[string]bool{"active": true, "paused": true, "completed": true, "failed": true}
		if !validStatus[req.Status] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}

		_, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE schedules SET status = $1, updated_at = NOW()
		WHERE id = $2
	`, req.Status, scheduleID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if req.Status == "active" {
			var params []byte
			var scheduleType string
			err := s.requestDB(r).QueryRowContext(r.Context(), `
				SELECT schedule_type, schedule_params FROM schedules WHERE id = $1
			`, scheduleID).Scan(&scheduleType, &params)
			if err == nil {
				nextRun := calculateNextRun(scheduleType, params)
				s.requestDB(r).ExecContext(r.Context(), `
					UPDATE schedules SET next_run_at = $1 WHERE id = $2
				`, nextRun, scheduleID)
				s.requestDB(r).ExecContext(r.Context(), `
					UPDATE schedule_device_executions SET status = 'pending', retry_count = 0
					WHERE schedule_id = $1 AND status IN ('completed', 'failed')
				`, scheduleID)
			}
		} else if req.Status == "completed" || req.Status == "failed" {
			s.requestDB(r).ExecContext(r.Context(), `
				UPDATE schedule_device_executions SET status = $1 WHERE schedule_id = $2
			`, req.Status, scheduleID)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":         scheduleID,
			"status":     req.Status,
			"updated_at": time.Now(),
		})
		return
	}

	updates := []string{}
	args := []interface{}{scheduleID}

	if req.Name != "" {
		updates = append(updates, "name = $1")
		args = append(args, req.Name)
	}
	if req.Description != "" {
		updates = append(updates, "description = $2")
		args = append(args, req.Description)
	}
	if req.MaxRetries != nil {
		maxRetries := *req.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 3
		}
		if maxRetries > 10 {
			maxRetries = 10
		}
		updates = append(updates, fmt.Sprintf("max_retries = $%d", len(args)))
		args = append(args, maxRetries)
	}
	if req.RetryInterval != nil {
		retryInterval := *req.RetryInterval
		if retryInterval <= 0 {
			retryInterval = 60
		}
		updates = append(updates, fmt.Sprintf("retry_interval = $%d", len(args)))
		args = append(args, retryInterval)
	}
	if req.ScheduleParams != nil {
		updates = append(updates, fmt.Sprintf("schedule_params = $%d", len(args)))
		args = append(args, req.ScheduleParams)
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "no updates"})
		return
	}

	args = append(args, scheduleID)
	query := fmt.Sprintf("UPDATE schedules SET %s, updated_at = NOW() WHERE id = $%d RETURNING id, updated_at",
		joinUpdates(updates), len(args))

	var updatedAt time.Time
	var id string
	err = s.requestDB(r).QueryRowContext(r.Context(), query, args...).Scan(&id, &updatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":         id,
		"updated_at": updatedAt,
	})
}

func (s *APIServer) handleDeleteScriptSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")

	var status string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT status FROM schedules WHERE id = $1
	`, scheduleID).Scan(&status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}

	if status == "active" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete active schedule, pause it first"})
		return
	}

	_, err = s.requestDB(r).ExecContext(r.Context(), `DELETE FROM schedules WHERE id = $1`, scheduleID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *APIServer) handlePreviewScriptSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")

	var schedule Schedule
	var params, devices []byte

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, tenant_id, name, description, script_id, schedule_type, schedule_params, target_devices,
		       max_retries, retry_interval, status, next_run_at, last_run_at
		FROM schedules WHERE id = $1
	`, scheduleID).Scan(
		&schedule.ID, &schedule.TenantID, &schedule.Name, &schedule.Description,
		&schedule.ScriptID, &schedule.ScheduleType, &params, &devices,
		&schedule.MaxRetries, &schedule.RetryInterval, &schedule.Status,
		&schedule.NextRunAt, &schedule.LastRunAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}
	if params != nil && len(params) > 0 {
		schedule.ScheduleParams = params
	}
	if devices != nil && len(devices) > 0 {
		schedule.TargetDevices = devices
	}

	var scriptName string
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT name FROM scripts WHERE id = $1
	`, schedule.ScriptID).Scan(&scriptName); err == nil {
		schedule.ScriptName = scriptName
	}

	var deviceIDs []string
	if err := json.Unmarshal(devices, &deviceIDs); err != nil {
		deviceIDs = []string{}
	}

	var executions []map[string]interface{}
	for _, deviceID := range deviceIDs {
		executions = append(executions, map[string]interface{}{
			"device_id":      deviceID,
			"status":         "pending",
			"retry_count":    0,
			"max_retries":    schedule.MaxRetries,
			"retry_interval": schedule.RetryInterval,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"schedule": map[string]interface{}{
			"id":             schedule.ID,
			"name":           schedule.Name,
			"description":    schedule.Description,
			"script_id":      schedule.ScriptID,
			"script_name":    schedule.ScriptName,
			"schedule_type":  schedule.ScheduleType,
			"max_retries":    schedule.MaxRetries,
			"retry_interval": schedule.RetryInterval,
			"status":         schedule.Status,
			"next_run_at":    schedule.NextRunAt,
			"last_run_at":    schedule.LastRunAt,
		},
		"preview": map[string]interface{}{
			"device_count": len(deviceIDs),
			"devices":      deviceIDs,
			"executions":   executions,
		},
	})
}

func (s *APIServer) handleListScriptScheduleDevices(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")
	statusFilter := r.URL.Query().Get("status")

	query := `
		SELECT id, schedule_id, device_id, status, retry_count, stdout, stderr, exit_code,
		       duration_ms, started_at, completed_at, next_retry_at, last_retry_at, created_at, updated_at
		FROM schedule_device_executions WHERE schedule_id = $1
	`
	args := []interface{}{scheduleID}
	argIdx := 2

	if statusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.requestDB(r).QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var executions []map[string]interface{}
	for rows.Next() {
		var id, schedID, deviceID, status string
		var retryCount int
		var stdout, stderr string
		var exitCode sql.NullInt64
		var dur sql.NullInt64
		var startedAt, completedAt, nextRetryAt, lastRetryAt sql.NullTime
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &schedID, &deviceID, &status, &retryCount, &stdout, &stderr,
			&exitCode, &dur, &startedAt, &completedAt, &nextRetryAt, &lastRetryAt,
			&createdAt, &updatedAt); err != nil {
			continue
		}

		exec := map[string]interface{}{
			"id":          id,
			"schedule_id": schedID,
			"device_id":   deviceID,
			"status":      status,
			"retry_count": retryCount,
			"stdout":      stdout,
			"stderr":      stderr,
			"created_at":  createdAt,
			"updated_at":  updatedAt,
		}
		if exitCode.Valid {
			exec["exit_code"] = int(exitCode.Int64)
		}
		if dur.Valid {
			exec["duration_ms"] = dur.Int64
		}
		if startedAt.Valid {
			exec["started_at"] = startedAt.Time
		}
		if completedAt.Valid {
			exec["completed_at"] = completedAt.Time
		}
		if nextRetryAt.Valid {
			exec["next_retry_at"] = nextRetryAt.Time
		}
		if lastRetryAt.Valid {
			exec["last_retry_at"] = lastRetryAt.Time
		}
		executions = append(executions, exec)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"executions": executions})
}

func (s *APIServer) handleRetryScriptScheduleDevice(w http.ResponseWriter, r *http.Request) {
	execID := r.PathValue("execID")

	var retryCount int
	var nextRetryAt sql.NullTime
	var status string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT retry_count, next_retry_at, status FROM schedule_device_executions WHERE id = $1
	`, execID).Scan(&retryCount, &nextRetryAt, &status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "execution not found"})
		return
	}

	var scheduleID string
	err = s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT schedule_id FROM schedule_device_executions WHERE id = $1
	`, execID).Scan(&scheduleID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}

	var maxRetries int
	var schedStatus string
	err = s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT max_retries, status FROM schedules WHERE id = $1
	`, scheduleID).Scan(&maxRetries, &schedStatus)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}

	if status != "failed" && status != "pending" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "can only retry failed or pending executions"})
		return
	}

	if retryCount >= maxRetries {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "max retries exceeded"})
		return
	}

	if schedStatus != "active" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schedule must be active"})
		return
	}

	_, err = s.requestDB(r).ExecContext(r.Context(), `
		UPDATE schedule_device_executions SET status = 'pending', retry_count = retry_count + 1,
		           next_retry_at = NOW() + INTERVAL '30 seconds', last_retry_at = NOW()
		WHERE id = $1
	`, execID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "retry_scheduled"})
}

func (s *APIServer) handleListScriptScheduleExecutions(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, schedule_id, device_id, status, retry_count, started_at, completed_at, created_at
		FROM schedule_device_executions sde
		JOIN schedules s ON sde.schedule_id = s.id
		WHERE s.tenant_id = $1
		ORDER BY sde.created_at DESC LIMIT 100
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var executions []map[string]interface{}
	for rows.Next() {
		var id, schedID, deviceID, status string
		var retryCount int
		var startedAt, completedAt, createdAt sql.NullTime

		if err := rows.Scan(&id, &schedID, &deviceID, &status, &retryCount, &startedAt, &completedAt, &createdAt); err != nil {
			continue
		}

		exec := map[string]interface{}{
			"id":          id,
			"schedule_id": schedID,
			"device_id":   deviceID,
			"status":      status,
			"retry_count": retryCount,
			"created_at":  createdAt.Time,
		}
		if startedAt.Valid {
			exec["started_at"] = startedAt.Time
		}
		if completedAt.Valid {
			exec["completed_at"] = completedAt.Time
		}
		executions = append(executions, exec)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"executions": executions})
}

func calculateNextRun(scheduleType string, params json.RawMessage) time.Time {
	now := time.Now()
	switch scheduleType {
	case "hourly":
		nextHour := now.Hour() + 1
		next := time.Date(now.Year(), now.Month(), now.Day(), nextHour, 0, 0, 0, now.Location())
		if next.Before(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	case "daily":
		var cfg struct {
			Time string `json:"time"`
		}
		if params != nil && len(params) > 0 {
			json.Unmarshal(params, &cfg)
		}
		if cfg.Time == "" {
			cfg.Time = "09:00"
		}
		t := parseTimeOfDay(cfg.Time)
		next := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
		if next.Before(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	case "weekly":
		var cfg struct {
			Day  string `json:"day"`
			Time string `json:"time"`
		}
		if params != nil && len(params) > 0 {
			json.Unmarshal(params, &cfg)
		}
		if cfg.Day == "" {
			cfg.Day = "monday"
		}
		if cfg.Time == "" {
			cfg.Time = "09:00"
		}
		dayNum := map[string]int{"monday": 1, "tuesday": 2, "wednesday": 3, "thursday": 4, "friday": 5, "saturday": 6, "sunday": 0}[cfg.Day]
		t := parseTimeOfDay(cfg.Time)
		daysUntil := (dayNum - int(now.Weekday()) + 7) % 7
		if daysUntil == 0 {
			daysUntil = 7
		}
		next := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location()).Add(time.Duration(daysUntil) * 24 * time.Hour)
		return next
	case "monthly":
		var cfg struct {
			Date int    `json:"date"`
			Time string `json:"time"`
		}
		if params != nil && len(params) > 0 {
			json.Unmarshal(params, &cfg)
		}
		if cfg.Date <= 0 {
			cfg.Date = 1
		}
		if cfg.Time == "" {
			cfg.Time = "09:00"
		}
		t := parseTimeOfDay(cfg.Time)
		next := time.Date(now.Year(), now.Month(), cfg.Date, t.Hour(), t.Minute(), 0, 0, now.Location())
		if next.Before(now) {
			next = next.AddDate(0, 1, 0)
		}
		return next
	default:
		return now
	}
}

func parseTimeOfDay(timeStr string) time.Time {
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return time.Date(0, 0, 0, 9, 0, 0, 0, time.UTC)
	}
	return t
}

func joinUpdates(updates []string) string {
	result := ""
	for i, u := range updates {
		if i > 0 {
			result += ", "
		}
		result += u
	}
	return result
}
