package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
)

func TestReferenceModuleRouteUsesDedicatedAccessClass(t *testing.T) {
	s := &APIServer{}
	if got := s.classifyRoute(http.MethodGet, "/api/modules/com.example.backup/devices/device-a"); got != AccessModule {
		t.Fatalf("route access = %v, want AccessModule", got)
	}
	if got := s.classifyRoute(http.MethodGet, "/api/modules/other.module/devices/device-a"); got != AccessDenied {
		t.Fatalf("unregistered module route access = %v, want AccessDenied", got)
	}
	if got := s.classifyRoute(http.MethodPost, "/api/modules/com.example.backup/devices/device-a"); got != AccessDenied {
		t.Fatalf("wrong-method module route access = %v, want AccessDenied", got)
	}
}

func TestReferenceModuleDeviceIDParsesOnlyReferenceRoute(t *testing.T) {
	deviceID, ok := referenceModuleDeviceID("/api/modules/com.example.backup/devices/device-a")
	if !ok || deviceID != "device-a" {
		t.Fatalf("device id = %q, ok = %v", deviceID, ok)
	}
	for _, path := range []string{
		"/api/modules/com.example.backup/devices/",
		"/api/modules/com.example.backup/devices/device-a/extra",
		"/api/modules/other.module/devices/device-a",
	} {
		if _, ok := referenceModuleDeviceID(path); ok {
			t.Fatalf("unexpectedly accepted path %q", path)
		}
	}
}

func TestReferenceModuleHTTPHandlerAllowsAuthorizedDeviceScope(t *testing.T) {
	authorizer, identities, moduleID := newPlatformModuleAuth(t)
	if moduleID != referenceModuleID {
		t.Fatalf("module id = %q, want %q", moduleID, referenceModuleID)
	}
	token := issuePlatformModuleToken(t, identities, moduleID, []string{referenceModuleDevicePermission})

	reached := false
	handler := newReferenceModuleDeviceHTTPHandler(
		authorizer,
		func(*http.Request) (modules.ResourceScope, error) {
			return modules.ResourceScope{MSPID: "msp-a", ClientID: "client-a", SiteID: "site-a"}, nil
		},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = true
			claims, ok := ModulePrincipalFromContext(r.Context())
			if !ok || claims.ModuleID != referenceModuleID {
				t.Fatal("validated module principal missing")
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/modules/com.example.backup/devices/device-a", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent || !reached {
		t.Fatalf("status = %d, reached = %v", resp.Code, reached)
	}
}

func TestReferenceModuleHTTPHandlerDeniesSiblingAndCrossTenantScopes(t *testing.T) {
	authorizer, identities, moduleID := newPlatformModuleAuth(t)
	token := issuePlatformModuleToken(t, identities, moduleID, []string{referenceModuleDevicePermission})

	cases := []modules.ResourceScope{
		{MSPID: "msp-a", ClientID: "client-a", SiteID: "site-b"},
		{MSPID: "msp-a", ClientID: "client-b", SiteID: "site-b"},
		{MSPID: "msp-b", ClientID: "client-b", SiteID: "site-b"},
	}
	for _, target := range cases {
		t.Run(target.MSPID+"/"+target.ClientID+"/"+target.SiteID, func(t *testing.T) {
			handler := newReferenceModuleDeviceHTTPHandler(
				authorizer,
				func(*http.Request) (modules.ResourceScope, error) { return target, nil },
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("protected handler reached") }),
			)
			req := httptest.NewRequest(http.MethodGet, "/api/modules/com.example.backup/devices/device-a", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != http.StatusForbidden {
				t.Fatalf("target %+v status = %d, want %d", target, resp.Code, http.StatusForbidden)
			}
		})
	}
}

func TestReferenceModuleRouteFailsClosedWithoutConfiguredAuthorizer(t *testing.T) {
	s := &APIServer{}
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/modules/com.example.backup/devices/device-a", nil)
	s.withAccessControl(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("ordinary mux reached")
	})).ServeHTTP(resp, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusServiceUnavailable)
	}
}
