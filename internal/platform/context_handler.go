package platform

import (
	"encoding/json"
	"net/http"
	"time"
)

type contextResponse struct {
	UserID          string          `json:"user_id"`
	Email           string          `json:"email"`
	Roles           []string        `json:"roles"`
	Permissions     []string        `json:"permissions"`
	MSPID           string          `json:"msp_id"`
	MSPName         string          `json:"msp_name"`
	MSPActive       bool            `json:"msp_active"`
	ClientID        string          `json:"client_id"`
	ClientName      string          `json:"client_name"`
	SiteID          string          `json:"site_id"`
	SiteName        string          `json:"site_name"`
	Branding        json.RawMessage `json:"branding,omitempty"`
	SupportGrantID  string          `json:"support_grant_id"`
	SupportGrantExp string          `json:"support_grant_expires_at,omitempty"`
	PlatformRole    bool            `json:"platform_role"`
	AuthenticatedAt string          `json:"authenticated_at"`
}

func (s *APIServer) handleContext(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(ctxKeyUserID).(string)
	if !ok || principal == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session required"})
		return
	}

	roles := getRoles(r)
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	clientID, _ := r.Context().Value(ctxKeyClientID).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(string)

	resp := contextResponse{
		UserID:          principal,
		Roles:           roles,
		MSPID:           mspID,
		ClientID:        clientID,
		SiteID:          siteID,
		PlatformRole:    isPlatformGlobal(roles),
		AuthenticatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if principal != "" {
		var email string
		s.db.DB().QueryRow(`SELECT email FROM users WHERE id = $1`, principal).Scan(&email)
		resp.Email = email
	}

	if mspID != "" {
		var name string
		var active bool
		s.db.DB().QueryRow(`SELECT name, is_active FROM msp_tenants WHERE id = $1`, mspID).Scan(&name, &active)
		resp.MSPName = name
		resp.MSPActive = active
	}

	if clientID != "" {
		s.db.DB().QueryRow(`SELECT name FROM client_organizations WHERE id = $1`, clientID).Scan(&resp.ClientName)
	}

	if siteID != "" {
		s.db.DB().QueryRow(`SELECT name FROM sites WHERE id = $1`, siteID).Scan(&resp.SiteName)
	}

	if grantID, _ := r.Context().Value(ctxKeySupportGrantID).(string); grantID != "" {
		resp.SupportGrantID = grantID
		var expiresAt time.Time
		s.db.DB().QueryRow(`SELECT expires_at FROM support_access_grants WHERE id = $1`, grantID).Scan(&expiresAt)
		resp.SupportGrantExp = expiresAt.UTC().Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, resp)
}
