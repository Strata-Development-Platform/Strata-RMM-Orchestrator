package platform

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultOffboardingRetentionDays = 90
	minOffboardingRetentionDays     = 30
	maxOffboardingRetentionDays     = 3650
)

type offboardingRequest struct {
	Reason        string `json:"reason"`
	RetentionDays int    `json:"retention_days,omitempty"`
}

func normalizeOffboardingRequest(req offboardingRequest) (offboardingRequest, string) {
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		return req, "reason is required"
	}
	if len(req.Reason) > 500 {
		return req, "reason must be 500 characters or fewer"
	}
	if req.RetentionDays == 0 {
		req.RetentionDays = defaultOffboardingRetentionDays
	}
	if req.RetentionDays < minOffboardingRetentionDays || req.RetentionDays > maxOffboardingRetentionDays {
		return req, "retention_days must be between 30 and 3650"
	}
	return req, ""
}

// handleOffboardMSP atomically disables new access while retaining tenant data.
// Physical deletion is intentionally a separate, retention-gated operation.
func (s *APIServer) handleOffboardMSP(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	var req offboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	var validationError string
	req, validationError = normalizeOffboardingRequest(req)
	if validationError != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": validationError})
		return
	}

	db := s.requestDB(r)
	var exists bool
	if err := db.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM msp_tenants WHERE id = $1)`, mspID).Scan(&exists); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to validate MSP"})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "msp not found"})
		return
	}

	actor, _ := r.Context().Value(ctxKeyUserID).(string)
	offboardingID := uuid.New().String()
	upsertResult, err := db.ExecContext(r.Context(), `
		INSERT INTO msp_offboarding (
			id, msp_id, state, reason, requested_by, retention_until
		)
		VALUES ($1, $2, 'requested', $3, $4, NOW() + make_interval(days => $5))
		ON CONFLICT (msp_id) DO UPDATE
		SET reason = EXCLUDED.reason,
		    retention_until = GREATEST(msp_offboarding.retention_until, EXCLUDED.retention_until),
		    updated_at = NOW()
		WHERE msp_offboarding.state <> 'deletion_approved'
	`, offboardingID, mspID, req.Reason, actor, req.RetentionDays)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "offboarding cannot be changed after deletion approval"})
		return
	}
	affected, err := upsertResult.RowsAffected()
	if err != nil || affected != 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "offboarding cannot be changed after deletion approval"})
		return
	}

	statements := []struct {
		query string
		args  []interface{}
	}{
		{`UPDATE msp_tenants SET is_active = false, updated_at = NOW() WHERE id = $1`, []interface{}{mspID}},
		{`UPDATE plan_entitlements SET status = 'cancelled', grace_period_ends_at = NULL, updated_at = NOW() WHERE msp_id = $1`, []interface{}{mspID}},
		{`UPDATE memberships
		  SET status = 'revoked'
		  WHERE status = 'active' AND (
		    (scope_type = 'msp' AND scope_id = $1::text)
		    OR (scope_type = 'client' AND scope_id IN (
		      SELECT id::text FROM client_organizations WHERE msp_id = $1::uuid
		    ))
		    OR (scope_type = 'site' AND scope_id IN (
		      SELECT s.id::text
		      FROM sites s JOIN client_organizations c ON c.id = s.client_id
		      WHERE c.msp_id = $1::uuid
		    ))
		  )`, []interface{}{mspID}},
		{`UPDATE support_access_grants
		  SET status = 'revoked', revoked_at = COALESCE(revoked_at, NOW())
		  WHERE msp_id = $1 AND status = 'active'`, []interface{}{mspID}},
		{`UPDATE enrollment_tokens_v2 SET is_revoked = true WHERE msp_id = $1 AND is_revoked = false`, []interface{}{mspID}},
		{`UPDATE agent_registrations
		  SET approved = false
		  WHERE approved = true AND device_id IN (SELECT id FROM devices WHERE msp_id = $1)`, []interface{}{mspID}},
		{`UPDATE devices SET status = 'disabled', updated_at = NOW() WHERE msp_id = $1 AND status <> 'disabled'`, []interface{}{mspID}},
		{`UPDATE custom_domains SET verification_status = 'suspended' WHERE msp_id = $1 AND verification_status <> 'suspended'`, []interface{}{mspID}},
		{`UPDATE msp_offboarding SET state = 'access_revoked', access_revoked_at = COALESCE(access_revoked_at, NOW()), updated_at = NOW() WHERE msp_id = $1 AND state <> 'deletion_approved'`, []interface{}{mspID}},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(r.Context(), statement.query, statement.args...); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "offboarding access revocation failed"})
			return
		}
	}

	s.auditControlPlane(r, mspID, "msp.offboarded", "msp_offboarding", mspID,
		map[string]interface{}{"retention_days": req.RetentionDays, "reason": req.Reason})
	s.writeOffboardingStatus(w, r, mspID, http.StatusOK)
}

func (s *APIServer) handleGetOffboarding(w http.ResponseWriter, r *http.Request) {
	s.writeOffboardingStatus(w, r, r.PathValue("mspID"), http.StatusOK)
}

func (s *APIServer) writeOffboardingStatus(w http.ResponseWriter, r *http.Request, mspID string, status int) {
	var response struct {
		MSPID              string     `json:"msp_id"`
		State              string     `json:"state"`
		Reason             string     `json:"reason"`
		RequestedBy        string     `json:"requested_by"`
		RequestedAt        time.Time  `json:"requested_at"`
		AccessRevokedAt    *time.Time `json:"access_revoked_at,omitempty"`
		RetentionUntil     time.Time  `json:"retention_until"`
		DeletionApprovedBy *string    `json:"deletion_approved_by,omitempty"`
		DeletionApprovedAt *time.Time `json:"deletion_approved_at,omitempty"`
	}
	response.MSPID = mspID
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT state, reason, requested_by, requested_at, access_revoked_at,
		       retention_until, deletion_approved_by, deletion_approved_at
		FROM msp_offboarding WHERE msp_id = $1
	`, mspID).Scan(
		&response.State, &response.Reason, &response.RequestedBy, &response.RequestedAt,
		&response.AccessRevokedAt, &response.RetentionUntil,
		&response.DeletionApprovedBy, &response.DeletionApprovedAt,
	)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "offboarding record not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "offboarding status unavailable"})
		return
	}
	writeJSON(w, status, response)
}

func (s *APIServer) handleApproveMSPDeletion(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	var req struct {
		ConfirmSlug string `json:"confirm_slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	req.ConfirmSlug = strings.TrimSpace(req.ConfirmSlug)
	actor, _ := r.Context().Value(ctxKeyUserID).(string)
	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE msp_offboarding o
		SET state = 'deletion_approved',
		    deletion_approved_by = $3,
		    deletion_approved_at = NOW(),
		    updated_at = NOW()
		FROM msp_tenants m
		WHERE o.msp_id = $1
		  AND m.id = o.msp_id
		  AND m.slug = $2
		  AND o.state IN ('access_revoked', 'retained')
		  AND o.retention_until <= NOW()
	`, mspID, req.ConfirmSlug, actor)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "deletion approval failed"})
		return
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "deletion requires the exact MSP slug, revoked access, and an expired retention period",
		})
		return
	}
	s.auditControlPlane(r, mspID, "msp.deletion_approved", "msp_offboarding", mspID, nil)
	s.writeOffboardingStatus(w, r, mspID, http.StatusOK)
}
