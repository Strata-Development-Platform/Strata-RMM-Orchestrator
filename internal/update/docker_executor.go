package update

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var errComposeImageMissing = errors.New("compose environment does not define STRATA_ORCHESTRATOR_IMAGE")

type DockerUpgradeExecutor struct {
	ComposeFile   string
	EnvFile       string
	Project       string
	JournalFile   string
	HealthTimeout time.Duration
}

func (e DockerUpgradeExecutor) composeArgs(extra ...string) []string {
	args := []string{"compose", "--project-name", e.Project, "--env-file", e.EnvFile, "-f", e.ComposeFile}
	return append(args, extra...)
}

func runDocker(ctx context.Context, args ...string) ([]byte, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("docker is required: %w", err)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("docker %s failed: %w: %s", strings.Join(args, " "), err, string(output))
	}
	return output, nil
}

func (e DockerUpgradeExecutor) currentConfiguredImage() (string, error) {
	file, err := os.Open(e.EnvFile)
	if err != nil {
		return "", fmt.Errorf("open compose environment: %w", err)
	}
	defer func() { _ = file.Close() }()
	const prefix = "STRATA_ORCHESTRATOR_IMAGE="
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			image := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if !isImmutableOCIReference(image) {
				return "", fmt.Errorf("configured orchestrator image is not immutable")
			}
			return image, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read compose environment: %w", err)
	}
	return "", errComposeImageMissing
}

func (e DockerUpgradeExecutor) liveImage(ctx context.Context) (string, error) {
	idBytes, err := runDocker(ctx, e.composeArgs("ps", "-q", "orchestrator")...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(idBytes))
	if id == "" {
		return "", fmt.Errorf("orchestrator compose container is not running")
	}
	imageBytes, err := runDocker(ctx, "inspect", "--format", "{{.Config.Image}}", id)
	if err != nil {
		return "", err
	}
	image := strings.TrimSpace(string(imageBytes))
	if !isImmutableOCIReference(image) {
		return "", fmt.Errorf("live orchestrator image is not an immutable digest reference")
	}
	return image, nil
}

func replaceEnvImage(path, image string) error {
	if !isImmutableOCIReference(image) {
		return fmt.Errorf("replacement orchestrator image is not immutable")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("compose environment must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("compose environment permissions are too broad")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const prefix = "STRATA_ORCHESTRATOR_IMAGE="
	lines := strings.Split(strings.TrimSuffix(string(payload), "\n"), "\n")
	found := false
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[index] = prefix + image
			found = true
		}
	}
	if !found {
		lines = append(lines, prefix+image)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".install-env-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

func (e DockerUpgradeExecutor) healthy(ctx context.Context) error {
	timeout := e.HealthTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := runDocker(ctx, e.composeArgs("exec", "-T", "orchestrator", "wget", "-q", "-O", "-", "http://127.0.0.1:8080/health/ready")...); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("orchestrator readiness did not pass before timeout")
}

func (e DockerUpgradeExecutor) validate() error {
	if e.Project == "" || !filepath.IsAbs(e.ComposeFile) || !filepath.IsAbs(e.EnvFile) || !filepath.IsAbs(e.JournalFile) {
		return fmt.Errorf("docker upgrade executor requires project and absolute compose/env/journal paths")
	}
	return nil
}

func (e DockerUpgradeExecutor) finishResolvedRollback(journal *DockerUpgradeJournal) error {
	journal.State = DockerUpgradeRolledBack
	journal.UpdatedAt = time.Now().UTC()
	if err := WriteDockerUpgradeJournal(e.JournalFile, *journal); err != nil {
		return err
	}
	if err := os.Remove(e.JournalFile); err != nil {
		return fmt.Errorf("remove resolved docker upgrade journal: %w", err)
	}
	return nil
}

// Apply performs the mutation after the caller has completed the shared runtime
// preflight and OCI Sigstore verification. The previous immutable digest is
// retained until candidate health is durably confirmed.
func (e DockerUpgradeExecutor) Apply(ctx context.Context, candidate OCIReleaseCandidate, preflight PreflightResult, currentVersion, currentSourceSHA, manifestSHA256 string) error {
	if err := e.validate(); err != nil {
		return err
	}
	if !preflight.Pass {
		return fmt.Errorf("shared runtime preflight must pass before docker apply")
	}
	if _, err := os.Lstat(e.JournalFile); err == nil {
		return fmt.Errorf("unresolved docker upgrade journal blocks competing apply")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect docker upgrade journal: %w", err)
	}

	configured, configuredErr := e.currentConfiguredImage()
	if configuredErr != nil && !errors.Is(configuredErr, errComposeImageMissing) {
		return configuredErr
	}
	live, err := e.liveImage(ctx)
	if err != nil {
		return err
	}
	if configuredErr == nil && configured != live {
		return fmt.Errorf("live orchestrator image does not match protected compose state")
	}
	if configuredErr != nil {
		// Installations created before the upgrade transaction persisted the
		// immutable image only in the running Compose container. Adopt that exact
		// live digest into the protected env before any candidate mutation.
		configured = live
		if err := replaceEnvImage(e.EnvFile, configured); err != nil {
			return fmt.Errorf("adopt live immutable image into protected compose state: %w", err)
		}
	}
	if candidate.Image == configured {
		return fmt.Errorf("candidate image is already deployed")
	}

	now := time.Now().UTC()
	journal := DockerUpgradeJournal{
		Schema: 1, TransactionID: fmt.Sprintf("docker-%d", now.UnixNano()), State: DockerUpgradePrepared,
		CurrentVersion: currentVersion, CandidateVersion: candidate.Version,
		CurrentSourceSHA: currentSourceSHA, CandidateSourceSHA: candidate.SourceSHA,
		CurrentImage: configured, CandidateImage: candidate.Image, ManifestSHA256: manifestSHA256,
		SourceSchemaVersion: preflight.SourceSchemaVersion, TargetSchemaVersion: preflight.TargetSchemaVersion,
		ComposeProject: e.Project, ComposeFile: e.ComposeFile, CreatedAt: now, UpdatedAt: now,
	}
	if err := WriteDockerUpgradeJournal(e.JournalFile, journal); err != nil {
		return err
	}
	setState := func(state DockerUpgradeState) error {
		journal.State = state
		journal.UpdatedAt = time.Now().UTC()
		return WriteDockerUpgradeJournal(e.JournalFile, journal)
	}
	if _, err := runDocker(ctx, "pull", candidate.Image); err != nil {
		if finishErr := e.finishResolvedRollback(&journal); finishErr != nil {
			return fmt.Errorf("pull candidate failed and transaction cleanup failed: %v; %w", finishErr, err)
		}
		return err
	}
	if err := setState(DockerUpgradePulled); err != nil {
		return err
	}
	if err := replaceEnvImage(e.EnvFile, candidate.Image); err != nil {
		return err
	}
	if _, err := runDocker(ctx, e.composeArgs("config", "--quiet")...); err != nil {
		if restoreErr := replaceEnvImage(e.EnvFile, configured); restoreErr != nil {
			journal.State = DockerUpgradeRecoveryNeeded
			journal.UpdatedAt = time.Now().UTC()
			_ = WriteDockerUpgradeJournal(e.JournalFile, journal)
			return fmt.Errorf("compose validation failed and previous image state could not be restored: %v; %w", restoreErr, err)
		}
		if finishErr := e.finishResolvedRollback(&journal); finishErr != nil {
			return fmt.Errorf("compose validation failed and transaction cleanup failed: %v; %w", finishErr, err)
		}
		return err
	}
	if err := setState(DockerUpgradeApplying); err != nil {
		return err
	}
	if _, err := runDocker(ctx, e.composeArgs("up", "-d", "--no-deps", "orchestrator")...); err == nil {
		if err := setState(DockerUpgradeVerifying); err != nil {
			return err
		}
		if err := e.healthy(ctx); err == nil {
			if err := setState(DockerUpgradeComplete); err != nil {
				return err
			}
			return os.Remove(e.JournalFile)
		}
	}
	if err := setState(DockerUpgradeRollback); err != nil {
		return err
	}
	if err := replaceEnvImage(e.EnvFile, configured); err != nil {
		journal.State = DockerUpgradeRecoveryNeeded
		journal.UpdatedAt = time.Now().UTC()
		_ = WriteDockerUpgradeJournal(e.JournalFile, journal)
		return fmt.Errorf("candidate failed and previous image could not be restored in compose state: %w", err)
	}
	if _, err := runDocker(ctx, e.composeArgs("up", "-d", "--no-deps", "orchestrator")...); err != nil {
		journal.State = DockerUpgradeRecoveryNeeded
		journal.UpdatedAt = time.Now().UTC()
		_ = WriteDockerUpgradeJournal(e.JournalFile, journal)
		return fmt.Errorf("candidate failed and rollback start failed: %w", err)
	}
	if err := e.healthy(ctx); err != nil {
		journal.State = DockerUpgradeRecoveryNeeded
		journal.UpdatedAt = time.Now().UTC()
		_ = WriteDockerUpgradeJournal(e.JournalFile, journal)
		return fmt.Errorf("candidate failed and previous image health could not be verified: %w", err)
	}
	if err := e.finishResolvedRollback(&journal); err != nil {
		return err
	}
	return fmt.Errorf("candidate failed; previous immutable image was restored")
}
