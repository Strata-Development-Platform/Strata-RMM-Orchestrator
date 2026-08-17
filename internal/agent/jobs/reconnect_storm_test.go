//go:build jobintegration

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func stormCommand(t *testing.T, tenantID, agentID, eventID, targetID string) []byte {
	t.Helper()
	payload, err := json.Marshal(CommandEnvelope{
		SchemaVersion: 1,
		EventID:       eventID,
		JobID:         "storm-job-" + eventID,
		TargetID:      targetID,
		MSPID:         tenantID,
		ClientID:      "storm-client",
		DeviceID:      "storm-device-" + agentID,
		AgentID:       agentID,
		CorrelationID: "storm-correlation-" + eventID,
		Attempt:       1,
		IssuedAt:      time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:     time.Now().Add(2 * time.Minute).UTC().Format(time.RFC3339),
		CommandType:   "storm_test",
		Payload:       json.RawMessage(`{"bounded":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// TestBoundedReconnectStormPreservesTenantAgentIsolation is the retained FD-05
// integration harness. It deliberately queues work while endpoint consumers are
// offline, repeatedly tears down and recreates stable durable consumers, and
// verifies that every logical command executes once on only its enrolled
// tenant/agent identity.
func TestBoundedReconnectStormPreservesTenantAgentIsolation(t *testing.T) {
	nc := integrationNATS(t)

	const (
		agentsPerTenant = 3
		rounds          = 4
	)
	tenants := []string{
		"storm-tenant-a",
		"storm-tenant-b",
	}

	type endpoint struct {
		tenantID   string
		agentID    string
		ledger     *ReceiptLedger
		registry   *HandlerRegistry
		dispatcher *JobDispatcher
		executions atomic.Int32
	}

	var endpoints []*endpoint
	for _, tenantID := range tenants {
		for i := 0; i < agentsPerTenant; i++ {
			ep := &endpoint{
				tenantID: tenantID,
				agentID:  fmt.Sprintf("agent-%d", i),
			}
			ep.ledger, _ = integrationLedger(t)
			ep.registry = NewHandlerRegistry()
			endpointRef := ep
			ep.registry.Register("storm_test", func(_ context.Context, cmd *CommandEnvelope) (string, int, []byte, error) {
				if cmd.MSPID != endpointRef.tenantID || cmd.AgentID != endpointRef.agentID {
					t.Fatalf("cross-scope execution: endpoint=%s/%s command=%s/%s", endpointRef.tenantID, endpointRef.agentID, cmd.MSPID, cmd.AgentID)
				}
				endpointRef.executions.Add(1)
				return StateSucceeded, 0, []byte(`{"storm":"ok"}`), nil
			})
			endpoints = append(endpoints, ep)
		}
	}

	startAll := func() {
		t.Helper()
		for _, ep := range endpoints {
			ep.dispatcher = NewJobDispatcher(nc, ep.ledger, ep.registry, zap.NewNop(), ep.tenantID, ep.agentID)
			if err := ep.dispatcher.Start(context.Background()); err != nil {
				t.Fatalf("start %s/%s: %v", ep.tenantID, ep.agentID, err)
			}
		}
	}
	stopAll := func() {
		t.Helper()
		for _, ep := range endpoints {
			if ep.dispatcher != nil {
				if err := ep.dispatcher.Stop(); err != nil {
					t.Fatalf("stop %s/%s: %v", ep.tenantID, ep.agentID, err)
				}
				ep.dispatcher = nil
			}
		}
	}
	defer stopAll()

	for round := 0; round < rounds; round++ {
		// All endpoint consumers are offline while the control plane publishes one
		// logical command per endpoint. JetStream must retain every command.
		stopAll()
		for _, ep := range endpoints {
			eventID := fmt.Sprintf("storm-r%d-%s-%s", round, ep.tenantID, ep.agentID)
			targetID := "target-" + eventID
			subject := fmt.Sprintf("tenant.%s.cmd.%s", ep.tenantID, ep.agentID)
			if err := nc.Publish(subject, stormCommand(t, ep.tenantID, ep.agentID, eventID, targetID)); err != nil {
				t.Fatalf("publish %s: %v", subject, err)
			}
		}
		if err := nc.Flush(); err != nil {
			t.Fatal(err)
		}

		startAll()
		wantEach := int32(round + 1)
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			allDone := true
			for _, ep := range endpoints {
				if ep.executions.Load() != wantEach {
					allDone = false
					break
				}
			}
			if allDone {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		for _, ep := range endpoints {
			if got := ep.executions.Load(); got != wantEach {
				t.Fatalf("round %d endpoint %s/%s executions=%d want=%d", round, ep.tenantID, ep.agentID, got, wantEach)
			}
		}

		// Republish the exact same logical commands while consumers are online.
		// Durable receipt identity must suppress every duplicate execution.
		for _, ep := range endpoints {
			eventID := fmt.Sprintf("storm-r%d-%s-%s", round, ep.tenantID, ep.agentID)
			targetID := "target-" + eventID
			subject := fmt.Sprintf("tenant.%s.cmd.%s", ep.tenantID, ep.agentID)
			if err := nc.Publish(subject, stormCommand(t, ep.tenantID, ep.agentID, eventID, targetID)); err != nil {
				t.Fatal(err)
			}
		}
		if err := nc.Flush(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(300 * time.Millisecond)
		for _, ep := range endpoints {
			if got := ep.executions.Load(); got != wantEach {
				t.Fatalf("duplicate execution after round %d for %s/%s: got=%d want=%d", round, ep.tenantID, ep.agentID, got, wantEach)
			}
		}
	}
}
