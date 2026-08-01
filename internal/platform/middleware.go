package platform

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

const platformDomain = "rmm.stratadevplatform.com"

type contextKey string

const (
	ctxKeyUserID         contextKey = "userID"
	ctxKeyEmail          contextKey = "email"
	ctxKeyRole           contextKey = "role"
	ctxKeyTenantID       contextKey = "tenantID"
	ctxKeyMSPID          contextKey = "mspID"
	ctxKeyClientID       contextKey = "clientID"
	ctxKeySiteID         contextKey = "siteID"
	ctxKeyAuthMethod     contextKey = "authMethod"
	ctxKeyTokenID        contextKey = "tokenID"
	ctxKeyPlatformID     contextKey = "platformID"
	ctxKeySupportGrantID contextKey = "supportGrantID"
	ctxKeyTokenUse       contextKey = "tokenUse"
	ctxKeyDBTransaction  contextKey = "dbTransaction"
	ctxKeyAuthorization  contextKey = "authorization"
)

type Principal struct {
	UserID         string
	Email          string
	TokenID        string
	TokenUse       string
	PlatformID     string
	MSPID          string
	ClientID       string
	SiteID         string
	LegacyTenantID string
	Authorization  AuthorizationResult
	AuthMethod     string
	SupportGrantID string
}

type RouteAccess int

const (
	AccessPublic RouteAccess = iota
	AccessUser
	AccessAgent
	AccessAdmin
)

type Route struct {
	Method string
	Path   string
	Access RouteAccess
}

func extractBearerToken(authHeader string) string {
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	if parts[1] == "" {
		return ""
	}
	return parts[1]
}

func (s *APIServer) resolveMSPByHost(host string) (mspID, slug string) {
	host = strings.ToLower(strings.Split(host, ":")[0])

	if host == platformDomain || host == "localhost" || host == "127.0.0.1" {
		// Platform host — no MSP context. Returns empty to indicate platform scope.
		return "", ""
	}

	if strings.HasSuffix(host, "."+platformDomain) {
		slug := strings.TrimSuffix(host, "."+platformDomain)
		var id string
		err := s.db.DB().QueryRow(`
			SELECT id FROM msp_tenants
			WHERE slug = $1 AND is_active = true AND onboarding_status = 'active'
		`, slug).Scan(&id)
		if err == nil {
			return id, slug
		}
	}

	var domainMSPID, domainSlug string
	err := s.db.DB().QueryRow(`
		SELECT m.id, m.slug FROM msp_tenants m
		JOIN custom_domains d ON d.msp_id = m.id
		WHERE d.hostname = $1 AND d.verification_status IN ('verified', 'active')
		  AND m.is_active = true AND m.onboarding_status = 'active'
		LIMIT 1
	`, host).Scan(&domainMSPID, &domainSlug)
	if err == nil {
		return domainMSPID, domainSlug
	}

	return "", ""
}

func (s *APIServer) withBranding(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mspID, _ := s.resolveMSPByHost(r.Host)
		if mspID != "" {
			// Store branding in a separate context key, not security headers
			ctx := context.WithValue(r.Context(), ctxKeyMSPID, mspID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *APIServer) validateAndBuildPrincipal(rawToken string) (*Principal, error) {
	tg := s.tokenGen
	if tg == nil {
		var err error
		tg, err = auth.NewTokenGeneratorOrFail("")
		if err != nil {
			return nil, err
		}
	}
	claims, err := tg.Validate(rawToken)
	if err != nil {
		return nil, err
	}

	p := &Principal{
		TokenID:        claims.TokenID,
		TokenUse:       claims.TokenUse,
		LegacyTenantID: claims.TenantID,
		MSPID:          claims.MSPID,
		ClientID:       claims.ClientID,
		SiteID:         claims.SiteID,
		AuthMethod:     "jwt",
	}

	if claims.Subject != "" {
		p.UserID = claims.Subject
	}

	if s.allowClaimPrincipal {
		scope := inferredAuthorizationScope(claims.MSPID, claims.ClientID, claims.SiteID)
		grants := make([]AuthorizationGrant, 0, len(claims.Roles))
		for _, role := range claims.Roles {
			grants = append(grants, AuthorizationGrant{Role: role, SourceType: scope.Type, SourceID: scope.ID})
		}
		p.Authorization = newAuthorizationResult(scope, grants)
		return p, nil
	}
	if s.db == nil {
		return nil, fmt.Errorf("identity database is not configured")
	}
	tx, err := s.db.DB().BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("starting identity transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.Exec(`
		SELECT
			set_config('app.user_id', $1, true),
			set_config('app.msp_id', $2, true),
			set_config('app.client_id', $3, true),
			set_config('app.site_id', $4, true),
			set_config('app.tenant_id', $5, true),
			set_config('app.scope_type', $6, true)
	`, claims.Subject, claims.MSPID, claims.ClientID, claims.SiteID, claims.TenantID,
		string(inferredAuthorizationScope(claims.MSPID, claims.ClientID, claims.SiteID).Type)); err != nil {
		return nil, fmt.Errorf("establishing identity security context: %w", err)
	}
	if claims.TokenUse == "agent" {
		var tenantID, agentID, mspID, clientID, siteID string
		err := tx.QueryRow(`
			SELECT d.tenant_id::text, ar.agent_id, d.msp_id::text, d.client_id::text, d.site_id::text
			FROM agent_registrations ar
			JOIN devices d ON d.id = ar.device_id
			JOIN tenants t ON t.id = d.tenant_id
			JOIN msp_tenants m ON m.id = d.msp_id
			JOIN client_organizations c ON c.id = d.client_id AND c.msp_id = m.id
			JOIN sites s ON s.id = d.site_id AND s.client_id = c.id
			WHERE ar.agent_id = $1
			  AND ar.approved = true
			  AND d.status <> 'disabled'
			  AND t.is_active = true
			  AND m.is_active = true
			  AND c.is_active = true
			  AND s.is_active = true
		`, claims.Subject).Scan(&tenantID, &agentID, &mspID, &clientID, &siteID)
		if err != nil || agentID != claims.AgentID {
			return nil, fmt.Errorf("agent identity is inactive or revoked")
		}
		p.LegacyTenantID = tenantID
		p.MSPID, p.ClientID, p.SiteID = mspID, clientID, siteID
		scope := AuthorizationScope{
			Type: ScopeSite, ID: siteID, PlatformID: postgres.SingletonPlatformID,
			MSPID: mspID, ClientID: clientID, SiteID: siteID,
		}
		p.Authorization = newAuthorizationResult(scope, []AuthorizationGrant{{
			Role: "agent", SourceType: ScopeSite, SourceID: siteID,
		}})
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing agent identity transaction: %w", err)
		}
		committed = true
		return p, nil
	}

	var email string
	if err := tx.QueryRow(
		`SELECT email FROM users WHERE id = $1 AND is_active = true AND email_verified_at IS NOT NULL`,
		claims.Subject,
	).Scan(&email); err != nil {
		return nil, fmt.Errorf("user identity is inactive or revoked")
	}
	p.Email = email
	authorization, err := s.resolveAuthorization(
		context.Background(), tx, claims.Subject, claims.MSPID, claims.ClientID, claims.SiteID,
	)
	if err != nil {
		return nil, err
	}
	p.Authorization = authorization
	p.PlatformID = authorization.Selected.PlatformID
	p.MSPID = authorization.Selected.MSPID
	p.ClientID = authorization.Selected.ClientID
	p.SiteID = authorization.Selected.SiteID
	p.LegacyTenantID = authorization.Selected.ClientID
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing user identity transaction: %w", err)
	}
	committed = true

	return p, nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func principalToContext(ctx context.Context, p *Principal) context.Context {
	ctx = context.WithValue(ctx, ctxKeyUserID, p.UserID)
	ctx = context.WithValue(ctx, ctxKeyTenantID, p.LegacyTenantID)
	ctx = context.WithValue(ctx, ctxKeyMSPID, p.MSPID)
	ctx = context.WithValue(ctx, ctxKeyClientID, p.ClientID)
	ctx = context.WithValue(ctx, ctxKeySiteID, p.SiteID)
	ctx = context.WithValue(ctx, ctxKeyRole, strings.Join(p.Authorization.Roles, ","))
	ctx = context.WithValue(ctx, ctxKeyAuthorization, p.Authorization)
	ctx = context.WithValue(ctx, ctxKeyAuthMethod, p.AuthMethod)
	ctx = context.WithValue(ctx, ctxKeyTokenID, p.TokenID)
	ctx = context.WithValue(ctx, ctxKeyTokenUse, p.TokenUse)
	ctx = context.WithValue(ctx, ctxKeySupportGrantID, p.SupportGrantID)
	return ctx
}

func (s *APIServer) withAccessControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		access := s.classifyRoute(r.Method, r.URL.Path)
		if access == AccessPublic {
			next.ServeHTTP(w, r)
			return
		}

		rawToken := extractBearerToken(r.Header.Get("Authorization"))
		if rawToken == "" {
			http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
			return
		}

		principal, err := s.validateAndBuildPrincipal(rawToken)
		if err != nil {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		if access == AccessAgent && principal.TokenUse != "agent" {
			http.Error(w, "this endpoint requires an agent token", http.StatusForbidden)
			return
		}
		if access != AccessAgent && principal.TokenUse != "user" {
			http.Error(w, "agent tokens cannot access this endpoint", http.StatusForbidden)
			return
		}
		if hasSupportRole(principal.Authorization.Roles) {
			principal.SupportGrantID = strings.TrimSpace(r.Header.Get("X-Support-Grant-ID"))
		}

		r.Header.Set("X-Tenant-ID", principal.LegacyTenantID)
		if principal.MSPID != "" {
			r.Header.Set("X-MSP-ID", principal.MSPID)
		}
		if principal.ClientID != "" {
			r.Header.Set("X-Client-ID", principal.ClientID)
		}

		if principal.MSPID != "" && r.URL.Query().Get("msp_id") != "" && r.URL.Query().Get("msp_id") != principal.MSPID {
			http.Error(w, `{"error":"cross-MSP access denied"}`, http.StatusForbidden)
			return
		}
		if principal.ClientID != "" && r.URL.Query().Get("client_id") != "" && r.URL.Query().Get("client_id") != principal.ClientID {
			http.Error(w, `{"error":"cross-client access denied"}`, http.StatusForbidden)
			return
		}

		if access == AccessAgent && !hasAgentRole(principal.Authorization.Roles) {
			http.Error(w, "agent role required", http.StatusForbidden)
			return
		}
		if access == AccessAgent {
			ctx := principalToContext(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if !s.enforceProviderSetupGate(w, r, principal.Authorization) {
			return
		}
		if access == AccessAdmin {
			if !principal.Authorization.IsPlatformGlobal() {
				http.Error(w, `{"error":"admin privileges required"}`, http.StatusForbidden)
				return
			}
		} else if !hasMSPRole(principal.Authorization.Roles) && !hasPlatformRole(principal.Authorization.Roles) {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		ctx := principalToContext(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *APIServer) classifyRoute(method, path string) RouteAccess {
	allRoutes := s.publicRoutes()
	allRoutes = append(allRoutes, s.agentRoutes()...)
	allRoutes = append(allRoutes, s.scopedUserRoutes()...)
	allRoutes = append(allRoutes, s.adminRoutes()...)
	for _, r := range allRoutes {
		if r.Method == method && matchPath(r.Path, path) {
			return r.Access
		}
	}
	// Privileged namespaces fail closed. The explicit route inventory above
	// documents supported operations; these prefix guards ensure a newly added
	// handler cannot silently inherit ordinary user access if its inventory entry
	// is missed during review.
	if strings.HasPrefix(path, "/api/v1/admin/") ||
		strings.HasPrefix(path, "/api/v2/platform/") ||
		strings.HasPrefix(path, "/api/v2/deployment/") {
		return AccessAdmin
	}
	return AccessUser
}

func (s *APIServer) scopedUserRoutes() []Route {
	return []Route{
		// These handlers perform scope- and role-specific assignment checks. They
		// are intentionally inventoried before the privileged-prefix fallback.
		{Method: "POST", Path: "/api/v1/admin/users", Access: AccessUser},
		{Method: "PUT", Path: "/api/v1/admin/users/{userID}/tenants", Access: AccessUser},
		{Method: "PUT", Path: "/api/v1/admin/users/{userID}/memberships", Access: AccessUser},
	}
}

func (s *APIServer) agentRoutes() []Route {
	return []Route{
		{Method: "POST", Path: "/api/v1/agent/config", Access: AccessAgent},
		{Method: "POST", Path: "/api/v2/devices/{deviceID}/capabilities", Access: AccessAgent},
		{Method: "POST", Path: "/api/v2/devices/{deviceID}/inventory", Access: AccessAgent},
	}
}

func matchPath(pattern, path string) bool {
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	p := strings.Split(strings.Trim(path, "/"), "/")
	if len(pp) != len(p) {
		return false
	}
	for i := range pp {
		if strings.HasPrefix(pp[i], "{") && strings.HasSuffix(pp[i], "}") {
			continue
		}
		if pp[i] != p[i] {
			return false
		}
	}
	return true
}

func (s *APIServer) publicRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/", Access: AccessPublic},
		{Method: "GET", Path: "/health", Access: AccessPublic},
		{Method: "GET", Path: "/health/live", Access: AccessPublic},
		{Method: "GET", Path: "/health/ready", Access: AccessPublic},
		{Method: "GET", Path: "/metrics", Access: AccessPublic},
		{Method: "POST", Path: "/api/v1/auth/login", Access: AccessPublic},
		{Method: "POST", Path: "/api/v1/auth/invitations/inspect", Access: AccessPublic},
		{Method: "POST", Path: "/api/v1/auth/invitations/accept", Access: AccessPublic},
		{Method: "POST", Path: "/api/v1/agent/register", Access: AccessPublic},
		{Method: "POST", Path: "/api/v1/enrollment/validate", Access: AccessPublic},
		{Method: "GET", Path: "/install.sh", Access: AccessPublic},
		{Method: "GET", Path: "/releases/latest/agent/{os}/{arch}", Access: AccessPublic},
		{Method: "GET", Path: "/api/v1/auth/me", Access: AccessUser},
		{Method: "POST", Path: "/api/v1/auth/logout", Access: AccessUser},
	}
}

func (s *APIServer) adminRoutes() []Route {
	return []Route{
		// Platform-only routes
		{Method: "GET", Path: "/api/v2/platform/msps", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/msps", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v2/platform/msps/{mspID}", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/msps/{mspID}/owner-invitation", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/msps/{mspID}/suspend", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/msps/{mspID}/activate", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/msps/{mspID}/offboarding", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v2/platform/msps/{mspID}/offboarding", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/msps/{mspID}/offboarding/approve-deletion", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v2/platform/msps/{mspID}/export", Access: AccessAdmin},
		{Method: "PATCH", Path: "/api/v2/platform/msps/{mspID}/entitlement", Access: AccessAdmin},
		{Method: "PATCH", Path: "/api/v2/platform/domains/{domainID}/certificate", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/support-grants", Access: AccessAdmin},
		{Method: "DELETE", Path: "/api/v2/platform/support-grants/{grantID}", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v2/platform/provider/profile", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/provider/setup", Access: AccessAdmin},
		{Method: "PATCH", Path: "/api/v2/platform/provider/profile", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v2/deployment/state", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v2/deployment/history", Access: AccessAdmin},
		// Legacy admin routes
		{Method: "GET", Path: "/api/v1/admin/users", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v1/admin/customers", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v1/admin/update/check", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v1/admin/update/apply", Access: AccessAdmin},
	}
}

func hasPlatformRole(roles []string) bool {
	for _, r := range roles {
		switch r {
		case "platform_owner", "platform_admin", "platform_support",
			"platform_billing", "platform_security_auditor", "platform_viewer":
			return true
		}
	}
	return false
}

func hasMSPRole(roles []string) bool {
	for _, r := range roles {
		switch r {
		case "msp_owner", "msp_admin", "msp_technician", "msp_viewer",
			"client_admin", "client_viewer", "technician", "patch_manager",
			"automation_operator", "billing_manager", "auditor", "viewer",
			"admin":
			return true
		}
	}
	return false
}

func hasAgentRole(roles []string) bool {
	for _, role := range roles {
		if role == "agent" {
			return true
		}
	}
	return false
}

// isMSPOwner returns true only for actual MSP owner-level roles.
func isMSPOwner(roles []string) bool {
	for _, r := range roles {
		if r == "msp_owner" || r == "msp_admin" {
			return true
		}
	}
	return false
}

// isPlatformGlobal returns true only for platform-level roles.
