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

func generateToken() (string, string) {
	b := make([]byte, 32)
	rand.Read(b)
	raw := hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(hash[:])
}

func (s *APIServer) handleCreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}

	var req enrollmentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id required"})
		return
	}

	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	if req.MaxUses > 100 {
		req.MaxUses = 100
	}

	rawToken, tokenHash := generateToken()
	id := uuid.New().String()
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err := s.db.DB().ExecContext(r.Context(), `
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

	var id, mspID, clientID, siteID string
	var useCount, maxUses int
	var expiresAt time.Time
	var isRevoked bool

	err := s.db.DB().QueryRowContext(r.Context(), `
		SELECT id, msp_id, client_id, COALESCE(site_id, ''), use_count, max_uses, expires_at, is_revoked
		FROM enrollment_tokens_v2 WHERE token_hash = $1
	`, tokenHash).Scan(&id, &mspID, &clientID, &siteID, &useCount, &maxUses, &expiresAt, &isRevoked)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid token"})
		return
	}

	if isRevoked {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "token revoked"})
		return
	}

	if time.Now().After(expiresAt) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "token expired"})
		return
	}

	if useCount >= maxUses {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "token exhausted"})
		return
	}

	_, err = s.db.DB().ExecContext(r.Context(), `
		UPDATE enrollment_tokens_v2 SET use_count = use_count + 1 WHERE id = $1
	`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "consume failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":     true,
		"msp_id":    mspID,
		"client_id": clientID,
		"site_id":   siteID,
	})
}

func (s *APIServer) handleListEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}

	rows, err := s.db.DB().QueryContext(r.Context(), `
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


