//go:build jobintegration

package jobs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"
)

func integrationNATS(t *testing.T) *nats.Conn {
	t.Helper()
	url := os.Getenv("TEST_NATS_URL")
	if url == "" {
		t.Fatal("TEST_NATS_URL is required")
	}
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func integrationLedger(t *testing.T) (*ReceiptLedger, *bbolt.DB) {
	t.Helper()
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "receipts.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close ledger: %v", err)
		}
	})
	ledger, err := NewReceiptLedger(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return ledger, db
}

func testCommand(t *testing.T, eventID, targetID string) []byte {
	t.Helper()
	payload, err := json.Marshal(CommandEnvelope{
		SchemaVersion: 1, EventID: eventID, JobID: "10000000-0000-0000-0000-000000000010",
		TargetID: targetID, MSPID: "10000000-0000-0000-0000-000000000001",
		ClientID: "10000000-0000-0000-0000-000000000002",
		DeviceID: "device-1", AgentID: "agent-1", CorrelationID: "correlation-1",
		Attempt: 1, IssuedAt: time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:   time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
		CommandType: "test", Payload: json.RawMessage(`{"value":"ok"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func waitMessage(t *testing.T, messages <-chan []byte) []byte {
	t.Helper()
	select {
	case message := <-messages:
		return message
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for NATS message")
		return nil
	}
}

func TestResultReplaysAfterAgentRestartUntilReceipt(t *testing.T) {
	nc := integrationNATS(t)
	ledger, _ := integrationLedger(t)
	registry := NewHandlerRegistry()
	registry.Register("test", func(context.Context, *CommandEnvelope) (string, int, []byte, error) {
		return StateSucceeded, 0, []byte(`{"value":"ok"}`), nil
	})

	results := make(chan []byte, 4)
	sub, err := nc.Subscribe("tenant.10000000-0000-0000-0000-000000000001.agent.agent-1.result", func(msg *nats.Msg) {
		results <- append([]byte(nil), msg.Data...)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	first := NewJobDispatcher(nc, ledger, registry, zap.NewNop(), "10000000-0000-0000-0000-000000000001", "agent-1")
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("tenant.10000000-0000-0000-0000-000000000001.cmd.agent-1", testCommand(t, "event-restart", "target-restart")); err != nil {
		t.Fatal(err)
	}
	original := waitMessage(t, results)
	if err := first.Stop(); err != nil {
		t.Fatal(err)
	}

	second := NewJobDispatcher(nc, ledger, registry, zap.NewNop(), "10000000-0000-0000-0000-000000000001", "agent-1")
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Stop() }()
	replayed := waitMessage(t, results)
	if string(original) != string(replayed) {
		t.Fatalf("replayed result changed\noriginal: %s\nreplayed: %s", original, replayed)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(replayed, &result); err != nil {
		t.Fatal(err)
	}
	receipt, err := json.Marshal(map[string]interface{}{"message_id": result["message_id"]})
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("tenant.10000000-0000-0000-0000-000000000001.agent.agent-1.result.ack", receipt); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pending, err := ledger.GetUnacknowledgedResults()
		if err == nil && len(pending) == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("result receipt was not persisted")
}

func TestRunningCommandCancellationProducesTerminalResult(t *testing.T) {
	nc := integrationNATS(t)
	ledger, _ := integrationLedger(t)
	registry := NewHandlerRegistry()
	registry.Register("test", func(ctx context.Context, _ *CommandEnvelope) (string, int, []byte, error) {
		<-ctx.Done()
		return StateFailed, -1, nil, ctx.Err()
	})
	dispatcher := NewJobDispatcher(nc, ledger, registry, zap.NewNop(), "10000000-0000-0000-0000-000000000001", "agent-1")
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dispatcher.Stop() }()

	results := make(chan []byte, 1)
	sub, err := nc.Subscribe("tenant.10000000-0000-0000-0000-000000000001.agent.agent-1.result", func(msg *nats.Msg) {
		results <- append([]byte(nil), msg.Data...)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	acks := make(chan []byte, 1)
	ackSub, err := nc.Subscribe("tenant.10000000-0000-0000-0000-000000000001.agent.agent-1.ack", func(msg *nats.Msg) {
		acks <- append([]byte(nil), msg.Data...)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ackSub.Unsubscribe() }()
	if err := nc.Publish("tenant.10000000-0000-0000-0000-000000000001.cmd.agent-1", testCommand(t, "event-cancel", "target-cancel")); err != nil {
		t.Fatal(err)
	}
	waitMessage(t, acks)
	cancel, err := json.Marshal(map[string]string{"target_id": "target-cancel"})
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("tenant.10000000-0000-0000-0000-000000000001.cmd.agent-1.cancel", cancel); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(waitMessage(t, results), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != StateCancelled {
		t.Fatalf("expected cancelled result, got %s", result.Status)
	}
}
