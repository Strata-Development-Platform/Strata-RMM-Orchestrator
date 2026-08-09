package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

func newPlatformModuleAuth(t *testing.T) (*modules.APIAuthorizer, *modules.IdentityManager, string) {
	t.Helper()
	registry := modules.NewRegistry()
	manifest := modules.Manifest{
		ID: "com.example.backup", Name: "Example Backup", Version: "1.0.0",
		APIVersion: modules.CurrentAPIVersion, Publisher: "Example Inc.",
		Permissions: []string{"devices.read", "alerts.read"},
	}
	installed, err := registry.Install(manifest)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := registry.Enable(installed.Manifest.ID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	identities, err := modules.NewIdentityManager(
		registry,
		auth.NewTokenGenerator("01234567890123456789012345678901"),
		modules.NewMemoryRevocationStore(),
	)
	if err != nil {
		t.Fatalf("identity manager: %v", err)
	}
	authorizer, err := modules.NewAPIAuthorizer(identities)
	if err != nil {
		t.Fatalf("authorizer: %v", err)
	}
	return authorizer, identities, installed.Manifest.ID
}

func issuePlatformModuleToken(t *testing.T, identities *modules.IdentityManager, moduleID string, permissions []string) string {
	t.Helper()
	credential, err := identities.Issue(context.Background(), modules.ServiceIdentityRequest{
		ModuleID: moduleID, MSPID: "msp-a", ClientID: "client-a", SiteID: "site-a",
		Permissions: permissions, TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return credential.Token
}

func TestWithModuleAuthorizationAllowsAuthorizedTargetAndSetsPrincipal(t *testing.T) {
	authorizer, identities, moduleID := newPlatformModuleAuth(t)
	token := issuePlatformModuleToken(t, identities, moduleID, []string{"devices.read"})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ModulePrincipalFromContext(r.Context())
		if !ok || claims.ModuleID != moduleID {
			t.Fatal("validated module principal missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := WithModuleAuthorization(authorizer, moduleID, "devices.read", func(*http.Request) (modules.ResourceScope, error) {
		return modules.ResourceScope{MSPID: "msp-a", ClientID: "client-a", SiteID: "site-a"}, nil
	}, next)

	req := httptest.NewRequest(http.MethodGet, "/api/modules/com.example.backup/device", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
}

func TestWithModuleAuthorizationDeniesMissingBearer(t *testing.T) {
	authorizer, _, moduleID := newPlatformModuleAuth(t)
	handler := WithModuleAuthorization(authorizer, moduleID, "devices.read", func(*http.Request) (modules.ResourceScope, error) {
		return modules.ResourceScope{MSPID: "msp-a"}, nil
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler reached") }))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/modules/com.example.backup/device", nil))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestWithModuleAuthorizationDeniesSiblingSite(t *testing.T) {
	authorizer, identities, moduleID := newPlatformModuleAuth(t)
	token := issuePlatformModuleToken(t, identities, moduleID, []string{"devices.read"})
	handler := WithModuleAuthorization(authorizer, moduleID, "devices.read", func(*http.Request) (modules.ResourceScope, error) {
		return modules.ResourceScope{MSPID: "msp-a", ClientID: "client-a", SiteID: "site-b"}, nil
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler reached") }))
	req := httptest.NewRequest(http.MethodGet, "/api/modules/com.example.backup/device", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusForbidden)
	}
}

func TestWithModuleAuthorizationDeniesPermissionEscalation(t *testing.T) {
	authorizer, identities, moduleID := newPlatformModuleAuth(t)
	token := issuePlatformModuleToken(t, identities, moduleID, []string{"devices.read"})
	handler := WithModuleAuthorization(authorizer, moduleID, "alerts.read", func(*http.Request) (modules.ResourceScope, error) {
		return modules.ResourceScope{MSPID: "msp-a", ClientID: "client-a", SiteID: "site-a"}, nil
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler reached") }))
	req := httptest.NewRequest(http.MethodGet, "/api/modules/com.example.backup/alerts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusForbidden)
	}
}

func TestWithModuleAuthorizationFailsClosedWhenMisconfigured(t *testing.T) {
	handler := WithModuleAuthorization(nil, "module", "devices.read", nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler reached")
	}))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/modules/module/device", nil))
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusServiceUnavailable)
	}
}
