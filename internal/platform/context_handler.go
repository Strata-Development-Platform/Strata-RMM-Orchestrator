package platform

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

type contextScope struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
	Role     string `json:"role"`
}

type contextEntitlement struct {
	PlanSlug   string          `json:"plan_slug"`
	Status     string          `json:"status"`
	MaxDevices int             `json:"max_devices"`
	MaxUsers   int             `json:"max_users"`
	Features   json.RawMessage `json:"features"`
}

type contextResponse struct {
	UserID              string              `json:"user_id"`
	Email               string              `json:"email"`
	TenantID            string              `json:"tenant_id"`
	Roles               []string            `json:"roles"`
	Permissions         []string            `json:"permissions"`
	AvailableScopes     []contextScope      `json:"available_scopes"`
	MSPID               string              `json:"msp_id"`
	MSPName             string              `json:"msp_name"`
	MSPActive           bool                `json:"msp_active"`
	ClientID            string              `json:"client_id"`
	ClientName          string              `json:"client_name"`
	SiteID              string              `json:"site_id"`
	SiteName            string              `json:"site_name"`
	Branding            json.RawMessage     `json:"branding,omitempty"`
	Entitlement         *contextEntitlement `json:"entitlement,omitempty"`
	SupportGrantID      string              `json:"support_grant_id,omitempty"`
	SupportGrantExp     string              `json:"support_grant_expires_at,omitempty"`
	PlatformRole        bool                `json:"platform_role"`
	PlatformID          string              `json:"platform_id"`
	ProviderDisplayName string              `json:"provider_display_name"`
	SetupComplete       bool                `json:"setup_complete"`
	AuthenticatedAt     string              `json:"authenticated_at"`
}

func (s *APIServer) handleContext(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ctxKeyUserID).(string)
	if !ok || userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session required"})
		return
	}
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "identity database unavailable"})
		return
	}

	roles := getRoles(r)
	tenantID, _ := r.Context().Value(ctxKeyTenantID).(string)
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	clientID, _ := r.Context().Value(ctxKeyClientID).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(string)
	db := s.requestDB(r)
	resp := contextResponse{
		UserID:          userID,
		TenantID:        tenantID,
		Roles:           roles,
		Permissions:     permissionsForRoles(roles),
		AvailableScopes: []contextScope{},
		MSPID:           mspID,
		ClientID:        clientID,
		SiteID:          siteID,
		PlatformRole:    isPlatformGlobal(roles),
		AuthenticatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	profile, err := postgres.GetProviderBusinessProfile(r.Context(), db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "provider setup status unavailable"})
		return
	}
	resp.PlatformID = profile.ID
	resp.ProviderDisplayName = profile.DisplayName
	resp.SetupComplete = profile.SetupComplete

	if err := db.QueryRowContext(r.Context(),
		`SELECT email FROM users WHERE id = $1 AND is_active = true`,
		userID,
	).Scan(&resp.Email); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "active user not found"})
		return
	}

	scopes, err := loadAvailableScopes(r, db, userID, roles)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "workspace scopes unavailable"})
		return
	}
	resp.AvailableScopes = scopes

	if mspID != "" {
		if err := db.QueryRowContext(r.Context(),
			`SELECT name, is_active FROM msp_tenants WHERE id = $1`,
			mspID,
		).Scan(&resp.MSPName, &resp.MSPActive); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "MSP scope unavailable"})
			return
		}

		var branding []byte
		err := db.QueryRowContext(r.Context(), `
			SELECT jsonb_build_object(
				'display_name', display_name,
				'logo_light', COALESCE(logo_light, ''),
				'logo_dark', COALESCE(logo_dark, ''),
				'favicon', COALESCE(favicon, ''),
				'primary_color', primary_color,
				'accent_color', accent_color,
				'sidebar_bg', sidebar_bg,
				'header_bg', header_bg,
				'login_bg', login_bg,
				'portal_title', portal_title,
				'welcome_text', welcome_text
			)
			FROM branding_profiles WHERE msp_id = $1
		`, mspID).Scan(&branding)
		if err != nil && err != sql.ErrNoRows {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "branding unavailable"})
			return
		}
		if len(branding) > 0 {
			resp.Branding = append(json.RawMessage(nil), branding...)
		}

		var entitlement contextEntitlement
		var features []byte
		err = db.QueryRowContext(r.Context(), `
			SELECT p.slug, pe.status, p.max_devices, p.max_users, p.features
			FROM plan_entitlements pe
			JOIN plans p ON p.id = pe.plan_id
			WHERE pe.msp_id = $1
		`, mspID).Scan(
			&entitlement.PlanSlug,
			&entitlement.Status,
			&entitlement.MaxDevices,
			&entitlement.MaxUsers,
			&features,
		)
		if err != nil && err != sql.ErrNoRows {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "entitlements unavailable"})
			return
		}
		if err == nil {
			entitlement.Features = append(json.RawMessage(nil), features...)
			resp.Entitlement = &entitlement
		}
	}

	if clientID != "" {
		if err := db.QueryRowContext(r.Context(),
			`SELECT name FROM client_organizations WHERE id = $1 AND is_active = true`,
			clientID,
		).Scan(&resp.ClientName); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "client scope unavailable"})
			return
		}
	}
	if siteID != "" {
		if err := db.QueryRowContext(r.Context(),
			`SELECT name FROM sites WHERE id = $1 AND is_active = true`,
			siteID,
		).Scan(&resp.SiteName); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "site scope unavailable"})
			return
		}
	}

	if grantID, _ := r.Context().Value(ctxKeySupportGrantID).(string); grantID != "" {
		if mspID == "" || !hasSupportRole(roles) || !s.supportGrantAllows(r, mspID) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "support grant is invalid or expired"})
			return
		}
		var expiresAt time.Time
		if err := db.QueryRowContext(r.Context(), `
			SELECT expires_at
			FROM support_access_grants
			WHERE id = $1 AND msp_id = $2 AND status = 'active' AND expires_at > NOW()
		`, grantID, mspID).Scan(&expiresAt); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "support grant is invalid or expired"})
			return
		}
		resp.SupportGrantID = grantID
		resp.SupportGrantExp = expiresAt.UTC().Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, resp)
}

func loadAvailableScopes(r *http.Request, db dbExecutor, userID string, roles []string) ([]contextScope, error) {
	if isPlatformGlobal(roles) {
		platformRole := "platform_admin"
		for _, role := range roles {
			if role == "platform_owner" {
				platformRole = role
				break
			}
		}
		rows, err := db.QueryContext(r.Context(), `
			SELECT scope_type, scope_id, scope_name, parent_id, role
			FROM (
				SELECT 'platform' AS scope_type, id::text AS scope_id,
				       COALESCE(NULLIF(display_name, ''), name) AS scope_name,
				       '' AS parent_id, $1 AS role, 0 AS sort_order
				FROM platforms WHERE id = $2
				UNION ALL
				SELECT 'msp', id::text, name, '', $1, 1
				FROM msp_tenants WHERE is_active = true
			) scopes
			ORDER BY sort_order, scope_name
		`, platformRole, postgres.SingletonPlatformID)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		return scanContextScopes(rows)
	}

	rows, err := db.QueryContext(r.Context(), `
		SELECT mb.scope_type, mb.scope_id,
		       CASE mb.scope_type
		         WHEN 'msp' THEN COALESCE(m.name, '')
		         WHEN 'client' THEN COALESCE(c.name, '')
		         WHEN 'site' THEN COALESCE(s.name, '')
		         ELSE 'Platform'
		       END,
		       CASE mb.scope_type
		         WHEN 'client' THEN COALESCE(c.msp_id::text, '')
		         WHEN 'site' THEN COALESCE(s.client_id::text, '')
		         ELSE ''
		       END,
		       mb.role
		FROM memberships mb
		LEFT JOIN msp_tenants m ON mb.scope_type = 'msp' AND m.id::text = mb.scope_id AND m.is_active = true
		LEFT JOIN client_organizations c ON mb.scope_type = 'client' AND c.id::text = mb.scope_id AND c.is_active = true
		LEFT JOIN sites s ON mb.scope_type = 'site' AND s.id::text = mb.scope_id AND s.is_active = true
		WHERE mb.user_id = $1
		  AND mb.status = 'active'
		  AND (mb.expires_at IS NULL OR mb.expires_at > NOW())
		ORDER BY mb.scope_type, 3
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanContextScopes(rows)
}

func scanContextScopes(rows *sql.Rows) ([]contextScope, error) {
	scopes := make([]contextScope, 0)
	for rows.Next() {
		var scope contextScope
		if err := rows.Scan(&scope.Type, &scope.ID, &scope.Name, &scope.ParentID, &scope.Role); err != nil {
			return nil, err
		}
		if scope.Name != "" {
			scopes = append(scopes, scope)
		}
	}
	return scopes, rows.Err()
}

func permissionsForRoles(roles []string) []string {
	set := make(map[string]struct{})
	for _, role := range roles {
		for _, permission := range rolePermissions(role) {
			set[permission] = struct{}{}
		}
	}
	permissions := make([]string, 0, len(set))
	for permission := range set {
		permissions = append(permissions, permission)
	}
	sort.Strings(permissions)
	return permissions
}

func rolePermissions(role string) []string {
	switch role {
	case "platform_owner", "platform_admin":
		return []string{"platform:manage", "msp:manage", "support:manage", "security:read"}
	case "platform_support":
		return []string{"support:access"}
	case "platform_billing":
		return []string{"billing:manage", "usage:read"}
	case "platform_security_auditor", "platform_viewer":
		return []string{"platform:read", "security:read"}
	case "msp_owner", "msp_admin":
		return []string{"msp:manage", "client:manage", "site:manage", "device:manage", "job:manage"}
	case "msp_technician":
		return []string{"client:read", "site:read", "device:manage", "job:manage"}
	case "msp_viewer", "client_viewer":
		return []string{"client:read", "site:read", "device:read", "job:read"}
	case "client_admin":
		return []string{"client:manage", "site:manage", "device:read", "job:read"}
	default:
		return nil
	}
}
