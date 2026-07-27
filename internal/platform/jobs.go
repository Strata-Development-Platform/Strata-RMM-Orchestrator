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
}

func (s *APIServer) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	clientID := r.URL.Query().Get("client_id")
	if mspID == "" || clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and client_id required"})
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

	id := uuid.New().String()
	payloadJSON, _ := json.Marshal(req.Payload)

	_, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO jobs (id, msp_id, client_id, created_by, type, status, priority,
		                  payload, idempotency_key, max_retries, max_devices, expires_at)
		VALUES ($1, $2, $3, $4, $5, 'queued', $6, $7, $8, $9, $10, NOW() + INTERVAL '24 hours')
	`, id, mspID, clientID, r.Header.Get("X-User-Email"), req.Type, req.Priority,
		payloadJSON, nullIfEmpty(req.IdempotencyKey), req.MaxRetries, len(req.DeviceIDs))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	for _, deviceID := range req.DeviceIDs {
		targetID := uuid.New().String()
		s.requestDB(r).ExecContext(r.Context(), `
			INSERT INTO job_targets (id, job_id, device_id)
			VALUES ($1, $2, $3)
		`, targetID, id, deviceID)
	}

	// Dispatch to NATS for each device
	if s.nats != nil {
		for _, deviceID := range req.DeviceIDs {
			cmdPayload, _ := json.Marshal(map[string]interface{}{
				"job_id":  id,
				"type":    req.Type,
				"payload": req.Payload,
			})
			subject := fmt.Sprintf("tenant.*.cmd.%s", deviceID)
			s.nats.Publish(subject, cmdPayload)
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

	var mspID, clientID, jobType, status, payloadStr string
	var createdAt, expiresAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT msp_id, client_id, type, status, payload::text, created_at, expires_at
		FROM jobs WHERE id = $1
	`, jobID).Scan(&mspID, &clientID, &jobType, &status, &payloadStr, &createdAt, &expiresAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT device_id, status, COALESCE(error_message, ''), started_at, completed_at
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
		if err := rows.Scan(&deviceID, &targetStatus, &errMsg, &startedAt, &completedAt); err != nil {
			continue
		}
		targets = append(targets, map[string]interface{}{
			"device_id": deviceID, "status": targetStatus,
			"error_message": errMsg,
			"started_at":    nullTimeStr(startedAt),
			"completed_at":  nullTimeStr(completedAt),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":         jobID,
		"msp_id":     mspID,
		"client_id":  clientID,
		"type":       jobType,
		"status":     status,
		"created_at": createdAt.UTC().Format(time.RFC3339),
		"targets":    targets,
	})
}

func (s *APIServer) handleListJobs(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	clientID := r.URL.Query().Get("client_id")
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}

	query := `SELECT id, type, status, priority, max_devices, completed_count, failed_count, created_at
		FROM jobs WHERE msp_id = $1`
	args := []interface{}{mspID}
	argIdx := 2

	if clientID != "" {
		query += fmt.Sprintf(" AND client_id = $%d", argIdx)
		args = append(args, clientID)
		argIdx++
	}
	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := s.requestDB(r).QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var jobs []map[string]interface{}
	for rows.Next() {
		var id, jobType, status string
		var priority, maxDev, compCount, failCount int
		var createdAt time.Time
		if err := rows.Scan(&id, &jobType, &status, &priority, &maxDev, &compCount, &failCount, &createdAt); err != nil {
			continue
		}
		jobs = append(jobs, map[string]interface{}{
			"id": id, "type": jobType, "status": status, "priority": priority,
			"max_devices": maxDev, "completed_count": compCount, "failed_count": failCount,
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

	res, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE jobs SET status = 'cancelled', completed_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'queued', 'dispatched')
	`, jobID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found or already completed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func nullTimeStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// NATS handlers for job results
func (s *APIServer) handleJobResultNATS(m *natsMsg) {
	var result struct {
		JobID    string          `json:"job_id"`
		DeviceID string          `json:"device_id"`
		Status   string          `json:"status"`
		Error    string          `json:"error,omitempty"`
		Data     json.RawMessage `json:"data,omitempty"`
	}
	if err := json.Unmarshal(m.Data, &result); err != nil || result.JobID == "" {
		return
	}

	status := result.Status
	if status == "" {
		status = "succeeded"
	}

	dataJSON, _ := json.Marshal(result.Data)
	s.db.DB().Exec(`
		UPDATE job_targets SET status = $1, result = $2, error_message = $3,
		                       completed_at = NOW()
		WHERE job_id = $4 AND device_id = $5
	`, status, string(dataJSON), result.Error, result.JobID, result.DeviceID)

	// Update job completion counts
	s.db.DB().Exec(`
		UPDATE jobs SET
			completed_count = (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status = 'succeeded'),
			failed_count = (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status = 'failed'),
			status = CASE
				WHEN (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status IN ('pending','queued','dispatched','running')) = 0
				THEN 'completed' ELSE status END,
			completed_at = CASE
				WHEN (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status IN ('pending','queued','dispatched','running')) = 0
				THEN NOW() ELSE NULL END
		WHERE id = $1
	`, result.JobID)
}

func (s *APIServer) handleNATSResult(w http.ResponseWriter, r *http.Request) {
	var msg natsMsg
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid"})
		return
	}
	s.handleJobResultNATS(&msg)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type natsMsg struct {
	Subject string          `json:"subject"`
	Data    json.RawMessage `json:"data"`
}
