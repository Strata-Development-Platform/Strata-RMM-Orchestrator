package platform

import (
	"os"
	"strings"
	"testing"
)

func TestInteractiveScriptRunUsesDurableOutbox(t *testing.T) {
	source, err := os.ReadFile("script_handlers.go")
	if err != nil {
		t.Fatalf("read script_handlers.go: %v", err)
	}

	text := string(source)
	start := strings.Index(text, "func (s *APIServer) handleRunScript(")
	end := strings.Index(text, "func (s *APIServer) handleScriptExecutions(")
	if start < 0 || end <= start {
		t.Fatal("could not isolate handleRunScript")
	}
	handler := text[start:end]

	if strings.Contains(handler, ".nats.Publish(") {
		t.Fatal("interactive script run must not publish directly to core NATS")
	}
	for _, required := range []string{
		"INSERT INTO script_executions",
		"INSERT INTO jobs",
		"INSERT INTO job_targets",
		"INSERT INTO job_outbox",
		"'job.dispatch'",
	} {
		if !strings.Contains(handler, required) {
			t.Fatalf("interactive script run is missing durable dispatch step %q", required)
		}
	}
}

func TestInteractiveScriptRunUsesRequestTenantBoundary(t *testing.T) {
	source, err := os.ReadFile("script_handlers.go")
	if err != nil {
		t.Fatalf("read script_handlers.go: %v", err)
	}

	text := string(source)
	start := strings.Index(text, "func (s *APIServer) handleRunScript(")
	end := strings.Index(text, "func (s *APIServer) handleScriptExecutions(")
	if start < 0 || end <= start {
		t.Fatal("could not isolate handleRunScript")
	}
	handler := text[start:end]

	if !strings.Contains(handler, `tenantID := r.PathValue("tenantID")`) {
		t.Fatal("script execution must derive the execution tenant from the authorized route")
	}
	if !strings.Contains(handler, "WHERE id::text = $1 AND tenant_id = $2") {
		t.Fatal("script execution target lookup must be tenant scoped")
	}
	if !strings.Contains(handler, "tenant_id = $2 OR is_public = TRUE") {
		t.Fatal("script lookup must preserve public-script use without adopting the owner tenant")
	}
}
