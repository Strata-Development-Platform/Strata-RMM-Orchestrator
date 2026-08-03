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

func maintenanceWindowAllows(db dbExecutor, mspID, clientID, siteID, deviceID string, executeAt time.Time) (bool, error) {
	var configured, covered bool
	err := db.QueryRow(`
		SELECT
			EXISTS (
				SELECT 1 FROM maintenance_windows
				WHERE (msp_id = $1 OR client_id = $2 OR site_id = $3 OR device_id = $4)
				  AND end_time >= NOW() AND (expires_at IS NULL OR expires_at >= NOW())
			),
			EXISTS (
				SELECT 1 FROM maintenance_windows
				WHERE (msp_id = $1 OR client_id = $2 OR site_id = $3 OR device_id = $4)
				  AND start_time <= $5 AND end_time >= $5
				  AND (expires_at IS NULL OR expires_at >= $5)
			)
	`, mspID, clientID, siteID, deviceID, executeAt).Scan(&configured, &covered)
	if err != nil {
		return false, err
	}
	return !configured || covered, nil
}

func checkApprovalRequired(db dbExecutor, mspID, actionName string, isBulk bool) (*ApprovalPolicy, bool, error) {
	policy, err := loadApprovalPolicy(db, mspID, actionName)
	if err != nil {
		return nil, false, err
	}
	if policy == nil {
		policy = defaultApprovalPolicy(actionName)
	}
	if isBulk && policy.ApprovalRequired {
		policy.ApprovalRequired = true
	}
	return policy, policy.ApprovalRequired, nil
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
		allowed, err := maintenanceWindowAllows(s.requestDB(r), mspID, clientID, siteID, deviceID, availableAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "maintenance-window check failed"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "action must execute within an applicable maintenance window"})
			return
		}
	}

	cap, err := loadAgentCapabilities(s.requestDB(r), deviceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "capability check failed"})
		return
	}
	if cap != nil && !isActionSupportedByCapabilities(rawReq.Action, cap) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "action unsupported by agent capabilities"})
		return
	}

	_, approvalRequired, err := checkApprovalRequired(s.requestDB(r), mspID, rawReq.Action, false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approval policy check failed"})
		return
	}
	if approvalRequired {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "action requires approval; use /api/v2/approvals"})
		return
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
			allowed, err := maintenanceWindowAllows(s.requestDB(r), mspID, clientID, siteID, deviceID, availableAt)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "maintenance-window check failed"})
				return
			}
			if !allowed {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "all targets must be covered by an applicable maintenance window"})
				return
			}
		}

		cap, err := loadAgentCapabilities(s.requestDB(r), deviceID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "capability check failed"})
			return
		}
		if cap != nil && !isActionSupportedByCapabilities(req.Action, cap) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "one or more devices do not support this action"})
			return
		}
		if tenantID == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device tenant identity missing"})
			return
		}
		tenantIDs[tenantID] = struct{}{}
	}

	_, approvalRequired, err := checkApprovalRequired(s.requestDB(r), mspID, req.Action, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approval policy check failed"})
		return
	}
	if approvalRequired {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "bulk action requires approval; use /api/v2/approvals"})
		return
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

// CMDB - Device Relationships

func (s *APIServer) handleGetDeviceRelationships(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, source_device_id, target_device_id, relationship_type, metadata,
		       is_active, verified_at, created_at
		FROM device_relationships WHERE msp_id = $1
		ORDER BY created_at DESC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var relationships []map[string]interface{}
	for rows.Next() {
		var id, srcID, tgtID, relType string
		var metadata []byte
		var isActive bool
		var verifiedAt sql.NullTime
		var createdAt time.Time

		err := rows.Scan(&id, &srcID, &tgtID, &relType, &metadata, &isActive, &verifiedAt, &createdAt)
		if err != nil {
			continue
		}

		rel := map[string]interface{}{
			"id":                id,
			"source_device_id":  srcID,
			"target_device_id":  tgtID,
			"relationship_type": relType,
			"is_active":         isActive,
			"created_at":        createdAt,
		}
		if verifiedAt.Valid {
			rel["verified_at"] = verifiedAt.Time
		}
		if metadata != nil && len(metadata) > 0 {
			var meta map[string]interface{}
			if err := json.Unmarshal(metadata, &meta); err == nil {
				rel["metadata"] = meta
			}
		}
		relationships = append(relationships, rel)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"relationships": relationships})
}

func (s *APIServer) handleCreateDeviceRelationship(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	var req struct {
		SourceDeviceID   string                 `json:"source_device_id"`
		TargetDeviceID   string                 `json:"target_device_id"`
		RelationshipType string                 `json:"relationship_type"`
		ClientID         string                 `json:"client_id,omitempty"`
		SiteID           string                 `json:"site_id,omitempty"`
		Metadata         map[string]interface{} `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.SourceDeviceID == "" || req.TargetDeviceID == "" || req.RelationshipType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_device_id, target_device_id, and relationship_type required"})
		return
	}

	tx, err := s.db.DB().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database unavailable"})
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(r.Context(), `
		SELECT set_config('app.msp_id', $1, true),
		       set_config('app.user_id', (SELECT id::text FROM users WHERE id = (SELECT user_id FROM memberships WHERE scope_type = 'msp' AND scope_id = $1 AND status = 'active' LIMIT 1))::text, true),
		       set_config('app.role', 'msp_admin', true),
		       set_config('app.permission', 'write', true)
	`, mspID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	owners, err := s.ValidateDeviceAncestry(r.Context(), tx, []string{req.SourceDeviceID, req.TargetDeviceID}, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}
	if owners == nil || len(owners) != 2 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "source or target device not found in authorized MSP scope"})
		return
	}

	// Verify caller-supplied client_id/site_id against device ancestry.
	// Derive client/site from the source device when not provided.
	var resolvedClient, resolvedSite string
	if owners[0].ClientID != "" {
		resolvedClient = owners[0].ClientID
	}
	if owners[0].SiteID != "" {
		resolvedSite = owners[0].SiteID
	}
	if req.ClientID != "" && req.ClientID != resolvedClient && resolvedClient != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "client_id does not match device ancestry"})
		return
	}
	if req.SiteID != "" && req.SiteID != resolvedSite && resolvedSite != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "site_id does not match device ancestry"})
		return
	}
	finalClient := req.ClientID
	if finalClient == "" {
		finalClient = resolvedClient
	}
	finalSite := req.SiteID
	if finalSite == "" {
		finalSite = resolvedSite
	}

	var id string
	metadataJSON, _ := json.Marshal(req.Metadata)
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO device_relationships (msp_id, client_id, site_id, source_device_id, target_device_id, relationship_type, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (msp_id, source_device_id, target_device_id, relationship_type) DO UPDATE SET
			client_id = $2,
			site_id = $3,
			metadata = $7,
			is_active = true,
			updated_at = NOW()
		RETURNING id
	`, mspID, finalClient, finalSite, req.SourceDeviceID, req.TargetDeviceID, req.RelationshipType, metadataJSON).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":  id,
		"msg": "device relationship created/updated",
	})
}

func (s *APIServer) handleDeleteDeviceRelationship(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}
	relationshipID := r.PathValue("relationshipID")

	_, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE device_relationships SET is_active = false WHERE id = $1 AND msp_id = $2
	`, relationshipID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"msg": "device relationship deactivated",
	})
}

func (s *APIServer) handleGetDeviceDependencies(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, source_device_id, target_device_id, relationship_type, metadata, is_active
		FROM device_relationships
		WHERE (source_device_id = $1 OR target_device_id = $1) AND msp_id = $2
		ORDER BY created_at DESC
	`, deviceID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}
	defer rows.Close()

	var dependencies []map[string]interface{}
	for rows.Next() {
		var id, srcID, tgtID, relType string
		var metadata []byte
		var isActive bool

		err := rows.Scan(&id, &srcID, &tgtID, &relType, &metadata, &isActive)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan relationship"})
			return
		}

		dep := map[string]interface{}{
			"id":                id,
			"source_device_id":  srcID,
			"target_device_id":  tgtID,
			"relationship_type": relType,
			"is_active":         isActive,
		}
		if metadata != nil && len(metadata) > 0 {
			var meta map[string]interface{}
			if err := json.Unmarshal(metadata, &meta); err == nil {
				dep["metadata"] = meta
			}
		}
		dependencies = append(dependencies, dep)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":    deviceID,
		"dependencies": dependencies,
		"count":        len(dependencies),
	})
}

func (s *APIServer) handleGetDeviceImpact(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT source_device_id, relationship_type
		FROM device_relationships
		WHERE target_device_id = $1 AND msp_id = $2 AND is_active = true
	`, deviceID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}
	defer rows.Close()

	var impacted []map[string]interface{}
	for rows.Next() {
		var srcID, relType string
		if err := rows.Scan(&srcID, &relType); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan relationship"})
			return
		}
		impacted = append(impacted, map[string]interface{}{
			"affected_device_id": srcID,
			"relationship_type":  relType,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":        deviceID,
		"impacted_by":      deviceID,
		"affected_count":   len(impacted),
		"affected_devices": impacted,
	})
}

// CMDB - Network Addresses

func (s *APIServer) handleGetNetworkAddresses(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, device_id, ip_address, ip_family, network_type, interface_name,
		       vlan_id, subnet_cidr, is_primary, created_at
		FROM network_addresses WHERE msp_id = $1
		ORDER BY created_at DESC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}
	defer rows.Close()

	var addresses []map[string]interface{}
	for rows.Next() {
		var id, deviceID string
		var ipAddress string
		var ipFamily, vlanID int
		var networkType, interfaceName, subnetCIDR string
		var isPrimary bool
		var createdAt time.Time

		err := rows.Scan(&id, &deviceID, &ipAddress, &ipFamily, &networkType, &interfaceName, &vlanID, &subnetCIDR, &isPrimary, &createdAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan network address"})
			return
		}

		addr := map[string]interface{}{
			"id":           id,
			"device_id":    deviceID,
			"ip_address":   ipAddress,
			"ip_family":    ipFamily,
			"network_type": networkType,
			"is_primary":   isPrimary,
			"created_at":   createdAt,
		}
		if interfaceName != "" {
			addr["interface_name"] = interfaceName
		}
		if subnetCIDR != "" {
			addr["subnet_cidr"] = subnetCIDR
		}
		if vlanID > 0 {
			addr["vlan_id"] = vlanID
		}
		addresses = append(addresses, addr)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"addresses": addresses,
		"count":     len(addresses),
	})
}

func (s *APIServer) handleSubmitNetworkAddress(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	var req struct {
		DeviceID      string `json:"device_id"`
		IPAddress     string `json:"ip_address"`
		IPFamily      int    `json:"ip_family,omitempty"`
		NetworkType   string `json:"network_type"`
		InterfaceName string `json:"interface_name,omitempty"`
		VlanID        int    `json:"vlan_id,omitempty"`
		SubnetCIDR    string `json:"subnet_cidr,omitempty"`
		IsPrimary     bool   `json:"is_primary"`
		ClientID      string `json:"client_id,omitempty"`
		SiteID        string `json:"site_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.DeviceID == "" || req.IPAddress == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id and ip_address required"})
		return
	}

	tx, err := s.db.DB().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database unavailable"})
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(r.Context(), `
		SELECT set_config('app.msp_id', $1, true),
		       set_config('app.user_id', (SELECT id::text FROM users WHERE id = (SELECT user_id FROM memberships WHERE scope_type = 'msp' AND scope_id = $1 AND status = 'active' LIMIT 1))::text, true),
		       set_config('app.role', 'msp_admin', true),
		       set_config('app.permission', 'write', true)
	`, mspID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	owners, err := s.ValidateDeviceAncestry(r.Context(), tx, []string{req.DeviceID}, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}
	if owners == nil || len(owners) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "device not found in authorized MSP scope"})
		return
	}

	// Verify caller-supplied client_id/site_id against device ancestry.
	var resolvedClient, resolvedSite string
	if owners[0].ClientID != "" {
		resolvedClient = owners[0].ClientID
	}
	if owners[0].SiteID != "" {
		resolvedSite = owners[0].SiteID
	}
	if req.ClientID != "" && req.ClientID != resolvedClient && resolvedClient != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "client_id does not match device ancestry"})
		return
	}
	if req.SiteID != "" && req.SiteID != resolvedSite && resolvedSite != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "site_id does not match device ancestry"})
		return
	}
	finalClient := req.ClientID
	if finalClient == "" {
		finalClient = resolvedClient
	}
	finalSite := req.SiteID
	if finalSite == "" {
		finalSite = resolvedSite
	}

	if req.IPFamily == 0 {
		req.IPFamily = 4
	}
	if req.NetworkType == "" {
		req.NetworkType = "internal"
	}

	var id string
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO network_addresses (msp_id, client_id, site_id, device_id, ip_address, ip_family, network_type, interface_name, vlan_id, subnet_cidr, is_primary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (device_id, ip_address) DO UPDATE SET
			client_id = $2,
			site_id = $3,
			network_type = $7,
			interface_name = $8,
			vlan_id = $9,
			subnet_cidr = $10,
			is_primary = $11,
			updated_at = NOW()
		RETURNING id
	`, mspID, finalClient, finalSite, req.DeviceID, req.IPAddress, req.IPFamily, req.NetworkType, req.InterfaceName, req.VlanID, req.SubnetCIDR, req.IsPrimary).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":  id,
		"msg": "network address added/updated",
	})
}

// CMDB - Device Inventory

func (s *APIServer) handleGetDevicePackages(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, device_id, name, version, release, arch, source, install_date, package_type, status, created_at
		FROM device_packages WHERE device_id = $1 AND msp_id = $2
		ORDER BY created_at DESC
	`, deviceID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}
	defer rows.Close()

	var packagesList []map[string]interface{}
	for rows.Next() {
		var id, devID, name, version, release, arch, source, pkgType, status string
		var installDate time.Time
		var createdAt time.Time

		err := rows.Scan(&id, &devID, &name, &version, &release, &arch, &source, &installDate, &pkgType, &status, &createdAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan device package"})
			return
		}

		pkg := map[string]interface{}{
			"id":         id,
			"device_id":  devID,
			"name":       name,
			"version":    version,
			"status":     status,
			"created_at": createdAt,
		}
		if release != "" {
			pkg["release"] = release
		}
		if arch != "" {
			pkg["arch"] = arch
		}
		if source != "" {
			pkg["source"] = source
		}
		if !installDate.IsZero() {
			pkg["install_date"] = installDate
		}
		packagesList = append(packagesList, pkg)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id": deviceID,
		"packages":  packagesList,
		"count":     len(packagesList),
	})
}

func (s *APIServer) handleSubmitDevicePackages(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	var req struct {
		Packages []struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Release     string `json:"release,omitempty"`
			Arch        string `json:"arch,omitempty"`
			Source      string `json:"source,omitempty"`
			InstallDate string `json:"install_date,omitempty"`
			PackageType string `json:"package_type,omitempty"`
		} `json:"packages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if len(req.Packages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "packages array required"})
		return
	}

	tx, err := s.db.DB().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database unavailable"})
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(r.Context(), `
		SELECT set_config('app.msp_id', $1, true),
		       set_config('app.user_id', (SELECT id::text FROM users WHERE id = (SELECT user_id FROM memberships WHERE scope_type = 'msp' AND scope_id = $1 AND status = 'active' LIMIT 1))::text, true),
		       set_config('app.role', 'msp_admin', true),
		       set_config('app.permission', 'write', true)
	`, mspID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	owners, err := s.ValidateDeviceAncestry(r.Context(), tx, []string{deviceID}, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}
	if owners == nil || len(owners) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "device not found in authorized MSP scope"})
		return
	}

	// Verify caller-supplied client_id/site_id against device ancestry.
	if owners[0].ClientID != "" {
		var resolvedClient string
		if err := tx.QueryRowContext(r.Context(), `SELECT COALESCE(client_id::text,'') FROM devices WHERE id = $1::uuid`, deviceID).Scan(&resolvedClient); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
			return
		}
		_ = resolvedClient
	}

	inserted := 0
	for _, pkg := range req.Packages {
		if pkg.Name == "" || pkg.Version == "" {
			continue
		}
		pkgType := pkg.PackageType
		if pkgType == "" {
			pkgType = "deb"
		}

		var installDate time.Time
		if pkg.InstallDate != "" {
			installDate, err = time.Parse(time.RFC3339, pkg.InstallDate)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid install_date format"})
				return
			}
		}

		_, err = tx.ExecContext(r.Context(), `
			INSERT INTO device_packages (device_id, msp_id, name, version, release, arch, source, install_date, package_type, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'installed')
			ON CONFLICT (device_id, name) DO UPDATE SET
				version = $4,
				release = $5,
				arch = $6,
				source = $7,
				install_date = $8,
				package_type = $9,
				updated_at = NOW()
		`, deviceID, mspID, pkg.Name, pkg.Version, pkg.Release, pkg.Arch, pkg.Source, installDate, pkgType)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to sync package"})
			return
		}
		inserted++
	}

	if err = tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit transaction"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id": deviceID,
		"msg":       fmt.Sprintf("synced %d packages", inserted),
	})
}

func (s *APIServer) handleGetDeviceServices(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, device_id, name, port, protocol, state, process_name, binary_path, created_at
		FROM device_services WHERE device_id = $1 AND msp_id = $2
		ORDER BY created_at DESC
	`, deviceID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}
	defer rows.Close()

	var services []map[string]interface{}
	for rows.Next() {
		var id, devID, name, protocol, state, procName, binPath string
		var port int
		var createdAt time.Time

		err := rows.Scan(&id, &devID, &name, &port, &protocol, &state, &procName, &binPath, &createdAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan device service"})
			return
		}

		svc := map[string]interface{}{
			"id":         id,
			"device_id":  devID,
			"name":       name,
			"protocol":   protocol,
			"state":      state,
			"created_at": createdAt,
		}
		if port > 0 {
			svc["port"] = port
		}
		if procName != "" {
			svc["process_name"] = procName
		}
		if binPath != "" {
			svc["binary_path"] = binPath
		}
		services = append(services, svc)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id": deviceID,
		"services":  services,
		"count":     len(services),
	})
}

// sanitizeDBError prevents database error details from leaking to clients.
func sanitizeDBError(err string) string {
	if strings.Contains(err, "pq:") || strings.Contains(err, "sql:") || strings.Contains(err, "driver:") {
		return "internal server error"
	}
	return err
}
