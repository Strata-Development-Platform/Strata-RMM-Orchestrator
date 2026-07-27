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
	ctxKeyUserID   contextKey = "userID"
	ctxKeyEmail    contextKey = "email"
	ctxKeyRole     contextKey = "role"
	ctxKeyTenantID contextKey = "tenantID"
	ctxKeyMSPID    contextKey = "mspID"
	ctxKeyClientID contextKey = "clientID"
	ctxKeySiteID   contextKey = "siteID"
)

type RouteAccess int

const (
	AccessPublic   RouteAccess = iota
	AccessUser
	AccessAdmin
)

type Route struct {
	Method string
	Path   string
	Access RouteAccess
}

func (s *APIServer) resolveMSPByHost(host string) (mspID, slug string) {
	host = strings.ToLower(strings.Split(host, ":")[0])

	if host == platformDomain || host == "localhost" || host == "127.0.0.1" {
		if s.db == nil {
			return "", ""
		}
		var id string
		err := s.db.DB().QueryRow(`SELECT id FROM msp_tenants ORDER BY created_at ASC LIMIT 1`).Scan(&id)
		if err != nil {
			return "", ""
		}
		return id, "strata"
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

func (s *APIServer) withAccessControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		access := s.classifyRoute(r.Method, r.URL.Path)
		if access == AccessPublic {
			next.ServeHTTP(w, r)
			return
		}

		tokenStr := r.Header.Get("Authorization")
		if tokenStr == "" {
			tokenStr = r.URL.Query().Get("token")
		}
		if tokenStr == "" {
			http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
			return
		}

		tokenGen := auth.NewTokenGenerator("")
		claims, err := tokenGen.Validate(tokenStr)
		if err != nil {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		r.Header.Set("X-Tenant-ID", claims.TenantID)
		if claims.MSPID != "" {
			r.Header.Set("X-MSP-ID", claims.MSPID)
		}
		if claims.ClientID != "" {
			r.Header.Set("X-Client-ID", claims.ClientID)
		}
		if claims.AgentID != "" {
			r.Header.Set("X-Agent-ID", claims.AgentID)
		}

		ctx := context.WithValue(r.Context(), ctxKeyTenantID, claims.TenantID)
		ctx = context.WithValue(ctx, ctxKeyMSPID, claims.MSPID)
		ctx = context.WithValue(ctx, ctxKeyClientID, claims.ClientID)
		ctx = context.WithValue(ctx, ctxKeySiteID, claims.SiteID)
		ctx = context.WithValue(ctx, ctxKeyRole, strings.Join(claims.Roles, ","))

		if claims.MSPID != "" && r.URL.Query().Get("msp_id") != "" && r.URL.Query().Get("msp_id") != claims.MSPID {
			http.Error(w, `{"error":"cross-MSP access denied"}`, http.StatusForbidden)
			return
		}
		if claims.ClientID != "" && r.URL.Query().Get("client_id") != "" && r.URL.Query().Get("client_id") != claims.ClientID {
			http.Error(w, `{"error":"cross-client access denied"}`, http.StatusForbidden)
			return
		}

		if access == AccessAdmin {
			isAdmin := false
			for _, role := range claims.Roles {
				if role == "admin" {
					isAdmin = true
					break
				}
			}
			if !isAdmin {
				http.Error(w, `{"error":"admin privileges required"}`, http.StatusForbidden)
				return
			}
		}

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
		{Method: "POST", Path: "/api/v1/agent/register", Access: AccessPublic},
		{Method: "POST", Path: "/api/v1/agent/config", Access: AccessPublic},
		{Method: "GET", Path: "/api/v1/auth/me", Access: AccessUser},
	}
}

func (s *APIServer) adminRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/api/v1/admin/users", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v1/admin/users", Access: AccessAdmin},
		{Method: "PUT", Path: "/api/v1/admin/users/{userID}/tenants", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v1/admin/customers", Access: AccessAdmin},
		{Method: "GET", Path: "/api/v1/admin/update/check", Access: AccessAdmin},
		{Method: "POST", Path: "/api/v1/admin/update/apply", Access: AccessAdmin},
	}
}
