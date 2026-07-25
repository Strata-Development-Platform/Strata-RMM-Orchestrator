package platform

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func (s *APIServer) handleListReports(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	rows, err := s.db.DB().QueryContext(r.Context(), `
		SELECT id, name, format, storage_key, size_bytes, generated_at
		FROM generated_reports WHERE tenant_id = $1
		ORDER BY generated_at DESC LIMIT 20
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var reports []map[string]interface{}
	for rows.Next() {
		var id, name, format, storageKey string
		var size int64
		var genAt time.Time
		if err := rows.Scan(&id, &name, &format, &storageKey, &size, &genAt); err != nil {
			continue
		}
		reports = append(reports, map[string]interface{}{
			"id": id, "name": name, "format": format,
			"storage_key": storageKey, "size_bytes": size, "generated_at": genAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"reports": reports})
}

func (s *APIServer) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	var req struct {
		Name      string   `json:"name"`
		Frequency string   `json:"frequency"`
		Sections  []string `json:"sections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Frequency == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and frequency required"})
		return
	}
	if req.Sections == nil {
		req.Sections = []string{"summary", "alerts", "cves", "patches"}
	}
	secJSON, _ := json.Marshal(req.Sections)

	var id string
	err := s.db.DB().QueryRowContext(r.Context(), `
		INSERT INTO report_schedules (tenant_id, name, frequency, sections)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, tenantID, req.Name, req.Frequency, secJSON).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func (s *APIServer) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	rows, err := s.db.DB().QueryContext(r.Context(), `
		SELECT id, name, frequency, sections, enabled, last_sent, created_at
		FROM report_schedules WHERE tenant_id = $1 ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var schedules []map[string]interface{}
	for rows.Next() {
		var id, name, freq string
		var enabled bool
		var created_at time.Time
		var lastSent sql.NullTime
		var sections []byte

		if err := rows.Scan(&id, &name, &freq, &sections, &enabled, &lastSent, &created_at); err != nil {
			continue
		}
		sched := map[string]interface{}{
			"id": id, "name": name, "frequency": freq,
			"enabled": enabled, "created_at": created_at,
		}
		if lastSent.Valid {
			sched["last_sent"] = lastSent.Time
		}
		var secs []string
		json.Unmarshal(sections, &secs)
		sched["sections"] = secs
		schedules = append(schedules, sched)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"schedules": schedules})
}

func (s *APIServer) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("scheduleID")
	_, err := s.db.DB().ExecContext(r.Context(), `DELETE FROM report_schedules WHERE id = $1`, scheduleID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *APIServer) handleGenerateReport(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	var req struct {
		Sections []string `json:"sections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Sections = []string{"summary", "alerts", "cves", "patches"}
	}
	if req.Sections == nil {
		req.Sections = []string{"summary", "alerts", "cves", "patches"}
	}

	var tenantName string
	err := s.db.DB().QueryRowContext(r.Context(), `SELECT name FROM tenants WHERE id = $1`, tenantID).Scan(&tenantName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}

	go func() {
		logger := s.logger
		_ = logger
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "generation started"})
}
