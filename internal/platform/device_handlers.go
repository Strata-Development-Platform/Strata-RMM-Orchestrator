package platform

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type operationDef struct {
	JobType          string
	Destructive      bool
	RequiresAdmin    bool
	RequiresReason   bool
	SupportsSchedule bool
	AllowOffline     bool
	Timeout          time.Duration
}

var operationRegistry = map[string]operationDef{
	"refresh":         {JobType: "device.refresh", Destructive: false, SupportsSchedule: true, Timeout: 5 * time.Minute},
	"reboot":          {JobType: "device.reboot", Destructive: true, RequiresAdmin: true, RequiresReason: true, AllowOffline: true, Timeout: 2 * time.Minute},
	"shutdown":        {JobType: "device.shutdown", Destructive: true, RequiresAdmin: true, RequiresReason: true, AllowOffline: true, Timeout: 2 * time.Minute},
	"service_start":   {JobType: "device.service_start", RequiresAdmin: true, SupportsSchedule: true, Timeout: 30 * time.Second},
	"service_stop":    {JobType: "device.service_stop", RequiresAdmin: true, SupportsSchedule: true, Timeout: 30 * time.Second},
	"service_restart": {JobType: "device.service_restart", RequiresAdmin: true, SupportsSchedule: true, Timeout: 30 * time.Second},
	"process_kill":    {JobType: "device.process_kill", RequiresAdmin: true, RequiresReason: true, Timeout: 10 * time.Second},
}

type bulkActionReq struct {
	DeviceIDs []string `json:"device_ids"`
	Action    string   `json:"action"`
	Reason    string   `json:"reason,omitempty"`
	Schedule  string   `json:"schedule_at,omitempty"`
}

func validateIdempotencyKey(r *http.Request) (string, error) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) > 128 {
		return "", fmt.Errorf("idempotency key is too long")
	}
	return key, nil
}

func endpointRequestHash(value interface{}) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

func requestIPAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func maintenanceWindowAllows(db dbExecutor, clientID, deviceID string, executeAt time.Time) (bool, error) {
	var configured, covered bool
	err := db.QueryRow(`
		SELECT
			EXISTS (
				SELECT 1 FROM maintenance_windows
				WHERE tenant_id = $1 AND end_time >= NOW()
			),
			EXISTS (
				SELECT 1 FROM maintenance_windows
				WHERE tenant_id = $1
				  AND start_time <= $3 AND end_time >= $3
				  AND (COALESCE(cardinality(device_ids), 0) = 0 OR $2 = ANY(device_ids))
			)
	`, clientID, deviceID, executeAt).Scan(&configured, &covered)
	if err != nil {
		return false, err
	}
	return !configured || covered, nil
}

func writeEndpointAudit(r *http.Request, db dbExecutor, tenantID, action, resource string, details interface{}) error {
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if tenantID == "" || userID == "" {
		return fmt.Errorf("missing audit identity")
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(r.Context(), `
		INSERT INTO audit_log (id, tenant_id, user_id, action, resource, details, ip_address)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NULLIF($6, '')::inet)
	`, tenantID, userID, action, resource, encoded, requestIPAddress(r))
	return err
}

func writeIdempotentEndpointResult(w http.ResponseWriter, r *http.Request, db dbExecutor, mspID, key, requestHash string) (bool, error) {
	if key == "" {
		return false, nil
	}
	var jobID, status, existingHash string
	err := db.QueryRowContext(r.Context(), `
		SELECT id::text, status, COALESCE(request_hash, '')
		FROM jobs WHERE msp_id = $1 AND idempotency_key = $2
	`, mspID, key).Scan(&jobID, &status, &existingHash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if existingHash != requestHash {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency key already used for a different request"})
		return true, nil
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"job_id": jobID, "status": status, "duplicate": true})
	return true, nil
}

func (s *APIServer) handleListDevices(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
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

	args := []interface{}{mspID}
	argIdx := 2
	query := `SELECT d.id, d.hostname, d.os, d.arch, d.agent_version, d.status,
	                  d.last_heartbeat, d.created_at,
	                  COALESCE(s.name, ''), COALESCE(c.name, ''),
	                  COALESCE(d.client_id::text, ''), COALESCE(d.site_id::text, '')
	           FROM devices d
	           LEFT JOIN sites s ON d.site_id = s.id
	           LEFT JOIN client_organizations c ON d.client_id = c.id
	           WHERE d.msp_id = $1`

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
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") AS filtered"
	var total int
	if err := s.requestDB(r).QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	allowedSorts := map[string]bool{"hostname": true, "os": true, "status": true, "last_heartbeat": true, "created_at": true}
	sortBy := q.Get("sort_by")
	sortDir := q.Get("sort_dir")
	if sortDir != "desc" {
		sortDir = "asc"
	}
	if !allowedSorts[sortBy] {
		sortBy = "hostname"
	}
	query += fmt.Sprintf(" ORDER BY %s %s LIMIT %d OFFSET %d", sortBy, sortDir, limit, offset)

	rows, err := s.requestDB(r).Query(query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer func() { _ = rows.Close() }()

	type deviceRow struct {
		ID           string     `json:"id"`
		Hostname     string     `json:"hostname"`
		OS           string     `json:"os"`
		Arch         string     `json:"arch"`
		AgentVersion string     `json:"agent_version"`
		Status       string     `json:"status"`
		LastHb       *time.Time `json:"last_heartbeat"`
		CreatedAt    time.Time  `json:"created_at"`
		SiteName     string     `json:"site_name"`
		ClientName   string     `json:"client_name"`
		ClientID     string     `json:"client_id"`
		SiteID       string     `json:"site_id"`
	}

	var devices []deviceRow
	for rows.Next() {
		var d deviceRow
		if err := rows.Scan(&d.ID, &d.Hostname, &d.OS, &d.Arch, &d.AgentVersion, &d.Status,
			&d.LastHb, &d.CreatedAt, &d.SiteName, &d.ClientName, &d.ClientID, &d.SiteID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan error"})
			return
		}
		devices = append(devices, d)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan error"})
		return
	}
	if devices == nil {
		devices = []deviceRow{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": devices, "total": total, "limit": limit, "offset": offset,
	})
}

func (s *APIServer) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" || deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

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

func (s *APIServer) handleDeviceDetailInventory(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" || deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	var hostname, osType, arch, agentVer, status string
	var lastHB *time.Time
	var memTotal, diskTotal int64
	var cpuCores int
	err := s.requestDB(r).QueryRow(`
		SELECT hostname, os, arch, agent_version, status, last_heartbeat,
		       COALESCE(cpu_cores, 0), COALESCE(ram_total_mb, 0), COALESCE(disk_total_mb, 0)
		FROM devices WHERE id = $1 AND msp_id = $2
	`, deviceID, mspID).Scan(&hostname, &osType, &arch, &agentVer, &status, &lastHB, &cpuCores, &memTotal, &diskTotal)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}

	hw := map[string]interface{}{
		"hostname": hostname, "os": osType, "arch": arch,
		"agent_version": agentVer, "status": status,
		"cpu_cores": cpuCores, "memory_mb": memTotal, "disk_mb": diskTotal,
	}
	if lastHB != nil {
		hw["last_heartbeat"] = lastHB.UTC().Format(time.RFC3339)
		age := time.Since(*lastHB).Seconds()
		hw["data_age_seconds"] = age
		hw["online"] = age < 120
	} else {
		hw["online"] = false
	}
	writeJSON(w, http.StatusOK, hw)
}

func (s *APIServer) handleDeviceAction(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" || deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	var rawReq struct {
		Action    string `json:"action"`
		Reason    string `json:"reason"`
		Param     string `json:"param"`
		Service   string `json:"service"`
		ProcessID int    `json:"process_id"`
		Schedule  string `json:"schedule_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&rawReq); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	op, ok := operationRegistry[rawReq.Action]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported action"})
		return
	}

	if rawReq.Schedule != "" && !op.SupportsSchedule {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action does not support scheduling"})
		return
	}
	if op.RequiresReason && strings.TrimSpace(rawReq.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason required"})
		return
	}
	if op.RequiresAdmin {
		if !s.AuthorizeMSPManage(w, r, mspID) {
			return
		}
	} else if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	// Verify device exists and belongs to the authorized request scope.
	var clientID, siteID, tenantID, devStatus string
	var isActive bool
	err := s.requestDB(r).QueryRow(`
		SELECT COALESCE(d.client_id::text,''), COALESCE(d.site_id::text,''),
		       COALESCE(d.tenant_id::text,''), d.status, COALESCE(d.status, '') != 'disabled' FROM devices d
		WHERE d.id = $1 AND d.msp_id = $2
	`, deviceID, mspID).Scan(&clientID, &siteID, &tenantID, &devStatus, &isActive)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}
	if !isActive {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "device is disabled"})
		return
	}
	requestClientID, _ := r.Context().Value(ctxKeyClientID).(string)
	requestSiteID, _ := r.Context().Value(ctxKeySiteID).(string)
	if (requestClientID != "" && requestClientID != clientID) || (requestSiteID != "" && requestSiteID != siteID) {
		writeAuthorizationDenied(w)
		return
	}

	if !op.AllowOffline && devStatus != "online" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "device is offline and action does not support offline execution"})
		return
	}

	// Validate operation-specific parameters
	switch rawReq.Action {
	case "service_start", "service_stop", "service_restart":
		if rawReq.Service == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service required"})
			return
		}
	case "process_kill":
		if rawReq.ProcessID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid process_id required"})
			return
		}
	}

	// Check MSP isn't suspended
	var mspActive bool
	if err := s.requestDB(r).QueryRow(`SELECT is_active FROM msp_tenants WHERE id = $1`, mspID).Scan(&mspActive); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "msp check failed"})
		return
	}
	if !mspActive {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "msp is suspended"})
		return
	}

	var availableAt time.Time
	var expiresAt time.Time
	now := time.Now().UTC()
	availableAt = now
	if rawReq.Schedule != "" {
		t, err := time.Parse(time.RFC3339, rawReq.Schedule)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schedule_at (RFC 3339 required)"})
			return
		}
		if t.Before(now) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schedule_at must be in the future"})
			return
		}
		availableAt = t.UTC()
		expiresAt = availableAt.Add(24 * time.Hour)
	} else {
		expiresAt = now.Add(24 * time.Hour)
	}

	if op.Destructive {
		allowed, err := maintenanceWindowAllows(s.requestDB(r), clientID, deviceID, availableAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "maintenance-window check failed"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "action must execute within an applicable maintenance window"})
			return
		}
	}

	jobID := uuid.New().String()
	targetID := uuid.New().String()
	correlationID := uuid.New().String()

	payloadMap := map[string]interface{}{
		"action": rawReq.Action, "reason": rawReq.Reason,
		"service": rawReq.Service, "process_id": rawReq.ProcessID,
		"schedule_at": rawReq.Schedule,
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode job failed"})
		return
	}
	idempotencyKey, err := validateIdempotencyKey(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	requestHash, err := endpointRequestHash(map[string]interface{}{"device_id": deviceID, "job_type": op.JobType, "payload": payloadMap})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode idempotency fingerprint failed"})
		return
	}

	db := s.requestDB(r)
	if handled, err := writeIdempotentEndpointResult(w, r, db, mspID, idempotencyKey, requestHash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "idempotency lookup failed"})
		return
	} else if handled {
		return
	}
	if _, err := db.ExecContext(r.Context(), `
		INSERT INTO jobs (id, msp_id, client_id, site_id, created_by, type, status, priority,
		                  payload, max_retries, max_devices, expires_at, correlation_id,
		                  idempotency_key, request_hash, scheduled_for)
		VALUES ($1, $2, $3, $4, $5, $6, 'queued', 10, $7, 1, 1, $8, $9, NULLIF($10,''), $11, $12)
	`, jobID, mspID, clientID, siteID, "api:"+rawReq.Action, op.JobType,
		payload, expiresAt, correlationID, idempotencyKey, requestHash, availableAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create job failed"})
		return
	}

	if _, err := db.ExecContext(r.Context(), `
		INSERT INTO job_targets (id, job_id, device_id, msp_id, status)
		VALUES ($1, $2, $3, $4, 'queued')
	`, targetID, jobID, deviceID, mspID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create target failed"})
		return
	}

	outboxPayload, err := json.Marshal(map[string]interface{}{
		"job_id": jobID, "target_id": targetID, "device_id": deviceID,
		"type": op.JobType, "payload": payloadMap,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode dispatch failed"})
		return
	}
	if _, err := db.ExecContext(r.Context(), `
		INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
		VALUES (gen_random_uuid(), $1, $2, 'job.dispatch', $3, $4)
	`, mspID, jobID, outboxPayload, availableAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create dispatch failed"})
		return
	}
	if err := writeEndpointAudit(r, db, tenantID, "endpoint.action.queued", "job:"+jobID, map[string]interface{}{
		"job_id": jobID, "device_id": deviceID, "action": rawReq.Action,
		"reason": rawReq.Reason, "scheduled_for": availableAt, "correlation_id": correlationID,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write audit evidence failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"job_id": jobID, "target_id": targetID, "status": "queued",
		"action": rawReq.Action, "correlation_id": correlationID,
	})
}

func (s *APIServer) handleBulkDeviceAction(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}

	var req bulkActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if len(req.DeviceIDs) == 0 || req.Action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_ids and action required"})
		return
	}
	if len(req.DeviceIDs) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "max 100 devices per bulk operation"})
		return
	}

	op, ok := operationRegistry[req.Action]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported action"})
		return
	}
	if req.Action != "refresh" && req.Action != "reboot" && req.Action != "shutdown" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action does not support bulk execution"})
		return
	}
	if req.Schedule != "" && !op.SupportsSchedule {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action does not support scheduling"})
		return
	}
	if op.RequiresReason && strings.TrimSpace(req.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason required"})
		return
	}
	if op.RequiresAdmin {
		if !s.AuthorizeMSPManage(w, r, mspID) {
			return
		}
	} else if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	var mspActive bool
	if err := s.requestDB(r).QueryRow(`SELECT is_active FROM msp_tenants WHERE id = $1`, mspID).Scan(&mspActive); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "msp check failed"})
		return
	}
	if !mspActive {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "msp is suspended"})
		return
	}

	now := time.Now().UTC()
	availableAt := now
	expires := now.Add(24 * time.Hour)
	if req.Schedule != "" {
		scheduledAt, err := time.Parse(time.RFC3339, req.Schedule)
		if err != nil || !scheduledAt.After(now) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schedule_at must be a future RFC 3339 timestamp"})
			return
		}
		availableAt = scheduledAt.UTC()
		expires = availableAt.Add(24 * time.Hour)
	}
	jobID := uuid.New().String()
	correlationID := uuid.New().String()

	// Deduplicate device IDs
	seen := make(map[string]bool)
	var uniqueIDs []string
	for _, id := range req.DeviceIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
			return
		}
		if !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	if len(uniqueIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no valid device ids supplied"})
		return
	}
	sort.Strings(uniqueIDs)
	req.DeviceIDs = append([]string(nil), uniqueIDs...)

	// All targets must resolve inside the authorized scope; mixed or missing sets fail atomically.
	requestClientID, _ := r.Context().Value(ctxKeyClientID).(string)
	requestSiteID, _ := r.Context().Value(ctxKeySiteID).(string)
	tenantIDs := make(map[string]struct{})
	for _, deviceID := range uniqueIDs {
		var clientID, siteID, tenantID, status string
		err := s.requestDB(r).QueryRow(`
			SELECT COALESCE(client_id::text, ''), COALESCE(site_id::text, ''),
			       COALESCE(tenant_id::text, ''), status
			FROM devices WHERE id = $1 AND msp_id = $2
		`, deviceID, mspID).Scan(&clientID, &siteID, &tenantID, &status)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "one or more devices were not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device validation failed"})
			return
		}
		if status == "disabled" ||
			(requestClientID != "" && requestClientID != clientID) ||
			(requestSiteID != "" && requestSiteID != siteID) {
			writeAuthorizationDenied(w)
			return
		}
		if !op.AllowOffline && status != "online" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "one or more devices are offline and the action does not support offline execution"})
			return
		}
		if op.Destructive {
			allowed, err := maintenanceWindowAllows(s.requestDB(r), clientID, deviceID, availableAt)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "maintenance-window check failed"})
				return
			}
			if !allowed {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "all targets must be covered by an applicable maintenance window"})
				return
			}
		}
		if tenantID == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device tenant identity missing"})
			return
		}
		tenantIDs[tenantID] = struct{}{}
	}

	payload, err := json.Marshal(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode job failed"})
		return
	}
	idempotencyKey, err := validateIdempotencyKey(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	requestHash, err := endpointRequestHash(map[string]interface{}{"device_ids": uniqueIDs, "job_type": op.JobType, "payload": req})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode idempotency fingerprint failed"})
		return
	}
	db := s.requestDB(r)
	if handled, err := writeIdempotentEndpointResult(w, r, db, mspID, idempotencyKey, requestHash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "idempotency lookup failed"})
		return
	} else if handled {
		return
	}
	if _, err := db.ExecContext(r.Context(), `
		INSERT INTO jobs (id, msp_id, created_by, type, status, priority,
		                  payload, max_retries, max_devices, expires_at, correlation_id,
		                  idempotency_key, request_hash, scheduled_for)
		VALUES ($1, $2, $3, $4, 'queued', 10, $5, 1, $6, $7, $8, NULLIF($9,''), $10, $11)
	`, jobID, mspID, "api:bulk:"+req.Action, op.JobType,
		payload, len(uniqueIDs), expires, correlationID, idempotencyKey, requestHash, availableAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create job failed"})
		return
	}

	for _, deviceID := range uniqueIDs {
		targetID := uuid.New().String()
		if _, err := db.ExecContext(r.Context(), `INSERT INTO job_targets (id, job_id, device_id, msp_id, status) VALUES ($1,$2,$3,$4,'queued')`, targetID, jobID, deviceID, mspID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create target failed"})
			return
		}
		opPayload, err := json.Marshal(map[string]interface{}{
			"job_id": jobID, "target_id": targetID, "device_id": deviceID,
			"type": op.JobType, "payload": req,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode dispatch failed"})
			return
		}
		if _, err := db.ExecContext(r.Context(), `INSERT INTO job_outbox (id,msp_id,aggregate_id,event_type,payload,available_at) VALUES (gen_random_uuid(),$1,$2,'job.dispatch',$3,$4)`, mspID, jobID, opPayload, availableAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create dispatch failed"})
			return
		}
	}
	for tenantID := range tenantIDs {
		if err := writeEndpointAudit(r, db, tenantID, "endpoint.bulk_action.queued", "job:"+jobID, map[string]interface{}{
			"job_id": jobID, "device_ids": uniqueIDs, "action": req.Action,
			"reason": req.Reason, "scheduled_for": availableAt, "correlation_id": correlationID,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write audit evidence failed"})
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"job_id": jobID, "status": "queued", "action": req.Action,
		"target_count": len(uniqueIDs), "correlation_id": correlationID,
	})
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n <= 0 {
		return def
	}
	return n
}
