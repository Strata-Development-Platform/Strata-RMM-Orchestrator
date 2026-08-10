package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

func TestModuleLifecycleRequestClassificationIsExplicit(t *testing.T) {
	allowed := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v2/platform/modules"},
		{http.MethodPost, "/api/v2/platform/modules/com.example.backup/enable"},
		{http.MethodPost, "/api/v2/platform/modules/com.example.backup/disable"},
		{http.MethodPost, "/api/v2/platform/modules/com.example.backup/quarantine"},
		{http.MethodDelete, "/api/v2/platform/modules/com.example.backup/uninstall"},
	}
	for _, test := range allowed {
		if !isModuleLifecycleRequest(test.method, test.path) {
			t.Fatalf("expected lifecycle route %s %s", test.method, test.path)
		}
	}

	denied := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v2/platform/modules"},
		{http.MethodDelete, "/api/v2/platform/modules/com.example.backup"},
		{http.MethodPost, "/api/v2/platform/modules/com.example.backup/uninstall"},
		{http.MethodDelete, "/api/v2/platform/modules/com.example.backup/enable"},
		{http.MethodPost, "/api/v2/platform/modules/com.example.backup/unknown"},
		{http.MethodPost, "/api/v2/platform/modules//enable"},
		{http.MethodPost, "/api/v2/platform/modules/com.example.backup/enable/extra"},
	}
	for _, test := range denied {
		if isModuleLifecycleRequest(test.method, test.path) {
			t.Fatalf("unexpected lifecycle route %s %s", test.method, test.path)
		}
	}
}

func TestModuleLifecycleActorRequiresPlatformGlobalAuthorization(t *testing.T) {
	scope := AuthorizationScope{
		Type:       ScopePlatform,
		ID:         postgres.SingletonPlatformID,
		PlatformID: postgres.SingletonPlatformID,
	}
	platformAdmin := newAuthorizationResult(scope, []AuthorizationGrant{{
		Role:       "platform_admin",
		SourceType: ScopePlatform,
		SourceID:   postgres.SingletonPlatformID,
	}})

	r := httptest.NewRequest(http.MethodPost, moduleLifecycleCollectionPath, nil)
	ctx := context.WithValue(r.Context(), ctxKeyAuthorization, platformAdmin)
	ctx = context.WithValue(ctx, ctxKeyUserID, "operator-123")
	r = r.WithContext(ctx)
	actor, ok := moduleLifecycleActor(r)
	if !ok || actor != "operator-123" {
		t.Fatalf("actor = %q, ok=%v", actor, ok)
	}

	mspScope := AuthorizationScope{Type: ScopeMSP, ID: "msp-1", MSPID: "msp-1", PlatformID: postgres.SingletonPlatformID}
	mspAdmin := newAuthorizationResult(mspScope, []AuthorizationGrant{{Role: "msp_admin", SourceType: ScopeMSP, SourceID: "msp-1"}})
	ctx = context.WithValue(r.Context(), ctxKeyAuthorization, mspAdmin)
	r = r.WithContext(ctx)
	if _, ok := moduleLifecycleActor(r); ok {
		t.Fatal("MSP administrator unexpectedly accepted as module lifecycle actor")
	}
}

func TestDecodeModuleLifecycleReasonRequiresStrictAuditableReason(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantOK     bool
		wantStatus int
	}{
		{name: "valid", body: `{"reason":"operator maintenance"}`, wantOK: true, wantStatus: http.StatusOK},
		{name: "empty", body: `{"reason":"   "}`, wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"reason":"maintenance","actor":"spoofed"}`, wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "multiple values", body: `{"reason":"maintenance"} {"reason":"second"}`, wantOK: false, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			w := httptest.NewRecorder()
			reason, ok := decodeModuleLifecycleReason(w, r)
			if ok != test.wantOK {
				t.Fatalf("ok=%v, want %v; body=%s", ok, test.wantOK, w.Body.String())
			}
			if test.wantOK {
				if reason != "operator maintenance" {
					t.Fatalf("reason=%q", reason)
				}
				return
			}
			if w.Code != test.wantStatus {
				t.Fatalf("status=%d, want %d", w.Code, test.wantStatus)
			}
		})
	}
}

func TestModuleLifecycleIDRejectsMalformedPath(t *testing.T) {
	id, action, ok := moduleLifecycleID("/api/v2/platform/modules/com.example.backup/disable")
	if !ok || id != "com.example.backup" || action != "disable" {
		t.Fatalf("id=%q action=%q ok=%v", id, action, ok)
	}
	if _, _, ok := moduleLifecycleID("/api/v2/platform/modules/com.example.backup"); ok {
		t.Fatal("malformed lifecycle path accepted")
	}
}
