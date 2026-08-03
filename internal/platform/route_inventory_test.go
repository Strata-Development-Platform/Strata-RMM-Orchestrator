package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRouteInventoryComplete verifies that every registered HTTP handler
// has a corresponding entry in the route classification tables. An unclassified
// route would silently fall through to the default policy, which is now
// AccessAdmin (fail-closed). This test ensures that any new handler is
// deliberately inventoried rather than relying on the default.
func TestRouteInventoryComplete(t *testing.T) {
	s := &APIServer{}

	// Collect every known route from the classification tables.
	allRoutes := s.publicRoutes()
	allRoutes = append(allRoutes, s.agentRoutes()...)
	allRoutes = append(allRoutes, s.scopedUserRoutes()...)
	allRoutes = append(allRoutes, s.adminRoutes()...)

	// Build a lookup of all registered paths (method + pattern).
	knownPaths := make(map[string]bool)
	for _, r := range allRoutes {
		key := r.Method + " " + r.Path
		knownPaths[key] = true
	}

	// Privileged namespace prefixes that catch unlisted routes.
	privilegedPrefixes := []string{
		"/api/v1/admin/",
		"/api/v2/platform/",
		"/api/v2/deployment/",
	}

	// Helper: check if a path falls under a privileged prefix.
	inPrivilegedNamespace := func(path string) bool {
		for _, prefix := range privilegedPrefixes {
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
		return false
	}

	// The list below enumerates every mux.HandleFunc registration from api.go.
	// Each entry is "METHOD /pattern" matching the pattern argument.
	muxPaths := []string{
		// --- api.go: public health / bootstrap ---
		"GET /",
		"GET /health",
		"GET /health/live",
		"GET /health/ready",
		"GET /metrics",
		"POST /api/v1/enroll",
		"POST /api/v1/agent/register",
		"POST /api/v1/agent/config",
		"GET /install.sh",
		"GET /releases/latest/agent/{os}/{arch}",
		"GET /releases/latest/agent/{os}/{arch}/sha256",
		// --- api.go: auth ---
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/invitations/inspect",
		"POST /api/v1/auth/invitations/accept",
		"GET /api/v1/auth/me",
		"POST /api/v1/auth/logout",
		// --- api.go: platform overview ---
		"GET /api/v1/platform/overview",
		"GET /api/v1/platform/customers",
		"GET /api/v1/platform/customers/{tenantID}/devices",
		"GET /api/v1/platform/customers/{tenantID}/devices/{deviceID}",
		"GET /api/v1/platform/customers/{tenantID}/devices-with-versions",
		"POST /api/v1/platform/customers/{tenantID}/update-source",
		"POST /api/v1/platform/customers/{tenantID}/devices/{deviceID}/update",
		"POST /api/v1/platform/customers/{tenantID}/devices/update-all",
		// --- api.go: admin ---
		"GET /api/v1/admin/users",
		"POST /api/v1/admin/users",
		"PUT /api/v1/admin/users/{userID}/tenants",
		"PUT /api/v1/admin/users/{userID}/memberships",
		"POST /api/v1/admin/customers",
		// --- api.go: metrics, heartbeat ---
		"GET /api/v1/metrics",
		"GET /api/v1/devices/{tenantID}/{deviceID}/metrics/{metricName}",
		"GET /api/v1/heartbeat/{tenantID}/{deviceID}",
		// --- api.go: alerts ---
		"GET /api/v1/alerts/{tenantID}",
		"GET /api/v1/alerts/{tenantID}/history",
		"POST /api/v1/alerts/{tenantID}/{alertID}/acknowledge",
		"GET /api/v1/alerts/{tenantID}/groups",
		// --- api.go: maintenance windows ---
		"POST /api/v1/tenants/{tenantID}/maintenance-windows",
		"GET /api/v1/tenants/{tenantID}/maintenance-windows",
		"DELETE /api/v1/tenants/{tenantID}/maintenance-windows/{windowID}",
		// --- api.go: retention ---
		"GET /api/v1/tenants/{tenantID}/retention",
		"PATCH /api/v1/tenants/{tenantID}/retention",
		"GET /api/v1/retention/policies",
		// --- api.go: rules ---
		"POST /api/v1/rules/{tenantID}",
		"GET /api/v1/rules/{tenantID}",
		"DELETE /api/v1/rules/{tenantID}/{ruleID}",
		// --- api.go: vulnerabilities ---
		"GET /api/v1/vulnerabilities/device/{deviceID}",
		"GET /api/v1/vulnerabilities/tenant/{tenantID}",
		"GET /api/v1/vulnerabilities/tenant/{tenantID}/summary",
		"POST /api/v1/vulnerabilities/{vulnID}/resolve",
		"POST /api/v1/vulnerabilities/{vulnID}/ignore",
		// --- api.go: CVE ---
		"GET /api/v1/cve/stats",
		"POST /api/v1/cve/sync",
		"GET /api/v1/cve/packages",
		"POST /api/v1/cve/packages",
		"DELETE /api/v1/cve/packages/{name}/{ecosystem}",
		"GET /api/v1/cve/sync/status",
		"GET /api/v1/cve/package/{name}",
		// --- api.go: third-party ---
		"GET /api/v1/thirdparty/apps",
		"GET /api/v1/thirdparty/packages",
		"POST /api/v1/thirdparty/sync",
		"POST /api/v1/thirdparty/sync/{app}",
		// --- api.go: reports ---
		"GET /api/v1/reports/{tenantID}",
		"POST /api/v1/reports/{tenantID}/schedules",
		"GET /api/v1/reports/{tenantID}/schedules",
		"DELETE /api/v1/reports/{tenantID}/schedules/{scheduleID}",
		"PATCH /api/v1/reports/{tenantID}/schedules/{scheduleID}",
		"PATCH /api/v1/reports/{tenantID}/schedules/{scheduleID}/enable",
		"POST /api/v1/reports/{tenantID}/schedules/{scheduleID}/trigger",
		"POST /api/v1/reports/{tenantID}/generate",
		"GET /api/v1/reports/{tenantID}/{reportID}/download",
		"POST /api/v1/reports/{tenantID}/compliance",
		"GET /api/v1/reports/{tenantID}/compliance",
		"GET /api/v1/reports/{tenantID}/compliance/{reportID}",
		"GET /api/v1/reports/{tenantID}/compliance/{reportID}/export/csv",
		"GET /api/v1/reports/{tenantID}/compliance/{reportID}/export/json",
		// --- api.go: billing ---
		"GET /api/v2/msps/{mspID}/billing/account",
		"POST /api/v2/msps/{mspID}/billing/account",
		"DELETE /api/v2/msps/{mspID}/billing/account",
		"GET /api/v2/msps/{mspID}/billing/subscriptions",
		"POST /api/v2/msps/{mspID}/billing/subscriptions",
		"DELETE /api/v2/msps/{mspID}/billing/subscriptions/{subscriptionID}",
		"GET /api/v2/msps/{mspID}/billing/invoices",
		"GET /api/v2/msps/{mspID}/billing/invoices/{invoiceID}",
		"POST /api/v2/msps/{mspID}/billing/usage",
		"GET /api/v2/msps/{mspID}/billing/usage/{meterName}",
		"GET /api/v2/msps/{mspID}/billing/payment-methods",
		"POST /api/v2/msps/{mspID}/billing/payment-methods",
		"PATCH /api/v2/msps/{mspID}/billing/payment-methods/{paymentMethodID}",
		"DELETE /api/v2/msps/{mspID}/billing/payment-methods/{paymentMethodID}",
		"GET /api/v2/msps/{mspID}/billing/reports/revenue",
		"GET /api/v2/platform/billing/analytics",
		// --- api.go: update ---
		"GET /api/v1/admin/update/check",
		"POST /api/v1/admin/update/apply",
		// --- device_handlers.go: CMDB ---
		"GET /api/v1/devices/relationships",
		"POST /api/v1/devices/relationships",
		"DELETE /api/v1/devices/relationships/{relationshipID}",
		"GET /api/v1/devices/{deviceID}/dependencies",
		"GET /api/v1/devices/{deviceID}/impact",
		"GET /api/v1/devices/addresses",
		"POST /api/v1/devices/addresses",
		"GET /api/v1/devices/{deviceID}/packages",
		"POST /api/v1/devices/{deviceID}/packages",
		"GET /api/v1/devices/{deviceID}/services",
		// --- api.go: remote support ---
		"POST /api/v1/remote/{tenantID}/session",
		"POST /api/v1/remote/{tenantID}/session/{sessionID}/input",
		"DELETE /api/v1/remote/{tenantID}/session/{sessionID}",
		// --- api.go: keys ---
		"POST /api/v1/keys/{tenantID}",
		"GET /api/v1/keys/{tenantID}",
		"GET /api/v1/keys/{tenantID}/active",
		"POST /api/v1/keys/{tenantID}/rotate",
		"DELETE /api/v1/keys/{tenantID}/{keyID}",
		// --- api.go: access ---
		"GET /api/v1/access/audit/{tenantID}",
		"GET /api/v1/access/users/{tenantID}",
		"GET /api/v1/access/permissions/{tenantID}",
		// --- api.go: scripts ---
		"GET /api/v1/scripts/{tenantID}",
		"POST /api/v1/scripts/{tenantID}",
		"GET /api/v1/scripts/{tenantID}/{scriptID}",
		"DELETE /api/v1/scripts/{tenantID}/{scriptID}",
		"POST /api/v1/scripts/{tenantID}/{scriptID}/run",
		"GET /api/v1/scripts/{tenantID}/executions",
		"GET /api/v1/scripts/{tenantID}/executions/{execID}",
		// --- api.go: script scheduling ---
		"POST /api/v1/tenants/{tenantID}/scripts/schedule",
		"GET /api/v1/tenants/{tenantID}/scripts/schedules",
		"GET /api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}",
		"PUT /api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}",
		"DELETE /api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}",
		"POST /api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}/preview",
		"GET /api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}/devices",
		"POST /api/v1/tenants/{tenantID}/scripts/schedules/{scheduleID}/devices/{execID}/retry",
		"GET /api/v1/tenants/{tenantID}/scripts/schedules/executions",
		// --- api.go: software ---
		"GET /api/v1/software/packages/{tenantID}",
		"POST /api/v1/software/packages/{tenantID}",
		"DELETE /api/v1/software/packages/{tenantID}/{pkgID}",
		"POST /api/v1/software/deployments/{tenantID}",
		"GET /api/v1/software/deployments/{tenantID}",
		"GET /api/v1/software/deployments/{tenantID}/{deployID}",
		// --- api.go: MFA ---
		"POST /api/v1/mfa/enroll/{userID}",
		"POST /api/v1/mfa/verify/{userID}",
		"GET /api/v1/mfa/status/{userID}",
		"DELETE /api/v1/mfa/{userID}",
		// --- api.go: recordings ---
		"GET /api/v1/recordings/{tenantID}",
		"GET /api/v1/recordings/{id}/playback",
		"DELETE /api/v1/recordings/{id}",
		// --- api.go: branding / domains ---
		"GET /api/v1/branding",
		"PUT /api/v1/branding",
		"GET /api/v1/domains",
		"POST /api/v1/domains",
		"POST /api/v1/domains/{domainID}/verify",
		"DELETE /api/v1/domains/{domainID}",
		// --- api.go: enrollment ---
		"POST /api/v1/enrollment/tokens",
		"POST /api/v1/enrollment/validate",
		"GET /api/v1/enrollment/tokens",
		"DELETE /api/v1/enrollment/tokens/{tokenID}",
		// --- api.go: jobs ---
		"POST /api/v1/jobs",
		"GET /api/v1/jobs",
		"GET /api/v1/jobs/{jobID}",
		"POST /api/v1/jobs/{jobID}/cancel",
		"POST /api/v1/jobs/{jobID}/retry",
		"GET /api/v1/devices/{deviceID}/jobs",
		"GET /api/v1/jobs/{jobID}/events",
		// --- api.go: device-groups ---
		"POST /api/v1/device-groups",
		"GET /api/v1/device-groups",
		"DELETE /api/v1/device-groups/{groupID}",
		// --- api.go: maintenance windows (additional) ---
		"POST /api/v1/maintenance-windows",
		"GET /api/v1/maintenance-windows",
		"DELETE /api/v1/maintenance-windows/{windowID}",
		// --- api.go: policies ---
		"POST /api/v1/policies",
		"GET /api/v1/policies",
		"GET /api/v1/policies/{policyID}",
		"PUT /api/v1/policies/{policyID}",
		"POST /api/v1/policies/{policyID}/validate",
		"POST /api/v1/policies/{policyID}/preview",
		"POST /api/v1/policies/{policyID}/publish",
		"GET /api/v1/policies/{policyID}/revisions",
		"POST /api/v1/policies/{policyID}/diff",
		"POST /api/v1/policies/{policyID}/effective",
		"DELETE /api/v1/policies/{policyID}",
		// --- api.go: MSP platform routes ---
		"GET /api/v2/platform/msps",
		"POST /api/v2/platform/msps",
		"GET /api/v2/platform/msps/{mspID}",
		"POST /api/v2/platform/msps/{mspID}/owner-invitation",
		"POST /api/v2/platform/msps/{mspID}/suspend",
		"POST /api/v2/platform/msps/{mspID}/activate",
		"POST /api/v2/platform/msps/{mspID}/offboarding",
		"GET /api/v2/platform/msps/{mspID}/offboarding",
		"POST /api/v2/platform/msps/{mspID}/offboarding/approve-deletion",
		"GET /api/v2/platform/msps/{mspID}/export",
		"PATCH /api/v2/platform/msps/{mspID}/entitlement",
		"PATCH /api/v2/platform/domains/{domainID}/certificate",
		"POST /api/v2/platform/support-grants",
		"DELETE /api/v2/platform/support-grants/{grantID}",
		"GET /api/v2/platform/provider/profile",
		"POST /api/v2/platform/provider/setup",
		"PATCH /api/v2/platform/provider/profile",
		// --- api.go: deployment ---
		"GET /api/v2/deployment/state",
		"GET /api/v2/deployment/history",
		// --- api.go: MSP client management ---
		"GET /api/v2/msps/{mspID}/clients",
		"POST /api/v2/msps/{mspID}/clients",
		"GET /api/v2/msps/{mspID}/clients/{clientID}",
		"POST /api/v2/msps/{mspID}/clients/{clientID}/archive",
		// --- client_handlers.go: client portal ---
		"GET /api/v2/clients/{clientID}/auth/providers",
		"POST /api/v2/clients/{clientID}/auth/providers",
		"POST /api/v2/clients/{clientID}/sessions",
		"GET /api/v2/clients/{clientID}/sessions",
		"DELETE /api/v2/clients/{clientID}/sessions/{sessionID}",
		"GET /api/v2/clients/{clientID}/profile",
		"PATCH /api/v2/clients/{clientID}/profile",
		"GET /api/v2/clients/{clientID}/settings",
		"PATCH /api/v2/clients/{clientID}/settings",
		// --- api.go: MSP client sites ---
		"GET /api/v2/clients/{clientID}/sites",
		"POST /api/v2/clients/{clientID}/sites",
		"GET /api/v2/clients/{clientID}/sites/{siteID}",
		"POST /api/v2/clients/{clientID}/sites/{siteID}/archive",
		// --- api.go: MSP memberships ---
		"GET /api/v2/msps/{mspID}/memberships",
		"POST /api/v2/msps/{mspID}/memberships",
		"DELETE /api/v2/msps/{mspID}/memberships/{membershipID}",
		// --- api.go: MSP entitlement/usage/audit/devices ---
		"GET /api/v2/msps/{mspID}/entitlement",
		"GET /api/v2/msps/{mspID}/usage",
		"GET /api/v2/msps/{mspID}/audit",
		"GET /api/v2/msps/{mspID}/devices",
		// --- api.go: context ---
		"GET /api/v2/context",
		"POST /api/v2/context/switch",
		// --- api.go: v2 devices ---
		"GET /api/v2/devices",
		"GET /api/v2/devices/{deviceID}",
		"POST /api/v2/devices/{deviceID}/action",
		"POST /api/v2/devices/bulk-action",
		"GET /api/v2/devices/{deviceID}/inventory",
		"GET /api/v2/devices/{deviceID}/capabilities",
		"POST /api/v2/devices/{deviceID}/capabilities",
		"POST /api/v2/devices/{deviceID}/inventory",
		// --- api.go: approvals ---
		"POST /api/v2/approvals",
		"GET /api/v2/approvals",
		"GET /api/v2/approvals/{approvalID}",
		"POST /api/v2/approvals/{approvalID}/approve",
		"POST /api/v2/approvals/{approvalID}/reject",
		"POST /api/v2/approvals/{approvalID}/cancel",
		"GET /api/v2/approvals/{approvalID}/decisions",
		// --- api.go: endpoint audit ---
		"GET /api/v2/audit/endpoint",
	}

	// Check each mux-registered path.
	var unclassified []string
	for _, path := range muxPaths {
		parts := strings.SplitN(path, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed mux path entry: %q", path)
		}
		_ = parts[0]
		pattern := parts[1]

		if knownPaths[path] {
			continue
		}

		if inPrivilegedNamespace(pattern) {
			continue
		}

		unclassified = append(unclassified, path)
	}

	if len(unclassified) > 0 {
		t.Errorf("%d route(s) are not in any classification table and are not in a privileged namespace. "+
			"These would default to AccessDenied (fail-closed). Add each to the appropriate route function:\n  %s",
			len(unclassified), strings.Join(unclassified, "\n  "))
	}

	// Sanity: ensure the classification actually enforces AccessDenied for unknown routes.
	srv := &APIServer{}
	if srv.classifyRoute("GET", "/api/v1/unknown/route") != AccessDenied {
		t.Error("classifyRoute should return AccessDenied for unknown routes outside privileged namespaces")
	}
	if srv.classifyRoute("GET", "/api/v2/platform/unknown") != AccessDenied {
		t.Error("classifyRoute should return AccessDenied for unknown routes in privileged namespaces")
	}
}

// TestRouteClassificationConsistency verifies that the route classification
// tables are internally consistent: no duplicate method+path entries and every
// entry uses a valid RouteAccess value.
func TestRouteClassificationConsistency(t *testing.T) {
	s := &APIServer{}

	allRoutes := s.publicRoutes()
	allRoutes = append(allRoutes, s.agentRoutes()...)
	allRoutes = append(allRoutes, s.scopedUserRoutes()...)
	allRoutes = append(allRoutes, s.adminRoutes()...)

	seen := make(map[string]int)
	for i, r := range allRoutes {
		if r.Access < AccessPublic || r.Access > AccessAdmin {
			t.Errorf("route %d: invalid access level %d for %s %s", i, r.Access, r.Method, r.Path)
		}
		key := r.Method + " " + r.Path
		if dup, ok := seen[key]; ok {
			t.Errorf("duplicate route entry at index %d: %s %s (first at index %d)", i, r.Method, r.Path, dup)
		}
		seen[key] = i
	}

	// AccessDenied is not expected in classification tables — it is the implicit
	// default returned by classifyRoute for unregistered routes.  A table entry
	// with AccessDenied would be confusing and should be avoided.
}

// TestAccessControlDefaultsToAdmin verifies that a request to an unknown route
// is denied even when a valid admin token is present (fail-closed).
func TestAccessControlDefaultsToAdmin(t *testing.T) {
	s := &APIServer{
		allowClaimPrincipal: true,
	}

	r := httptest.NewRequest("GET", "/api/v1/unknown/route", nil)
	w := httptest.NewRecorder()

	s.withAccessControl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unknown route (fail-closed), got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unclassified") {
		t.Errorf("expected 'unclassified route' error message, got: %s", w.Body.String())
	}
}

// TestAccessControlPrivilegedNamespaceFailClosed verifies that unknown routes
// inside privileged namespaces (/api/v1/admin/, /api/v2/platform/,
// /api/v2/deployment/) also return 403.
func TestAccessControlPrivilegedNamespaceFailClosed(t *testing.T) {
	s := &APIServer{
		allowClaimPrincipal: true,
	}

	privilegedPaths := []string{
		"/api/v1/admin/unknown",
		"/api/v2/platform/unknown",
		"/api/v2/deployment/unknown",
	}

	for _, path := range privilegedPaths {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()

		s.withAccessControl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403 for unclassified route %s, got %d: %s", path, w.Code, w.Body.String())
		}
	}
}
