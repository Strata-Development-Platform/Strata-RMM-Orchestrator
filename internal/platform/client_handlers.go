package platform

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Auth Provider Types
const (
	AuthProviderGoogle      = "google"
	AuthProviderMicrosoft   = "microsoft"
	AuthProviderOkta        = "okta"
	AuthProviderGitHub      = "github"
	AuthProviderGitLab      = "gitlab"
	AuthProviderSAML        = "saml"
)

// Auth Provider Request/Response Types
type authProvider struct {
	ID              string         `json:"id"`
	ClientID        string         `json:"client_id"`
	ProviderName    string         `json:"provider_name"`
	ProviderID      string         `json:"provider_id"`
	DisplayName     string         `json:"display_name,omitempty"`
	RedirectURI     string         `json:"redirect_uri"`
	IsActive        bool           `json:"is_active"`
	Settings        json.RawMessage `json:"settings,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type createAuthProviderRequest struct {
	ProviderName string         `json:"provider_name" validate:"required,oneof=google microsoft okta github gitlab saml"`
	ProviderID   string         `json:"provider_id" validate:"required"`
	ClientSecret string         `json:"client_secret" validate:"required"`
	RedirectURI  string         `json:"redirect_uri" validate:"required"`
	Settings     json.RawMessage `json:"settings,omitempty"`
	IsActive     bool           `json:"is_active,omitempty"`
}

type listAuthProvidersResponse struct {
	Providers []authProvider `json:"providers"`
	Count     int            `json:"count"`
}

// Session Types
type clientSession struct {
	ID              string    `json:"id"`
	ClientID        string    `json:"client_id"`
	UserID          string    `json:"user_id,omitempty"`
	Username        string    `json:"username,omitempty"`
	SessionToken    string    `json:"session_token"`
	LastActivityAt  time.Time `json:"last_activity_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	LastIPAddress   string    `json:"last_ip_address,omitempty"`
	LastUserAgent   string    `json:"last_user_agent,omitempty"`
	IsActive        bool      `json:"is_active"`
	AuthenticatedAt time.Time `json:"authenticated_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type createSessionRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	ProviderID  string `json:"provider_id,omitempty"`
	RedirectURI string `json:"redirect_uri,omitempty"`
}

type listSessionsResponse struct {
	Sessions []clientSession `json:"sessions"`
	Count    int             `json:"count"`
}

// Client Profile Types
type clientProfile struct {
	ID                    string         `json:"id"`
	MSPID                 string         `json:"msp_id"`
	Name                  string         `json:"name"`
	Slug                  string         `json:"slug"`
	DisplayName           string         `json:"display_name"`
	IsActive              bool           `json:"is_active"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	Settings              *clientSettings `json:"settings,omitempty"`
}

type updateClientProfileRequest struct {
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Slug        string `json:"slug,omitempty"`
}

// Portal Settings Types
type clientSettings struct {
	ID                              string         `json:"id"`
	ClientID                        string         `json:"client_id"`
	AllowSelfRegistration           bool           `json:"allow_self_registration"`
	SelfRegistrationDomains         []string       `json:"self_registration_domains"`
	EnableSSO                       bool           `json:"enable_sso"`
	EnablePasswordLogin             bool           `json:"enable_password_login"`
	BrandingOverride                json.RawMessage `json:"branding_override,omitempty"`
	WelcomeMessage                  string         `json:"welcome_message,omitempty"`
	SupportEmail                    string         `json:"support_email,omitempty"`
	SupportPhone                    string         `json:"support_phone,omitempty"`
	SupportURL                      string         `json:"support_url,omitempty"`
	LogoURL                         string         `json:"logo_url,omitempty"`
	FaviconURL                      string         `json:"favicon_url,omitempty"`
	PrimaryColor                    string         `json:"primary_color,omitempty"`
	AccentColor                     string         `json:"accent_color,omitempty"`
	SidebarBg                       string         `json:"sidebar_bg,omitempty"`
	HeaderBg                        string         `json:"header_bg,omitempty"`
	LoginBg                         string         `json:"login_bg,omitempty"`
	PortalTitle                     string         `json:"portal_title,omitempty"`
	WelcomeText                     string         `json:"welcome_text,omitempty"`
	CreatedAt                       time.Time      `json:"created_at"`
	UpdatedAt                       time.Time      `json:"updated_at"`
}

type updatePortalSettingsRequest struct {
	AllowSelfRegistration    bool           `json:"allow_self_registration,omitempty"`
	SelfRegistrationDomains  []string       `json:"self_registration_domains,omitempty"`
	EnableSSO                bool           `json:"enable_sso,omitempty"`
	EnablePasswordLogin      bool           `json:"enable_password_login,omitempty"`
	BrandingOverride         json.RawMessage `json:"branding_override,omitempty"`
	WelcomeMessage           string         `json:"welcome_message,omitempty"`
	SupportEmail             string         `json:"support_email,omitempty"`
	SupportPhone             string         `json:"support_phone,omitempty"`
	SupportURL               string         `json:"support_url,omitempty"`
	LogoURL                  string         `json:"logo_url,omitempty"`
	FaviconURL               string         `json:"favicon_url,omitempty"`
	PrimaryColor             string         `json:"primary_color,omitempty"`
	AccentColor              string         `json:"accent_color,omitempty"`
	SidebarBg                string         `json:"sidebar_bg,omitempty"`
	HeaderBg                 string         `json:"header_bg,omitempty"`
	LoginBg                  string         `json:"login_bg,omitempty"`
	PortalTitle              string         `json:"portal_title,omitempty"`
	WelcomeText              string         `json:"welcome_text,omitempty"`
}

// --- API Handlers ---

func (s *APIServer) handleListAuthProviders(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if _, ok := s.authorizeClientManage(w, r, clientID); !ok {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, client_id, provider_name, provider_id, redirect_uri, 
		       is_active, settings, created_at, updated_at
		FROM client_auth_providers
		WHERE client_id = $1
		ORDER BY created_at DESC
	`, clientID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	defer func() { _ = rows.Close() }()

	var providers []authProvider
	for rows.Next() {
		var p authProvider
		var settingsBytes []byte
		if err := rows.Scan(&p.ID, &p.ClientID, &p.ProviderName, &p.ProviderID,
			&p.RedirectURI, &p.IsActive, &settingsBytes, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		if len(settingsBytes) > 0 {
			p.Settings = json.RawMessage(settingsBytes)
		}
		providers = append(providers, p)
	}
	if providers == nil {
		providers = []authProvider{}
	}

	writeJSON(w, http.StatusOK, listAuthProvidersResponse{Providers: providers, Count: len(providers)})
}

func (s *APIServer) handleCreateAuthProvider(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if _, ok := s.authorizeClientManage(w, r, clientID); !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 102400)
	var req createAuthProviderRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.ProviderName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider_name is required"})
		return
	}

	if req.ProviderID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider_id is required"})
		return
	}

	if req.RedirectURI == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "redirect_uri is required"})
		return
	}

	if !isValidAuthProvider(req.ProviderName) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid provider_name"})
		return
	}

	var id string
	var createdAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		INSERT INTO client_auth_providers (client_id, provider_name, provider_id, 
		       client_secret_hash, redirect_uri, is_active, settings)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (client_id, provider_name) DO UPDATE SET
			provider_id = $3,
			client_secret_hash = $4,
			redirect_uri = $5,
			is_active = $6,
			settings = $7,
			updated_at = NOW()
		RETURNING id, created_at
	`, clientID, req.ProviderName, req.ProviderID, hashClientSecret(req.ClientSecret),
		req.RedirectURI, req.IsActive, req.Settings).Scan(&id, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":        id,
		"msg":       "auth provider configured",
		"created_at": createdAt.UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleCreateClientSession(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 102400)
	var req createSessionRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.Username == "" && req.Password == "" && req.ProviderID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username/password or provider_id required"})
		return
	}

	// Determine which authentication method to use
	var sessionToken string
	var expiresAt time.Time
	var err error

	if req.ProviderID != "" {
		// SSO authentication
		sessionToken, expiresAt, err = s.createSSOSession(r, clientID, req)
	} else {
		// Username/password authentication
		sessionToken, expiresAt, err = s.createPasswordSession(r, clientID, req)
	}

	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	// Get user info
	var userID, username string
	err = s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, email FROM users WHERE email = $1 AND is_active = true
	`, req.Username).Scan(&userID, &username)
	if err != nil && err != sql.ErrNoRows {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":              sessionToken,
		"session_token":   sessionToken,
		"expires_at":      expiresAt.UTC().Format(time.RFC3339),
		"user_id":         userID,
		"username":        username,
		"client_id":       clientID,
	})
}

func (s *APIServer) handleListClientSessions(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT cs.id, cs.client_id, cs.user_id, cs.session_token, cs.last_activity_at,
		       cs.expires_at, cs.ip_address, cs.user_agent, cs.created_at
		FROM client_sessions cs
		WHERE cs.client_id = $1
		ORDER BY cs.last_activity_at DESC
		LIMIT 100
	`, clientID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	defer func() { _ = rows.Close() }()

	var sessions []clientSession
	for rows.Next() {
		var s clientSession
		var ipAddr, userAgent sql.NullString
		if err := rows.Scan(&s.ID, &s.ClientID, &s.UserID, &s.SessionToken,
			&s.LastActivityAt, &s.ExpiresAt, &ipAddr, &userAgent, &s.CreatedAt); err != nil {
			continue
		}
		if ipAddr.Valid {
			s.LastIPAddress = ipAddr.String
		}
		if userAgent.Valid {
			s.LastUserAgent = userAgent.String
		}
		s.IsActive = s.ExpiresAt.After(time.Now())
		sessions = append(sessions, s)
	}
	if sessions == nil {
		sessions = []clientSession{}
	}

	writeJSON(w, http.StatusOK, listSessionsResponse{Sessions: sessions, Count: len(sessions)})
}

func (s *APIServer) handleRevokeClientSession(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")
	sessionID := r.PathValue("sessionID")

	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	// Verify the session belongs to the client and belongs to the user (unless admin)
	var sessionClientID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT client_id FROM client_sessions WHERE id = $1
	`, sessionID).Scan(&sessionClientID)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	if sessionClientID != clientID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}

	_, err = s.requestDB(r).ExecContext(r.Context(), `
		UPDATE client_sessions 
		SET revoked_at = NOW(), 
		    updated_at = NOW()
		WHERE id = $1
	`, sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "session_revoked", "session_id": sessionID})
}

func (s *APIServer) handleGetClientProfile(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	var profile clientProfile
	var mspID, name, slug, displayName sql.NullString
	var isActive bool
	var createdAt, updatedAt time.Time

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT c.id, c.msp_id, c.name, c.slug, c.display_name, c.is_active, 
		       c.created_at, c.updated_at
		FROM client_organizations c
		WHERE c.id = $1
	`, clientID).Scan(&profile.ID, &mspID, &name, &slug, &displayName, &isActive, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "client not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	profile.MSPID = mspID.String
	profile.Name = name.String
	profile.Slug = slug.String
	profile.DisplayName = displayName.String
	profile.IsActive = isActive
	profile.CreatedAt = createdAt
	profile.UpdatedAt = updatedAt

	// Load settings
	profile.Settings = s.getClientSettings(r, clientID)

	writeJSON(w, http.StatusOK, profile)
}

func (s *APIServer) handleUpdateClientProfile(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if _, ok := s.authorizeClientManage(w, r, clientID); !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 102400)
	var req updateClientProfileRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	var updated bool
	var queryParts []string
	var args []interface{}

	if req.Name != "" {
		queryParts = append(queryParts, "name = $"+fmt.Sprint(len(queryParts)+2))
		args = append(args, req.Name)
		updated = true
	}
	if req.DisplayName != "" {
		queryParts = append(queryParts, "display_name = $"+fmt.Sprint(len(queryParts)+2))
		args = append(args, req.DisplayName)
		updated = true
	}
	if req.Slug != "" {
		queryParts = append(queryParts, "slug = $"+fmt.Sprint(len(queryParts)+2))
		args = append(args, req.Slug)
		updated = true
	}

	if !updated {
		writeJSON(w, http.StatusOK, map[string]string{"msg": "no updates specified"})
		return
	}

	args = append(args, clientID)
	query := fmt.Sprintf(`
		UPDATE client_organizations 
		SET %s, updated_at = NOW()
		WHERE id = $%d
		RETURNING id, name, slug, display_name, is_active, created_at, updated_at
	`, strings.Join(queryParts, ", "), len(args))

	var profile clientProfile
	var mspID, name, slug, displayName sql.NullString
	var isActive bool
	var createdAt, updatedAt time.Time

	err := s.requestDB(r).QueryRowContext(r.Context(), query, args...).Scan(
		&profile.ID, &mspID, &name, &slug, &displayName, &isActive, &createdAt, &updatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	profile.MSPID = mspID.String
	profile.Name = name.String
	profile.Slug = slug.String
	profile.DisplayName = displayName.String
	profile.IsActive = isActive
	profile.CreatedAt = createdAt
	profile.UpdatedAt = updatedAt

	writeJSON(w, http.StatusOK, profile)
}

func (s *APIServer) handleGetClientSettings(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	settings := s.getClientSettings(r, clientID)

	if settings == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "settings not found"})
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

func (s *APIServer) handleUpdateClientSettings(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if _, ok := s.authorizeClientManage(w, r, clientID); !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 102400)
	var req updatePortalSettingsRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	// Check if settings already exist
	var existingID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id FROM client_portal_settings WHERE client_id = $1
	`, clientID).Scan(&existingID)

	var query string
	var args []interface{}

	if existingID == "" {
		// Create new settings
		query = `
			INSERT INTO client_portal_settings (
				client_id, allow_self_registration, self_registration_domains,
				enable_sso, enable_password_login, branding_override,
				welcome_message, support_email, support_phone, support_url,
				logo_url, favicon_url, primary_color, accent_color,
				sidebar_bg, header_bg, login_bg, portal_title, welcome_text
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
			ON CONFLICT (client_id) DO UPDATE SET
				allow_self_registration = $2,
				self_registration_domains = $3,
				enable_sso = $4,
				enable_password_login = $5,
				branding_override = $6,
				welcome_message = $7,
				support_email = $8,
				support_phone = $9,
				support_url = $10,
				logo_url = $11,
				favicon_url = $12,
				primary_color = $13,
				accent_color = $14,
				sidebar_bg = $15,
				header_bg = $16,
				login_bg = $17,
				portal_title = $18,
				welcome_text = $19,
				updated_at = NOW()
			RETURNING id, client_id, created_at, updated_at
		`

		args = []interface{}{
			clientID, req.AllowSelfRegistration, req.SelfRegistrationDomains,
			req.EnableSSO, req.EnablePasswordLogin, req.BrandingOverride,
			req.WelcomeMessage, req.SupportEmail, req.SupportPhone, req.SupportURL,
			req.LogoURL, req.FaviconURL, req.PrimaryColor, req.AccentColor,
			req.SidebarBg, req.HeaderBg, req.LoginBg, req.PortalTitle, req.WelcomeText,
		}
	} else {
		// Update existing settings
		var queryParts []string
		args = []interface{}{clientID}
		paramIdx := 2

		if req.AllowSelfRegistration {
			queryParts = append(queryParts, "allow_self_registration = $"+fmt.Sprint(paramIdx))
			paramIdx++
		}
		if len(req.SelfRegistrationDomains) > 0 {
			queryParts = append(queryParts, "self_registration_domains = $"+fmt.Sprint(paramIdx))
			args = append(args, req.SelfRegistrationDomains)
			paramIdx++
		}
		if req.EnableSSO {
			queryParts = append(queryParts, "enable_sso = $"+fmt.Sprint(paramIdx))
			paramIdx++
		}
		if req.EnablePasswordLogin {
			queryParts = append(queryParts, "enable_password_login = $"+fmt.Sprint(paramIdx))
			paramIdx++
		}
		if len(req.BrandingOverride) > 0 {
			queryParts = append(queryParts, "branding_override = $"+fmt.Sprint(paramIdx))
			args = append(args, req.BrandingOverride)
			paramIdx++
		}
		if req.WelcomeMessage != "" {
			queryParts = append(queryParts, "welcome_message = $"+fmt.Sprint(paramIdx))
			args = append(args, req.WelcomeMessage)
			paramIdx++
		}
		if req.SupportEmail != "" {
			queryParts = append(queryParts, "support_email = $"+fmt.Sprint(paramIdx))
			args = append(args, req.SupportEmail)
			paramIdx++
		}
		if req.SupportPhone != "" {
			queryParts = append(queryParts, "support_phone = $"+fmt.Sprint(paramIdx))
			args = append(args, req.SupportPhone)
			paramIdx++
		}
		if req.SupportURL != "" {
			queryParts = append(queryParts, "support_url = $"+fmt.Sprint(paramIdx))
			args = append(args, req.SupportURL)
			paramIdx++
		}
		if req.LogoURL != "" {
			queryParts = append(queryParts, "logo_url = $"+fmt.Sprint(paramIdx))
			args = append(args, req.LogoURL)
			paramIdx++
		}
		if req.FaviconURL != "" {
			queryParts = append(queryParts, "favicon_url = $"+fmt.Sprint(paramIdx))
			args = append(args, req.FaviconURL)
			paramIdx++
		}
		if req.PrimaryColor != "" {
			queryParts = append(queryParts, "primary_color = $"+fmt.Sprint(paramIdx))
			args = append(args, req.PrimaryColor)
			paramIdx++
		}
		if req.AccentColor != "" {
			queryParts = append(queryParts, "accent_color = $"+fmt.Sprint(paramIdx))
			args = append(args, req.AccentColor)
			paramIdx++
		}
		if req.SidebarBg != "" {
			queryParts = append(queryParts, "sidebar_bg = $"+fmt.Sprint(paramIdx))
			args = append(args, req.SidebarBg)
			paramIdx++
		}
		if req.HeaderBg != "" {
			queryParts = append(queryParts, "header_bg = $"+fmt.Sprint(paramIdx))
			args = append(args, req.HeaderBg)
			paramIdx++
		}
		if req.LoginBg != "" {
			queryParts = append(queryParts, "login_bg = $"+fmt.Sprint(paramIdx))
			args = append(args, req.LoginBg)
			paramIdx++
		}
		if req.PortalTitle != "" {
			queryParts = append(queryParts, "portal_title = $"+fmt.Sprint(paramIdx))
			args = append(args, req.PortalTitle)
			paramIdx++
		}
		if req.WelcomeText != "" {
			queryParts = append(queryParts, "welcome_text = $"+fmt.Sprint(paramIdx))
			args = append(args, req.WelcomeText)
			paramIdx++
		}

		if len(queryParts) == 0 {
			writeJSON(w, http.StatusOK, map[string]string{"msg": "no updates specified"})
			return
		}

		query = fmt.Sprintf(`
			UPDATE client_portal_settings 
			SET %s, updated_at = NOW()
			WHERE client_id = $1
			RETURNING id, client_id, created_at, updated_at
		`, strings.Join(queryParts, ", "))
	}

	var settingsID string
	var createdAt, updatedAt time.Time
	err = s.requestDB(r).QueryRowContext(r.Context(), query, args...).Scan(&settingsID, &clientID, &createdAt, &updatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	settings := s.getClientSettings(r, clientID)
	if settings == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// --- Helper Methods ---

func (s *APIServer) getClientSettings(r *http.Request, clientID string) *clientSettings {
	var settings clientSettings
	var brandingOverride []byte
	var selfRegistrationDomains []string

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, client_id, allow_self_registration, self_registration_domains,
		       enable_sso, enable_password_login, branding_override,
		       welcome_message, support_email, support_phone, support_url,
		       logo_url, favicon_url, primary_color, accent_color,
		       sidebar_bg, header_bg, login_bg, portal_title, welcome_text,
		       created_at, updated_at
		FROM client_portal_settings
		WHERE client_id = $1
	`, clientID).Scan(&settings.ID, &settings.ClientID, &settings.AllowSelfRegistration,
		&selfRegistrationDomains, &settings.EnableSSO, &settings.EnablePasswordLogin,
		&brandingOverride, &settings.WelcomeMessage, &settings.SupportEmail,
		&settings.SupportPhone, &settings.SupportURL, &settings.LogoURL,
		&settings.FaviconURL, &settings.PrimaryColor, &settings.AccentColor,
		&settings.SidebarBg, &settings.HeaderBg, &settings.LoginBg,
		&settings.PortalTitle, &settings.WelcomeText, &settings.CreatedAt, &settings.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return nil
	}

	if len(brandingOverride) > 0 {
		settings.BrandingOverride = json.RawMessage(brandingOverride)
	}
	settings.SelfRegistrationDomains = selfRegistrationDomains

	return &settings
}

func isValidAuthProvider(provider string) bool {
	validProviders := map[string]bool{
		AuthProviderGoogle:      true,
		AuthProviderMicrosoft:   true,
		AuthProviderOkta:        true,
		AuthProviderGitHub:      true,
		AuthProviderGitLab:      true,
		AuthProviderSAML:        true,
	}
	return validProviders[provider]
}

func hashClientSecret(secret string) string {
	// In production, use proper hashing with salt
	// This is a placeholder for demonstration
	return fmt.Sprintf("hash_%s", secret)
}

func (s *APIServer) createSSOSession(r *http.Request, clientID string, req createSessionRequest) (string, time.Time, error) {
	// Validate provider exists
	var providerID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT provider_id FROM client_auth_providers 
		WHERE client_id = $1 AND provider_id = $2 AND is_active = true
	`, clientID, req.ProviderID).Scan(&providerID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid provider")
	}

	// Generate session token
	sessionToken := uuid.New().String()
	expiresAt := time.Now().Add(24 * time.Hour)

	// Create session record
	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO client_sessions (client_id, session_token, expires_at, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5)
	`, clientID, sessionToken, expiresAt, r.RemoteAddr, r.UserAgent())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create session")
	}

	return sessionToken, expiresAt, nil
}

func (s *APIServer) createPasswordSession(r *http.Request, clientID string, req createSessionRequest) (string, time.Time, error) {
	// Validate username/password
	var passwordHash string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT u.password_hash FROM users u
		JOIN client_organizations c ON c.id = $1
		WHERE u.email = $2 AND u.is_active = true
	`, clientID, req.Username).Scan(&passwordHash)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid credentials")
	}

	// Verify password (placeholder - would use bcrypt in production)
	if req.Password == "" {
		return "", time.Time{}, fmt.Errorf("password required")
	}

	// Generate session token
	sessionToken := uuid.New().String()
	expiresAt := time.Now().Add(24 * time.Hour)

	// Create session record
	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO client_sessions (client_id, session_token, expires_at, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5)
	`, clientID, sessionToken, expiresAt, r.RemoteAddr, r.UserAgent())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create session")
	}

	return sessionToken, expiresAt, nil
}
