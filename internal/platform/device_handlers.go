package platform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type deviceFilter struct {
	MSPID         string `json:"msp_id"`
	ClientID      string `json:"client_id,omitempty"`
	SiteID        string `json:"site_id,omitempty"`
	DeviceGroupID string `json:"device_group_id,omitempty"`
	Search        string `json:"search,omitempty"`
	Status        string `json:"status,omitempty"`
	OS            string `json:"os,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"`
	Tag           string `json:"tag,omitempty"`
	Online        string `json:"online,omitempty"`
	LastSeenFrom  string `json:"last_seen_from,omitempty"`
	LastSeenTo    string `json:"last_seen_to,omitempty"`
	Limit         int    `json:"limit"`
	Offset        int    `json:"offset"`
	SortBy        string `json:"sort_by,omitempty"`
	SortDir       string `json:"sort_dir,omitempty"`
}

func (s *APIServer) handleListDevices(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}

	q := r.URL.Query()
	limit := parseInt(q.Get("limit"), 50)
	offset := parseInt(q.Get("offset"), 0)
	if limit > 200 {
		limit = 200
	}

	query := `SELECT d.id, d.hostname, d.os, d.arch, d.agent_version, d.status,
	                  d.last_heartbeat, d.created_at,
	                  COALESCE(s.name, ''), COALESCE(c.name, ''),
	                  COALESCE(d.client_id::text, ''), COALESCE(d.site_id::text, '')
	           FROM devices d
	           LEFT JOIN sites s ON d.site_id = s.id
	           LEFT JOIN client_organizations c ON d.client_id = c.id
	           WHERE d.msp_id = $1`
	args := []interface{}{mspID}
	argIdx := 2

	if clientID := q.Get("client_id"); clientID != "" {
		query += fmt.Sprintf(" AND d.client_id = $%d", argIdx)
		args = append(args, clientID)
		argIdx++
	}
	if siteID := q.Get("site_id"); siteID != "" {
		query += fmt.Sprintf(" AND d.site_id = $%d", argIdx)
		args = append(args, siteID)
		argIdx++
	}
	if status := q.Get("status"); status != "" {
		query += fmt.Sprintf(" AND d.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if search := q.Get("search"); search != "" {
		query += fmt.Sprintf(" AND d.hostname ILIKE $%d", argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if os := q.Get("os"); os != "" {
		query += fmt.Sprintf(" AND d.os ILIKE $%d", argIdx)
		args = append(args, os+"%")
		argIdx++
	}
	if agentVer := q.Get("agent_version"); agentVer != "" {
		query += fmt.Sprintf(" AND d.agent_version = $%d", argIdx)
		args = append(args, agentVer)
		argIdx++
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM (" + query + ") AS filtered"
	var total int
	s.requestDB(r).QueryRow(countQuery, args...).Scan(&total)

	sortBy := q.Get("sort_by")
	sortDir := q.Get("sort_dir")
	if sortDir != "desc" {
		sortDir = "asc"
	}
	allowedSorts := map[string]bool{"hostname": true, "os": true, "status": true, "last_heartbeat": true, "created_at": true}
	if !allowedSorts[sortBy] {
		sortBy = "hostname"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortDir)
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := s.requestDB(r).Query(query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type deviceRow struct {
		ID            string    `json:"id"`
		Hostname      string    `json:"hostname"`
		OS            string    `json:"os"`
		Arch          string    `json:"arch"`
		AgentVersion  string    `json:"agent_version"`
		Status        string    `json:"status"`
		LastHeartbeat *time.Time `json:"last_heartbeat"`
		CreatedAt     time.Time `json:"created_at"`
		SiteName      string    `json:"site_name"`
		ClientName    string    `json:"client_name"`
		ClientID      string    `json:"client_id"`
		SiteID        string    `json:"site_id"`
	}

	var devices []deviceRow
	for rows.Next() {
		var d deviceRow
		if err := rows.Scan(&d.ID, &d.Hostname, &d.OS, &d.Arch, &d.AgentVersion, &d.Status,
			&d.LastHeartbeat, &d.CreatedAt,
			&d.SiteName, &d.ClientName, &d.ClientID, &d.SiteID); err != nil {
			continue
		}
		devices = append(devices, d)
	}
	if devices == nil {
		devices = []deviceRow{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": devices,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

func (s *APIServer) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)

	var d struct {
		ID            string     `json:"id"`
		Hostname      string     `json:"hostname"`
		OS            string     `json:"os"`
		Arch          string     `json:"arch"`
		AgentVersion  string     `json:"agent_version"`
		Status        string     `json:"status"`
		LastHeartbeat *time.Time `json:"last_heartbeat"`
		CreatedAt     time.Time  `json:"created_at"`
		ClientID      string     `json:"client_id"`
		SiteID        string     `json:"site_id"`
		TenantID      string     `json:"tenant_id"`
	}
	err := s.requestDB(r).QueryRow(`
		SELECT id, hostname, os, arch, agent_version, status, last_heartbeat,
		       created_at, COALESCE(client_id::text, ''), COALESCE(site_id::text, ''),
		       COALESCE(tenant_id::text, '')
		FROM devices WHERE id = $1 AND msp_id = $2
	`, deviceID, mspID).Scan(&d.ID, &d.Hostname, &d.OS, &d.Arch, &d.AgentVersion,
		&d.Status, &d.LastHeartbeat, &d.CreatedAt, &d.ClientID, &d.SiteID, &d.TenantID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	if n <= 0 {
		return def
	}
	return n
}

func (s *APIServer) handleDeviceAction(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)

	var req struct {
		Action    string `json:"action"`
		Reason    string `json:"reason,omitempty"`
		Param     string `json:"param,omitempty"`
		Schedule  string `json:"schedule_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action required"})
		return
	}

	destructive := map[string]bool{"reboot": true, "shutdown": true, "disable": true}
	if destructive[req.Action] && req.Reason == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason required for destructive actions"})
		return
	}

	clientID, siteID := "", ""
	s.requestDB(r).QueryRow(`SELECT COALESCE(client_id::text,''), COALESCE(site_id::text,'') FROM devices WHERE id = $1`, deviceID).Scan(&clientID, &siteID)

	jobType := "device." + req.Action
	id := uuid.New().String()
	payload, _ := json.Marshal(req)
	now := time.Now()

	_, err := s.requestDB(r).Exec(`
		INSERT INTO jobs (id, msp_id, client_id, site_id, created_by, type, status, priority,
		                  payload, max_retries, max_devices, expires_at, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, 'queued', 10, $7, 1, 1, $8, $9)
	`, id, mspID, clientID, siteID, "api:"+req.Action, jobType,
		payload, now.Add(24*time.Hour), uuid.New().String())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	targetID := uuid.New().String()
	s.requestDB(r).Exec(`
		INSERT INTO job_targets (id, job_id, device_id, msp_id, status)
		VALUES ($1, $2, $3, $4, 'queued')
	`, targetID, id, deviceID, mspID)

	outboxPayload, _ := json.Marshal(map[string]interface{}{
		"job_id": id, "target_id": targetID, "device_id": deviceID,
		"type": jobType, "payload": req,
	})
	s.requestDB(r).Exec(`
		INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
		VALUES (gen_random_uuid(), $1, $2, 'job.dispatch', $3, NOW())
	`, mspID, id, outboxPayload)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"job_id": id, "status": "queued", "action": req.Action,
	})
}

func (s *APIServer) handleBulkDeviceAction(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	var req struct {
		DeviceIDs []string `json:"device_ids"`
		Action    string   `json:"action"`
		Reason    string   `json:"reason,omitempty"`
		Schedule  string   `json:"schedule_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.DeviceIDs) == 0 || req.Action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_ids and action required"})
		return
	}
	if len(req.DeviceIDs) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "max 100 devices per bulk operation"})
		return
	}

	destructive := map[string]bool{"reboot": true, "shutdown": true, "disable": true}
	if destructive[req.Action] && req.Reason == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason required"})
		return
	}

	jobType := "bulk." + req.Action
	jobID := uuid.New().String()
	payload, _ := json.Marshal(req)
	now := time.Now()
	expires := now.Add(24 * time.Hour)

	s.requestDB(r).Exec(`
		INSERT INTO jobs (id, msp_id, created_by, type, status, priority,
		                  payload, max_retries, max_devices, expires_at, correlation_id)
		VALUES ($1, $2, $3, $4, 'queued', 10, $5, 1, $6, $7, $8)
	`, jobID, mspID, "api:bulk:"+req.Action, jobType,
		payload, len(req.DeviceIDs), expires, uuid.New().String())

	for _, deviceID := range req.DeviceIDs {
		targetID := uuid.New().String()
		s.requestDB(r).Exec(`
			INSERT INTO job_targets (id, job_id, device_id, msp_id, status)
			VALUES ($1, $2, $3, $4, 'queued')
		`, targetID, jobID, deviceID, mspID)

		outboxPayload, _ := json.Marshal(map[string]interface{}{
			"job_id": jobID, "target_id": targetID, "device_id": deviceID,
			"type": jobType, "payload": req,
		})
		s.requestDB(r).Exec(`
			INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
			VALUES (gen_random_uuid(), $1, $2, 'job.dispatch', $3, NOW())
		`, mspID, jobID, outboxPayload)
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"job_id": jobID, "status": "queued", "target_count": len(req.DeviceIDs),
	})
}
