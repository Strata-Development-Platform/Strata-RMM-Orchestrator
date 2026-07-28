package platform

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ApprovalPolicy struct {
	ID                 string   `json:"id"`
	MSPID              string   `json:"msp_id"`
	ActionName         string   `json:"action_name"`
	ApprovalRequired   bool     `json:"approval_required"`
	MinApprovers       int      `json:"min_approvers"`
	AllowedRoles       []string `json:"allowed_roles"`
	RequireSeparation  bool     `json:"require_separation"`
	ApprovalExpiresSec int      `json:"approval_expires_sec"`
	AllowEmergency     bool     `json:"allow_emergency"`
}

type ApprovalRequest struct {
	ID                string          `json:"id"`
	MSPID             string          `json:"msp_id"`
	ClientID          string          `json:"client_id,omitempty"`
	SiteID            string          `json:"site_id,omitempty"`
	RequesterUserID   string          `json:"requester_user_id"`
	ActionName        string          `json:"action_name"`
	Reason            string          `json:"reason"`
	DeviceIDs         []string        `json:"device_ids"`
	DeviceCount       int             `json:"device_count"`
	ScheduleAt        *time.Time      `json:"schedule_at,omitempty"`
	TargetHash        string          `json:"target_hash"`
	Status            string          `json:"status"`
	PolicySnapshot    json.RawMessage `json:"policy_snapshot"`
	CorrelationID     string          `json:"correlation_id,omitempty"`
	RequestHash       string          `json:"request_hash,omitempty"`
	IdempotencyKey    string          `json:"idempotency_key,omitempty"`
	ExpiresAt         time.Time       `json:"expires_at"`
	EmergencyOverride bool            `json:"emergency_override"`
	DecidedAt         *time.Time      `json:"decided_at,omitempty"`
	DecidedBy         string          `json:"decided_by,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

type ApprovalDecision struct {
	ID             string    `json:"id"`
	RequestID      string    `json:"request_id"`
	ApproverUserID string    `json:"approver_user_id"`
	Decision       string    `json:"decision"`
	Reason         string    `json:"reason"`
	CreatedAt      time.Time `json:"created_at"`
}

type policySnapshot struct {
	ApprovalRequired   bool     `json:"approval_required"`
	MinApprovers       int      `json:"min_approvers"`
	AllowedRoles       []string `json:"allowed_roles"`
	RequireSeparation  bool     `json:"require_separation"`
	ApprovalExpiresSec int      `json:"approval_expires_sec"`
	AllowEmergency     bool     `json:"allow_emergency"`
}

func loadApprovalPolicy(db dbExecutor, mspID, actionName string) (*ApprovalPolicy, error) {
	var p ApprovalPolicy
	var allowedRolesStr string
	err := db.QueryRow(`
		SELECT id::text, msp_id::text, action_name, approval_required, min_approvers,
		       array_to_string(allowed_roles, ','), require_separation,
		       approval_expires_secs, allow_emergency
		FROM endpoint_approval_policies
		WHERE msp_id = $1 AND action_name = $2
	`, mspID, actionName).Scan(&p.ID, &p.MSPID, &p.ActionName, &p.ApprovalRequired,
		&p.MinApprovers, &allowedRolesStr, &p.RequireSeparation,
		&p.ApprovalExpiresSec, &p.AllowEmergency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if allowedRolesStr != "" {
		p.AllowedRoles = strings.Split(allowedRolesStr, ",")
	}
	if p.AllowedRoles == nil {
		p.AllowedRoles = []string{}
	}
	return &p, nil
}

func defaultApprovalPolicy(actionName string) *ApprovalPolicy {
	destructiveActions := map[string]bool{"reboot": true, "shutdown": true, "process_kill": true}
	_, isDestructive := destructiveActions[actionName]
	return &ApprovalPolicy{
		ApprovalRequired:   isDestructive,
		MinApprovers:       1,
		AllowedRoles:       []string{"msp_owner", "msp_admin"},
		RequireSeparation:  true,
		ApprovalExpiresSec: 3600,
		AllowEmergency:     false,
	}
}

func parsePolicySnapshot(snap json.RawMessage) (*policySnapshot, error) {
	if len(snap) == 0 || string(snap) == "{}" || string(snap) == "" {
		return nil, fmt.Errorf("missing or empty policy snapshot")
	}
	var ps policySnapshot
	decoder := json.NewDecoder(strings.NewReader(string(snap)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ps); err != nil {
		return nil, fmt.Errorf("malformed policy snapshot: %w", err)
	}
	return &ps, nil
}

func (s *APIServer) handleCreateApprovalRequest(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	requestClientID, _ := r.Context().Value(ctxKeyClientID).(string)
	requestSiteID, _ := r.Context().Value(ctxKeySiteID).(string)
	if mspID == "" || userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp or user context"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		ActionName string   `json:"action_name"`
		Reason     string   `json:"reason"`
		DeviceIDs  []string `json:"device_ids"`
		ScheduleAt string   `json:"schedule_at,omitempty"`
		Emergency  bool     `json:"emergency"`
		Service    string   `json:"service,omitempty"`
		ProcessID  int      `json:"process_id,omitempty"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	op, ok := operationRegistry[req.ActionName]
	if !ok || len(req.DeviceIDs) == 0 || len(req.DeviceIDs) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "supported action and 1-100 device_ids required"})
		return
	}
	if op.RequiresReason && strings.TrimSpace(req.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason required"})
		return
	}
	if req.ScheduleAt != "" && !op.SupportsSchedule {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action does not support scheduling"})
		return
	}
	if req.ActionName == "process_kill" && req.ProcessID <= 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid non-system process_id required"})
		return
	}
	if strings.HasPrefix(req.ActionName, "service_") && strings.TrimSpace(req.Service) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service required"})
		return
	}

	db := s.requestDB(r)
	policy, err := loadApprovalPolicy(db, mspID, req.ActionName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load approval policy failed"})
		return
	}
	if policy == nil {
		policy = defaultApprovalPolicy(req.ActionName)
	}
	if !policy.ApprovalRequired {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval not required for this action"})
		return
	}
	if req.Emergency && !policy.AllowEmergency {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "emergency override is not allowed by policy"})
		return
	}
	if req.Emergency && strings.TrimSpace(req.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "emergency override reason required"})
		return
	}

	now := time.Now().UTC()
	var scheduleAt *time.Time
	if req.ScheduleAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.ScheduleAt)
		if err != nil || !parsed.After(now) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schedule_at must be a future RFC3339 timestamp"})
			return
		}
		utc := parsed.UTC()
		scheduleAt = &utc
	}
	var mspActive bool
	if err := db.QueryRowContext(r.Context(), `SELECT is_active FROM msp_tenants WHERE id=$1`, mspID).Scan(&mspActive); err != nil || !mspActive {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "msp is suspended or unavailable"})
		return
	}

	seen := make(map[string]bool)
	uniqueIDs := make([]string, 0, len(req.DeviceIDs))
	for _, value := range req.DeviceIDs {
		id := strings.TrimSpace(value)
		if id == "" || seen[id] {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
			return
		}
		seen[id] = true
		uniqueIDs = append(uniqueIDs, id)
	}
	sort.Strings(uniqueIDs)
	if len(uniqueIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no valid device ids supplied"})
		return
	}

	var approvalClientID, approvalSiteID string
	for _, deviceID := range uniqueIDs {
		var clientID, siteID, status string
		err := db.QueryRowContext(r.Context(), `
			SELECT COALESCE(client_id::text,''), COALESCE(site_id::text,''), status
			FROM devices WHERE id=$1 AND msp_id=$2
		`, deviceID, mspID).Scan(&clientID, &siteID, &status)
		if err != nil || status == "disabled" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "one or more devices are unavailable"})
			return
		}
		if (requestClientID != "" && requestClientID != clientID) ||
			(requestSiteID != "" && requestSiteID != siteID) {
			writeAuthorizationDenied(w)
			return
		}
		if approvalClientID == "" {
			approvalClientID, approvalSiteID = clientID, siteID
		} else if approvalClientID != clientID || approvalSiteID != siteID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval targets must share one client and site scope"})
			return
		}
		capabilities, err := loadAgentCapabilities(db, deviceID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load capabilities failed"})
			return
		}
		if !isActionSupportedByCapabilities(req.ActionName, capabilities) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "one or more devices do not report support for this action"})
			return
		}
	}

	operationPayload := map[string]interface{}{
		"action": req.ActionName, "reason": req.Reason, "service": req.Service,
		"process_id": req.ProcessID, "schedule_at": req.ScheduleAt,
	}
	operationJSON, err := json.Marshal(operationPayload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode operation payload failed"})
		return
	}
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode policy snapshot failed"})
		return
	}
	requestHash, err := endpointRequestHash(map[string]interface{}{
		"device_ids": uniqueIDs, "operation": operationPayload, "policy": policy,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode request fingerprint failed"})
		return
	}
	idempotencyKey, err := validateIdempotencyKey(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if idempotencyKey != "" {
		var existingID, existingHash, existingStatus string
		err := db.QueryRowContext(r.Context(), `
			SELECT id::text, request_hash, status FROM endpoint_approval_requests
			WHERE msp_id=$1 AND idempotency_key=$2
		`, mspID, idempotencyKey).Scan(&existingID, &existingHash, &existingStatus)
		if err == nil {
			if existingHash != requestHash {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency key already used for a different approval request"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"approval_id": existingID, "status": existingStatus, "duplicate": true})
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "idempotency lookup failed"})
			return
		}
	}

	approvalID := uuid.NewString()
	correlationID := uuid.NewString()
	expiresAt := now.Add(time.Duration(policy.ApprovalExpiresSec) * time.Second)
	_, err = db.ExecContext(r.Context(), `
		INSERT INTO endpoint_approval_requests
			(id,msp_id,client_id,site_id,requester_user_id,action_name,reason,device_ids,
			 device_count,schedule_at,target_hash,status,policy_snapshot,operation_payload,
			 correlation_id,request_hash,idempotency_key,expires_at,emergency_override)
		VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,
		        'pending',$12,$13,$14,$15,NULLIF($16,''),$17,$18)
	`, approvalID, mspID, approvalClientID, approvalSiteID, userID, req.ActionName,
		req.Reason, "{"+strings.Join(uniqueIDs, ",")+"}", len(uniqueIDs), scheduleAt,
		requestHash, string(policyJSON), string(operationJSON), correlationID,
		requestHash, idempotencyKey, expiresAt, req.Emergency)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create approval request failed"})
		return
	}
	targets, _ := json.Marshal(uniqueIDs)
	if err := writeEndpointAuditEvidence(r, db, &EndpointAuditEntry{
		MSPID: mspID, ClientID: approvalClientID, SiteID: approvalSiteID,
		ActorUserID: userID, ActorRole: strings.Join(getRoles(r), ","),
		RequestSource: "api", NormalizedIP: requestIPAddress(r),
		Action: "endpoint.approval.created", Targets: targets, Reason: req.Reason,
		RequestHash: requestHash, IdempotencyKey: idempotencyKey,
		PolicySnapshot: policyJSON, ApprovalState: "pending",
		CorrelationID: correlationID, ScheduleAt: scheduleAt,
		StateTransition: "created->pending",
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write audit evidence failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"approval_id": approvalID, "status": "pending", "correlation_id": correlationID,
		"expires_at": expiresAt.Format(time.RFC3339), "min_approvers": policy.MinApprovers,
	})
}
func (s *APIServer) handleListApprovalRequests(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	clientID, _ := r.Context().Value(ctxKeyClientID).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(string)
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	statusFilter := r.URL.Query().Get("status")
	limit := parseInt(r.URL.Query().Get("limit"), 50)
	if limit > 200 {
		limit = 200
	}
	offset := parseInt(r.URL.Query().Get("offset"), 0)

	query := `SELECT id::text, msp_id::text, requester_user_id, action_name, reason,
	                  device_count::int, schedule_at, status, correlation_id, expires_at,
	                  emergency_override, decided_at, decided_by, created_at
	           FROM endpoint_approval_requests WHERE msp_id = $1`
	args := []interface{}{mspID}
	argIdx := 2

	if clientID != "" {
		query += fmt.Sprintf(" AND (client_id::text = $%d OR client_id IS NULL)", argIdx)
		args = append(args, clientID)
		argIdx++
	}
	if siteID != "" {
		query += fmt.Sprintf(" AND (site_id::text = $%d OR site_id IS NULL)", argIdx)
		args = append(args, siteID)
		argIdx++
	}
	if statusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}
	query += " ORDER BY created_at DESC LIMIT $%d OFFSET $%d"
	query = fmt.Sprintf(query, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.requestDB(r).QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer func() { _ = rows.Close() }()

	type approvalRow struct {
		ID                string     `json:"id"`
		MSPID             string     `json:"msp_id"`
		RequesterUserID   string     `json:"requester_user_id"`
		ActionName        string     `json:"action_name"`
		Reason            string     `json:"reason"`
		DeviceCount       int        `json:"device_count"`
		ScheduleAt        *time.Time `json:"schedule_at,omitempty"`
		Status            string     `json:"status"`
		CorrelationID     string     `json:"correlation_id,omitempty"`
		ExpiresAt         time.Time  `json:"expires_at"`
		EmergencyOverride bool       `json:"emergency_override"`
		DecidedAt         *time.Time `json:"decided_at,omitempty"`
		DecidedBy         string     `json:"decided_by,omitempty"`
		CreatedAt         time.Time  `json:"created_at"`
	}

	var result []approvalRow
	for rows.Next() {
		var a approvalRow
		if err := rows.Scan(&a.ID, &a.MSPID, &a.RequesterUserID, &a.ActionName, &a.Reason,
			&a.DeviceCount, &a.ScheduleAt, &a.Status, &a.CorrelationID, &a.ExpiresAt,
			&a.EmergencyOverride, &a.DecidedAt, &a.DecidedBy, &a.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan error"})
		return
	}
	if result == nil {
		result = []approvalRow{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"approvals": result})
}

func (s *APIServer) handleGetApprovalRequest(w http.ResponseWriter, r *http.Request) {
	approvalID := r.PathValue("approvalID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" || approvalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	var a ApprovalRequest
	var schedule sql.NullTime
	var clientID, siteID sql.NullString
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id::text, msp_id::text, COALESCE(client_id::text, ''), COALESCE(site_id::text, ''),
		       requester_user_id, action_name, reason, device_count, schedule_at, target_hash,
		       status, policy_snapshot, COALESCE(correlation_id, ''), COALESCE(request_hash, ''),
		       COALESCE(idempotency_key, ''), expires_at, emergency_override, decided_at,
		       COALESCE(decided_by, ''), created_at
		FROM endpoint_approval_requests WHERE id = $1 AND msp_id = $2
	`, approvalID, mspID).Scan(&a.ID, &a.MSPID, &clientID, &siteID, &a.RequesterUserID,
		&a.ActionName, &a.Reason, &a.DeviceCount, &schedule, &a.TargetHash,
		&a.Status, &a.PolicySnapshot, &a.CorrelationID, &a.RequestHash,
		&a.IdempotencyKey, &a.ExpiresAt, &a.EmergencyOverride, &a.DecidedAt,
		&a.DecidedBy, &a.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval request not found"})
		return
	}
	if clientID.Valid {
		a.ClientID = clientID.String
	}
	if siteID.Valid {
		a.SiteID = siteID.String
	}
	if schedule.Valid {
		a.ScheduleAt = &schedule.Time
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id::text, request_id::text, approver_user_id, decision, reason, created_at
		FROM endpoint_approval_decisions WHERE request_id = $1 ORDER BY created_at
	`, approvalID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load decisions failed"})
		return
	}
	defer func() { _ = rows.Close() }()

	var decisions []ApprovalDecision
	for rows.Next() {
		var d ApprovalDecision
		if err := rows.Scan(&d.ID, &d.RequestID, &d.ApproverUserID, &d.Decision, &d.Reason, &d.CreatedAt); err != nil {
			continue
		}
		decisions = append(decisions, d)
	}
	if decisions == nil {
		decisions = []ApprovalDecision{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"request":   a,
		"decisions": decisions,
	})
}

func (s *APIServer) handleApproveRequest(w http.ResponseWriter, r *http.Request) {
	approvalID := r.PathValue("approvalID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if mspID == "" || userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if _, err := uuid.Parse(approvalID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid approval id"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}
	var decisionReq struct{ Reason string `json:"reason"` }
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decisionReq); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	db := s.requestDB(r)
	var requesterID, status, actionName, policyText, operationText, correlationID, requestHash, deviceIDsText string
	var clientID, siteID sql.NullString
	var scheduleAt sql.NullTime
	var expiresAt time.Time
	err := db.QueryRowContext(r.Context(), `
		SELECT requester_user_id,status,action_name,policy_snapshot::text,operation_payload::text,
		       COALESCE(correlation_id,''),COALESCE(request_hash,''),array_to_string(device_ids,','),
		       client_id::text,site_id::text,schedule_at,expires_at
		FROM endpoint_approval_requests
		WHERE id=$1 AND msp_id=$2 FOR UPDATE
	`, approvalID, mspID).Scan(&requesterID,&status,&actionName,&policyText,&operationText,
		&correlationID,&requestHash,&deviceIDsText,&clientID,&siteID,&scheduleAt,&expiresAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval request not found"})
		return
	}
	if status != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "request is " + status})
		return
	}
	if !expiresAt.After(time.Now().UTC()) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval request has expired"})
		return
	}
	policy, err := parsePolicySnapshot(json.RawMessage(policyText))
	if err != nil || policy.MinApprovers < 1 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored approval policy"})
		return
	}
	if requesterID == userID && policy.RequireSeparation {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "requester cannot self-approve"})
		return
	}
	allowed := false
	for _, role := range getRoles(r) {
		for _, permitted := range policy.AllowedRoles {
			if role == permitted {
				allowed = true
			}
		}
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient role to approve"})
		return
	}
	_, err = db.ExecContext(r.Context(), `
		INSERT INTO endpoint_approval_decisions(id,request_id,msp_id,approver_user_id,decision,reason)
		VALUES(gen_random_uuid(),$1,$2,$3,'approved',$4)
	`, approvalID, mspID, userID, decisionReq.Reason)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval decision already recorded or invalid"})
		return
	}
	var approvedCount int
	if err := db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM endpoint_approval_decisions WHERE request_id=$1 AND decision='approved'
	`, approvalID).Scan(&approvedCount); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "count approvals failed"})
		return
	}
	if approvedCount < policy.MinApprovers {
		if _, err := db.ExecContext(r.Context(), `
			UPDATE endpoint_approval_requests SET updated_at=NOW() WHERE id=$1 AND status='pending'
		`, approvalID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update approval request failed"})
			return
		}
		if err := writeEndpointAuditEvidence(r, db, &EndpointAuditEntry{
			MSPID: mspID, ClientID: clientID.String, SiteID: siteID.String,
			ActorUserID: userID, ActorRole: strings.Join(getRoles(r), ","),
			RequestSource: "api", NormalizedIP: requestIPAddress(r),
			Action: "endpoint.approval.decision", Reason: decisionReq.Reason,
			RequestHash: requestHash, PolicySnapshot: json.RawMessage(policyText),
			ApprovalState: "pending", CorrelationID: correlationID,
			StateTransition: "pending->pending",
			ResultSummary: fmt.Sprintf("%d of %d approvals recorded", approvedCount, policy.MinApprovers),
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write audit evidence failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "pending", "approval_id": approvalID,
			"approvals_remaining": policy.MinApprovers - approvedCount,
		})
		return
	}

	op, ok := operationRegistry[actionName]
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "operation is no longer registered"})
		return
	}
	var operationPayload map[string]interface{}
	if err := json.Unmarshal([]byte(operationText), &operationPayload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored operation payload"})
		return
	}
	now := time.Now().UTC()
	availableAt := now
	if scheduleAt.Valid && scheduleAt.Time.After(now) {
		availableAt = scheduleAt.Time.UTC()
	}
	jobExpiresAt := availableAt.Add(24 * time.Hour)
	deviceIDs := strings.Split(deviceIDsText, ",")
	jobID := uuid.NewString()
	payloadJSON, err := json.Marshal(operationPayload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode operation payload failed"})
		return
	}
	_, err = db.ExecContext(r.Context(), `
		INSERT INTO jobs(id,msp_id,client_id,site_id,created_by,type,status,priority,payload,
		                 max_retries,max_devices,expires_at,correlation_id,request_hash,
		                 scheduled_for,approval_request_id)
		VALUES($1,$2,$3,$4,$5,$6,'queued',10,$7,1,$8,$9,$10,$11,$12,$13)
	`, jobID,mspID,clientID,siteID,"api:approval:"+actionName,op.JobType,payloadJSON,
		len(deviceIDs),jobExpiresAt,correlationID,requestHash,availableAt,approvalID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approved operation already dispatched or job creation failed"})
		return
	}

	for _, value := range deviceIDs {
		deviceID := strings.TrimSpace(value)
		var targetClientID, targetSiteID, deviceStatus, agentID string
		err := db.QueryRowContext(r.Context(), `
			SELECT COALESCE(client_id::text,''),COALESCE(site_id::text,''),status,COALESCE(agent_id,'')
			FROM devices WHERE id=$1 AND msp_id=$2
		`, deviceID,mspID).Scan(&targetClientID,&targetSiteID,&deviceStatus,&agentID)
		if err != nil || targetClientID != clientID.String || targetSiteID != siteID.String ||
			deviceStatus == "disabled" || agentID == "" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "approved target scope or agent state changed"})
			return
		}
		capabilities, err := loadAgentCapabilities(db, deviceID)
		if err != nil || !isActionSupportedByCapabilities(actionName, capabilities) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "approved target no longer supports this action"})
			return
		}
		if op.Destructive {
			covered, err := maintenanceWindowAllows(db, targetClientID, deviceID, availableAt)
			if err != nil || !covered {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "approved target is outside an applicable maintenance window"})
				return
			}
		}
		targetStatus := "queued"
		if deviceStatus != "online" {
			if !op.AllowOffline {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "approved target is offline"})
				return
			}
			targetStatus = "waiting"
		}
		targetID := uuid.NewString()
		_, err = db.ExecContext(r.Context(), `
			INSERT INTO job_targets(id,job_id,device_id,agent_id,msp_id,status,approval_status,
			                        offline_at)
			VALUES($1,$2,$3,$4,$5,$6,'approved',
			       CASE WHEN $6='waiting' THEN NOW() ELSE NULL END)
		`,targetID,jobID,deviceID,agentID,mspID,targetStatus)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create approved target failed"})
			return
		}
		if targetStatus == "queued" {
			eventID := jobID+":"+targetID+":1"
			outboxPayload, err := json.Marshal(map[string]interface{}{
				"schema_version":1,"event_id":eventID,"job_id":jobID,"target_id":targetID,
				"msp_id":mspID,"client_id":targetClientID,"site_id":targetSiteID,
				"device_id":deviceID,"agent_id":agentID,"correlation_id":correlationID,
				"attempt":1,"issued_at":now.Format(time.RFC3339),
				"expires_at":jobExpiresAt.Format(time.RFC3339),
				"command_type":op.JobType,"type":op.JobType,"payload":operationPayload,
			})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode dispatch failed"})
				return
			}
			if _, err := db.ExecContext(r.Context(), `
				INSERT INTO job_outbox(id,msp_id,aggregate_id,event_type,payload,available_at)
				VALUES(gen_random_uuid(),$1,$2,'job.dispatch',$3,$4)
			`,mspID,jobID,outboxPayload,availableAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create dispatch failed"})
				return
			}
		}
	}
	result, err := db.ExecContext(r.Context(), `
		UPDATE endpoint_approval_requests SET status='dispatched',decided_at=NOW(),
		       decided_by=$1,updated_at=NOW() WHERE id=$2 AND status='pending'
	`,userID,approvalID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "finalize approval failed"})
		return
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval state changed"})
		return
	}
	targets, _ := json.Marshal(deviceIDs)
	if err := writeEndpointAuditEvidence(r, db, &EndpointAuditEntry{
		MSPID:mspID,ClientID:clientID.String,SiteID:siteID.String,
		ActorUserID:userID,ActorRole:strings.Join(getRoles(r),","),
		RequestSource:"api",NormalizedIP:requestIPAddress(r),
		Action:"endpoint.approval.dispatched",Targets:targets,
		Reason:decisionReq.Reason,RequestHash:requestHash,
		PolicySnapshot:json.RawMessage(policyText),ApprovalState:"dispatched",
		JobID:jobID,CorrelationID:correlationID,ScheduleAt:&availableAt,
		StateTransition:"pending->dispatched",
		ResultSummary:fmt.Sprintf("%d approvals satisfied",approvedCount),
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write audit evidence failed"})
		return
	}
	writeJSON(w,http.StatusOK,map[string]interface{}{"status":"dispatched","approval_id":approvalID,"job_id":jobID})
}
func (s *APIServer) handleRejectRequest(w http.ResponseWriter, r *http.Request) {
	approvalID := r.PathValue("approvalID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if mspID == "" || userID == "" || approvalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	var requesterID, status, policySnapStr string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT requester_user_id, status, COALESCE(policy_snapshot::text, '{}')
		FROM endpoint_approval_requests
		WHERE id = $1 AND msp_id = $2
		FOR UPDATE
	`, approvalID, mspID).Scan(&requesterID, &status, &policySnapStr)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval request not found"})
		return
	}
	if status != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "request is " + status})
		return
	}
	if requesterID == userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "requester cannot reject own request"})
		return
	}

	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE endpoint_approval_requests SET status='rejected', decided_at=NOW(),
		       decided_by=$1, updated_at=NOW() WHERE id=$2 AND status='pending'
	`, userID, approvalID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reject failed"})
		return
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval state changed"})
		return
	}

	auditEntry := EndpointAuditEntry{
		MSPID: mspID, ActorUserID: userID, Action: "endpoint.approval.rejected",
		ApprovalState: "rejected", StateTransition: "pending->rejected",
		PolicySnapshot: json.RawMessage(policySnapStr),
	}
	if err := writeEndpointAuditEvidence(r, s.requestDB(r), &auditEntry); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write audit evidence failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

func (s *APIServer) handleCancelApprovalRequest(w http.ResponseWriter, r *http.Request) {
	approvalID := r.PathValue("approvalID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if mspID == "" || approvalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	var requesterID, status, policySnapStr string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT requester_user_id, status, COALESCE(policy_snapshot::text, '{}')
		FROM endpoint_approval_requests
		WHERE id = $1 AND msp_id = $2
		FOR UPDATE
	`, approvalID, mspID).Scan(&requesterID, &status, &policySnapStr)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval request not found"})
		return
	}
	if status != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "request is " + status})
		return
	}
	if requesterID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the requester can cancel"})
		return
	}

	_, err = s.requestDB(r).ExecContext(r.Context(), `
		UPDATE endpoint_approval_requests SET status='cancelled', decided_at=NOW(),
		       decided_by=$1, updated_at=NOW() WHERE id=$2 AND status='pending'
	`, userID, approvalID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cancel failed"})
		return
	}

	auditEntry := EndpointAuditEntry{
		MSPID: mspID, ActorUserID: userID, Action: "endpoint.approval.cancelled",
		ApprovalState: "cancelled", StateTransition: "pending->cancelled",
		PolicySnapshot: json.RawMessage(policySnapStr),
	}
	if err := writeEndpointAuditEvidence(r, s.requestDB(r), &auditEntry); err != nil {
		s.logger.Warn("write audit evidence", zap.Error(err))
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *APIServer) handleApprovalDecisionHistory(w http.ResponseWriter, r *http.Request) {
	approvalID := r.PathValue("approvalID")
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if mspID == "" || approvalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id::text, request_id::text, approver_user_id, decision, reason, created_at
		FROM endpoint_approval_decisions WHERE request_id = $1 ORDER BY created_at
	`, approvalID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer func() { _ = rows.Close() }()

	var decisions []ApprovalDecision
	for rows.Next() {
		var d ApprovalDecision
		if err := rows.Scan(&d.ID, &d.RequestID, &d.ApproverUserID, &d.Decision, &d.Reason, &d.CreatedAt); err != nil {
			continue
		}
		decisions = append(decisions, d)
	}
	if decisions == nil {
		decisions = []ApprovalDecision{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"decisions": decisions})
}

//nolint:unused
func expirePendingApprovals(db dbExecutor) {
	_, err := db.Exec(`
		UPDATE endpoint_approval_requests SET status='expired', updated_at=NOW()
		WHERE status='pending' AND expires_at < NOW()
	`)
	if err != nil {
		fmt.Printf("expire pending approvals: %v\n", err)
	}
}

func transitionApprovalStatus(currentStatus, nextStatus string) error {
	valid := map[string]map[string]bool{
		"pending":   {"approved": true, "rejected": true, "cancelled": true, "expired": true},
		"approved":  {"dispatched": true, "expired": true},
		"rejected":  {},
		"cancelled": {},
		"expired":   {},
		"dispatched": {},
	}
	transitions, ok := valid[currentStatus]
	if !ok || !transitions[nextStatus] {
		return fmt.Errorf("invalid approval state transition: %s -> %s", currentStatus, nextStatus)
	}
	return nil
}
