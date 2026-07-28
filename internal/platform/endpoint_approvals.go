package platform

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	ID               string          `json:"id"`
	MSPID            string          `json:"msp_id"`
	ClientID         string          `json:"client_id,omitempty"`
	SiteID           string          `json:"site_id,omitempty"`
	RequesterUserID  string          `json:"requester_user_id"`
	ActionName       string          `json:"action_name"`
	Reason           string          `json:"reason"`
	DeviceIDs        []string        `json:"device_ids"`
	DeviceCount      int             `json:"device_count"`
	ScheduleAt       *time.Time      `json:"schedule_at,omitempty"`
	TargetHash       string          `json:"target_hash"`
	Status           string          `json:"status"`
	PolicySnapshot   json.RawMessage `json:"policy_snapshot"`
	CorrelationID    string          `json:"correlation_id,omitempty"`
	RequestHash      string          `json:"request_hash,omitempty"`
	IdempotencyKey   string          `json:"idempotency_key,omitempty"`
	ExpiresAt        time.Time       `json:"expires_at"`
	EmergencyOverride bool           `json:"emergency_override"`
	DecidedAt        *time.Time      `json:"decided_at,omitempty"`
	DecidedBy        string          `json:"decided_by,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

type ApprovalDecision struct {
	ID             string    `json:"id"`
	RequestID      string    `json:"request_id"`
	ApproverUserID string    `json:"approver_user_id"`
	Decision       string    `json:"decision"`
	Reason         string    `json:"reason"`
	CreatedAt      time.Time `json:"created_at"`
}

func loadApprovalPolicy(db dbExecutor, mspID, actionName string) (*ApprovalPolicy, error) {
	var p ApprovalPolicy
	var allowedRolesStr string
	err := db.QueryRow(`
		SELECT id::text, msp_id::text, action_name, approval_required, min_approvers,
		       array_to_string(allowed_roles, ','), require_separation,
		       approval_expires_sec, allow_emergency
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

func (s *APIServer) handleCreateApprovalRequest(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if mspID == "" || userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp or user context"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	var req struct {
		ActionName  string   `json:"action_name"`
		Reason      string   `json:"reason"`
		DeviceIDs   []string `json:"device_ids"`
		ScheduleAt  string   `json:"schedule_at,omitempty"`
		Emergency   bool     `json:"emergency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ActionName == "" || len(req.DeviceIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action_name and device_ids required"})
		return
	}
	if len(req.DeviceIDs) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "max 100 devices per request"})
		return
	}

	op, ok := operationRegistry[req.ActionName]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported action"})
		return
	}
	if op.RequiresReason && strings.TrimSpace(req.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason required"})
		return
	}

	policy, err := loadApprovalPolicy(s.requestDB(r), mspID, req.ActionName)
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

	now := time.Now().UTC()
	var availableAt *time.Time
	if req.ScheduleAt != "" {
		t, err := time.Parse(time.RFC3339, req.ScheduleAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schedule_at"})
			return
		}
		if t.Before(now) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schedule_at must be in the future"})
			return
		}
		availableAt = &t
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

	uniqueIDs := make([]string, 0, len(req.DeviceIDs))
	seen := make(map[string]bool)
	for _, id := range req.DeviceIDs {
		id = strings.TrimSpace(id)
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

	for _, deviceID := range uniqueIDs {
		var status string
		err := s.requestDB(r).QueryRow(`SELECT status FROM devices WHERE id = $1 AND msp_id = $2`, deviceID, mspID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "one or more devices not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device validation failed"})
			return
		}
		if status == "disabled" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "one or more devices are disabled"})
			return
		}
	}

	correlationID := uuid.New().String()
	targetHashRaw, _ := json.Marshal(map[string]interface{}{
		"device_ids": uniqueIDs, "action": req.ActionName,
	})
	targetHash := fmt.Sprintf("%x", sha256.Sum256(targetHashRaw))

	policySnap, _ := json.Marshal(policy)
	expiresAt := now.Add(time.Duration(policy.ApprovalExpiresSec) * time.Second)

	approvalID := uuid.New().String()
	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO endpoint_approval_requests
			(id, msp_id, requester_user_id, action_name, reason, device_ids, device_count,
			 schedule_at, target_hash, status, policy_snapshot, correlation_id, request_hash,
			 expires_at, emergency_override)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending', $10, $11, $12, $13, $14)
	`, approvalID, mspID, userID, req.ActionName, req.Reason,
		"{"+strings.Join(uniqueIDs, ",")+"}", len(uniqueIDs),
		availableAt, targetHash, string(policySnap),
		correlationID, targetHash, expiresAt, req.Emergency)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create approval request failed"})
		return
	}

	var mspSlug string
	_ = s.requestDB(r).QueryRow(`SELECT slug FROM msp_tenants WHERE id = $1`, mspID).Scan(&mspSlug)
	if err := writeEndpointAudit(r, s.requestDB(r), mspID, "endpoint.approval.created", "approval:"+approvalID, map[string]interface{}{
		"approval_id": approvalID, "action": req.ActionName, "device_count": len(uniqueIDs),
		"correlation_id": correlationID, "requires_approval": true,
	}); err != nil {
		s.logger.Warn("write approval audit", zap.Error(err))
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"approval_id": approvalID, "status": "pending",
		"correlation_id": correlationID, "expires_at": expiresAt.UTC().Format(time.RFC3339),
		"min_approvers": policy.MinApprovers, "allowed_roles": policy.AllowedRoles,
		"require_separation": policy.RequireSeparation,
	})
}

func (s *APIServer) handleListApprovalRequests(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
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
	roles := getRoles(r)
	if mspID == "" || userID == "" || approvalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	var requesterID, status, actionName, policySnapStr, correlationID, targetHash string
	var deviceCount int
	var requireSeparation bool
	var minApprovers int
	var allowedRolesStr, deviceIDsStr string
	var expiresAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT ar.requester_user_id, ar.status, ar.action_name, ar.policy_snapshot::text,
		       ar.correlation_id, COALESCE(ar.target_hash, ''), ar.device_count,
		       COALESCE(p.require_separation, true), COALESCE(p.min_approvers, 1),
		       COALESCE(array_to_string(p.allowed_roles, ','), 'msp_owner,msp_admin'),
		       ar.expires_at, array_to_string(ar.device_ids, ',')
		FROM endpoint_approval_requests ar
		LEFT JOIN endpoint_approval_policies p ON p.msp_id = ar.msp_id AND p.action_name = ar.action_name
		WHERE ar.id = $1 AND ar.msp_id = $2
	`, approvalID, mspID).Scan(&requesterID, &status, &actionName, &policySnapStr,
		&correlationID, &targetHash, &deviceCount, &requireSeparation,
		&minApprovers, &allowedRolesStr, &expiresAt, &deviceIDsStr)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval request not found"})
		return
	}
	if status != "pending" {
		if status == "approved" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "request already approved"})
		} else {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "request is " + status})
		}
		return
	}
	if time.Now().After(expiresAt) {
		s.requestDB(r).ExecContext(r.Context(),
			`UPDATE endpoint_approval_requests SET status='expired', updated_at=NOW() WHERE id=$1`, approvalID)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval request has expired"})
		return
	}

	if requesterID == userID && requireSeparation {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "requester cannot self-approve"})
		return
	}

	allowedRoles := strings.Split(allowedRolesStr, ",")
	hasAllowedRole := false
	for _, role := range roles {
		for _, allowed := range allowedRoles {
			if role == allowed {
				hasAllowedRole = true
				break
			}
		}
		if hasAllowedRole {
			break
		}
	}
	if !hasAllowedRole {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient role to approve"})
		return
	}

	tx, err := s.db.DB().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "begin transaction failed"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	var existingDecision string
	err = tx.QueryRowContext(r.Context(), `
		SELECT decision FROM endpoint_approval_decisions
		WHERE request_id = $1 AND approver_user_id = $2
	`, approvalID, userID).Scan(&existingDecision)
	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already submitted a decision"})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "check decision failed"})
		return
	}

	var approvedCount int
	_ = tx.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM endpoint_approval_decisions
		WHERE request_id = $1 AND decision = 'approved'
	`, approvalID).Scan(&approvedCount)

	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO endpoint_approval_decisions (id, request_id, approver_user_id, decision, reason)
		VALUES (gen_random_uuid(), $1, $2, 'approved', $3)
	`, approvalID, userID, req.Reason); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "record decision failed"})
		return
	}

	now := time.Now()
	if approvedCount+1 >= minApprovers {
		var locked bool
		err = tx.QueryRowContext(r.Context(), `
			UPDATE endpoint_approval_requests
			SET status = 'approved', decided_at = $1, decided_by = $2, updated_at = $1
			WHERE id = $3 AND status = 'pending'
			RETURNING true
		`, now, userID, approvalID).Scan(&locked)
		if err == sql.ErrNoRows {
			_ = tx.Rollback()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "request state changed"})
			return
		}
		if err != nil {
			_ = tx.Rollback()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approve failed"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	if approvedCount+1 >= minApprovers {
		deviceIDs := strings.Split(deviceIDsStr, ",")
		s.dispatchApprovedAction(mspID, approvalID, actionName, deviceIDs, correlationID, targetHash)
	}

	if err := writeEndpointAudit(r, s.requestDB(r), mspID, "endpoint.approval.approved", "approval:"+approvalID, map[string]interface{}{
		"approval_id": approvalID, "approver": userID, "action": actionName,
	}); err != nil {
		s.logger.Warn("write approval audit", zap.Error(err))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "approved", "approval_id": approvalID,
	})
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

	var requesterID, status string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT requester_user_id, status FROM endpoint_approval_requests
		WHERE id = $1 AND msp_id = $2
	`, approvalID, mspID).Scan(&requesterID, &status)
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

	_, err = s.requestDB(r).ExecContext(r.Context(), `
		UPDATE endpoint_approval_requests SET status='rejected', decided_at=NOW(),
		       decided_by=$1, updated_at=NOW() WHERE id=$2 AND status='pending'
	`, userID, approvalID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reject failed"})
		return
	}

	if err := writeEndpointAudit(r, s.requestDB(r), mspID, "endpoint.approval.rejected", "approval:"+approvalID, map[string]interface{}{
		"approval_id": approvalID, "rejecter": userID,
	}); err != nil {
		s.logger.Warn("write approval audit", zap.Error(err))
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

	var requesterID, status string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT requester_user_id, status FROM endpoint_approval_requests
		WHERE id = $1 AND msp_id = $2
	`, approvalID, mspID).Scan(&requesterID, &status)
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

func (s *APIServer) dispatchApprovedAction(mspID, approvalID, actionName string, deviceIDs []string, correlationID, targetHash string) {
	op, ok := operationRegistry[actionName]
	if !ok {
		s.logger.Warn("dispatch approved action: unknown action", zap.String("action", actionName))
		return
	}

	now := time.Now().UTC()
	availableAt := now
	expiresAt := now.Add(24 * time.Hour)
	jobID := uuid.New().String()

	payloadMap := map[string]interface{}{
		"action": actionName, "requester": "approval:" + approvalID,
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		s.logger.Error("dispatch approved action: marshal payload", zap.Error(err))
		return
	}

	db := s.db.DB()
	if db == nil {
		s.logger.Error("dispatch approved action: no db")
		return
	}

	if len(deviceIDs) == 1 {
		deviceID := deviceIDs[0]
		targetID := uuid.New().String()
		_, err = db.Exec(`
			INSERT INTO jobs (id, msp_id, created_by, type, status, priority,
			                  payload, max_retries, max_devices, expires_at, correlation_id,
			                  request_hash, scheduled_for, approval_request_id)
			VALUES ($1, $2, $3, $4, 'queued', 10, $5, 1, 1, $6, $7, $8, $9, $10)
		`, jobID, mspID, "api:approval:"+actionName, op.JobType,
			payload, expiresAt, correlationID, targetHash, availableAt, approvalID)
		if err != nil {
			s.logger.Error("dispatch approved action: create job", zap.Error(err))
			return
		}
		_, err = db.Exec(`
			INSERT INTO job_targets (id, job_id, device_id, msp_id, status)
			VALUES ($1, $2, $3, $4, 'queued')
		`, targetID, jobID, deviceID, mspID)
		if err != nil {
			s.logger.Error("dispatch approved action: create target", zap.Error(err))
			return
		}
		outboxPayload, _ := json.Marshal(map[string]interface{}{
			"job_id": jobID, "target_id": targetID, "device_id": deviceID,
			"type": op.JobType, "payload": payloadMap,
		})
		_, err = db.Exec(`
			INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
			VALUES (gen_random_uuid(), $1, $2, 'job.dispatch', $3, $4)
		`, mspID, jobID, outboxPayload, availableAt)
		if err != nil {
			s.logger.Error("dispatch approved action: create outbox", zap.Error(err))
		}
	} else {
		bulkPayload, _ := json.Marshal(map[string]interface{}{
			"device_ids": deviceIDs, "action": actionName,
		})
		_, err = db.Exec(`
			INSERT INTO jobs (id, msp_id, created_by, type, status, priority,
			                  payload, max_retries, max_devices, expires_at, correlation_id,
			                  request_hash, scheduled_for, approval_request_id)
			VALUES ($1, $2, $3, $4, 'queued', 10, $5, 1, $6, $7, $8, $9, $10, $11)
		`, jobID, mspID, "api:bulk-approval:"+actionName, op.JobType,
			bulkPayload, len(deviceIDs), expiresAt, correlationID, targetHash, availableAt, approvalID)
		if err != nil {
			s.logger.Error("dispatch approved action: create bulk job", zap.Error(err))
			return
		}
		for _, deviceID := range deviceIDs {
			targetID := uuid.New().String()
			_, err = db.Exec(`
				INSERT INTO job_targets (id, job_id, device_id, msp_id, status) VALUES ($1,$2,$3,$4,'queued')
			`, targetID, jobID, deviceID, mspID)
			if err != nil {
				s.logger.Error("dispatch approved action: create bulk target", zap.Error(err))
				continue
			}
			opPayload, _ := json.Marshal(map[string]interface{}{
				"job_id": jobID, "target_id": targetID, "device_id": deviceID,
				"type": op.JobType, "payload": payloadMap,
			})
			_, err = db.Exec(`
				INSERT INTO job_outbox (id,msp_id,aggregate_id,event_type,payload,available_at)
				VALUES (gen_random_uuid(),$1,$2,'job.dispatch',$3,$4)
			`, mspID, jobID, opPayload, availableAt)
			if err != nil {
				s.logger.Error("dispatch approved action: create bulk outbox", zap.Error(err))
			}
		}
	}

	_, _ = db.Exec(`
		UPDATE endpoint_approval_requests SET status='dispatched', updated_at=NOW() WHERE id=$1
	`, approvalID)
}

func expirePendingApprovals(db dbExecutor) {
	_, err := db.Exec(`
		UPDATE endpoint_approval_requests SET status='expired', updated_at=NOW()
		WHERE status='pending' AND expires_at < NOW()
	`)
	if err != nil {
		fmt.Printf("expire pending approvals: %v\n", err)
	}
}

func transitionApprovalStatus(db dbExecutor, currentStatus, nextStatus string) error {
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
