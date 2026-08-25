package platform

import (
	"os"
	"strings"
	"testing"
)

func TestParseScriptResultSubject(t *testing.T) {
	tests := []struct {
		name           string
		subject        string
		wantTenant     string
		wantAgent      string
		wantAuthorized bool
	}{
		{name: "valid", subject: "tenant.tenant-a.agent.agent-a.script.result", wantTenant: "tenant-a", wantAgent: "agent-a", wantAuthorized: true},
		{name: "wrong kind", subject: "tenant.tenant-a.agent.agent-a.software.result"},
		{name: "tenant wildcard", subject: "tenant.*.agent.agent-a.script.result"},
		{name: "agent wildcard", subject: "tenant.tenant-a.agent.*.script.result"},
		{name: "missing segment", subject: "tenant.tenant-a.agent.agent-a.result"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tenantID, agentID, ok := parseScriptResultSubject(tc.subject)
			if ok != tc.wantAuthorized || tenantID != tc.wantTenant || agentID != tc.wantAgent {
				t.Fatalf("parseScriptResultSubject(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.subject, tenantID, agentID, ok, tc.wantTenant, tc.wantAgent, tc.wantAuthorized)
			}
		})
	}
}

func TestScriptResultPersistenceBindsTrustedIdentity(t *testing.T) {
	source, err := os.ReadFile("script_handlers.go")
	if err != nil {
		t.Fatalf("read script_handlers.go: %v", err)
	}
	handlerStart := strings.Index(string(source), "func (s *APIServer) handleScriptResultNATS(")
	if handlerStart < 0 {
		t.Fatal("handleScriptResultNATS not found")
	}
	handler := string(source)[handlerStart:]

	for _, required := range []string{
		"parseScriptResultSubject(msg.Subject)",
		"result.ExecutionID == \"\" || result.DeviceID == \"\"",
		"sde.id::text = $1",
		"sde.schedule_id::text = $2",
		"sched.tenant_id::text = $3",
		"sde.device_id::text = $4",
		"d.agent_id::text = $5",
		"se.id = $7",
		"se.tenant_id::text = $8",
		"se.device_id::text = $9",
		"d.agent_id::text = $10",
		"se.status = 'running'",
		"RowsAffected()",
		"reject conflicting terminal script result",
	} {
		if !strings.Contains(handler, required) {
			t.Fatalf("script result handler is missing identity/replay contract %q", required)
		}
	}

	if strings.Contains(handler, "WHERE id = $7 AND status = 'running'") {
		t.Fatal("script result handler must not retain execution-id-only persistence")
	}
}
