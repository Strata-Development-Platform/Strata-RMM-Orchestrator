package platform

import (
	"context"
	"net/http"
	"strings"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

const platformDomain = "rmm.stratadevplatform.com"

type contextKey string

const (
	ctxKeyUserID        contextKey = "userID"
	ctxKeyEmail         contextKey = "email"
	ctxKeyRole          contextKey = "role"
	ctxKeyTenantID      contextKey = "tenantID"
	ctxKeyMSPID         contextKey = "mspID"
	ctxKeyClientID      contextKey = "clientID"
	ctxKeySiteID        contextKey = "siteID"
	ctxKeyAuthMethod    contextKey = "authMethod"
	ctxKeyTokenID       contextKey = "tokenID"
	ctxKeyPlatformID    contextKey = "platformID"
	ctxKeySupportGrantID contextKey = "supportGrantID"
	ctxKeyTokenUse      contextKey = "tokenUse"
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
	Roles          []string
	Permissions    []string
	AuthMethod     string
	SupportGrantID string
}

type RouteAccess int

const (
	AccessPublic  RouteAccess = iota
	AccessUser
	AccessAdmin
)

type Route struct {
	Method string
	Path   string
	Access RouteAccess
}

func extractBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	for _, prefix := range []string{"Bearer ", "bearer "} {
		if len(authHeader) > len(prefix) && strings.EqualFold(authHeader[:len(prefix)], prefix) {
			return authHeader[len(prefix):]
		}
	}
	return authHeader
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
		err := s.db.DB().QueryRow(`SELECT id FROM msp_tenants WHERE slug = $1 AND is_active = true`, slug).Scan(&id)
		if err == nil {
			return id, slug
		}
	}

	var domainMSPID, domainSlug string
	err := s.db.DB().QueryRow(`
		SELECT m.id, m.slug FROM msp_tenants m
		JOIN custom_domains d ON d.msp_id = m.id
		WHERE d.hostname = $1 AND d.verification_status IN ('verified', 'active')
		LIMIT 1
	`, host).Scan(&domainMSPID, &domainSlug)
	if err == nil {
		return domainMSPID, domainSlug
	}

	return "", ""
}

func (s *APIServer) withBranding(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mspID, slug := s.resolveMSPByHost(r.Host)
		if mspID != "" {
			ctx := context.WithValue(r.Context(), ctxKeyMSPID, mspID)
			r.Header.Set("X-MSP-ID", mspID)
			r.Header.Set("X-MSP-Slug", slug)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *APIServer) validateAndBuildPrincipal(rawToken string) (*Principal, error) {
	tokenGen := auth.NewTokenGenerator("")
	claims, err := tokenGen.Validate(rawToken)
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
		Roles:          claims.Roles,
		AuthMethod:     "jwt",
	}

	if claims.Subject != "" {
		p.UserID = claims.Subject
	}

	return p, nil
}

func principalToContext(ctx context.Context, p *Principal) context.Context {
	ctx = context.WithValue(ctx, ctxKeyUserID, p.UserID)
	ctx = context.WithValue(ctx, ctxKeyTenantID, p.LegacyTenantID)
	ctx = context.WithValue(ctx, ctxKeyMSPID, p.MSPID)
	ctx = context.WithValue(ctx, ctxKeyClientID, p.ClientID)
	ctx = context.WithValue(ctx, ctxKeySiteID, p.SiteID)
	ctx = context.WithValue(ctx, ctxKeyRole, strings.Join(p.Roles, ","))
	ctx = context.WithValue(ctx, ctxKeyAuthMethod, p.AuthMethod)
	ctx = context.WithValue(ctx, ctxKeyTokenID, p.TokenID)
	ctx = context.WithValue(ctx, ctxKeyTokenUse, p.TokenUse)
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
			rawToken = r.URL.Query().Get("token")
		}
		if rawToken == "" {
			http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
			return
		}

		principal, err := s.validateAndBuildPrincipal(rawToken)
		if err != nil {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		// Enforce token-use separation
		isAgentPath := strings.HasPrefix(r.URL.Path, "/api/v1/agent/") ||
			strings.HasPrefix(r.URL.Path, "/releases/") ||
			r.URL.Path == "/install.sh"
		isUserPath := !isAgentPath

		if isUserPath && principal.TokenUse == "agent" {
			http.Error(w, `{"error":"agent tokens cannot access this endpoint"}`, http.StatusForbidden)
			return
		}
		if isAgentPath && principal.TokenUse == "user" {
			// Agent endpoints may also accept user tokens for admin operations
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

		if access == AccessAdmin {
			if !hasPlatformRole(principal.Roles) {
				http.Error(w, `{"error":"admin privileges required"}`, http.StatusForbidden)
				return
			}
		} else if !hasMSPRole(principal.Roles) && !hasPlatformRole(principal.Roles) {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		ctx := principalToContext(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *APIServer) classifyRoute(method, path string) RouteAccess {
	allRoutes := s.publicRoutes()
	allRoutes = append(allRoutes, s.adminRoutes()...)
	for _, r := range allRoutes {
		if r.Method == method && matchPath(r.Path, path) {
			return r.Access
		}
	}
	if strings.HasPrefix(path, "/api/v1/admin/") {
		return AccessAdmin
	}
	return AccessUser
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
		{Method: "POST", Path: "/api/v1/auth/login", Access: AccessPublic},
		{Method: "GET", Path: "/install.sh", Access: AccessPublic},
		{Method: "GET", Path: "/releases/latest/agent/{os}/{arch}", Access: AccessPublic},
		{Method: "GET", Path: "/api/v1/auth/me", Access: AccessUser},
	}
}

func (s *APIServer) adminRoutes() []Route {
	return []Route{
		// Platform-only routes
		{Method: "GET", Path: "/api/v2/platform/msps", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/msps", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v2/platform/msps/{mspID}", Access: AccessAdmin},
		{Method: "PATCH", Path: "/api/v2/platform/msps/{mspID}", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/msps/{mspID}/suspend", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v2/platform/msps/{mspID}/activate", Access: AccessAdmin},
		// Legacy admin routes
		{Method: "GET", Path: "/api/v1/admin/users", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v1/admin/users", Access: AccessAdmin},
		{Method: "PUT", Path: "/api/v1/admin/users/{userID}/tenants", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v1/admin/customers", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v1/admin/update/check", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v1/admin/update/apply", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v1/enrollment/tokens", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v1/enrollment/tokens", Access: AccessAdmin},
	}
}

func hasPlatformRole(roles []string) bool {
	for _, r := range roles {
		switch r {
		case "platform_owner", "platform_admin", "platform_support",
			"platform_billing", "platform_security_auditor", "platform_viewer",
			"admin": // legacy compatibility
			return true
		}
	}
	return false
}

func hasMSPRole(roles []string) bool {
	for _, r := range roles {
		switch r {
		case "msp_owner", "msp_admin", "technician", "patch_manager",
			"automation_operator", "billing_manager", "auditor", "viewer",
			"admin": // legacy compatibility
			return true
		}
	}
	return false
}
