package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

type ReleaseServer struct {
	cacheDir  string
	repoOwner string
	repoName  string
	httpClient *http.Client
	mu        sync.Mutex
	cached    map[string]string // platformKey -> localPath
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []githubAsset  `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func NewReleaseServer(cacheDir, owner, repo string) *ReleaseServer {
	os.MkdirAll(cacheDir, 0755)
	return &ReleaseServer{
		cacheDir:   cacheDir,
		repoOwner:  owner,
		repoName:   repo,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		cached:     make(map[string]string),
	}
}

func (rs *ReleaseServer) getCachedBinary(ctx context.Context, platformKey string) (string, error) {
	rs.mu.Lock()
	if path, ok := rs.cached[platformKey]; ok {
		if _, err := os.Stat(path); err == nil {
			rs.mu.Unlock()
			return path, nil
		}
	}
	rs.mu.Unlock()

	cachedPath := filepath.Join(rs.cacheDir, fmt.Sprintf("agent-%s", platformKey))
	if _, err := os.Stat(cachedPath); err == nil {
		rs.mu.Lock()
		rs.cached[platformKey] = cachedPath
		rs.mu.Unlock()
		return cachedPath, nil
	}

	release, err := rs.fetchLatestRelease(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch release: %w", err)
	}

	assetName := fmt.Sprintf("strata-rmm")
	for _, a := range release.Assets {
		if a.Name == assetName {
			return rs.downloadAndCache(ctx, a.BrowserDownloadURL, cachedPath)
		}
	}
	return "", fmt.Errorf("no asset found for %s", platformKey)
}

func (rs *ReleaseServer) fetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", rs.repoOwner, rs.repoName)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StrataRMM/1.0")

	resp, err := rs.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func (rs *ReleaseServer) downloadAndCache(ctx context.Context, url, destPath string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := rs.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	f, err := os.Create(destPath + ".tmp")
	if err != nil {
		return "", err
	}

	h := sha256.New()
	written, err := io.Copy(f, io.TeeReader(resp.Body, h))
	if err != nil {
		f.Close()
		os.Remove(destPath + ".tmp")
		return "", err
	}
	f.Close()

	if err := os.Rename(destPath+".tmp", destPath); err != nil {
		return "", err
	}
	if err := os.Chmod(destPath, 0755); err != nil {
		return "", err
	}

	rs.mu.Lock()
	rs.cached[filepath.Base(destPath)] = destPath
	rs.mu.Unlock()

	_ = written
	_ = h

	return destPath, nil
}

func (s *APIServer) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	installScript := `#!/bin/bash
set -euo pipefail

SERVER_URL="http://${HOSTNAME:-localhost}:` + strings.TrimPrefix(s.addr, ":") + `"
BINARY_NAME="strata-rmm"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/strata-rmm"
DATA_DIR="/var/lib/strata-rmm"
SERVICE_NAME="strata-rmm-agent"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[1;34m'; NC='\033[0m'
log_info() { echo -e "${GREEN}*${NC} $1"; }
log_step() { echo -e "\n${BLUE}==>${NC} $1"; }

detect_platform() {
	OS=$(uname -s | tr '[:upper:]' '[:lower:]')
	ARCH=$(uname -m)
	case "$ARCH" in x86_64) ARCH="amd64" ;; aarch64) ARCH="arm64" ;; *) echo "Unsupported: $ARCH"; exit 1 ;; esac
	echo "$OS/$ARCH"
}

if [ "$(id -u)" -ne 0 ]; then echo "Must run as root. Use: curl ... | sudo bash"; exit 1; fi

PLATFORM=$(detect_platform)
echo ""
echo -e "${BLUE}Strata RMM Agent Installer${NC}"
echo ""

read -p "Enter deployment ID: " DEPLOYMENT_ID
if [ -z "$DEPLOYMENT_ID" ]; then echo "Deployment ID required"; exit 1; fi

log_step "Downloading agent..."
BINARY_URL="$SERVER_URL/releases/latest/agent/$PLATFORM"
if command -v curl &>/dev/null; then
	curl -sL -o "$INSTALL_DIR/$BINARY_NAME" "$BINARY_URL"
elif command -v wget &>/dev/null; then
	wget -q -O "$INSTALL_DIR/$BINARY_NAME" "$BINARY_URL"
else
	echo "Need curl or wget"; exit 1
fi
chmod 755 "$INSTALL_DIR/$BINARY_NAME"
log_info "Installed to $INSTALL_DIR/$BINARY_NAME"

log_step "Setting up directories..."
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
chmod 755 "$CONFIG_DIR"
chmod 700 "$DATA_DIR"
cat > "$CONFIG_DIR/agent.yaml" <<EOF
agent:
  deployment_id: "$DEPLOYMENT_ID"
  log_level: "info"
  data_dir: "$DATA_DIR"
nats:
  urls: ["nats://$SERVER_URL"]
  reconnect_wait: 5s
  max_reconnects: -1
collect:
  interval: 60s
update:
  enabled: true
  manifest_url: "$SERVER_URL"
EOF
log_info "Config created"

log_step "Installing systemd service..."
cat > "/etc/systemd/system/$SERVICE_NAME.service" <<SERVICEEOF
[Unit]
Description=Strata RMM Agent
After=network.target
[Service]
ExecStart=$INSTALL_DIR/$BINARY_NAME agent --config $CONFIG_DIR/agent.yaml
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
SERVICEEOF
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

log_step "Starting agent..."
systemctl start "$SERVICE_NAME" 2>/dev/null || true
sleep 2

echo ""
echo -e "${GREEN}Installation Complete!${NC}"
echo "  Deployment ID: $DEPLOYMENT_ID"
echo "  Server:        $SERVER_URL"
echo "  Service:       $SERVICE_NAME"
echo "  Logs:          journalctl -u $SERVICE_NAME -f"
echo ""
read -p "Reboot now? [Y/n]: " REBOOT
REBOOT="${REBOOT:-Y}"
if [[ "$REBOOT" =~ ^[Yy]$ ]]; then echo "Rebooting..."; sleep 3; reboot; fi
`
	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Header().Set("Content-Disposition", "attachment; filename=install.sh")
	w.Write([]byte(installScript))
}

func (s *APIServer) handleReleaseBinary(w http.ResponseWriter, r *http.Request) {
	osName := r.PathValue("os")
	arch := r.PathValue("arch")

	if osName == "" || arch == "" {
		http.Error(w, "os and arch required", http.StatusBadRequest)
		return
	}

	platformKey := fmt.Sprintf("%s/%s", osName, arch)

	binaryPath, err := s.releaseServer.getCachedBinary(r.Context(), platformKey)
	if err != nil {
		s.logger.Warn("release binary not found", zap.Error(err))
		http.Error(w, "binary not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=strata-rmm-%s-%s", osName, arch))
	http.ServeFile(w, r, binaryPath)
}
