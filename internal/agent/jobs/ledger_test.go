package jobs

import (
	"os"
	"testing"
	"time"

	"go.etcd.io/bbolt"
	"go.uber.org/zap"
)

func newTestLedger(t *testing.T) *ReceiptLedger {
	t.Helper()
	file, err := os.CreateTemp("", "ledger-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := bbolt.Open(name, 0600, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove db: %v", err)
		}
	})
	ledger, err := NewReceiptLedger(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return ledger
}

func TestReceiptLedgerPersistsExactResultUntilAcknowledged(t *testing.T) {
	ledger := newTestLedger(t)
	receipt := &CommandReceipt{
		EventID: "evt-1", JobID: "job-1", TargetID: "target-1", DeviceID: "device-1",
		ReceivedAt: time.Now().UTC().Format(time.RFC3339), State: StateReceived,
	}
	if err := ledger.RecordReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	if err := ledger.MarkRunning(receipt.EventID); err != nil {
		t.Fatal(err)
	}
	envelope := []byte(`{"message_id":"result-1","device_id":"device-1"}`)
	if err := ledger.MarkComplete(receipt.EventID, StateSucceeded, "result-1", envelope); err != nil {
		t.Fatal(err)
	}
	pending, err := ledger.GetUnacknowledgedResults()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || string(pending[0].ResultEnvelope) != string(envelope) {
		t.Fatalf("unexpected pending results: %#v", pending)
	}
	if err := ledger.MarkResultAcknowledged("result-1"); err != nil {
		t.Fatal(err)
	}
	pending, err = ledger.GetUnacknowledgedResults()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("acknowledged result was replayable: %#v", pending)
	}
}

func TestReceiptLedgerRejectsDuplicateWithoutOverwrite(t *testing.T) {
	ledger := newTestLedger(t)
	original := &CommandReceipt{EventID: "evt-1", Attempt: 1, ReceivedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := ledger.RecordReceipt(original); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordReceipt(&CommandReceipt{EventID: "evt-1", Attempt: 2}); err == nil {
		t.Fatal("expected duplicate receipt error")
	}
	got, err := ledger.GetReceipt("evt-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt != 1 {
		t.Fatalf("duplicate overwrote receipt: attempt=%d", got.Attempt)
	}
}

func TestCleanupOnlyDeletesAcknowledgedResults(t *testing.T) {
	ledger := newTestLedger(t)
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	for _, receipt := range []*CommandReceipt{
		{EventID: "pending", ReceivedAt: old, State: StateSucceeded, ResultMsgID: "r1", ResultEnvelope: []byte(`{}`)},
		{EventID: "acked", ReceivedAt: old, State: StateSucceeded, ResultMsgID: "r2", ResultEnvelope: []byte(`{}`), ResultAcked: true},
	} {
		if err := ledger.RecordReceipt(receipt); err != nil {
			t.Fatal(err)
		}
	}
	if err := ledger.Cleanup(time.Now().Add(-24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !ledger.IsDuplicate("pending") {
		t.Fatal("unacknowledged result was deleted")
	}
	if ledger.IsDuplicate("acked") {
		t.Fatal("acknowledged result was not deleted")
	}
}
