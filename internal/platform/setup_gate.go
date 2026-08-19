package platform

import (
	"net/http"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

const setupRequiredCode = "provider_setup_required"

type setupGateRoute struct {
	method string
	path   string
	reason string
}

// setupGateAllowlist is exact by design. Authentication/session operations let
// an administrator inspect or end the session; provider-profile operations are
// the only writes needed to recover or complete setup. Health routes are public
// and therefore exit access control before this list is consulted. There are no
// authenticated HTTP recovery endpoints in the current route registry.
var setupGateAllowlist = []setupGateRoute{
	{method: http.MethodGet, path: "/api/v1/auth/me", reason: "inspect the current authenticated session"},
	{method: http.MethodPost, path: "/api/v1/auth/logout", reason: "end the current authenticated session"},
	{method: http.MethodGet, path: "/api/v2/context", reason: "load the server-owned session workspace and setup state"},
	{method: http.MethodPost, path: "/api/v2/context/switch", reason: "select a session workspace without administering provider resources"},
	{method: http.MethodGet, path: "/api/v2/platform/provider/profile", reason: "inspect setup state and recover saved profile values"},
	{method: http.MethodPost, path: "/api/v2/platform/provider/setup", reason: "complete provider setup"},
	{method: http.MethodPatch, path: "/api/v2/platform/provider/profile", reason: "repair provider profile values"},
}

func (s *APIServer) enforceProviderSetupGate(w http.ResponseWriter, r *http.Request, authorization AuthorizationResult) bool {
	if s.allowClaimPrincipal {
		return true
	}
	if !authorization.HasPlatformAdministratorMembership() {
		return true
	}
	for _, allowed := range setupGateAllowlist {
		if r.Method == allowed.method && r.URL.Path == allowed.path {
			return true
		}
	}
	if s.db == nil || s.db.DB() == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "provider setup status unavailable",
			"code":  "provider_setup_status_unavailable",
		})
		return false
	}
	var setupComplete bool
	if err := s.db.DB().QueryRowContext(r.Context(), `
		SELECT setup_completed_at IS NOT NULL AND setup_contract_version >= $2
		FROM platforms
		WHERE id = $1
	`, authorization.Selected.PlatformID, postgres.CurrentProviderSetupContractVersion).Scan(&setupComplete); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "provider setup status unavailable",
			"code":  "provider_setup_status_unavailable",
		})
		return false
	}
	if setupComplete {
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusPreconditionRequired, map[string]string{
		"error":     "provider setup is required before provider administration",
		"code":      setupRequiredCode,
		"setup_url": "/provider/setup",
	})
	return false
}
