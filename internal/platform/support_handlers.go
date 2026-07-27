package platform

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type createSupportGrantRequest struct {
	PlatformUserID string   `json:"platform_user_id"`
	MSPID          string   `json:"msp_id"`
	Reason         string   `json:"reason"`
	TicketRef      string   `json:"ticket_ref"`
	Permissions    []string `json:"permissions"`
	DurationMinute int      `json:"duration_minutes"`
}

func (s *APIServer) handleCreateSupportGrant(w http.ResponseWriter, r *http.Request) {
	var req createSupportGrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.PlatformUserID == "" || req.MSPID == "" || req.Reason == "" || req.TicketRef == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platform_user_id, msp_id, reason and ticket_ref are required"})
		return
	}
	if req.DurationMinute <= 0 || req.DurationMinute > 480 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "duration_minutes must be between 1 and 480"})
		return
	}
	if len(req.Permissions) == 0 {
		req.Permissions = []string{"read"}
	}
	for _, permission := range req.Permissions {
		if permission != "read" && permission != "write" && permission != "execute" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported support permission"})
			return
		}
	}

	approverID, _ := r.Context().Value(ctxKeyUserID).(string)
	grantID := uuid.NewString()
	expiresAt := time.Now().UTC().Add(time.Duration(req.DurationMinute) * time.Minute)
	result, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO support_access_grants (
			id, platform_user_id, msp_id, reason, ticket_ref, approved_by,
			approved_at, expires_at, status, permissions
		)
		SELECT $1, $2, m.id, $3, $4, $5, NOW(), $6, 'active', $7
		FROM msp_tenants m
		WHERE m.id = $8
	`, grantID, req.PlatformUserID, req.Reason, req.TicketRef, approverID, expiresAt, pq.Array(req.Permissions), req.MSPID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "support grant could not be created"})
		return
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		writeAuthorizationDenied(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":          grantID,
		"msp_id":      req.MSPID,
		"expires_at":  expiresAt.Format(time.RFC3339),
		"permissions": req.Permissions,
	})
}

func (s *APIServer) handleRevokeSupportGrant(w http.ResponseWriter, r *http.Request) {
	grantID := r.PathValue("grantID")
	revokedBy, _ := r.Context().Value(ctxKeyUserID).(string)
	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE support_access_grants
		SET status = 'revoked', revoked_at = NOW(), revoked_by = $2
		WHERE id = $1 AND status = 'active'
	`, grantID, revokedBy)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "support grant could not be revoked"})
		return
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		writeAuthorizationDenied(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) supportGrantAllows(r *http.Request, mspID string) bool {
	grantID, _ := r.Context().Value(ctxKeySupportGrantID).(string)
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if grantID == "" || userID == "" || mspID == "" {
		return false
	}
	requiredPermission := "read"
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		requiredPermission = "write"
	}
	var allowed bool
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM support_access_grants g
			JOIN msp_tenants m ON m.id = g.msp_id
			WHERE g.id = $1
			  AND g.platform_user_id = $2
			  AND g.msp_id = $3
			  AND g.status = 'active'
			  AND g.revoked_at IS NULL
			  AND g.expires_at > NOW()
			  AND m.is_active = true
			  AND $4 = ANY(g.permissions)
		)
	`, grantID, userID, mspID, requiredPermission).Scan(&allowed)
	return err == nil && allowed
}

func hasSupportRole(roles []string) bool {
	for _, role := range roles {
		if role == "platform_support" {
			return true
		}
	}
	return false
}
