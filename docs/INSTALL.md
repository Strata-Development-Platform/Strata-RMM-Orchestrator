# Installation Guide

## Architecture Overview

```
┌──────────────────────────────────────────────────────┐
│                  Strata RMM Platform                  │
│                                                        │
│  NATS (messaging) + TimescaleDB (metrics) + MinIO/S3  │
│  Orchestrator (API + engine)                           │
└────────────────────────────────────────────────────────┘
        ▲                          │
        │ NATS (port 4222)         │ HTTPS (port 8080)
        ▼                          ▼
┌──────────────┐          ┌──────────────────┐
│   Agents     │          │  Web UI / API    │
│ (bare metal) │          │  (browser/tool)  │
└──────────────┘          └──────────────────┘
```

---

## Option A: Docker Compose (Recommended)

### Quick Start

```bash
# Prerequisites: Docker + Docker Compose
curl -O https://raw.githubusercontent.com/Strata-Development-Platform/Strata-RMM-Orchestrator/main/deploy/docker/docker-compose.yml
docker compose up -d

# Verify
curl http://localhost:8080/health
# → {"status":"ok","time":"..."}
```

### Services

| Service | Port | Purpose |
|---------|------|---------|
| Orchestrator | 8080 | REST API + engines |
| NATS | 4222 | Message bus |
| NATS monitoring | 8222 | NATS dashboard |
| TimescaleDB | 5432 | Database |
| MinIO | 9000 | Object storage (recordings) |
| MinIO Console | 9001 | Storage admin UI |
| Grafana (optional) | 3000 | Metrics dashboards |

### Configuration

Set environment variables in `docker-compose.yml` or a `.env` file:

```bash
# Storage (recordings)
STORAGE_BACKEND=minio
STORAGE_BUCKET=strata-recordings
STORAGE_ENDPOINT=minio:9000
STORAGE_ACCESS_KEY=minioadmin
STORAGE_SECRET_KEY=minioadmin

# Database
POSTGRES_PASSWORD=strata_dev   # Change in production!
```

### Updating

```bash
docker compose pull
docker compose up -d
```

---

## Option B: Bare Metal (Linux)

### Prerequisites

```bash
# Ubuntu/Debian
sudo apt update && sudo apt install -y curl gnupg postgresql-common

# CentOS/RHEL/Fedora
sudo dnf install -y curl
```

### Step 1: Install NATS

```bash
curl -sf https://binaries.nats.dev/nats-io/nats-server/v2.10@latest | bash
sudo mv nats-server /usr/local/bin/
sudo tee /etc/systemd/system/nats.service <<'EOF'
[Unit]
Description=NATS Server
After=network.target

[Service]
ExecStart=/usr/local/bin/nats-server -js -m 8222
Restart=always
User=nobody

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl enable --now nats
```

### Step 2: Install TimescaleDB

```bash
# Ubuntu/Debian
sudo apt install -y gnupg
curl -fsSL https://packagecloud.io/timescale/timescaledb/gpgkey | sudo gpg --dearmor -o /etc/apt/keyrings/timescale.gpg
echo "deb https://packagecloud.io/timescale/timescaledb/ubuntu/ $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/timescaledb.list
sudo apt update
sudo apt install -y timescaledb-2-postgresql-16
sudo timescaledb-tune --yes
sudo systemctl restart postgresql

# Create database
sudo -u postgres psql -c "CREATE USER strata WITH PASSWORD 'strata_dev';"
sudo -u postgres psql -c "CREATE DATABASE strata_rmm OWNER strata;"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE strata_rmm TO strata;"
```

### Step 3: Install MinIO (Optional — for session recordings)

```bash
curl -LO https://dl.min.io/server/minio/release/linux-amd64/minio
chmod +x minio
sudo mv minio /usr/local/bin/
sudo mkdir -p /data/minio
MINIO_ROOT_USER=minioadmin MINIO_ROOT_PASSWORD=minioadmin minio server /data/minio --console-address :9001 &
```

### Step 4: Install Orchestrator

```bash
# Download latest release
curl -LO https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/latest/download/strata-orchestrator-linux-amd64
chmod +x strata-orchestrator-linux-amd64
sudo mv strata-orchestrator-linux-amd64 /usr/local/bin/strata-rmm

# Create user
sudo useradd -r -s /bin/false strata-rmm
sudo mkdir -p /var/lib/strata-rmm /etc/strata-rmm
sudo chown strata-rmm:strata-rmm /var/lib/strata-rmm /etc/strata-rmm

# Systemd service
sudo tee /etc/systemd/system/strata-rmm.service <<'EOF'
[Unit]
Description=Strata RMM Orchestrator
After=nats.service postgresql.service
Requires=nats.service postgresql.service

[Service]
ExecStart=/usr/local/bin/strata-rmm orchestrator \
  --nats-url nats://localhost:4222 \
  --timescale-dsn "postgres://strata:strata_dev@localhost:5432/strata_rmm?sslmode=disable" \
  --api-addr :8080 \
  --tunnel-addr :8443 \
  --storage-backend local
Restart=always
RestartSec=5
User=strata-rmm
Group=strata-rmm

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now strata-rmm

# Verify
curl http://localhost:8080/health
```

### Step 5: First-Time Setup

```bash
# Create your first customer
curl -X POST http://localhost:8080/api/v1/admin/customers \
  -H 'Content-Type: application/json' \
  -d '{"name": "My Company", "admin_email": "admin@example.com"}'

# Response includes: deployment_id, id
# Save the deployment_id — you'll need it to install agents
```

### Updating (Bare Metal)

```bash
# Check for update
strata-rmm orchestrator update --check

# Apply update
strata-rmm orchestrator update

# Or via API
curl -X POST http://localhost:8080/api/v1/admin/update/apply
```

---

## Option C: Kubernetes (Helm)

```bash
# Add Helm repo
helm repo add strata-rmm https://strata-development-platform.github.io/helm-charts
helm repo update

# Install
helm upgrade --install strata-rmm strata-rmm/strata-rmm \
  --namespace strata-rmm --create-namespace \
  --set global.environment=production

# Or from local chart
helm upgrade --install strata-rmm ./deploy/helm/strata-rmm \
  --values ./deploy/helm/strata-rmm/values.yaml

# Update
helm upgrade strata-rmm strata-rmm/strata-rmm
```

---

## Agent Installation

### Linux (systemd)

```bash
# One-command install with deployment ID
curl -sSL https://releases.strata-rmm.io/install.sh | sudo bash -s -- --deployment-id YOUR_DEPLOYMENT_ID
```

Or manually:

```bash
# Download
curl -LO https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/latest/download/strata-agent-linux-amd64
chmod +x strata-agent-linux-amd64
sudo mv strata-agent-linux-amd64 /usr/local/bin/strata-rmm

# Run
strata-rmm agent --deployment-id YOUR_DEPLOYMENT_ID --nats-url nats://your-server:4222
```

### Windows (MSI)

1. Download the MSI from [GitHub Releases](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases)
2. Run the installer
3. The agent will prompt for a deployment ID on first run
4. Or configure in `C:\ProgramData\StrataRMM\agent.yaml`

### macOS

```bash
# Download
curl -LO https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/latest/download/strata-agent-darwin-amd64
chmod +x strata-agent-darwin-amd64
sudo mv strata-agent-darwin-amd64 /usr/local/bin/strata-rmm

# Remove quarantine (required for unsigned binaries)
xattr -d com.apple.quarantine /usr/local/bin/strata-rmm

# Run
strata-rmm agent --deployment-id YOUR_DEPLOYMENT_ID
```

---

## Network Requirements

| Direction | Port | Protocol | Service |
|-----------|------|----------|---------|
| Inbound | 8080 | HTTP | REST API / Web UI |
| Inbound | 8443 | TCP | Tunnel gateway |
| Inbound | 4222 | TCP | NATS client connections |
| Agent → Server | 4222 | TCP | NATS (outbound) |
| Agent → Server | 8080 | HTTP | Registration (outbound) |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `STORAGE_BACKEND` | `local` | `minio`, `s3`, `local`, `none` |
| `STORAGE_BUCKET` | `strata-recordings` | Storage bucket name |
| `STORAGE_ENDPOINT` | — | MinIO/S3 endpoint |
| `STORAGE_REGION` | — | AWS region (S3 only) |
| `STORAGE_ACCESS_KEY` | — | Storage access key |
| `STORAGE_SECRET_KEY` | — | Storage secret key |
| `STORAGE_USE_SSL` | `false` | Enable TLS for storage |
| `STORAGE_KMS_KEY_ID` | — | AWS KMS key ID for encryption |

---

## Verification

After installation, run the smoke test:

```bash
./scripts/smoke_test.sh
# Or
make smoke-test
```

Expected output:
```
[PASS] Health endpoint returns ok
[PASS] Enrollment token generated
[PASS] CVE database has 10+ records
[PASS] MFA enrollment generated secret
[PASS] Recording list endpoint works
[PASS] All smoke tests passed!
```
