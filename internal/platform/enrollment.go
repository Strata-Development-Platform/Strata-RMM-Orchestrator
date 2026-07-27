package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type enrollmentTokenRequest struct {
	ClientID    string `json:"client_id"`
	SiteID      string `json:"site_id,omitempty"`
	MaxUses     int    `json:"max_uses"`
	Description string `json:"description,omitempty"`
}

type enrollmentTokenResponse struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	MaxUses   int    `json:"max_uses"`
}

func generateToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw := hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(hash[:]), nil
}

func (s *APIServer) handleCreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var req enrollmentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id required"})
		return
	}
	if !s.AuthorizeClientAccess(w, r, req.ClientID) {
		return
	}

	var mspID string
	if err := s.requestDB(r).QueryRowContext(
		r.Context(),
		`SELECT msp_id FROM client_organizations WHERE id = $1`,
		req.ClientID,
	).Scan(&mspID); err != nil {
		writeAuthorizationDenied(w)
		return
	}
	if req.SiteID != "" && !s.AuthorizeSiteAccess(w, r, req.SiteID) {
		return
	}

	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	if req.MaxUses > 100 {
		req.MaxUses = 100
	}

	rawToken, tokenHash, err := generateToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token generation failed"})
		return
	}
	id := uuid.New().String()
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO enrollment_tokens_v2 (id, msp_id, client_id, site_id, token_hash, max_uses, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, mspID, req.ClientID, nullIfEmpty(req.SiteID), tokenHash, req.MaxUses, expiresAt, req.Description)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, enrollmentTokenResponse{
		ID:        id,
		Token:     rawToken,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
		MaxUses:   req.MaxUses,
	})
}

func (s *APIServer) handleValidateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}

	hash := sha256.Sum256([]byte(req.Token))
	tokenHash := hex.EncodeToString(hash[:])

	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	tx, err := s.db.DB().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database transaction unavailable"})
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(
		r.Context(),
		`SELECT set_config('app.enrollment_hash', $1, true)`,
		tokenHash,
	); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database security context unavailable"})
		return
	}

	var id, mspID, clientID, siteID string
	err = tx.QueryRowContext(r.Context(), `
		SELECT et.id, et.msp_id, et.client_id, COALESCE(et.site_id::text, '')
		FROM enrollment_tokens_v2 et
		JOIN msp_tenants m ON m.id = et.msp_id
		JOIN client_organizations c ON c.id = et.client_id AND c.msp_id = et.msp_id
		WHERE et.token_hash = $1
		  AND et.is_revoked = false
		  AND et.expires_at > NOW()
		  AND et.use_count < et.max_uses
		  AND m.is_active = true
		  AND c.is_active = true
	`, tokenHash).Scan(&id, &mspID, &clientID, &siteID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid token"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token validation failed"})
		return
	}
	committed = true

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":     true,
		"msp_id":    mspID,
		"client_id": clientID,
		"site_id":   siteID,
	})
}

func (s *APIServer) handleListEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, client_id, COALESCE(site_id::text, ''), max_uses, use_count,
		       expires_at, is_revoked, created_at
		FROM enrollment_tokens_v2 WHERE msp_id = $1
		ORDER BY created_at DESC LIMIT 100
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var tokens []map[string]interface{}
	for rows.Next() {
		var id, clientID, siteID string
		var maxUses, useCount int
		var expiresAt, createdAt time.Time
		var isRevoked bool
		if err := rows.Scan(&id, &clientID, &siteID, &maxUses, &useCount, &expiresAt, &isRevoked, &createdAt); err != nil {
			continue
		}
		tokens = append(tokens, map[string]interface{}{
			"id": id, "client_id": clientID, "site_id": siteID,
			"max_uses": maxUses, "use_count": useCount,
			"expires_at": expiresAt.UTC().Format(time.RFC3339),
			"is_revoked": isRevoked, "created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}
	if tokens == nil {
		tokens = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tokens": tokens})
}
