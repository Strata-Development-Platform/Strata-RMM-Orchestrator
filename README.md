# Strata RMM

Strata RMM Community Edition is open-source under
[AGPL-3.0-or-later](LICENSE). Commercial white-label deployments are available
only under a separately executed Enterprise License. See
[LICENSING.md](LICENSING.md) and [TRADEMARKS.md](TRADEMARKS.md).

A horizontally-scalable, multi-tenant Remote Monitoring & Management platform with cross-platform agents (Go), supporting both SaaS and self-hosted deployments. Built to match the capabilities of Kaseya VSA, Datto RMM, and NinjaRMM.

## Quick Start

```bash
# Docker (recommended local-development topology)
git clone https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator.git
cd Strata-RMM-Orchestrator
export JWT_SECRET="$(openssl rand -hex 32)"
export STRATA_METRICS_TOKEN="$(openssl rand -hex 32)"
export POSTGRES_PASSWORD="$(openssl rand -hex 24)"
export MINIO_ROOT_PASSWORD="$(openssl rand -hex 24)"
export GRAFANA_ADMIN_PASSWORD="$(openssl rand -hex 24)"
export STRATA_METRICS_TOKEN_FILE="$(mktemp)"
printf '%s' "$STRATA_METRICS_TOKEN" > "$STRATA_METRICS_TOKEN_FILE"
chmod 600 "$STRATA_METRICS_TOKEN_FILE"
docker compose -f deploy/docker/docker-compose.yml up -d

# Verify
curl http://localhost:8080/health/live
```

This Compose topology is for local development; its dependency credentials and
unencrypted transports are not production settings. Start with the
[documentation index](docs/index.md), or follow the
[installation guide](docs/INSTALL.md) for production preflight, TLS,
authentication, bootstrap, and supported deployment guidance.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                   Strata RMM Platform                     │
├──────────────────────────────────────────────────────────┤
│  Agent (Go) ──NATS──► Orchestrator ──► TimescaleDB      │
│    │                        │              │             │
│    │                    PostgreSQL (RLS)  Grafana       │
│    │                        │                          │
│    │                    MinIO / S3 (recordings)         │
│    │                        │                          │
│  Probe (SNMP/Flow) ──NATS──┘                            │
└──────────────────────────────────────────────────────────┘
```

- **Agent**: Cross-platform Go binary (Windows/Linux/macOS), ~15MB, BBolt offline queue
- **Orchestrator**: Go stdlib `net/http`, NATS consumer, TimescaleDB batch writer, alerting, patching, scripting, remote control
- **NATS**: Message bus with tenant subject isolation (`tenant.{id}.*`)
- **TimescaleDB**: Hypertables with compression + continuous aggregates for metrics
- **PostgreSQL**: Row-Level Security for multi-tenant data isolation
- **MinIO/S3**: Object storage for session recordings and PDF reports

## CLI Commands

| Command | Description |
|---------|-------------|
| `strata-rmm version` | Print version info (`--output=json`) |
| `strata-rmm agent --enrollment-token X` | Securely enroll and start the monitoring agent |
| `strata-rmm orchestrator` | Platform services |
| `strata-rmm orchestrator update` | Self-update orchestrator |
| `strata-rmm probe` | Network probe (SNMP, flow) |

## Features

This section summarizes the intended product surface and includes capabilities
that are partial or environment-dependent. Consult the
[feature completeness matrix](docs/FEATURE_COMPLETENESS_MATRIX.md) before using
it as release or acceptance evidence.

**Agent Management**
- Cross-platform (Windows MSI, Linux systemd, macOS binary)
- Deployment ID onboarding — one ID per customer, auto-registers on first check-in
- Agent auto-update with staged rollout, canary %, rollback, cosign verification
- ECDSA P256 identity + JWT authentication

**Remote Monitoring**
- CPU: percent, cores, load average (1/5/15)
- Memory: total/used/available/percent, swap
- Disk: per-partition total/used/free/percent, I/O counters
- Network: per-interface bytes/packets/errors
- System: uptime, hostname, OS, platform version
- Process: top CPU/memory consumers
- Software inventory (dpkg/Win32_Product)
- 60s collection interval (configurable)

**Alerting Engine**
- Threshold rules (GT/GTE/LT/LTE/EQ/NEQ)
- Heartbeat monitoring with configurable timeout
- Alert state machine: OK → Firing → Resolved
- Cooldowns to prevent alert storms
- Auto-resolution on metric recovery
- Notifications: Slack, webhook, email, Teams, PagerDuty

**Remote Access**
- Built-in remote control (agent-side screen capture + input injection)
- NATS-relayed RDP/SSH/VNC tunnels
- Session recording with SHA256 verification
- Presigned URL playback with MFA gate
- Configurable retention (default 90d)

**Scripting Engine**
- PowerShell, Bash, Python, Batch execution
- Parameter interpolation (`{{param}}` syntax)
- Timeout enforcement, stdout/stderr capture
- Execution history with full output viewer
- Run on-demand or scheduled

**Patch Management**
- Windows OS updates (PowerShell/WU API)
- Linux OS updates (apt/dnf/zypper/pacman)
- Patch policies with approval modes (auto/manual)
- Deployment scheduling with device targeting
- Patch compliance tracking per device

**Software Deployment**
- Package library: MSI, EXE, DEB, RPM, AppImage
- SHA256 checksum verification
- Silent install with custom arguments
- Per-device deployment status tracking
- Install and uninstall support

**Third-Party Patching**
- 10 pre-configured apps: Chrome, Firefox, Adobe Reader, 7-Zip, VLC, Teams, Zoom, Notepad++, LibreOffice
- Auto-version discovery from vendor APIs
- Auto-creates deployable packages
- 24h automatic sync cycle

**Vulnerability Management**
- CVE correlation against software inventory
- OSV.dev batch API (17 tracked packages)
- Optional NVD API sync
- Auto-remediation on version updates
- Manual resolve/ignore workflow
- Per-device vulnerability state tracking

**Network Monitoring (SNMP)**
- SNMP v1/v2c/v3 polling
- Network device discovery (ARP/SNMP)
- NetFlow v9/IPFIX/sFlow collection
- Interface status and bandwidth tracking
- Standalone probe binary

**Security & Compliance**
- Multi-factor authentication (TOTP/RFC 6238)
- Row-Level Security for all tables
- NATS subject isolation per tenant
- Per-tenant AES-256-GCM encryption keys
- Immutable audit log
- Rate limiting per endpoint
- Security headers (CSP, nosniff, X-Frame-Options)
- Request body size limits (10MB)
- Cosign keyless signing for releases
- SPDX SBOM for all artifacts

**Reporting**
- PDF report generation with executive summary
- Configurable sections: alerts, CVEs, patches
- Scheduled delivery (daily/weekly/monthly)
- On-demand generation
- Storage to MinIO/S3

**Web UI**
- Dark mode with theme toggle
- Platform overview dashboard
- Priority issues widget
- Per-customer drill-down (devices, alerts, CVEs, recordings, settings)
- User management with tenant scoping
- Script library with execution history
- Software package library + deployment
- Third-party patching management
- Report schedules + generated reports
- MFA enrollment flow
- Collapsible sidebar with icons
- Toast notifications, skeleton loaders
- Responsive design

**Administration**
- User authentication (email/password + JWT)
- RBAC: admin, technician, viewer roles
- Team/tenant scoping for users
- Customer onboarding with deployment ID
- Orchestrator self-update from GitHub
- User management with create/scope/delete
- Audit log with access review
- Platform overview with aggregate stats

**Deployment Options**
- Docker Compose (recommended for single-server)
- Bare metal Linux (systemd service)
- Kubernetes (Helm chart)
- KOTS (self-hosted marketplace)
- Air-gapped deployments
- Multi-region support

## API

The platform exposes 60+ REST endpoints. Key categories:
- Auth: login, me, MFA enrollment
- Platform: overview, customers, update
- Admin: users, customers, update
- Alerts: active, history, rules CRUD, acknowledge
- Vulnerabilities: per-device, per-tenant, summary, resolve/ignore
- CVE: stats, sync, packages, package detail
- Scripts: CRUD, run, executions, result detail
- Software: packages CRUD, deployments CRUD
- Third-party: apps list, sync, packages
- Recordings: list, playback, delete
- Keys: CRUD, rotate, revoke
- Access: audit log, users, permissions
- Remote: session start/stop

## Project Structure

```
├── cmd/                    # CLI commands
│   ├── agent/              # Agent startup + update + scripts + software + remote
│   ├── probe/              # SNMP/flow network probe
│   └── orchestrator/       # Platform services + update
├── internal/
│   ├── agent/              # Agent core, collectors, comms, update, scripts, software, remote
│   ├── alerting/           # Alert engine + notifications
│   ├── inventory/          # Device CRUD, CVE sync, vulnerability, third-party
│   ├── monitoring/         # Metrics/events/heartbeat ingestion
│   ├── patch/              # Patch management + executors
│   ├── platform/           # API server + all route handlers
│   ├── probe/              # SNMP, flow, discovery
│   ├── remote/             # Tunnel relay, recording, cleanup
│   ├── reporting/          # PDF report engine
│   └── update/             # Agent + orchestrator update clients
├── pkg/
│   ├── auth/               # JWT, enrollment, TOTP/MFA, rate limiting
│   ├── encrypt/            # Per-tenant AES-256-GCM keys
│   ├── postgres/           # Schema migrations (1-24)
│   ├── storage/            # Backend interface: MinIO, S3, Local, Mock
│   └── timescale/          # TimescaleDB client + migrations
├── deploy/
│   ├── docker/             # Docker Compose (NATS, TimescaleDB, MinIO, Grafana)
│   ├── helm/               # K8s Helm chart + AgentUpdateChannel CRD
│   ├── grafana/            # Provisioned dashboards
│   └── kots/               # KOTS integration config
├── docs/
│   ├── ARCHITECTURE.md     # Architecture plan
│   ├── INSTALL.md          # Installation guide
│   ├── RUNBOOK.md          # Operations runbook
│   ├── SECURITY.md         # Security architecture
│   └── SOC2.md             # SOC 2 compliance evidence
├── scripts/
│   ├── install.sh          # Linux agent installer
│   ├── build-msi.sh        # Windows MSI builder
│   ├── smoke_test.sh       # End-to-end smoke test
│   └── loadtest.sh         # Performance load test
├── ui/                     # React 18 + TypeScript + Tailwind web UI
└── .github/workflows/      # CI + release pipelines
```

## Development

```bash
# Prerequisites: Go 1.25+, Docker, Node 20+
make build              # Build all binaries
make test               # Run all tests
make lint               # Go vet
make coverage           # Test coverage report
make dev                # Start services + orchestrator
make smoke-test         # Run end-to-end smoke test
```

## Deployment

| Method | Docs |
|--------|------|
| Docker Compose | `docker compose up -d` |
| Bare Metal Linux | `docs/INSTALL.md` |
| Kubernetes Helm | `deploy/helm/strata-rmm/` |
| KOTS | `deploy/kots/` |

See [docs/INSTALL.md](docs/INSTALL.md) for detailed instructions.

## License

See [LICENSE](LICENSE) file.
