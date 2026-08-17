//go:build jobintegration

package jobs

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

func TestCommandPublishedWhileAgentOfflineExecutesAfterReconnect(t *testing.T) {
	nc := integrationNATS(t)
	ledger, _ := integrationLedger(t)
	registry := NewHandlerRegistry()
	var executions atomic.Int32
	registry.Register("test", func(context.Context, *CommandEnvelope) (string, int, []byte, error) {
		executions.Add(1)
		return StateSucceeded, 0, []byte(`{"value":"offline-replayed"}`), nil
	})

	results := make(chan []byte, 2)
	sub, err := nc.Subscribe("tenant.10000000-0000-0000-0000-000000000001.agent.agent-1.result", func(msg *nats.Msg) {
		results <- append([]byte(nil), msg.Data...)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	// Publish before the endpoint consumer exists. The STRATA_COMMANDS stream
	// must retain this command until the stable per-agent durable attaches.
	if err := nc.Publish("tenant.10000000-0000-0000-0000-000000000001.cmd.agent-1", testCommand(t, "event-offline-reconnect", "target-offline-reconnect")); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	dispatcher := NewJobDispatcher(nc, ledger, registry, zap.NewNop(), "10000000-0000-0000-0000-000000000001", "agent-1")
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dispatcher.Stop() }()

	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(waitMessage(t, results), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != StateSucceeded {
		t.Fatalf("expected succeeded result after reconnect, got %s", result.Status)
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("expected one execution after reconnect, got %d", got)
	}
}

func TestStaleRunningReceiptFailsClosedWithoutReexecution(t *testing.T) {
	nc := integrationNATS(t)
	ledger, _ := integrationLedger(t)
	registry := NewHandlerRegistry()
	var executions atomic.Int32
	registry.Register("test", func(context.Context, *CommandEnvelope) (string, int, []byte, error) {
		executions.Add(1)
		return StateSucceeded, 0, []byte(`{"value":"must-not-run"}`), nil
	})

	commandBytes := testCommand(t, "event-stale-running", "target-stale-running")
	var cmd CommandEnvelope
	if err := json.Unmarshal(commandBytes, &cmd); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordReceipt(&CommandReceipt{
		EventID: cmd.EventID, JobID: cmd.JobID, TargetID: cmd.TargetID,
		MSPID: cmd.MSPID, ClientID: cmd.ClientID, SiteID: cmd.SiteID,
		DeviceID: cmd.DeviceID, AgentID: cmd.AgentID, CorrelationID: cmd.CorrelationID,
		Attempt: cmd.Attempt, CommandType: cmd.CommandType,
		ReceivedAt: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), State: StateRunning,
	}); err != nil {
		t.Fatal(err)
	}

	results := make(chan []byte, 2)
	sub, err := nc.Subscribe("tenant.10000000-0000-0000-0000-000000000001.agent.agent-1.result", func(msg *nats.Msg) {
		results <- append([]byte(nil), msg.Data...)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	if err := nc.Publish("tenant.10000000-0000-0000-0000-000000000001.cmd.agent-1", commandBytes); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	dispatcher := NewJobDispatcher(nc, ledger, registry, zap.NewNop(), "10000000-0000-0000-0000-000000000001", "agent-1")
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dispatcher.Stop() }()

	var result struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
	}
	if err := json.Unmarshal(waitMessage(t, results), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != StateFailed {
		t.Fatalf("expected fail-closed result, got %s", result.Status)
	}
	if executions.Load() != 0 {
		t.Fatalf("ambiguous pre-crash execution was run again")
	}
	receipt, err := ledger.GetReceipt(cmd.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != StateFailed || len(receipt.ResultEnvelope) == 0 {
		t.Fatalf("expected durable terminal failed receipt, got state=%s envelope=%d", receipt.State, len(receipt.ResultEnvelope))
	}
}
