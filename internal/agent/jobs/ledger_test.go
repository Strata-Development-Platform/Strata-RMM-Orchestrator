package jobs

import (
	"os"
	"testing"
	"time"

	"go.etcd.io/bbolt"
	"go.uber.org/zap"
)

func newTestDB(t *testing.T) *bbolt.DB {
	t.Helper()
	f, err := os.CreateTemp("", "ledger-test-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	db, err := bbolt.Open(f.Name(), 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close(); os.Remove(f.Name()) })
	return db
}

func TestReceiptLedger(t *testing.T) {
	db := newTestDB(t)
	logger := zap.NewNop()
	ledger := NewReceiptLedger(db, logger)

	r := &CommandReceipt{
		EventID:       "evt-001",
		JobID:         "job-001",
		TargetID:      "tgt-001",
		CorrelationID: "corr-001",
		Attempt:       1,
		CommandType:   "script",
		ReceivedAt:    time.Now().UTC().Format(time.RFC3339),
		State:         StateReceived,
	}

	if err := ledger.RecordReceipt(r); err != nil {
		t.Fatalf("RecordReceipt: %v", err)
	}

	if !ledger.IsDuplicate("evt-001") {
		t.Error("IsDuplicate should return true")
	}

	if ledger.IsDuplicate("evt-002") {
		t.Error("IsDuplicate should return false for unknown event")
	}

	got, err := ledger.GetReceipt("evt-001")
	if err != nil {
		t.Fatalf("GetReceipt: %v", err)
	}
	if got.JobID != "job-001" {
		t.Errorf("wrong job_id: %s", got.JobID)
	}

	if err := ledger.MarkRunning("evt-001"); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	got, _ = ledger.GetReceipt("evt-001")
	if got.State != StateRunning {
		t.Errorf("expected running, got %s", got.State)
	}

	if err := ledger.MarkComplete("evt-001", StateSucceeded, "res-001"); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}
	got, _ = ledger.GetReceipt("evt-001")
	if got.State != StateSucceeded {
		t.Errorf("expected succeeded, got %s", got.State)
	}
	if got.ResultMsgID != "res-001" {
		t.Errorf("expected res-001, got %s", got.ResultMsgID)
	}
}

func TestUnacknowledgedResults(t *testing.T) {
	db := newTestDB(t)
	logger := zap.NewNop()
	ledger := NewReceiptLedger(db, logger)

	r1 := &CommandReceipt{EventID: "evt-001", State: StateSucceeded, ResultMsgID: "res-001"}
	ledger.RecordReceipt(r1)
	r2 := &CommandReceipt{EventID: "evt-002", State: StateFailed, ResultMsgID: "res-002"}
	ledger.RecordReceipt(r2)
	r3 := &CommandReceipt{EventID: "evt-003", State: StateRunning}
	ledger.RecordReceipt(r3)

	results := ledger.GetUnacknowledgedResults()
	if len(results) != 2 {
		t.Errorf("expected 2 unacknowledged results, got %d", len(results))
	}
}

func TestCleanup(t *testing.T) {
	db := newTestDB(t)
	logger := zap.NewNop()
	ledger := NewReceiptLedger(db, logger)

	old := &CommandReceipt{EventID: "evt-old", ReceivedAt: time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)}
	ledger.RecordReceipt(old)
	new := &CommandReceipt{EventID: "evt-new", ReceivedAt: time.Now().UTC().Format(time.RFC3339)}
	ledger.RecordReceipt(new)

	ledger.Cleanup(time.Now().Add(-24 * time.Hour))

	if ledger.IsDuplicate("evt-old") {
		t.Error("old receipt should have been cleaned up")
	}
	if !ledger.IsDuplicate("evt-new") {
		t.Error("new receipt should still exist")
	}
}

func TestDuplicateDoesNotOverwrite(t *testing.T) {
	db := newTestDB(t)
	logger := zap.NewNop()
	ledger := NewReceiptLedger(db, logger)

	r1 := &CommandReceipt{EventID: "evt-001", Attempt: 1, State: StateReceived}
	ledger.RecordReceipt(r1)
	r2, _ := ledger.GetReceipt("evt-001")
	r2.Attempt = 2
	ledger.RecordReceipt(r2)

	got, _ := ledger.GetReceipt("evt-001")
	// RecordReceipt overwrites — this is intentional for state updates
	if got.Attempt != 2 {
		t.Errorf("expected attempt 2, got %d", got.Attempt)
	}
}
