package platform

import (
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
	scriptID := r.PathValue("scriptID")
	var script Script
	var createdBy sql.NullString
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, tenant_id, name, description, language, content, parameters, timeout_sec, is_public, created_by, created_at, updated_at
		FROM scripts WHERE id = $1
	`, scriptID).Scan(
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
	scriptID := r.PathValue("scriptID")
	_, err := s.requestDB(r).ExecContext(r.Context(), `DELETE FROM scripts WHERE id = $1`, scriptID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *APIServer) handleRunScript(w http.ResponseWriter, r *http.Request) {
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
	var tenantID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, tenant_id, name, language, content, timeout_sec
		FROM scripts WHERE id = $1
	`, scriptID).Scan(&script.ID, &tenantID, &script.Name, &script.Language, &script.Content, &script.TimeoutSec)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "script not found"})
		return
	}

	params := req.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}

	var executions []map[string]interface{}
	for _, deviceID := range req.DeviceIDs {
		execID := uuid.New().String()

		_, err := s.requestDB(r).ExecContext(r.Context(), `
			INSERT INTO script_executions (id, script_id, tenant_id, device_id, status, parameters)
			VALUES ($1, $2, $3, $4, 'pending', $5)
		`, execID, scriptID, tenantID, deviceID, params)
		if err != nil {
			s.logger.Warn("create execution record", zap.Error(err))
			continue
		}

		cmdPayload, _ := json.Marshal(map[string]interface{}{
			"type":         "script_exec",
			"execution_id": execID,
			"language":     script.Language,
			"content":      script.Content,
			"parameters":   params,
			"timeout":      script.TimeoutSec,
		})

		subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, deviceID)
		if err := s.nats.Publish(subject, cmdPayload); err != nil {
			s.logger.Warn("publish script command", zap.Error(err))
			if _, updateErr := s.requestDB(r).ExecContext(
				r.Context(),
				`UPDATE script_executions SET status = 'failed', stderr = 'NATS publish failed' WHERE id = $1`,
				execID,
			); updateErr != nil {
				s.logger.Error("mark script execution failed", zap.Error(updateErr))
			}
		}

		executions = append(executions, map[string]interface{}{
			"execution_id": execID,
			"device_id":    deviceID,
			"status":       "pending",
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"script":     script.Name,
		"executions": executions,
		"count":      len(executions),
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
	execID := r.PathValue("execID")
	var exec ScriptExecution
	var scriptID, triggeredBy sql.NullString
	var exitCode sql.NullInt64
	var dur sql.NullInt64
	var startedNull, completedNull sql.NullTime
	var params []byte

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, script_id, tenant_id, device_id, triggered_by, status, stdout, stderr, exit_code, duration_ms, parameters, started_at, completed_at, created_at
		FROM script_executions WHERE id = $1
	`, execID).Scan(
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
		so.ProcessScheduleDeviceResult(map[string]interface{}{
			"execution_id": result.ExecutionID,
			"schedule_id":  result.ScheduleID,
			"device_id":    result.DeviceID,
			"status":       result.Status,
			"stdout":       result.Stdout,
			"stderr":       result.Stderr,
			"exit_code":    result.ExitCode,
			"duration_ms":  result.DurationMs,
		})
	}
}
