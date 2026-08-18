package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type OrchestratorUpdater struct {
	currentVersion string
	owner          string
	repo           string
	httpClient     *http.Client
	currentExe     string
	dataDir        string
}

type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []GitHubAsset `json:"assets"`
	Body    string        `json:"body"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type OrchestratorRelease struct {
	Version             string `json:"version"`
	SourceSHA           string `json:"source_sha"`
	SchemaCompatibility string `json:"schema_compatibility"`
	URL                 string `json:"url"`
	Checksum            string `json:"checksum"`
	Size                int64  `json:"size"`
	SigstoreBundleURL   string `json:"sigstore_bundle_url"`
	ReleaseTag          string `json:"release_tag"`
	Changelog           string `json:"changelog"`
}

func NewOrchestratorUpdater(currentVersion, owner, repo string) *OrchestratorUpdater {
	exe, _ := os.Executable()
	return &OrchestratorUpdater{
		currentVersion: currentVersion,
		owner:          owner,
		repo:           repo,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		currentExe:     exe,
		dataDir:        "/var/lib/strata-rmm",
	}
}

func (u *OrchestratorUpdater) Check(ctx context.Context) (*OrchestratorRelease, error) {
	// GitHub's latest-release endpoint is discovery only. No version, checksum,
	// compatibility, or artifact decision is trusted until the manifest below
	// has passed Sigstore verification against the protected release workflow.
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", u.owner, u.repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StrataRMM/1.0")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if release.TagName == "" {
		return nil, fmt.Errorf("release tag is missing")
	}

	manifestURL := releaseAssetURL(release, "release-manifest.json")
	bundleURL := releaseAssetURL(release, "release-manifest.json.sigstore.json")
	if manifestURL == "" || bundleURL == "" {
		return nil, fmt.Errorf("release is missing signed provenance manifest")
	}
	manifestBytes, err := u.fetchReleaseAsset(ctx, manifestURL, maxReleaseMetadataBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch release manifest: %w", err)
	}
	manifestBundle, err := u.fetchReleaseAsset(ctx, bundleURL, maxReleaseMetadataBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch release manifest Sigstore bundle: %w", err)
	}
	if err := u.verifySigstoreBytes(ctx, manifestBytes, manifestBundle, release.TagName); err != nil {
		return nil, fmt.Errorf("verify release manifest: %w", err)
	}

	var manifest ReleaseManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("decode verified release manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("validate verified release manifest: %w", err)
	}
	if strings.TrimPrefix(release.TagName, "v") != manifest.Version {
		return nil, fmt.Errorf("release tag %q does not match signed manifest version %q", release.TagName, manifest.Version)
	}
	allowed, err := manifest.AllowsUpgradeFrom(u.currentVersion)
	if err != nil {
		return nil, fmt.Errorf("evaluate signed release compatibility: %w", err)
	}
	if !allowed {
		return nil, nil
	}

	artifactName := fmt.Sprintf("strata-rmm-orchestrator-%s-%s-%s", manifest.Version, runtime.GOOS, runtime.GOARCH)
	artifact, ok := manifest.Artifact(artifactName)
	if !ok {
		return nil, fmt.Errorf("signed release manifest is missing artifact %s", artifactName)
	}
	if artifact.Kind != "orchestrator" || artifact.OS != runtime.GOOS || artifact.Arch != runtime.GOARCH {
		return nil, fmt.Errorf("signed artifact metadata does not match this orchestrator platform")
	}

	downloadURL := releaseAssetURL(release, artifact.Name)
	artifactBundleURL := releaseAssetURL(release, artifact.SigstoreBundle)
	if downloadURL == "" {
		return nil, fmt.Errorf("release is missing signed artifact %s", artifact.Name)
	}
	if artifactBundleURL == "" {
		return nil, fmt.Errorf("release is missing Sigstore bundle %s", artifact.SigstoreBundle)
	}

	return &OrchestratorRelease{
		Version:             manifest.Version,
		SourceSHA:           manifest.SourceSHA,
		SchemaCompatibility: manifest.SchemaCompatibility,
		URL:                 downloadURL,
		Checksum:            artifact.SHA256,
		Size:                artifact.Size,
		SigstoreBundleURL:   artifactBundleURL,
		ReleaseTag:          release.TagName,
		Changelog:           release.Body,
	}, nil
}

func (u *OrchestratorUpdater) fetchChecksum(ctx context.Context, checksumURL, filename string) string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	req.Header.Set("User-Agent", "StrataRMM/1.0")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	return checksumForFile(string(body), filename)
}

func checksumForFile(manifest, filename string) string {
	for _, line := range strings.Split(manifest, "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 || strings.TrimPrefix(parts[1], "*") != filename {
			continue
		}
		checksum := strings.ToLower(parts[0])
		if len(checksum) != sha256.Size*2 {
			return ""
		}
		if _, err := hex.DecodeString(checksum); err != nil {
			return ""
		}
		return checksum
	}
	return ""
}

func (u *OrchestratorUpdater) Download(ctx context.Context, release *OrchestratorRelease) (string, error) {
	if release == nil || release.Checksum == "" || release.SigstoreBundleURL == "" || release.ReleaseTag == "" || release.Size <= 0 {
		return "", fmt.Errorf("fully verified release provenance is required")
	}
	updateDir := filepath.Join(u.dataDir, "updates")
	if err := os.MkdirAll(updateDir, 0700); err != nil {
		return "", fmt.Errorf("create update directory: %w", err)
	}

	binaryPath := filepath.Join(updateDir, fmt.Sprintf("strata-rmm-%s", release.Version))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create download: %w", err)
	}
	req.Header.Set("User-Agent", "StrataRMM/1.0")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}
	if resp.ContentLength > release.Size && resp.ContentLength >= 0 {
		return "", fmt.Errorf("download size exceeds signed manifest size")
	}

	f, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}

	hasher := sha256.New()
	written, copyErr := io.Copy(f, io.TeeReader(io.LimitReader(resp.Body, release.Size+1), hasher))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(binaryPath)
		return "", fmt.Errorf("write: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(binaryPath)
		return "", fmt.Errorf("close downloaded release artifact: %w", closeErr)
	}
	if written != release.Size {
		_ = os.Remove(binaryPath)
		return "", fmt.Errorf("downloaded release artifact size %d does not match signed size %d", written, release.Size)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualChecksum, release.Checksum) {
		if err := os.Remove(binaryPath); err != nil {
			return "", fmt.Errorf("checksum mismatch; remove invalid release artifact: %w", err)
		}
		return "", fmt.Errorf("checksum mismatch for downloaded release artifact")
	}
	if err := u.verifySigstoreFile(ctx, binaryPath, release.SigstoreBundleURL, release.ReleaseTag); err != nil {
		_ = os.Remove(binaryPath)
		return "", fmt.Errorf("verify downloaded release artifact: %w", err)
	}
	if err := os.Chmod(binaryPath, 0755); err != nil {
		_ = os.Remove(binaryPath)
		return "", fmt.Errorf("chmod: %w", err)
	}

	return binaryPath, nil
}

func (u *OrchestratorUpdater) Apply(binaryPath string) error {
	backupPath := u.currentExe + ".bak"

	if err := os.Rename(u.currentExe, backupPath); err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	if err := os.Rename(binaryPath, u.currentExe); err != nil {
		_ = os.Rename(backupPath, u.currentExe)
		return fmt.Errorf("replace: %w", err)
	}

	return nil
}

func (u *OrchestratorUpdater) Verify(ctx context.Context, healthURL string) error {
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		resp, err := u.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if err == nil {
			_ = resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("health check failed after 5 attempts")
}

func (u *OrchestratorUpdater) Rollback() error {
	backupPath := u.currentExe + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup found")
	}

	if err := os.Rename(u.currentExe, u.currentExe+".failed"); err != nil {
		return fmt.Errorf("rename failed binary: %w", err)
	}
	if err := os.Rename(backupPath, u.currentExe); err != nil {
		return fmt.Errorf("restore backup: %w", err)
	}
	return nil
}

func (u *OrchestratorUpdater) DetectMode() string {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "kubernetes"
	}
	return "baremetal"
}

func (u *OrchestratorUpdater) Cleanup() {
	failed := u.currentExe + ".failed"
	_ = os.Remove(failed)
	backup := u.currentExe + ".bak"
	_ = os.Remove(backup)
	_ = os.RemoveAll(filepath.Join(u.dataDir, "updates"))
}

func (u *OrchestratorUpdater) CurrentVersion() string {
	return u.currentVersion
}

// TriggerRestart delegates restart, post-restart health verification, and
// automatic rollback to a transient systemd unit that survives this process.
// The command returns only if systemd could not accept the finalizer. On
// success this process exits and the finalizer becomes the source of truth for
// whether the candidate is finalized or rolled back.
func (u *OrchestratorUpdater) TriggerRestart() error {
	mode := u.DetectMode()
	if mode != "baremetal" {
		return fmt.Errorf("%s deployment requires the digest-pinned promoted release workflow", mode)
	}

	const finalizer = "/usr/lib/strata-rmm/finalize-orchestrator-upgrade.sh"
	if info, err := os.Stat(finalizer); err != nil || info.Mode()&0111 == 0 {
		return fmt.Errorf("upgrade finalizer is unavailable or not executable: %s", finalizer)
	}

	unit := fmt.Sprintf("strata-rmm-upgrade-finalize-%d", os.Getpid())
	cmd := exec.Command(
		"systemd-run",
		"--unit="+unit,
		"--collect",
		"--property=Type=oneshot",
		finalizer,
		u.currentExe,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launch external upgrade finalizer: %w: %s", err, strings.TrimSpace(string(output)))
	}

	os.Exit(0)
	return nil
}
