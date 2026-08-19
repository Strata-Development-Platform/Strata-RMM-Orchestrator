package update

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Reconcile resolves a retained Docker upgrade journal without blindly
// reapplying the candidate. It compares protected Compose state with the live
// container and converges only from states whose intent can be proven.
func (e DockerUpgradeExecutor) Reconcile(ctx context.Context) error {
	if err := e.validate(); err != nil {
		return err
	}
	journal, err := ReadDockerUpgradeJournal(e.JournalFile)
	if err != nil {
		return err
	}
	if journal.ComposeProject != e.Project || journal.ComposeFile != e.ComposeFile {
		return fmt.Errorf("docker upgrade journal does not match this compose deployment")
	}
	journal.Attempt++
	journal.UpdatedAt = time.Now().UTC()
	if err := journal.Validate(); err != nil {
		return err
	}
	if err := WriteDockerUpgradeJournal(e.JournalFile, journal); err != nil {
		return err
	}

	configured, err := e.currentConfiguredImage()
	if err != nil {
		return err
	}
	live, err := e.liveImage(ctx)
	if err != nil {
		return err
	}

	set := func(state DockerUpgradeState) error {
		journal.State = state
		journal.UpdatedAt = time.Now().UTC()
		return WriteDockerUpgradeJournal(e.JournalFile, journal)
	}
	finishCandidate := func() error {
		if err := e.healthy(ctx); err != nil {
			return rollbackRetainedDockerUpgrade(ctx, e, &journal)
		}
		if err := set(DockerUpgradeComplete); err != nil {
			return err
		}
		return os.Remove(e.JournalFile)
	}

	switch {
	case configured == journal.CandidateImage && live == journal.CandidateImage:
		return finishCandidate()
	case configured == journal.CurrentImage && live == journal.CurrentImage:
		if err := e.healthy(ctx); err != nil {
			_ = set(DockerUpgradeRecoveryNeeded)
			return fmt.Errorf("previous docker release is live but not healthy: %w", err)
		}
		return set(DockerUpgradeRolledBack)
	case configured == journal.CandidateImage && live == journal.CurrentImage:
		// Crash after protected env mutation but before candidate start. The live
		// release is still the known-good previous digest, so restore env state.
		if err := replaceEnvImage(e.EnvFile, journal.CurrentImage); err != nil {
			_ = set(DockerUpgradeRecoveryNeeded)
			return err
		}
		if err := e.healthy(ctx); err != nil {
			_ = set(DockerUpgradeRecoveryNeeded)
			return err
		}
		return set(DockerUpgradeRolledBack)
	case configured == journal.CurrentImage && live == journal.CandidateImage:
		// Crash during rollback after env restoration but before Compose replaced
		// the candidate. Complete rollback to the already-persisted prior digest.
		if journal.State != DockerUpgradeRollback && journal.State != DockerUpgradeRecoveryNeeded {
			_ = set(DockerUpgradeRecoveryNeeded)
			return fmt.Errorf("live candidate/protected previous mismatch has no proven rollback intent")
		}
		return rollbackRetainedDockerUpgrade(ctx, e, &journal)
	default:
		_ = set(DockerUpgradeRecoveryNeeded)
		return fmt.Errorf("docker upgrade live/configured images do not match retained transaction")
	}
}

func rollbackRetainedDockerUpgrade(ctx context.Context, e DockerUpgradeExecutor, journal *DockerUpgradeJournal) error {
	journal.State = DockerUpgradeRollback
	journal.UpdatedAt = time.Now().UTC()
	if err := WriteDockerUpgradeJournal(e.JournalFile, *journal); err != nil {
		return err
	}
	if err := replaceEnvImage(e.EnvFile, journal.CurrentImage); err != nil {
		journal.State = DockerUpgradeRecoveryNeeded
		journal.UpdatedAt = time.Now().UTC()
		_ = WriteDockerUpgradeJournal(e.JournalFile, *journal)
		return fmt.Errorf("restore previous compose image: %w", err)
	}
	if _, err := runDocker(ctx, e.composeArgs("up", "-d", "--no-deps", "orchestrator")...); err != nil {
		journal.State = DockerUpgradeRecoveryNeeded
		journal.UpdatedAt = time.Now().UTC()
		_ = WriteDockerUpgradeJournal(e.JournalFile, *journal)
		return fmt.Errorf("restart previous immutable docker release: %w", err)
	}
	if err := e.healthy(ctx); err != nil {
		journal.State = DockerUpgradeRecoveryNeeded
		journal.UpdatedAt = time.Now().UTC()
		_ = WriteDockerUpgradeJournal(e.JournalFile, *journal)
		return fmt.Errorf("previous immutable docker release did not recover: %w", err)
	}
	journal.State = DockerUpgradeRolledBack
	journal.UpdatedAt = time.Now().UTC()
	return WriteDockerUpgradeJournal(e.JournalFile, *journal)
}
