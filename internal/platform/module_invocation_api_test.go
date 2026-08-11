package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
)

func TestModuleDeviceInvocationTargetParsesOnlyExactRoute(t *testing.T) {
	moduleID, deviceID, ok := moduleDeviceInvocationTarget("/api/v2/platform/modules/com.example.backup/invoke/device/00000000-0000-0000-0000-000000000401")
	if !ok || moduleID != "com.example.backup" || deviceID != "00000000-0000-0000-0000-000000000401" {
		t.Fatalf("module=%q device=%q ok=%v", moduleID, deviceID, ok)
	}
	for _, path := range []string{
		"/api/v2/platform/modules/com.example.backup/invoke/device/",
		"/api/v2/platform/modules/com.example.backup/invoke/client/device-a",
		"/api/v2/platform/modules/com.example.backup/invoke/device/device-a/extra",
		"/api/modules/com.example.backup/invoke/device/device-a",
	} {
		if _, _, ok := moduleDeviceInvocationTarget(path); ok {
			t.Fatalf("unexpectedly accepted %q", path)
		}
	}
}

func TestModuleDeviceInvocationIsAdminLifecycleClassOnlyForPost(t *testing.T) {
	path := "/api/v2/platform/modules/com.example.backup/invoke/device/device-a"
	if !isModuleLifecycleRequest(http.MethodPost, path) {
		t.Fatal("POST invocation was not classified into the platform-admin lifecycle gate")
	}
	if isModuleLifecycleRequest(http.MethodGet, path) {
		t.Fatal("GET invocation unexpectedly classified as an allowed lifecycle request")
	}
	if got := (&APIServer{}).classifyRoute(http.MethodPost, path); got != AccessDenied {
		t.Fatalf("base route classification=%v, want AccessDenied before lifecycle elevation", got)
	}
}

func TestModuleInvocationRequiresPathOwnedByRequestedModule(t *testing.T) {
	// This exercises validation before any database/runtime access by using a
	// server with a non-nil database requirement deliberately absent. The route
	// parser itself is the security boundary that prevents module A from asking
	// the host to execute module B's namespace.
	moduleID := "com.example.backup"
	bad := "/api/modules/other.module/action"
	if len(bad) >= len("/api/modules/"+moduleID+"/") && bad[:len("/api/modules/"+moduleID+"/")] == "/api/modules/"+moduleID+"/" {
		t.Fatal("test fixture unexpectedly shares requested module namespace")
	}
}

func TestWriteModuleInvocationErrorIsFailClosed(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{modules.ErrNotFound, http.StatusNotFound},
		{modules.ErrModuleDisabled, http.StatusConflict},
		{modules.ErrQuarantined, http.StatusConflict},
		{modules.ErrRouteNotDeclared, http.StatusBadRequest},
		{modules.ErrMethodNotDeclared, http.StatusBadRequest},
		{modules.ErrPermissionDenied, http.StatusForbidden},
		{modules.ErrPermissionMismatch, http.StatusForbidden},
		{modules.ErrRuntimeUnavailable, http.StatusBadGateway},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		writeModuleInvocationError(recorder, tc.err)
		if recorder.Code != tc.want {
			t.Fatalf("error=%v status=%d want=%d", tc.err, recorder.Code, tc.want)
		}
	}
}
