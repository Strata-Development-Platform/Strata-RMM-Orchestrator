package platform

import (
	"os"
	"strings"
	"testing"
)

func isolateScriptHandler(t *testing.T, source, startName, endName string) string {
	t.Helper()
	start := strings.Index(source, "func (s *APIServer) "+startName+"(")
	end := strings.Index(source, "func (s *APIServer) "+endName+"(")
	if start < 0 || end <= start {
		t.Fatalf("could not isolate %s", startName)
	}
	return source[start:end]
}

func TestGetScriptBindsObjectLookupToAuthorizedTenant(t *testing.T) {
	source, err := os.ReadFile("script_handlers.go")
	if err != nil {
		t.Fatalf("read script_handlers.go: %v", err)
	}
	handler := isolateScriptHandler(t, string(source), "handleGetScript", "handleDeleteScript")

	for _, required := range []string{
		`tenantID := r.PathValue("tenantID")`,
		"WHERE id = $1 AND (tenant_id = $2 OR is_public = TRUE)",
		"scriptID, tenantID",
	} {
		if !strings.Contains(handler, required) {
			t.Fatalf("GET script handler is missing tenant-bound lookup contract %q", required)
		}
	}
}

func TestDeleteScriptRequiresOwningTenantAndDetectsNoop(t *testing.T) {
	source, err := os.ReadFile("script_handlers.go")
	if err != nil {
		t.Fatalf("read script_handlers.go: %v", err)
	}
	handler := isolateScriptHandler(t, string(source), "handleDeleteScript", "handleRunScript")

	for _, required := range []string{
		`tenantID := r.PathValue("tenantID")`,
		"DELETE FROM scripts WHERE id = $1 AND tenant_id = $2",
		"scriptID, tenantID",
		"RowsAffected()",
		"rowsAffected == 0",
		"http.StatusNotFound",
	} {
		if !strings.Contains(handler, required) {
			t.Fatalf("DELETE script handler is missing tenant/no-op contract %q", required)
		}
	}

	if strings.Contains(handler, "DELETE FROM scripts WHERE id = $1`" ) {
		t.Fatal("DELETE script handler must not retain an unscoped script-id-only delete")
	}
}
