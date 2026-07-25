package update

import (
	"os"
	"testing"

	"go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) *bbolt.DB {
	t.Helper()
	f, err := os.CreateTemp("", "update-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	db, err := bbolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(f.Name())
	})
	return db
}

func TestStoreInit(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

func TestStoreGetSetState(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	s.Init()

	state, err := s.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.Status != StatusUpToDate {
		t.Errorf("initial status: got %s, want up_to_date", state.Status)
	}

	state.CurrentVersion = "1.0.0"
	state.Status = StatusUpToDate
	if err := s.SetState(state); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	got, err := s.GetState()
	if err != nil {
		t.Fatalf("GetState after set: %v", err)
	}
	if got.CurrentVersion != "1.0.0" {
		t.Errorf("version: got %s, want 1.0.0", got.CurrentVersion)
	}
}

func TestStoreSetCurrentVersion(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	s.Init()

	if err := s.SetCurrentVersion("2.0.0"); err != nil {
		t.Fatalf("SetCurrentVersion: %v", err)
	}

	state, _ := s.GetState()
	if state.CurrentVersion != "2.0.0" {
		t.Errorf("version: got %s, want 2.0.0", state.CurrentVersion)
	}
	if state.Status != StatusUpToDate {
		t.Errorf("status: got %s, want up_to_date", state.Status)
	}
}

func TestStoreRecordFailure(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	s.Init()

	for i := 0; i < 2; i++ {
		if err := s.RecordFailure(); err != nil {
			t.Fatalf("RecordFailure %d: %v", i, err)
		}
	}

	state, _ := s.GetState()
	if state.FailedAttempts != 2 {
		t.Errorf("failed attempts: got %d, want 2", state.FailedAttempts)
	}
	if state.Status == StatusFailed {
		t.Error("should not be failed yet")
	}

	if err := s.RecordFailure(); err != nil {
		t.Fatalf("RecordFailure 3: %v", err)
	}

	state, _ = s.GetState()
	if state.Status != StatusFailed {
		t.Errorf("status: got %s, want failed", state.Status)
	}
}
