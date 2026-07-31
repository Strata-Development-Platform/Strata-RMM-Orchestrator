package monitoring

import "testing"

func TestExtractTenantAgent(t *testing.T) {
	tenant, agent := extractTenantAgent("tenant.tenant-1.agent.agent-1.metrics")
	if tenant != "tenant-1" || agent != "agent-1" {
		t.Fatalf("identity = %q/%q", tenant, agent)
	}

	for _, subject := range []string{
		"", "tenant.tenant-1.metrics", "tenant..agent.agent-1.metrics",
		"tenant.tenant-1.probe.agent-1.metrics", "tenant.tenant-1.agent..metrics",
	} {
		tenant, agent := extractTenantAgent(subject)
		if tenant != "" || agent != "" {
			t.Fatalf("malformed subject %q returned %q/%q", subject, tenant, agent)
		}
	}
}
