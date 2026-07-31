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
	Body    string      `json:"body"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type OrchestratorRelease struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	Checksum  string `json:"checksum"`
	Changelog string `json:"changelog"`
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
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", u.owner, u.repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(u.currentVersion, "v")

	comparison, err := compareSemanticVersions(latestVersion, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("compare release versions: %w", err)
	}
	if comparison <= 0 {
		return nil, nil
	}

	artifactName := fmt.Sprintf("strata-rmm-orchestrator-%s-%s-%s", latestVersion, runtime.GOOS, runtime.GOARCH)
	var downloadURL, checksumURL string

	for _, asset := range release.Assets {
		if asset.Name == artifactName {
			downloadURL = asset.BrowserDownloadURL
		}
		if asset.Name == "checksums.txt" {
			checksumURL = asset.BrowserDownloadURL
		}
	}

	if downloadURL == "" {
		return nil, fmt.Errorf("release is missing immutable artifact %s", artifactName)
	}

	if checksumURL == "" {
		return nil, fmt.Errorf("release is missing checksums.txt")
	}
	checksum := u.fetchChecksum(ctx, checksumURL, filepath.Base(downloadURL))
	if checksum == "" {
		return nil, fmt.Errorf("release checksum for %s is missing or invalid", filepath.Base(downloadURL))
	}

	return &OrchestratorRelease{
		Version:   latestVersion,
		URL:       downloadURL,
		Checksum:  checksum,
		Changelog: release.Body,
	}, nil
}

func (u *OrchestratorUpdater) fetchChecksum(ctx context.Context, checksumURL, filename string) string {
	req, _ := http.NewRequestWithContext(ctx, "GET", checksumURL, nil)
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
	if release == nil || release.Checksum == "" {
		return "", fmt.Errorf("verified release checksum is required")
	}
	updateDir := filepath.Join(u.dataDir, "updates")
	if err := os.MkdirAll(updateDir, 0700); err != nil {
		return "", fmt.Errorf("create update directory: %w", err)
	}

	binaryPath := filepath.Join(updateDir, fmt.Sprintf("strata-rmm-%s", release.Version))

	req, err := http.NewRequestWithContext(ctx, "GET", release.URL, nil)
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

	f, err := os.Create(binaryPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}

	hasher := sha256.New()
	_, err = io.Copy(f, io.TeeReader(resp.Body, hasher))
	if err != nil {
		f.Close()
		os.Remove(binaryPath)
		return "", fmt.Errorf("write: %w", err)
	}
	f.Close()

	if err := os.Chmod(binaryPath, 0755); err != nil {
		os.Remove(binaryPath)
		return "", fmt.Errorf("chmod: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualChecksum, release.Checksum) {
		if err := os.Remove(binaryPath); err != nil {
			return "", fmt.Errorf("checksum mismatch; remove invalid release artifact: %w", err)
		}
		return "", fmt.Errorf("checksum mismatch for downloaded release artifact")
	}

	return binaryPath, nil
}

func (u *OrchestratorUpdater) Apply(binaryPath string) error {
	backupPath := u.currentExe + ".bak"

	if err := os.Rename(u.currentExe, backupPath); err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	if err := os.Rename(binaryPath, u.currentExe); err != nil {
		os.Rename(backupPath, u.currentExe)
		return fmt.Errorf("replace: %w", err)
	}

	return nil
}

func (u *OrchestratorUpdater) Verify(ctx context.Context, healthURL string) error {
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
		resp, err := u.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if err == nil {
			resp.Body.Close()
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
	os.Remove(failed)
	backup := u.currentExe + ".bak"
	os.Remove(backup)
	os.RemoveAll(filepath.Join(u.dataDir, "updates"))
}

func (u *OrchestratorUpdater) CurrentVersion() string {
	return u.currentVersion
}

func (u *OrchestratorUpdater) TriggerRestart() error {
	mode := u.DetectMode()

	switch mode {
	case "baremetal":
		cmd := exec.Command("systemctl", "restart", "strata-rmm")
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("systemctl restart: %w", err)
		}
		os.Exit(0)
	case "docker":
		return fmt.Errorf("docker: run 'docker compose restart' manually")
	case "kubernetes":
		return fmt.Errorf("kubernetes: run 'kubectl rollout restart deployment/strata-rmm' manually")
	}
	return nil
}
