package software

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"go.etcd.io/bbolt"
)

func openSoftwareLedgerTestDB(t *testing.T) (*bbolt.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.db")
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open bbolt: %v", err)
	}
	return db, path
}

func TestSoftwareReceiptLedgerConcurrentDuplicateExecutesOnce(t *testing.T) {
	db, _ := openSoftwareLedgerTestDB(t)
	defer db.Close()
	ledger, err := newSoftwareReceiptLedger(db)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 32
	var executes atomic.Int32
	var inFlight atomic.Int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			disposition, _, beginErr := ledger.begin("dep-1\x00install", "fingerprint-1")
			if beginErr != nil {
				t.Errorf("begin: %v", beginErr)
				return
			}
			switch disposition {
			case softwareBeginExecute:
				executes.Add(1)
			case softwareBeginInFlight:
				inFlight.Add(1)
			default:
				t.Errorf("unexpected disposition: %v", disposition)
			}
		}()
	}
	wg.Wait()
	if executes.Load() != 1 {
		t.Fatalf("execute claims = %d, want 1", executes.Load())
	}
	if inFlight.Load() != workers-1 {
		t.Fatalf("in-flight duplicates = %d, want %d", inFlight.Load(), workers-1)
	}
}

func TestSoftwareReceiptLedgerTerminalResultSurvivesRestart(t *testing.T) {
	db, path := openSoftwareLedgerTestDB(t)
	ledger, err := newSoftwareReceiptLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	key, fingerprint := "dep-2\x00install", "fingerprint-2"
	if disposition, _, err := ledger.begin(key, fingerprint); err != nil || disposition != softwareBeginExecute {
		t.Fatalf("first begin = %v, %v", disposition, err)
	}
	want := SoftwareResult{Type: "software_result", DeploymentID: "dep-2", Action: "install", Status: "success", DurationMs: 17}
	if err := ledger.complete(key, fingerprint, want); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ledger, err = newSoftwareReceiptLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.resumeInterrupted(); err != nil {
		t.Fatal(err)
	}
	disposition, got, err := ledger.begin(key, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if disposition != softwareBeginReplay {
		t.Fatalf("disposition = %v, want replay", disposition)
	}
	if got != want {
		t.Fatalf("replayed result = %+v, want %+v", got, want)
	}
}

func TestSoftwareReceiptLedgerResumesOnlyInterruptedExecution(t *testing.T) {
	db, _ := openSoftwareLedgerTestDB(t)
	defer db.Close()
	ledger, err := newSoftwareReceiptLedger(db)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := ledger.begin("running\x00install", "fp-running"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ledger.begin("terminal\x00install", "fp-terminal"); err != nil {
		t.Fatal(err)
	}
	terminal := SoftwareResult{Type: "software_result", DeploymentID: "terminal", Action: "install", Status: "failed", ErrorMessage: "bounded failure"}
	if err := ledger.complete("terminal\x00install", "fp-terminal", terminal); err != nil {
		t.Fatal(err)
	}
	if err := ledger.resumeInterrupted(); err != nil {
		t.Fatal(err)
	}

	disposition, _, err := ledger.begin("running\x00install", "fp-running")
	if err != nil || disposition != softwareBeginExecute {
		t.Fatalf("interrupted begin = %v, %v; want execute", disposition, err)
	}
	disposition, got, err := ledger.begin("terminal\x00install", "fp-terminal")
	if err != nil || disposition != softwareBeginReplay || got != terminal {
		t.Fatalf("terminal begin = %v, %+v, %v; want replay", disposition, got, err)
	}
}

func TestSoftwareReceiptLedgerRejectsPayloadSubstitution(t *testing.T) {
	db, _ := openSoftwareLedgerTestDB(t)
	defer db.Close()
	ledger, err := newSoftwareReceiptLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	key := "dep-3\x00uninstall"
	if _, _, err := ledger.begin(key, "fingerprint-a"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ledger.begin(key, "fingerprint-b"); !errors.Is(err, errSoftwareCommandConflict) {
		t.Fatalf("conflicting payload error = %v", err)
	}
}
