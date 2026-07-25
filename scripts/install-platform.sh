#!/bin/bash
# Strata RMM Platform Installer
# One-command installer for the RMM backend platform on bare metal Linux
# Usage: curl -sSL https://raw.githubusercontent.com/.../install-platform.sh | sudo bash
set -euo pipefail

VERSION="${VERSION:-latest}"
RELEASE_URL="${RELEASE_URL:-https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/latest/download}"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="/var/lib/strata-rmm"
CONFIG_DIR="/etc/strata-rmm"
NATS_DIR="/var/lib/nats"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[1;34m'; NC='\033[0m'
BOLD='\033[1m'

step=0
total=8

log_step()   { step=$((step+1)); echo -e "\n${BLUE}[${step}/${total}]${NC} ${BOLD}$1${NC}"; }
log_info()   { echo -e "  ${GREEN}*${NC} $1"; }
log_warn()   { echo -e "  ${YELLOW}*${NC} $1"; }
log_error()  { echo -e "  ${RED}*${NC} $1"; }
spinner()    { local pid=$1; while kill -0 $pid 2>/dev/null; do echo -n '.'; sleep 2; done; echo ''; }

require_root() { if [ "$(id -u)" -ne 0 ]; then echo -e "${RED}Must run as root. Use: curl ... | sudo bash${NC}"; exit 1; fi }

detect_os() {
  OS=""
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
  fi
  if [ "$OS" != "ubuntu" ] && [ "$OS" != "debian" ] && [ "$OS" != "centos" ] && [ "$OS" != "rhel" ] && [ "$OS" != "fedora" ]; then
    log_warn "Detected: $OS (may not be fully supported)"
  else
    log_info "Detected: $OS $VERSION_ID"
  fi
}

prompt_password() {
  local prompt="$1"
  local password=""
  local confirm=""
  while true; do
    read -s -p "  $prompt: " password; echo
    read -s -p "  Confirm: " confirm; echo
    if [ "$password" != "$confirm" ]; then
      log_error "Passwords do not match. Try again."
    elif [ ${#password} -lt 8 ]; then
      log_error "Password must be at least 8 characters."
    else
      echo "$password"
      return
    fi
  done
}

# ────────────────────────────────────────────────────
# MAIN INSTALLATION
# ────────────────────────────────────────────────────
clear
echo -e "${BLUE}════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Strata RMM Platform Installer${NC}"
echo -e "${BLUE}════════════════════════════════════════════${NC}"
echo ""

require_root
detect_os

# Gather passwords
DB_PASSWORD=$(prompt_password "PostgreSQL password for 'strata' user")
ADMIN_EMAIL=""
read -p "  Admin email for first user [admin@localhost]: " ADMIN_EMAIL
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@localhost}"
ADMIN_PASSWORD=$(prompt_password "Admin user password")

# ── Step 1: System dependencies ──
log_step "Installing system dependencies"
if command -v apt-get &>/dev/null; then
  apt-get update -qq && apt-get install -y -qq curl gnupg postgresql-common wget &>/dev/null &
  spinner $!
  log_info "System packages installed"
elif command -v dnf &>/dev/null; then
  dnf install -y curl gnupg wget &>/dev/null &
  spinner $!
  log_info "System packages installed"
else
  log_warn "Unknown package manager. Install curl, gnupg, wget manually."
fi

# ── Step 2: NATS Server ──
log_step "Installing NATS Server"
if command -v nats-server &>/dev/null; then
  log_info "NATS already installed: $(nats-server --version 2>&1 | head -1)"
else
  wget -q "https://github.com/nats-io/nats-server/releases/download/v2.10.22/nats-server-v2.10.22-linux-amd64.tar.gz" -O /tmp/nats.tar.gz
  tar -xzf /tmp/nats.tar.gz -C /tmp/
  mv /tmp/nats-server-v2.10.22-linux-amd64/nats-server "$INSTALL_DIR/nats-server"
  rm -rf /tmp/nats-server-v2.10.22-linux-amd64 /tmp/nats.tar.gz
  log_info "NATS Server installed"
fi

mkdir -p "$NATS_DIR"
cat > /etc/systemd/system/nats.service <<'NATS_EOF'
[Unit]
Description=NATS Server
After=network.target
[Service]
ExecStart=/usr/local/bin/nats-server -js -m 8222 --store_dir /var/lib/nats
Restart=always
User=nobody
[Install]
WantedBy=multi-user.target
NATS_EOF
systemctl daemon-reload
systemctl enable nats
systemctl start nats
log_info "NATS service started"

# ── Step 3: TimescaleDB ──
log_step "Installing TimescaleDB + PostgreSQL"
if command -v timescaledb &>/dev/null || command -v psql &>/dev/null; then
  log_info "PostgreSQL already installed"
else
  if [ "$OS" = "ubuntu" ] || [ "$OS" = "debian" ]; then
    curl -fsSL https://packagecloud.io/timescale/timescaledb/gpgkey | gpg --dearmor -o /usr/share/keyrings/timescaledb.gpg
    echo "deb https://packagecloud.io/timescale/timescaledb/ubuntu/ $(lsb_release -cs) main" > /etc/apt/sources.list.d/timescaledb.list
    apt-get update -qq && apt-get install -y -qq timescaledb-2-postgresql-17 &>/dev/null &
    spinner $!
    timescaledb-tune --quiet --yes &>/dev/null || true
    systemctl restart postgresql
    log_info "TimescaleDB installed"
  elif [ "$OS" = "centos" ] || [ "$OS" = "rhel" ] || [ "$OS" = "fedora" ]; then
    log_warn "Manual TimescaleDB install required for $OS. See https://docs.timescale.com/install"
  fi
fi

sudo -u postgres psql -c "CREATE USER strata WITH PASSWORD '$DB_PASSWORD';" 2>/dev/null || log_warn "User 'strata' may already exist"
sudo -u postgres psql -c "CREATE DATABASE strata_rmm OWNER strata;" 2>/dev/null || log_warn "Database 'strata_rmm' may already exist"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE strata_rmm TO strata;" 2>/dev/null || true
log_info "Database configured"

# ── Step 4: MinIO (optional) ──
log_step "Installing MinIO (optional — for session recordings)"
INSTALL_MINIO=""
read -p "  Install MinIO for session recording storage? [Y/n]: " INSTALL_MINIO
INSTALL_MINIO="${INSTALL_MINIO:-Y}"
if [[ "$INSTALL_MINIO" =~ ^[Yy]$ ]]; then
  wget -q "https://dl.min.io/server/minio/release/linux-amd64/minio" -O "$INSTALL_DIR/minio"
  chmod +x "$INSTALL_DIR/minio"
  mkdir -p /data/minio
  MINIO_ROOT_USER="${MINIO_ROOT_USER:-minioadmin}"
  MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-minioadmin}"
  cat > /etc/systemd/system/minio.service <<'MINIO_EOF'
[Unit]
Description=MinIO Object Storage
After=network.target
[Service]
ExecStart=/usr/local/bin/minio server /data/minio --console-address :9001
Restart=always
User=root
Environment="MINIO_ROOT_USER=minioadmin"
Environment="MINIO_ROOT_PASSWORD=minioadmin"
[Install]
WantedBy=multi-user.target
MINIO_EOF
  systemctl daemon-reload
  systemctl enable minio
  systemctl start minio
  log_info "MinIO started on port 9000 (console: 9001)"
else
  log_info "Skipping MinIO — recordings will use local storage"
fi

# ── Step 5: Download Orchestrator ──
log_step "Downloading Strata RMM Orchestrator"
BINARY_URL="$RELEASE_URL/strata-rmm"
if [ "$VERSION" != "latest" ]; then
  BINARY_URL="https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/download/v$VERSION/strata-rmm"
fi
wget -q "$BINARY_URL" -O "$INSTALL_DIR/strata-rmm"
chmod +x "$INSTALL_DIR/strata-rmm"
log_info "Binary installed: $(ls -lh $INSTALL_DIR/strata-rmm | awk '{print $5}')"

# ── Step 6: Create systemd service ──
log_step "Configuring orchestrator service"
useradd -r -s /bin/false strata-rmm 2>/dev/null || true
mkdir -p "$DATA_DIR" "$CONFIG_DIR"
chown strata-rmm:strata-rmm "$DATA_DIR" "$CONFIG_DIR"

STORAGE_ARGS="--storage-backend local"
if [[ "$INSTALL_MINIO" =~ ^[Yy]$ ]]; then
  STORAGE_ARGS="--storage-backend minio --storage-endpoint localhost:9000 --storage-bucket strata-recordings"
fi

cat > /etc/systemd/system/strata-rmm.service <<SERVICE_EOF
[Unit]
Description=Strata RMM Orchestrator
After=nats.service postgresql.service
Requires=nats.service postgresql.service
[Service]
ExecStart=$INSTALL_DIR/strata-rmm orchestrator \\
  --nats-url nats://localhost:4222 \\
  --timescale-dsn "postgres://strata:$DB_PASSWORD@localhost:5432/strata_rmm?sslmode=disable" \\
  --api-addr :8080 \\
  --tunnel-addr :8443 \\
  $STORAGE_ARGS
Restart=always
RestartSec=5
User=strata-rmm
Group=strata-rmm
Environment="STORAGE_ACCESS_KEY=minioadmin"
Environment="STORAGE_SECRET_KEY=minioadmin"
[Install]
WantedBy=multi-user.target
SERVICE_EOF

systemctl daemon-reload
systemctl enable strata-rmm
systemctl start strata-rmm
log_info "Orchestrator service started"

# ── Step 7: Initial setup ──
log_step "Creating admin user and deployment ID"
sleep 3
RESULT=$(curl -sf -X POST http://localhost:8080/api/v1/admin/customers \
  -H 'Content-Type: application/json' \
  -d "{\"name\": \"Default Customer\", \"admin_email\": \"$ADMIN_EMAIL\"}" 2>/dev/null || echo "")

if [ -n "$RESULT" ]; then
  DEPLOYMENT_ID=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('deployment_id',''))" 2>/dev/null || echo "")
  TENANT_ID=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
  
  # Create the admin user
  curl -sf -X POST http://localhost:8080/api/v1/admin/users \
    -H 'Content-Type: application/json' \
    -d "{\"email\": \"$ADMIN_EMAIL\", \"password\": \"$ADMIN_PASSWORD\", \"role\": \"admin\", \"tenant_ids\": [\"$TENANT_ID\"]}" >/dev/null 2>&1 || true
fi

log_info "Admin user created: $ADMIN_EMAIL"

# ── Step 8: Summary ──
log_step "Installation complete!"
echo ""
echo -e "${GREEN}════════════════════════════════════════════${NC}"
echo -e "${GREEN}  Strata RMM is installed!${NC}"
echo -e "${GREEN}════════════════════════════════════════════${NC}"
echo ""
echo -e "  ${BOLD}Web UI / API:${NC}  http://$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}'):8080"
echo -e "  ${BOLD}Login email:${NC}   $ADMIN_EMAIL"
echo -e "  ${BOLD}Login password:${NC} [the password you entered]"
echo -e "  ${BOLD}Deployment ID:${NC} ${DEPLOYMENT_ID:-run 'curl localhost:8080/api/v1/admin/customers' to get it}"
echo ""
echo -e "  ${BOLD}Agent install:${NC}"
echo -e "  curl -sSL https://raw.githubusercontent.com/Strata-Development-Platform/Strata-RMM-Orchestrator/main/scripts/install.sh | sudo bash -s -- --deployment-id ${DEPLOYMENT_ID:-YOUR_ID}"
echo ""
echo -e "  ${BOLD}Services:${NC}"
echo -e "  Orchestrator:    http://localhost:8080/health"
echo -e "  NATS:            localhost:4222 (monitoring: localhost:8222)"
echo -e "  PostgreSQL:      localhost:5432"
echo -e "  MinIO:           http://localhost:9000 (console: http://localhost:9001)"
echo ""
echo -e "  ${BOLD}Manage:${NC}"
echo -e "  systemctl status strata-rmm   (check orchestrator)"
echo -e "  journalctl -u strata-rmm -f  (follow logs)"
echo -e "  strata-rmm orchestrator update --check  (check for updates)"
echo ""

read -p "  ${YELLOW}System reboot recommended. Reboot now? [Y/n]: ${NC}" REBOOT
REBOOT="${REBOOT:-Y}"
if [[ "$REBOOT" =~ ^[Yy]$ ]]; then
  log_info "Rebooting in 5 seconds..."
  sleep 5
  reboot
else
  log_info "Please reboot manually when convenient to ensure all services start correctly."
fi
