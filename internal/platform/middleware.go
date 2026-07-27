package platform

import (
	"context"
	"net/http"
	"strings"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

type contextKey string

const (
	ctxKeyUserID   contextKey = "userID"
	ctxKeyEmail    contextKey = "email"
	ctxKeyRole     contextKey = "role"
	ctxKeyTenantID contextKey = "tenantID"
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
		if claims.AgentID != "" {
			r.Header.Set("X-Agent-ID", claims.AgentID)
		}

		ctx := context.WithValue(r.Context(), ctxKeyTenantID, claims.TenantID)
		ctx = context.WithValue(ctx, ctxKeyRole, strings.Join(claims.Roles, ","))

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
