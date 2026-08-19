package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DockerUpgradeState string

const (
	DockerUpgradePrepared       DockerUpgradeState = "prepared"
	DockerUpgradePulled         DockerUpgradeState = "pulled"
	DockerUpgradeApplying       DockerUpgradeState = "applying"
	DockerUpgradeVerifying      DockerUpgradeState = "verifying"
	DockerUpgradeRollback       DockerUpgradeState = "rollback"
	DockerUpgradeRolledBack     DockerUpgradeState = "rolled_back"
	DockerUpgradeComplete       DockerUpgradeState = "complete"
	DockerUpgradeRecoveryNeeded DockerUpgradeState = "recovery_required"
)

type DockerUpgradeJournal struct {
	Schema              int                `json:"schema"`
	TransactionID       string             `json:"transaction_id"`
	State               DockerUpgradeState `json:"state"`
	CurrentVersion      string             `json:"current_version"`
	CandidateVersion    string             `json:"candidate_version"`
	CurrentSourceSHA    string             `json:"current_source_sha"`
	CandidateSourceSHA  string             `json:"candidate_source_sha"`
	CurrentImage        string             `json:"current_image"`
	CandidateImage      string             `json:"candidate_image"`
	ManifestSHA256      string             `json:"manifest_sha256"`
	SourceSchemaVersion int                `json:"source_schema_version"`
	TargetSchemaVersion int                `json:"target_schema_version"`
	ComposeProject      string             `json:"compose_project"`
	ComposeFile         string             `json:"compose_file"`
	Attempt             int                `json:"attempt"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

func (j DockerUpgradeJournal) Validate() error {
	if j.Schema != 1 {
		return fmt.Errorf("unsupported docker upgrade journal schema %d", j.Schema)
	}
	if strings.TrimSpace(j.TransactionID) == "" {
		return fmt.Errorf("docker upgrade journal transaction_id is required")
	}
	switch j.State {
	case DockerUpgradePrepared, DockerUpgradePulled, DockerUpgradeApplying, DockerUpgradeVerifying, DockerUpgradeRollback, DockerUpgradeRolledBack, DockerUpgradeComplete, DockerUpgradeRecoveryNeeded:
	default:
		return fmt.Errorf("invalid docker upgrade journal state %q", j.State)
	}
	if !isImmutableOCIReference(j.CurrentImage) || !isImmutableOCIReference(j.CandidateImage) {
		return fmt.Errorf("docker upgrade journal images must be immutable repository@sha256 references")
	}
	if j.CurrentImage == j.CandidateImage {
		return fmt.Errorf("docker upgrade candidate must differ from current image")
	}
	if len(j.CandidateSourceSHA) != 40 || !isHex(j.CandidateSourceSHA) {
		return fmt.Errorf("candidate source SHA must be a full git SHA")
	}
	if len(j.ManifestSHA256) != 64 || !isHex(j.ManifestSHA256) {
		return fmt.Errorf("manifest SHA-256 is invalid")
	}
	if strings.TrimSpace(j.ComposeProject) == "" || strings.TrimSpace(j.ComposeFile) == "" {
		return fmt.Errorf("compose project and file identity are required")
	}
	if !filepath.IsAbs(j.ComposeFile) {
		return fmt.Errorf("compose file must be an absolute path")
	}
	if j.Attempt < 0 || j.Attempt > 10 {
		return fmt.Errorf("docker upgrade reconciliation attempt is out of bounds")
	}
	if j.CreatedAt.IsZero() || j.UpdatedAt.IsZero() || j.UpdatedAt.Before(j.CreatedAt) {
		return fmt.Errorf("docker upgrade journal timestamps are invalid")
	}
	return nil
}

func isImmutableOCIReference(value string) bool {
	parts := strings.Split(value, "@")
	if len(parts) != 2 || parts[0] == "" || strings.Contains(parts[0], "://") {
		return false
	}
	digest := parts[1]
	return strings.HasPrefix(digest, "sha256:") && len(digest) == len("sha256:")+64 && isHex(strings.TrimPrefix(digest, "sha256:"))
}

// WriteDockerUpgradeJournal atomically persists root/private recovery state and
// fsyncs both file and parent directory before returning.
func WriteDockerUpgradeJournal(path string, journal DockerUpgradeJournal) error {
	if err := journal.Validate(); err != nil {
		return err
	}
	clean := filepath.Clean(path)
	if clean != path || !filepath.IsAbs(path) {
		return fmt.Errorf("docker upgrade journal path must be a clean absolute path")
	}
	dir := filepath.Dir(path)
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("docker upgrade journal path must not be a symlink")
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect docker upgrade journal: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create docker upgrade journal directory: %w", err)
	}
	payload, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("encode docker upgrade journal: %w", err)
	}
	payload = append(payload, '\n')
	tmp, err := os.CreateTemp(dir, ".docker-upgrade-*.tmp")
	if err != nil {
		return fmt.Errorf("create docker upgrade journal temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("protect docker upgrade journal temp file: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write docker upgrade journal: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync docker upgrade journal: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close docker upgrade journal: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit docker upgrade journal: %w", err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open docker upgrade journal directory: %w", err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync docker upgrade journal directory: %w", err)
	}
	return nil
}

func ReadDockerUpgradeJournal(path string) (DockerUpgradeJournal, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return DockerUpgradeJournal{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return DockerUpgradeJournal{}, fmt.Errorf("docker upgrade journal must not be a symlink")
	}
	if info.Mode().Perm()&0077 != 0 {
		return DockerUpgradeJournal{}, fmt.Errorf("docker upgrade journal permissions are too broad")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return DockerUpgradeJournal{}, err
	}
	var journal DockerUpgradeJournal
	if err := json.Unmarshal(payload, &journal); err != nil {
		return DockerUpgradeJournal{}, fmt.Errorf("decode docker upgrade journal: %w", err)
	}
	if err := journal.Validate(); err != nil {
		return DockerUpgradeJournal{}, err
	}
	return journal, nil
}
