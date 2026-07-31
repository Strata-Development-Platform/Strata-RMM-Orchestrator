package platform

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

func TestBuildAgentNATSPermissionsScopesEndpoint(t *testing.T) {
	policy, err := BuildAgentNATSPermissions("tenant-a", "agent-1")
	if err != nil {
		t.Fatal(err)
	}

	wantPublish := []string{
		"tenant.tenant-a.agent.agent-1.heartbeat",
		"tenant.tenant-a.agent.agent-1.metrics",
		"tenant.tenant-a.agent.agent-1.events",
		"tenant.tenant-a.agent.agent-1.ack",
		"tenant.tenant-a.agent.agent-1.result",
		"tenant.tenant-a.agent.agent-1.script.result",
		"tenant.tenant-a.agent.agent-1.software.result",
		"tenant.tenant-a.agent.agent-1.tunnel.*.frame",
		"tenant.tenant-a.agent.agent-1.tunnel.*.ctrl",
	}
	wantSubscribe := []string{
		"tenant.tenant-a.cmd.agent-1",
		"tenant.tenant-a.cmd.agent-1.cancel",
		"tenant.tenant-a.agent.agent-1.result.ack",
		"tenant.tenant-a.agent.agent-1.tunnel.*.input",
		"tenant.tenant-a.agent.agent-1.tunnel.*.ctrl",
	}
	if !reflect.DeepEqual(policy.Publish, wantPublish) {
		t.Fatalf("publish policy = %#v", policy.Publish)
	}
	if !reflect.DeepEqual(policy.Subscribe, wantSubscribe) {
		t.Fatalf("subscribe policy = %#v", policy.Subscribe)
	}

	for _, subject := range append(append([]string{}, policy.Publish...), policy.Subscribe...) {
		if strings.Contains(subject, "tenant.*") || strings.Contains(subject, "agent.*.") {
			t.Fatalf("identity wildcard leaked into %q", subject)
		}
		if strings.Contains(subject, "tenant-b") || strings.Contains(subject, "agent-2") {
			t.Fatalf("cross-identity permission %q", subject)
		}
	}
}

func TestBuildAgentNATSPermissionsRejectsSubjectInjection(t *testing.T) {
	tests := []struct {
		tenant string
		agent  string
	}{
		{"", "agent-1"},
		{"tenant-a", ""},
		{"tenant.*", "agent-1"},
		{"tenant-a", ">"},
		{"tenant.a", "agent-1"},
		{"tenant-a", "agent 1"},
	}
	for _, tc := range tests {
		if _, err := BuildAgentNATSPermissions(tc.tenant, tc.agent); err == nil {
			t.Fatalf("expected rejection for tenant=%q agent=%q", tc.tenant, tc.agent)
		}
	}
}

func TestAuthorizeAgentNATSCredential(t *testing.T) {
	const secret = "agent-nats-policy-test-secret-at-least-32-bytes"
	generator := auth.NewTokenGenerator(secret)
	token, err := generator.GenerateAgentToken("tenant-a", "agent-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := AuthorizeAgentNATSCredential(generator, token, time.Now())
	if err != nil {
		t.Fatalf("authorize agent: %v", err)
	}
	if grant.TenantID != "tenant-a" || grant.AgentID != "agent-1" {
		t.Fatalf("grant identity = %#v", grant)
	}
	if grant.Permission.Publish[0] != "tenant.tenant-a.agent.agent-1.heartbeat" {
		t.Fatalf("grant publish policy = %#v", grant.Permission.Publish)
	}
}

func TestAuthorizeAgentNATSCredentialRejectsUserAndInvalidTokens(t *testing.T) {
	const secret = "agent-nats-policy-test-secret-at-least-32-bytes"
	generator := auth.NewTokenGenerator(secret)
	userToken, err := generator.GenerateUserToken("user-1", "tenant-a", "tenant-a", "", "", []string{"technician"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for name, token := range map[string]string{
		"missing":    "",
		"malformed":  "not-a-jwt",
		"user token": userToken,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := AuthorizeAgentNATSCredential(generator, token, time.Now()); err == nil {
				t.Fatal("credential accepted")
			}
		})
	}
}
