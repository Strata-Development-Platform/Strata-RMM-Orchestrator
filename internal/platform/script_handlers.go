package platform

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type ScriptEngine struct {
	nc     *nats.Conn
	db     *sql.DB
	logger *zap.Logger
}

func NewScriptEngine(nc *nats.Conn, db *sql.DB, logger *zap.Logger) *ScriptEngine {
	return &ScriptEngine{nc: nc, db: db, logger: logger}
}

type Script struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Language    string          `json:"language"`
	Content     string          `json:"content"`
	Parameters  json.RawMessage `json:"parameters"`
	TimeoutSec  int             `json:"timeout_sec"`
	IsPublic    bool            `json:"is_public"`
	CreatedBy   *string         `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type ScriptExecution struct {
	ID          string          `json:"id"`
	ScriptID    *string         `json:"script_id,omitempty"`
	TenantID    string          `json:"tenant_id"`
	DeviceID    string          `json:"device_id"`
	TriggeredBy *string         `json:"triggered_by,omitempty"`
	Status      string          `json:"status"`
	Stdout      string          `json:"stdout,omitempty"`
	Stderr      string          `json:"stderr,omitempty"`
	ExitCode    *int            `json:"exit_code,omitempty"`
	DurationMs  *int64          `json:"duration_ms,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

func (s *APIServer) handleCreateScript(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Language    string          `json:"language"`
		Content     string          `json:"content"`
		Parameters  json.RawMessage `json:"parameters"`
		TimeoutSec  int             `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, content, and language required"})
		return
	}

	validLangs := map[string]bool{"powershell": true, "bash": true, "python": true, "batch": true}
	if !validLangs[req.Language] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid language"})
		return
	}

	if req.TimeoutSec <= 0 {
		req.TimeoutSec = 300
	}
	if req.Parameters == nil {
		req.Parameters = json.RawMessage("[]")
	}

	var scriptID string
	var createdAt, updatedAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		INSERT INTO scripts (tenant_id, name, description, language, content, parameters, timeout_sec)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`, tenantID, req.Name, req.Description, req.Language, req.Content, req.Parameters, req.TimeoutSec).Scan(&scriptID, &createdAt, &updatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          scriptID,
		"name":        req.Name,
		"language":    req.Language,
		"timeout_sec": req.TimeoutSec,
		"created_at":  createdAt,
	})
}

func (s *APIServer) handleListScripts(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, name, description, language, parameters, timeout_sec, is_public, created_by, created_at, updated_at
		FROM scripts WHERE tenant_id = $1 OR is_public = true
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var scripts []map[string]interface{}
	for rows.Next() {
		var id, name, desc, lang string
		var timeout int
		var isPublic bool
		var createdBy sql.NullString
		var createdAt, updatedAt time.Time
		var params []byte

		if err := rows.Scan(&id, &name, &desc, &lang, &params, &timeout, &isPublic, &createdBy, &createdAt, &updatedAt); err != nil {
			continue
		}
		s := map[string]interface{}{
			"id": id, "name": name, "description": desc, "language": lang,
			"timeout_sec": timeout, "is_public": isPublic, "created_at": createdAt, "updated_at": updatedAt,
		}
		if createdBy.Valid {
			s["created_by"] = createdBy.String
		}
		if len(params) > 0 {
			var p interface{}
			json.Unmarshal(params, &p)
			s["parameters"] = p
		}
		scripts = append(scripts, s)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"scripts": scripts})
}

func (s *APIServer) handleGetScript(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	scriptID := r.PathValue("scriptID")
	var script Script
	var createdBy sql.NullString
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, tenant_id, name, description, language, content, parameters, timeout_sec, is_public, created_by, created_at, updated_at
		FROM scripts WHERE id = $1 AND (tenant_id = $2 OR is_public = TRUE)
	`, scriptID, tenantID).Scan(
		&script.ID, &script.TenantID, &script.Name, &script.Description, &script.Language,
		&script.Content, &script.Parameters, &script.TimeoutSec, &script.IsPublic,
		&createdBy, &script.CreatedAt, &script.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "script not found"})
		return
	}
	if createdBy.Valid {
		script.CreatedBy = &createdBy.String
	}
	writeJSON(w, http.StatusOK, script)
}

func (s *APIServer) handleDeleteScript(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	scriptID := r.PathValue("scriptID")
	result, err := s.requestDB(r).ExecContext(r.Context(), `DELETE FROM scripts WHERE id = $1 AND tenant_id = $2`, scriptID, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "checking script deletion"})
		return
	}
	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "script not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type durableScriptTarget struct {
	DeviceID string
	AgentID  string
	MSPID    string
	ClientID string
}

// handleRunScript records both the script execution view and the generic job
// dispatch intent in the request-scoped SQL transaction. Broker publication is
// exclusively owned by Dispatcher after commit, so a process exit cannot leave
// a committed script execution without a recoverable dispatch record.
func (s *APIServer) handleRunScript(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	scriptID := r.PathValue("scriptID")
	var req struct {
		DeviceIDs  []string        `json:"device_ids"`
		Parameters json.RawMessage `json:"parameters,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.DeviceIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_ids required"})
		return
	}

	var script Script
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, tenant_id, name, language, content, timeout_sec, is_public
		FROM scripts
		WHERE id = $1 AND (tenant_id = $2 OR is_public = TRUE)
	`, scriptID, tenantID).Scan(
		&script.ID, &script.TenantID, &script.Name, &script.Language,
		&script.Content, &script.TimeoutSec, &script.IsPublic,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "script not found"})
		return
	}

	params := req.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}

	// Normalize target IDs before any state is written. The tenant in the URL is
	// authoritative; public scripts may be shared, but their owner tenant must
	// never become the execution/dispatch tenant.
	seen := make(map[string]struct{}, len(req.DeviceIDs))
	targets := make([]durableScriptTarget, 0, len(req.DeviceIDs))
	db := s.requestDB(r)
	for _, deviceID := range req.DeviceIDs {
		if deviceID == "" {
			continue
		}
		if _, duplicate := seen[deviceID]; duplicate {
			continue
		}
		seen[deviceID] = struct{}{}

		var target durableScriptTarget
		target.DeviceID = deviceID
		if err := db.QueryRowContext(r.Context(), `
			SELECT agent_id::text, msp_id::text, client_id::text
			FROM devices
			WHERE id::text = $1 AND tenant_id = $2
			  AND is_active = TRUE AND status <> 'disabled'
			  AND agent_id IS NOT NULL AND msp_id IS NOT NULL AND client_id IS NOT NULL
		`, deviceID, tenantID).Scan(&target.AgentID, &target.MSPID, &target.ClientID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "one or more devices are unavailable or outside the authorized tenant"})
			return
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_ids required"})
		return
	}

	executions := make([]map[string]interface{}, 0, len(targets))
	for _, target := range targets {
		execID := uuid.New().String()
		jobID := uuid.New().String()
		targetID := uuid.New().String()
		correlationID := uuid.New().String()
		scheduledFor := time.Now().UTC()
		expiresAt := scheduledFor.Add(72 * time.Hour)

		commandPayload := map[string]interface{}{
			"type":         "script_exec",
			"execution_id": execID,
			"language":     script.Language,
			"content":      script.Content,
			"parameters":   params,
			"timeout":      script.TimeoutSec,
		}
		payloadJSON, err := json.Marshal(commandPayload)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encoding script command"})
			return
		}
		requestHash := fmt.Sprintf("%x", sha256.Sum256(payloadJSON))

		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO script_executions (id, script_id, tenant_id, device_id, status, parameters)
			VALUES ($1, $2, $3, $4, 'pending', $5)
		`, execID, scriptID, tenantID, target.DeviceID, params); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating script execution"})
			return
		}

		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO jobs (id, msp_id, client_id, created_by, type, status, priority,
			                  payload, max_retries, max_devices, expires_at,
			                  correlation_id, scheduled_for, request_hash)
			VALUES ($1, $2, $3, 'script-api', 'script_exec', 'queued', 0, $4, 3, 1, $5, $6, $7, $8)
		`, jobID, target.MSPID, target.ClientID, payloadJSON, expiresAt, correlationID, scheduledFor, requestHash); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating durable script job"})
			return
		}

		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO job_targets (id, job_id, device_id, agent_id, msp_id, status)
			VALUES ($1, $2, $3, $4, $5, 'queued')
		`, targetID, jobID, target.DeviceID, target.AgentID, target.MSPID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating durable script target"})
			return
		}

		outboxPayload, err := json.Marshal(map[string]interface{}{
			"schema_version": 1,
			"event_id":       fmt.Sprintf("%s:%s:%d", jobID, targetID, 1),
			"job_id":         jobID,
			"target_id":      targetID,
			"msp_id":         target.MSPID,
			"client_id":      target.ClientID,
			"device_id":      target.DeviceID,
			"agent_id":       target.AgentID,
			"correlation_id": correlationID,
			"attempt":        1,
			"issued_at":      scheduledFor.Format(time.RFC3339),
			"expires_at":     expiresAt.Format(time.RFC3339),
			"command_type":   "script_exec",
			"payload":        commandPayload,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encoding script dispatch event"})
			return
		}
		if _, err := db.ExecContext(r.Context(), `
			INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
			VALUES (gen_random_uuid(), $1, $2, 'job.dispatch', $3, $4)
		`, target.MSPID, jobID, outboxPayload, scheduledFor); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "creating durable script dispatch event"})
			return
		}

		executions = append(executions, map[string]interface{}{
			"execution_id": execID,
			"job_id":       jobID,
			"device_id":    target.DeviceID,
			"status":       "pending",
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"script":     script.Name,
		"executions": executions,
		"count":      len(executions),
		"durable":    true,
	})
}

func (s *APIServer) handleScriptExecutions(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, script_id, device_id, status, exit_code, duration_ms, started_at, completed_at, created_at
		FROM script_executions WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT 50
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var executions []map[string]interface{}
	for rows.Next() {
		var id, deviceID, status string
		var scriptID, exitCode sql.NullString
		var dur sql.NullInt64
		var createdAt time.Time
		var startedNull, completedNull sql.NullTime

		if err := rows.Scan(&id, &scriptID, &deviceID, &status, &exitCode, &dur, &startedNull, &completedNull, &createdAt); err != nil {
			continue
		}
		exec := map[string]interface{}{
			"id": id, "device_id": deviceID, "status": status, "created_at": createdAt,
		}
		if scriptID.Valid {
			exec["script_id"] = scriptID.String
		}
		if exitCode.Valid {
			exec["exit_code"] = exitCode.String
		}
		if dur.Valid {
			exec["duration_ms"] = dur.Int64
		}
		if startedNull.Valid {
			exec["started_at"] = startedNull.Time
		}
		if completedNull.Valid {
			exec["completed_at"] = completedNull.Time
		}
		executions = append(executions, exec)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"executions": executions})
}

func (s *APIServer) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	execID := r.PathValue("execID")
	var exec ScriptExecution
	var scriptID, triggeredBy sql.NullString
	var exitCode sql.NullInt64
	var dur sql.NullInt64
	var startedNull, completedNull sql.NullTime
	var params []byte

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, script_id, tenant_id, device_id, triggered_by, status, stdout, stderr, exit_code, duration_ms, parameters, started_at, completed_at, created_at
		FROM script_executions WHERE id = $1 AND tenant_id = $2
	`, execID, tenantID).Scan(
		&exec.ID, &scriptID, &exec.TenantID, &exec.DeviceID, &triggeredBy,
		&exec.Status, &exec.Stdout, &exec.Stderr, &exitCode, &dur,
		&params, &startedNull, &completedNull, &exec.CreatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "execution not found"})
		return
	}
	if scriptID.Valid {
		exec.ScriptID = &scriptID.String
	}
	if triggeredBy.Valid {
		exec.TriggeredBy = &triggeredBy.String
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		exec.ExitCode = &code
	}
	if dur.Valid {
		ms := dur.Int64
		exec.DurationMs = &ms
	}
	if startedNull.Valid {
		exec.StartedAt = &startedNull.Time
	}
	if completedNull.Valid {
		exec.CompletedAt = &completedNull.Time
	}
	if len(params) > 0 {
		exec.Parameters = params
	}
	writeJSON(w, http.StatusOK, exec)
}

func (s *APIServer) handleScriptResultNATS(msg *nats.Msg) {
	var result struct {
		Type        string `json:"type"`
		ExecutionID string `json:"execution_id"`
		ScheduleID  string `json:"schedule_id"`
		DeviceID    string `json:"device_id"`
		Status      string `json:"status"`
		Stdout      string `json:"stdout"`
		Stderr      string `json:"stderr"`
		ExitCode    int    `json:"exit_code"`
		DurationMs  int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return
	}
	if result.Type != "script_result" || result.ExecutionID == "" {
		return
	}

	now := time.Now()
	_, err := s.db.DB().Exec(`
		UPDATE script_executions
		SET status = $1, stdout = $2, stderr = $3, exit_code = $4, duration_ms = $5, completed_at = $6
		WHERE id = $7 AND status = 'running'
	`, result.Status, result.Stdout, result.Stderr, result.ExitCode, result.DurationMs, now, result.ExecutionID)
	if err != nil {
		s.logger.Error("update script execution", zap.Error(err))
	}

	if result.ScheduleID != "" {
		so := NewScheduleOrchestrator(s.nats, s.db.DB(), s.logger)
		if err := so.ProcessScheduleDeviceResult(map[string]interface{}{
			"execution_id": result.ExecutionID,
			"schedule_id":  result.ScheduleID,
			"device_id":    result.DeviceID,
			"status":       result.Status,
			"stdout":       result.Stdout,
			"stderr":       result.Stderr,
			"exit_code":    result.ExitCode,
			"duration_ms":  result.DurationMs,
		}); err != nil {
			s.logger.Error("process schedule device result",
				zap.String("schedule_id", result.ScheduleID),
				zap.String("device_id", result.DeviceID),
				zap.Error(err))
		}
	}
}
