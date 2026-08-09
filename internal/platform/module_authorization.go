package platform

import (
	"context"
	"errors"
	"net/http"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

type modulePrincipalContextKey struct{}

// ModuleTargetResolver resolves the concrete organizational scope of the
// resource addressed by an HTTP request. Resolvers must derive scope from the
// authoritative resource, not trust arbitrary client-supplied scope headers.
type ModuleTargetResolver func(*http.Request) (modules.ResourceScope, error)

// WithModuleAuthorization protects a brokered module endpoint without adding
// module credentials to the ordinary user/agent middleware. The route declares
// the module identity and permission it expects; the resolver supplies the
// actual target-resource scope. Authorization remains fail closed.
func WithModuleAuthorization(authorizer *modules.APIAuthorizer, moduleID, permission string, resolveTarget ModuleTargetResolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorizer == nil || resolveTarget == nil || next == nil || moduleID == "" || permission == "" {
			http.Error(w, `{"error":"module authorization unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		rawToken := extractBearerToken(r.Header.Get("Authorization"))
		if rawToken == "" {
			http.Error(w, `{"error":"module authorization required"}`, http.StatusUnauthorized)
			return
		}
		target, err := resolveTarget(r)
		if err != nil {
			http.Error(w, `{"error":"module target unavailable"}`, http.StatusForbidden)
			return
		}
		claims, err := authorizer.Authorize(r.Context(), rawToken, modules.APIAuthorizationRequest{
			ModuleID: moduleID, Permission: permission, Scope: target,
		})
		if err != nil {
			if errors.Is(err, modules.ErrPermissionDenied) ||
				errors.Is(err, modules.ErrScopeDenied) ||
				errors.Is(err, modules.ErrModuleIdentityMismatch) ||
				errors.Is(err, modules.ErrIdentityRevoked) {
				http.Error(w, `{"error":"module access denied"}`, http.StatusForbidden)
				return
			}
			http.Error(w, `{"error":"invalid or expired module token"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), modulePrincipalContextKey{}, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ModulePrincipalFromContext returns the validated module claims attached by
// WithModuleAuthorization. Handlers must still perform their normal data-layer
// authorization/RLS operations; these claims never grant direct database access.
func ModulePrincipalFromContext(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(modulePrincipalContextKey{}).(*auth.Claims)
	return claims, ok && claims != nil
}
