#!/bin/sh
set -eu

SERVER_URL="${RMM_SERVER_URL:-https://rmm.stratadevplatform.com}"
INSTALL_DIR="${RMM_INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${RMM_CONFIG_DIR:-/etc/strata-rmm}"
DATA_DIR="${RMM_DATA_DIR:-/var/lib/strata-rmm}"
SERVICE_NAME="strata-rmm-agent"
BINARY_NAME="strata-agent"

fail() { printf 'Strata RMM install failed: %s\n' "$1" >&2; exit 1; }
info() { printf '==> %s\n' "$1"; }

[ "$(id -u)" -eq 0 ] || fail "run this installer as root"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required"
command -v systemctl >/dev/null 2>&1 || fail "systemd is required"

case "$SERVER_URL" in
  https://*) CURL_PROTO="=https" ;;
  http://localhost:*|http://127.0.0.1:*) CURL_PROTO="=http,https" ;;
  *) fail "RMM_SERVER_URL must use HTTPS outside local development" ;;
esac

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
[ "$OS" = "linux" ] || fail "this installer supports Linux only"
case "$(uname -m)" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) fail "unsupported architecture" ;;
esac

ENROLLMENT_TOKEN="${RMM_ENROLLMENT_TOKEN:-}"
if [ -z "$ENROLLMENT_TOKEN" ]; then
  if [ ! -t 0 ]; then
    fail "set RMM_ENROLLMENT_TOKEN for non-interactive installation"
  fi
  printf 'Enrollment token: ' >&2
  stty -echo
  IFS= read -r ENROLLMENT_TOKEN
  stty echo
  printf '\n' >&2
fi
[ -n "$ENROLLMENT_TOKEN" ] || fail "enrollment token is required"

TEMP_DIR="$(mktemp -d)"
trap 'stty echo 2>/dev/null || true; rm -rf "$TEMP_DIR"' EXIT HUP INT TERM
BINARY_URL="$SERVER_URL/releases/latest/agent/$OS/$ARCH"

info "Downloading the signed release candidate"
curl --fail --silent --show-error --location --proto "$CURL_PROTO" --tlsv1.2 \
	--connect-timeout 10 --max-time 300 --retry 3 --retry-delay 2 --retry-all-errors \
  --output "$TEMP_DIR/$BINARY_NAME" "$BINARY_URL"
curl --fail --silent --show-error --location --proto "$CURL_PROTO" --tlsv1.2 \
	--connect-timeout 10 --max-time 60 --retry 3 --retry-delay 2 --retry-all-errors \
  --output "$TEMP_DIR/$BINARY_NAME.sha256" "$BINARY_URL?checksum=sha256"
(cd "$TEMP_DIR" && sha256sum --check "$BINARY_NAME.sha256") || fail "agent checksum verification failed"

info "Installing agent files"
install -d -m 0750 "$CONFIG_DIR"
install -d -m 0700 "$DATA_DIR"
install -m 0755 "$TEMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"

umask 077
cat > "$CONFIG_DIR/agent.yaml" <<EOF
agent:
  enrollment_token: "$ENROLLMENT_TOKEN"
  register_url: "$SERVER_URL/api/v1/agent/register"
  log_level: info
  data_dir: "$DATA_DIR"
collect:
  interval: 60s
  enable_system: true
store:
  type: bbolt
  path: "$DATA_DIR/agent.db"
  queue_max_items: 10000
update:
  enabled: false
EOF
chmod 0600 "$CONFIG_DIR/agent.yaml"
unset ENROLLMENT_TOKEN RMM_ENROLLMENT_TOKEN

cat > "/etc/systemd/system/$SERVICE_NAME.service" <<EOF
[Unit]
Description=Strata RMM Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/$BINARY_NAME agent --config $CONFIG_DIR/agent.yaml
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR $CONFIG_DIR

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

info "Waiting for the agent to remain active"
ATTEMPT=0
while [ "$ATTEMPT" -lt 20 ]; do
  if systemctl is-active --quiet "$SERVICE_NAME"; then
    sleep 2
    systemctl is-active --quiet "$SERVICE_NAME" && break
  fi
  ATTEMPT=$((ATTEMPT + 1))
  sleep 1
done
systemctl is-active --quiet "$SERVICE_NAME" || {
  journalctl -u "$SERVICE_NAME" -n 50 --no-pager >&2 || true
  fail "agent service did not become active"
}

if grep -q 'enrollment_token:' "$CONFIG_DIR/agent.yaml"; then
  fail "agent did not consume the one-time enrollment token"
fi

info "Strata RMM agent installed and enrolled successfully"
printf 'Service: %s\nLogs: journalctl -u %s -f\n' "$SERVICE_NAME" "$SERVICE_NAME"
