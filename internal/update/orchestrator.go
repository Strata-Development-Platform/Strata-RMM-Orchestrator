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
	TagName string      `json:"tag_name"`
	Assets  []GitHubAsset `json:"assets"`
	Body    string      `json:"body"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type OrchestratorRelease struct {
	Version  string `json:"version"`
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
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

	platformKey := fmt.Sprintf("strata-orchestrator-%s-%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL, checksumURL string

	for _, asset := range release.Assets {
		if asset.Name == platformKey || asset.Name == platformKey+".tar.gz" {
			downloadURL = asset.BrowserDownloadURL
		}
		if asset.Name == "checksums.txt" {
			checksumURL = asset.BrowserDownloadURL
		}
	}

	if downloadURL == "" {
		altKey := fmt.Sprintf("strata-orchestrator_%s_%s", runtime.GOOS, runtime.GOARCH)
		for _, asset := range release.Assets {
			if strings.Contains(asset.Name, altKey) {
				downloadURL = asset.BrowserDownloadURL
				break
			}
		}
	}

	if downloadURL == "" {
		return nil, fmt.Errorf("no binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
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

	body, _ := io.ReadAll(resp.Body)
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.Contains(parts[1], strings.TrimSuffix(filename, ".tar.gz")) {
			return parts[0]
		}
	}
	return ""
}

func (u *OrchestratorUpdater) Download(ctx context.Context, release *OrchestratorRelease) (string, error) {
	if release == nil || release.Checksum == "" {
		return "", fmt.Errorf("verified release checksum is required")
	}
	updateDir := filepath.Join(u.dataDir, "updates")
	os.MkdirAll(updateDir, 0700)

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

	if release.Checksum != "" {
		actualChecksum := hex.EncodeToString(hasher.Sum(nil))
		if actualChecksum != release.Checksum {
			os.Remove(binaryPath)
			return "", fmt.Errorf("checksum mismatch: expected %s, got %s", release.Checksum, actualChecksum)
		}
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
