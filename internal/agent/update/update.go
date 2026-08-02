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
	"time"
)

type Client struct {
	manifestURL   string
	httpClient    *http.Client
	store         *Store
	currentExe    string
	dataDir       string
	channel       Channel
	checkInterval time.Duration
}

type ClientOptions struct {
	ManifestURL   string
	Store         *Store
	DataDir       string
	Channel       Channel
	CheckInterval time.Duration
	HTTPClient    *http.Client
}

func NewClient(opts ClientOptions) *Client {
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if opts.CheckInterval == 0 {
		opts.CheckInterval = 24 * time.Hour
	}
	if opts.Channel == "" {
		opts.Channel = ChannelStable
	}

	exe, _ := os.Executable()

	return &Client{
		manifestURL:   opts.ManifestURL,
		httpClient:    opts.HTTPClient,
		store:         opts.Store,
		currentExe:    exe,
		dataDir:       opts.DataDir,
		channel:       opts.Channel,
		checkInterval: opts.CheckInterval,
	}
}

func (c *Client) Check(ctx context.Context) (*Manifest, error) {
	url := fmt.Sprintf("%s/update-manifest-%s.json", c.manifestURL, c.channel)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest fetch returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	manifest, err := ParseManifest(body)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	state, _ := c.store.GetState()
	if manifest.Version == state.CurrentVersion {
		return nil, nil
	}

	return manifest, nil
}

func (c *Client) ShouldCheck(lastCheck time.Time) bool {
	return time.Since(lastCheck) >= c.checkInterval
}

func (c *Client) Download(ctx context.Context, manifest *Manifest) (string, error) {
	artifact, err := manifest.ArtifactFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", fmt.Errorf("get artifact: %w", err)
	}

	if err := c.store.SetPendingVersion(manifest.Version); err != nil {
		return "", fmt.Errorf("set pending: %w", err)
	}

	updateDir := filepath.Join(c.dataDir, "updates")
	if err := os.MkdirAll(updateDir, 0700); err != nil {
		return "", fmt.Errorf("create update dir: %w", err)
	}

	binaryPath := filepath.Join(updateDir, fmt.Sprintf("strata-agent-%s", manifest.Version))

	req, err := http.NewRequestWithContext(ctx, "GET", artifact.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.Create(binaryPath)
	if err != nil {
		return "", fmt.Errorf("create binary file: %w", err)
	}

	hasher := sha256.New()
	written, err := io.Copy(f, io.TeeReader(resp.Body, hasher))
	if err != nil {
		f.Close()
		os.Remove(binaryPath)
		return "", fmt.Errorf("write binary: %w", err)
	}
	f.Close()

	if err := os.Chmod(binaryPath, 0755); err != nil {
		os.Remove(binaryPath)
		return "", fmt.Errorf("chmod binary: %w", err)
	}

	if artifact.Size > 0 && written != artifact.Size {
		os.Remove(binaryPath)
		return "", fmt.Errorf("size mismatch: got %d, expected %d", written, artifact.Size)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if artifact.Checksum != "" && actualChecksum != artifact.Checksum {
		os.Remove(binaryPath)
		return "", fmt.Errorf("checksum mismatch: got %s, expected %s", actualChecksum, artifact.Checksum)
	}

	return binaryPath, nil
}

func (c *Client) Apply(binaryPath string) error {
	if err := c.store.SetState(&UpdateState{
		Status: StatusApplying,
	}); err != nil {
		return fmt.Errorf("set applying state: %w", err)
	}

	backupPath := c.currentExe + ".bak"
	if err := os.Rename(c.currentExe, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}

	if err := os.Rename(binaryPath, c.currentExe); err != nil {
		os.Rename(backupPath, c.currentExe)
		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}

func (c *Client) VerifyAndSwitch() error {
	cmd := exec.Command(c.currentExe, "version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("verify new binary: %w", err)
	}

	var versionInfo struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(output, &versionInfo); err != nil {
		return fmt.Errorf("parse version: %w", err)
	}

	if err := c.store.SetCurrentVersion(versionInfo.Version); err != nil {
		return err
	}

	_ = os.Remove(c.currentExe + ".bak")
	_ = os.Remove(filepath.Join(c.dataDir, "updates"))

	return nil
}

func (c *Client) Rollback() error {
	backupPath := c.currentExe + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup binary found")
	}

	if err := os.Rename(c.currentExe, c.currentExe+".failed"); err != nil {
		return fmt.Errorf("rename failed binary: %w", err)
	}
	if err := os.Rename(backupPath, c.currentExe); err != nil {
		return fmt.Errorf("restore backup: %w", err)
	}

	state, _ := c.store.GetState()
	if state.RollbackVersion != "" {
		state.CurrentVersion = state.RollbackVersion
	}
	state.Status = StatusRolledBack
	state.PendingVersion = ""
	state.FailedAttempts = 0
	return c.store.SetState(state)
}

func (c *Client) CurrentVersion() string {
	state, err := c.store.GetState()
	if err != nil {
		return ""
	}
	return state.CurrentVersion
}
