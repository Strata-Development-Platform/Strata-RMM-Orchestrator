package update

import (
	"os"
	"strings"
	"testing"
	"time"
)

func retainedJournalFor(t *testing.T, executor DockerUpgradeExecutor, candidate OCIReleaseCandidate, previous string, state DockerUpgradeState) DockerUpgradeJournal {
	t.Helper()
	now := time.Now().UTC()
	return DockerUpgradeJournal{
		Schema: 1, TransactionID: "retained", State: state,
		CurrentVersion: "1.9.9", CandidateVersion: candidate.Version,
		CurrentSourceSHA: strings.Repeat("1", 40), CandidateSourceSHA: candidate.SourceSHA,
		CurrentImage: previous, CandidateImage: candidate.Image,
		ManifestSHA256: strings.Repeat("c", 64), SourceSchemaVersion: 95, TargetSchemaVersion: 96,
		ComposeProject: executor.Project, ComposeFile: executor.ComposeFile,
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestDockerUpgradeReconcileFinalizesHealthyCandidate(t *testing.T) {
	executor, candidate, _, previous, envFile := dockerExecutorFixture(t)
	if err := replaceEnvImage(envFile, candidate.Image); err != nil { t.Fatal(err) }
	t.Setenv("FAKE_DOCKER_LIVE_IMAGE", candidate.Image)
	journal := retainedJournalFor(t, executor, candidate, previous, DockerUpgradeVerifying)
	if err := WriteDockerUpgradeJournal(executor.JournalFile, journal); err != nil { t.Fatal(err) }
	if err := executor.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if _, err := os.Stat(executor.JournalFile); !os.IsNotExist(err) {
		t.Fatalf("healthy candidate did not finalize transaction: %v", err)
	}
}

func TestDockerUpgradeReconcileRestoresEnvWhenLivePrevious(t *testing.T) {
	executor, candidate, _, previous, envFile := dockerExecutorFixture(t)
	if err := replaceEnvImage(envFile, candidate.Image); err != nil { t.Fatal(err) }
	t.Setenv("FAKE_DOCKER_LIVE_IMAGE", previous)
	journal := retainedJournalFor(t, executor, candidate, previous, DockerUpgradeApplying)
	if err := WriteDockerUpgradeJournal(executor.JournalFile, journal); err != nil { t.Fatal(err) }
	if err := executor.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	payload, err := os.ReadFile(envFile)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(payload), "STRATA_ORCHESTRATOR_IMAGE="+previous) {
		t.Fatalf("protected env did not converge to previous image: %s", payload)
	}
	got, err := ReadDockerUpgradeJournal(executor.JournalFile)
	if err != nil { t.Fatal(err) }
	if got.State != DockerUpgradeRolledBack {
		t.Fatalf("journal state = %q, want rolled_back", got.State)
	}
}

func TestDockerUpgradeReconcileCompletesInterruptedRollback(t *testing.T) {
	executor, candidate, _, previous, _ := dockerExecutorFixture(t)
	t.Setenv("FAKE_DOCKER_LIVE_IMAGE", candidate.Image)
	journal := retainedJournalFor(t, executor, candidate, previous, DockerUpgradeRollback)
	if err := WriteDockerUpgradeJournal(executor.JournalFile, journal); err != nil { t.Fatal(err) }
	if err := executor.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	got, err := ReadDockerUpgradeJournal(executor.JournalFile)
	if err != nil { t.Fatal(err) }
	if got.State != DockerUpgradeRolledBack {
		t.Fatalf("journal state = %q, want rolled_back", got.State)
	}
}

func TestDockerUpgradeReconcileFailsClosedOnUnprovenMismatch(t *testing.T) {
	executor, candidate, _, previous, envFile := dockerExecutorFixture(t)
	// Protected state says previous while live state says candidate, but the
	// retained lifecycle has no recorded rollback intent.
	if err := replaceEnvImage(envFile, previous); err != nil { t.Fatal(err) }
	t.Setenv("FAKE_DOCKER_LIVE_IMAGE", candidate.Image)
	journal := retainedJournalFor(t, executor, candidate, previous, DockerUpgradeApplying)
	if err := WriteDockerUpgradeJournal(executor.JournalFile, journal); err != nil { t.Fatal(err) }
	if err := executor.Reconcile(t.Context()); err == nil {
		t.Fatal("Reconcile() accepted ambiguous live/configured mismatch")
	}
	got, err := ReadDockerUpgradeJournal(executor.JournalFile)
	if err != nil { t.Fatal(err) }
	if got.State != DockerUpgradeRecoveryNeeded {
		t.Fatalf("journal state = %q, want recovery_required", got.State)
	}
}
