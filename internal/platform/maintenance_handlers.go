package platform

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// handleCreateDeviceGroupV2 is the replacement for handleCreateDeviceGroup that supports Smart Groups.
// The old handleCreateDeviceGroup remains for backward compatibility but is deprecated.

func (s *APIServer) handleCreateMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if tenantID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant_id required"})
		return
	}
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}

	var req struct {
		Name      string   `json:"name"`
		StartTime string   `json:"start_time"`
		EndTime   string   `json:"end_time"`
		DeviceIDs []string `json:"device_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.StartTime == "" || req.EndTime == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, start_time, end_time required"})
		return
	}

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid start_time"})
		return
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid end_time"})
		return
	}
	if endTime.Before(startTime) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "end_time must be after start_time"})
		return
	}

	id := uuid.New().String()

	deviceIDsJSON, _ := json.Marshal(req.DeviceIDs)
	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO maintenance_windows (id, tenant_id, name, start_time, end_time, device_ids, tags)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, '{}'::jsonb)
	`, id, tenantID, req.Name, startTime, endTime, deviceIDsJSON)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})
}

func (s *APIServer) handleListMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if tenantID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant_id required"})
		return
	}
	if !s.AuthorizeClientAccess(w, r, tenantID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, tenant_id, name, start_time, end_time, device_ids::text, created_at
		FROM maintenance_windows WHERE tenant_id = $1
		ORDER BY start_time DESC LIMIT 100
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var windows []map[string]interface{}
	for rows.Next() {
		var id, tenantID, name string
		var startTime, endTime, createdAt time.Time
		var deviceIDsStr string
		if err := rows.Scan(&id, &tenantID, &name, &startTime, &endTime, &deviceIDsStr, &createdAt); err != nil {
			continue
		}
		var deviceIDs []string
		if deviceIDsStr != "" {
			_ = json.Unmarshal([]byte(deviceIDsStr), &deviceIDs)
		}
		windows = append(windows, map[string]interface{}{
			"id": id, "tenant_id": tenantID, "name": name,
			"start_time": startTime.UTC().Format(time.RFC3339),
			"end_time":   endTime.UTC().Format(time.RFC3339),
			"device_ids": deviceIDs,
			"created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}
	if windows == nil {
		windows = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"maintenance_windows": windows})
}

func (s *APIServer) handleCreateDeviceGroup(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	clientID := r.URL.Query().Get("client_id")
	if mspID == "" || clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and client_id required"})
		return
	}

	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		DeviceIDs   []string `json:"device_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}

	id := uuid.New().String()
	_, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO device_groups (id, msp_id, client_id, name, description, member_ids)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, mspID, clientID, req.Name, req.Description, req.DeviceIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})
}

func (s *APIServer) handleListDeviceGroups(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	clientID := r.URL.Query().Get("client_id")
	if mspID == "" || clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id and client_id required"})
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, name, description, member_ids::text, created_at
		FROM device_groups WHERE msp_id = $1 AND client_id = $2
		ORDER BY name ASC
	`, mspID, clientID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var groups []map[string]interface{}
	for rows.Next() {
		var id, name, description, memberIDsStr string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &description, &memberIDsStr, &createdAt); err != nil {
			continue
		}
		groups = append(groups, map[string]interface{}{
			"id": id, "name": name, "description": description,
			"created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}
	if groups == nil {
		groups = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"device_groups": groups})
}

func (s *APIServer) handleDeleteDeviceGroup(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("groupID")
	res, err := s.requestDB(r).ExecContext(r.Context(), `DELETE FROM device_groups WHERE id = $1`, groupID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *APIServer) handleDeleteMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	windowID := r.PathValue("windowID")

	res, err := s.requestDB(r).ExecContext(r.Context(), `DELETE FROM maintenance_windows WHERE id = $1 AND tenant_id = $2`, windowID, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
