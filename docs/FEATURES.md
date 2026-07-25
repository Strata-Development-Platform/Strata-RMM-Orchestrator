# Strata RMM — Complete Feature Reference

## Overview

Strata RMM is a horizontally-scalable, multi-tenant Remote Monitoring & Management platform with cross-platform Go agents. It supports both SaaS and self-hosted deployments and is designed to match the capabilities of Kaseya VSA, Datto RMM, and NinjaRMM.

---

## 1. Agent Management

### Agent Deployment

| Platform | Method | Details |
|----------|--------|---------|
| Linux | Bash script | `curl ... | sudo bash -s -- --deployment-id X` → systemd service |
| Windows | MSI installer | WiX-based, service registration via sc.exe, PATH, Start Menu |
| macOS | Manual binary | Cross-compiled darwin/amd64 + arm64, ad-hoc codesign |

### Agent Identity

- **ECDSA P256 keypair** generated on first run
- **Deployment ID** ties agent to customer on first registration
- **JWT authentication** for all subsequent NATS communications
- Identity persisted to disk, survives restarts

### Agent Auto-Update

- Manifest-based update system with cosign signature verification
- **Staged rollout**: canary percentage, pause/resume, version approval/blocking
- **Automatic rollback** on health check failure (3 consecutive)
- BBolt-persisted update state (current/pending/rollback versions)
- NATS-controlled rollout commands per agent
- Configurable channel: stable/beta/alpha

### Agent Communication

| Subject Pattern | Direction | Content |
|----------------|-----------|---------|
| `tenant.{id}.agent.{id}.heartbeat` | Agent → Platform | Status, version, timestamp (30s interval) |
| `tenant.{id}.agent.{id}.metrics` | Agent → Platform | System metrics (60s interval) |
| `tenant.{id}.agent.{id}.events` | Agent → Platform | Events (startup, errors, updates) |
| `tenant.{id}.cmd.{id}` | Platform → Agent | Commands: tunnel, script, software, remote |
| `tenant.{id}.agent.{id}.script.result` | Agent → Platform | Script execution results |
| `tenant.{id}.agent.{id}.software.result` | Agent → Platform | Software install/uninstall results |
| `tenant.{id}.tunnel.{id}.frame` | Agent → Platform | Remote control JPEG frames |
| `tenant.{id}.tunnel.{id}.input` | Platform → Agent | Remote control input events |
| `tenant.{id}.tunnel.{id}.ctrl` | Bidirectional | Session control messages |

### Offline Queue

- BBolt-backed store-and-forward for metrics and events when NATS is disconnected
- Automatic replay on reconnect
- Configurable queue size limits

---

## 2. Remote Monitoring

### System Metrics (collected every 60s by default)

| Category | Metrics Collected |
|----------|-----------------|
| **CPU** | `cpu.percent`, `cpu.cores`, `load.1`, `load.5`, `load.15` |
| **Memory** | `memory.total`, `memory.used`, `memory.available`, `memory.percent`, `swap.total`, `swap.used`, `swap.percent` |
| **Disk** | Per partition: `disk.total`, `disk.used`, `disk.free`, `disk.percent` |
| **Disk I/O** | Per disk: `diskio.read_count`, `diskio.write_count`, `diskio.read_bytes`, `diskio.write_bytes`, `diskio.iops_in_progress` |
| **Network** | Per interface: `net.bytes_sent`, `net.bytes_recv`, `net.packets_sent`, `net.packets_recv`, `net.errors_in`, `net.errors_out` |
| **System** | `system.uptime`, `system.info` (hostname, OS, arch, platform) |

### TimescaleDB Storage

- **Hypertables**: `metrics`, `events`, `heartbeats`, `alerts`, `alerts_ts`
- **Continuous aggregates**: 1-minute and 1-hour downsampling
- **Compression**: Enabled after 7 days
- **Retention**: Configurable per metric type
- **Grafana dashboards**: System metrics + alerts overview

---

## 3. Alerting

### Rule Types

| Type | Description |
|------|-------------|
| **Threshold** | Fire when metric value crosses a threshold (GT/GTE/LT/LTE/EQ/NEQ) |
| **Heartbeat** | Fire when device hasn't reported in N minutes |

### Alert State Machine

```
OK → Firing → Resolved
  └── Cooldown prevents re-fire within N minutes
  └── Auto-resolves when condition clears
```

### Severity Levels

- `critical` — Immediate action required
- `warning` — Should be addressed
- `info` — Informational only

### Notification Channels

| Channel | Configuration |
|---------|--------------|
| Slack | Webhook URL |
| Webhook | Custom URL + payload template |
| Email | SMTP server + recipient list |
| Microsoft Teams | Webhook URL |
| PagerDuty | Integration key |

### CVE Alerts

- Automatic alert firing when critical/high CVEs detected on devices
- Auto-resolution when device patches the affected package
- Severity-based: critical, high → alert; medium, low → log only

---

## 4. Remote Access

### Built-in Remote Control

| Feature | Details |
|---------|---------|
| **Screen capture** | Linux X11 (ImageMagick), Windows (DirectX/BitBlt), macOS (CGDisplay), software test pattern |
| **Input injection** | Linux xdotool, Windows SendInput, macOS CGEvent |
| **Frame encoding** | JPEG, 5-10 FPS adaptive, configurable quality (10-100) |
| **Transport** | NATS subjects: frame/input/control |
| **Web viewer** | HTML5 canvas with JPEG rendering, toolbar with quality/FPS sliders |
| **Session lifecycle** | Start → active → disconnect (auto-cleanup) |

### RDP/SSH/VNC Tunnel

- NATS-relayed bidirectional streaming
- Protocol detection with default ports (RDP:3389, SSH:22, VNC:5900)
- JSON handshake protocol
- Byte counting for session tracking

### Session Recording

| Feature | Details |
|---------|---------|
| **Capture** | Raw byte-level session recording |
| **Storage** | MinIO, S3, or local filesystem via abstract Backend interface |
| **Verification** | SHA256 checksum computed on upload, stored in DB |
| **Playback** | Presigned URL (1-hour TTL), requires MFA code |
| **Retention** | Configurable per-tenant (default 90 days), automatic cleanup job |
| **Encryption** | Optional SSE-KMS (S3) or SSE-S3 (MinIO) |

---

## 5. Scripting Engine

### Supported Languages

| Language | Execution Method |
|----------|-----------------|
| **PowerShell** | `powershell -NoProfile -ExecutionPolicy Bypass -Command` |
| **Bash** | `/bin/bash -c` |
| **Python** | `python3 -c` |
| **Batch** | `cmd.exe /C <tempfile.bat>` |

### Features

| Feature | Details |
|---------|---------|
| **Parameter interpolation** | `{{param_name}}` syntax in script content |
| **Timeout enforcement** | Configurable per-script (default 300s) |
| **Output capture** | stdout + stderr, truncated at 100KB |
| **Execution history** | Full output viewer in Web UI |
| **Run on demand** | Select target devices, fill parameters, execute |
| **NATs dispatch** | Commands via `tenant.{id}.cmd.{id}`, results via `script.result` |
| **Script library** | Saved scripts with name, description, language, parameters |

---

## 6. Software Deployment

### Supported Package Types

| Type | Install Command |
|------|----------------|
| **MSI** | `msiexec.exe /i <file.msi> /quiet /norestart` |
| **EXE** | Direct execution with arguments |
| **DEB** | `dpkg -i <file.deb>` |
| **RPM** | `rpm -ivh <file.rpm>` |
| **AppImage** | Direct execution |
| **Script** | Shell execution |

### Features

| Feature | Details |
|---------|---------|
| **SHA256 verification** | Checksum match before install |
| **Silent install** | Pre-configured arguments per package type |
| **Multi-device deploy** | Select any number of target devices |
| **Per-device status** | pending → downloading → installing → success/failed |
| **Deployment history** | Full audit of all deployments with results |
| **Uninstall support** | MSI `/x`, DEB `dpkg -r`, RPM `rpm -e` |

### Third-Party Patching

| Application | Auto-Discovery Method |
|-------------|----------------------|
| Google Chrome | `versionhistory.googleapis.com` API |
| Mozilla Firefox | `product-details.mozilla.org` JSON |
| Adobe Acrobat Reader | Adobe release notes |
| 7-Zip | Download page parsing |
| VLC Media Player | VideoLAN directory listing |
| Microsoft Teams | Teams config API |
| Zoom | Zoom download API |
| Notepad++ | GitHub releases API |
| LibreOffice | Download page parsing |

All third-party apps are auto-discovered (24h interval), auto-packaged as deployable software_packages, and ready for installation via the software deployment system.

---

## 7. Vulnerability Management

### CVE Feed

| Feed | Coverage | Auth Required |
|------|----------|--------------|
| **OSV.dev** (primary) | 17 tracked packages, broad OSS coverage | None |
| **NVD** (optional) | Comprehensive CVE database | Free API key |

### Scan Process

1. Load CVE records from database (auto-synced every 6 hours)
2. Query device software inventory from `patch_inventory` snapshots
3. Match installed packages against CVE database
4. Record new vulnerabilities with severity, current version, fixed version
5. Auto-remediate when device reports updated package version
6. Fire alerts for critical/high severity findings

### Remediation Workflows

| Action | Description |
|--------|-------------|
| **Auto-remediate** | When device version ≥ fixed_in version, status → patched |
| **Manual resolve** | `POST /api/v1/vulnerabilities/{id}/resolve` |
| **Manual ignore** | `POST /api/v1/vulnerabilities/{id}/ignore` |

---

## 8. Network Monitoring

### SNMP Probe

| Feature | Details |
|---------|---------|
| **Versions** | v1, v2c, v3 with auth/priv |
| **Polling** | Configurable interval per target |
| **MIB** | CPU, memory, disk, interfaces, uptime |
| **Discovery** | ARP scan + SNMP walk |
| **Flow collection** | NetFlow v9, IPFIX, sFlow |

### Probe Deployment

- Standalone Go binary, same as agent
- Per-customer deployment on monitoring server
- Reports via NATS to orchestrator
- Configurable SNMP targets per customer

---

## 9. Security

### Authentication

| Method | Details |
|--------|---------|
| **User login** | Email/password → bcrypt → JWT (HS256, 8h TTL) |
| **Agent auth** | ECDSA P256 keypair + JWT |
| **MFA/TOTP** | RFC 6238, HMAC-SHA1, 6-digit, 30s window, ±30s skew |
| **API keys** | Machine-to-machine, revocable |

### Authorization

| Layer | Mechanism |
|-------|-----------|
| **Database** | PostgreSQL Row-Level Security (`current_setting('app.tenant_id')`) |
| **NATS** | Subject isolation (`tenant.{id}.*`) |
| **API** | JWT claim validation |
| **RBAC** | admin, technician, viewer roles |

### Multi-Tenant Isolation

- All tables have RLS policies enforcing `tenant_id`
- NATS subjects scoped per tenant
- Per-tenant encryption keys (AES-256-GCM)
- User access limited to assigned tenants

### Rate Limiting

| Endpoint | Rate | Burst |
|----------|------|-------|
| `/health` | 100/s | 200 |
| `/api/v1/enroll` | 5/s | 10 |
| `/api/v1/mfa` | 10/s | 20 |
| `/api/v1/recordings` | 20/s | 40 |
| Other | 30/s | 60 |

### Supply Chain Security

- Cosign keyless signing (OIDC-based)
- SPDX SBOM for all releases
- Multi-arch Docker images signed with cosign
- Trivy + Gosec scans in CI

---

## 10. Reporting

### Report Sections

| Section | Content |
|---------|---------|
| **Executive Summary** | Colored stat boxes: device count, online %, alert count, CVE count |
| **Active Alerts** | Table with severity, message, device, timestamp |
| **Open CVEs** | Table with CVE ID, severity, package, device |
| **Patch Status** | Deployment compliance summary |

### Scheduling

| Frequency | Description |
|-----------|-------------|
| Daily | Generated every 24h |
| Weekly | Generated on specified day |
| Monthly | Generated on specified day of month |

### Features

- On-demand generation via API
- Auto-stored to MinIO/S3
- Historical report listing in Web UI
- Configurable sections per schedule

---

## 11. Administration

### User Management

| Feature | Details |
|---------|---------|
| **Create user** | Email + password + role + tenant scope |
| **Roles** | admin (full access), technician (customer management), viewer (read-only) |
| **Tenant scoping** | Each user assigned to specific customers |
| **Active/inactive** | Disable access without deleting |

### Customer Onboarding

1. Admin creates customer → system generates `deployment_id`
2. Install command: `curl ... | sudo bash -s -- --deployment-id X`
3. Agent first check-in → auto-registers device in customer inventory

### Orchestrator Self-Update

| Mode | Behavior |
|------|----------|
| **Bare metal** | Download binary → SHA256 verify → atomic swap → health check → systemctl restart |
| **Docker** | Prints: `docker compose pull && docker compose up -d` |
| **Kubernetes** | Prints: `kubectl rollout restart deployment/strata-rmm` |

### Audit Log

- All API requests logged: timestamp, user, action, resource, IP address
- Immutable append-only table
- Accessible via API: `GET /api/v1/access/audit/{tenantID}`

---

## 12. Encryption Keys

### Key Management

| Operation | Description |
|-----------|-------------|
| **Create** | Generate 32-byte AES-256 key, store encrypted |
| **List** | Show all keys with status for tenant |
| **Get active** | Return currently active encryption key |
| **Rotate** | Create new key, mark old as rotating |
| **Revoke** | Mark key as compromised, disable |

### Supported Providers

| Provider | Use Case |
|----------|----------|
| **Local** | Self-hosted, key material in DB |
| **AWS KMS** | SaaS deployments with AWS |
| **GCP KMS** | SaaS deployments with GCP |
| **Azure Key Vault** | SaaS deployments with Azure |

---

## 13. Web UI

### Pages

| Page | Route | Description |
|------|-------|-------------|
| Login | `/login` | Email/password authentication |
| Dashboard | `/` | Platform overview with stat cards, priority issues, customer table |
| Customers | `/customers` | Customer list with deployment IDs, device/alert/CVE counts |
| Customer Detail | `/customers/:id` | Tabbed drill-down: Devices, Alerts, Vulnerabilities, Recordings, Settings |
| Remote Control | `/remote/:tid/:did` | Live screen view with quality/FPS controls |
| Users | `/admin/users` | User management with create + tenant scoping |
| Scripts | `/scripts` | Script library, editor, run dispatch, execution history |
| Software | `/software` | Package library, deployments, deploy modal |
| Patch Mgmt | `/thirdparty` | Third-party app list, sync controls |
| Reports | `/reports` | Generated reports, schedule management |
| Settings | `/admin/settings` | Platform status, CVE sync, API reference |
| My Settings | `/settings` | Profile info, MFA enrollment flow |

### UI Components

| Component | Purpose |
|-----------|---------|
| **Toast** | Success/error/info notifications, 4s auto-dismiss |
| **Skeleton** | Pulsing table/card/text loading placeholders |
| **ConfirmDialog** | Modal confirmation for destructive actions |
| **EmptyState** | Empty table/grid state with icon, message, CTA |
| **StatusBadge** | Colored pill with dot indicator for any status |
| **ThemeToggle** | Dark/light mode with localStorage persistence |
| **DataTable** | Sortable, filterable tables with row actions |

---

## 14. Deployment

### Docker Compose

```yaml
Services: NATS, TimescaleDB, Orchestrator, MinIO, Grafana (optional)
Ports:    8080 (API), 4222 (NATS), 5432 (DB), 9000 (MinIO), 3000 (Grafana)
Data:     Persistent volumes for DB, MinIO, Grafana
```

### Bare Metal Linux

```bash
Components: NATS 2.10+, TimescaleDB 2.15+ PG16, MinIO (optional)
Systemd:    strata-rmm.service with auto-restart
User:       Dedicated strata-rmm system user
Paths:      /usr/local/bin (binary), /var/lib/strata-rmm (data), /etc/strata-rmm (config)
```

### Kubernetes

- Helm chart with HPA, PDB, network policies, ingress
- Air-gapped deployment support
- Multi-region configuration
- AgentUpdateChannel CRD

---

## 15. API Reference

Total: **60+ REST endpoints**

| Category | Endpoints | Base Path |
|----------|-----------|-----------|
| Auth | 2 | `/api/v1/auth/` |
| Platform | 2 | `/api/v1/platform/` |
| Admin | 4 | `/api/v1/admin/` |
| Agents | 1 | `/api/v1/agent/` |
| Alerts | 6 | `/api/v1/alerts/`, `/api/v1/rules/` |
| Vulnerabilities | 5 | `/api/v1/vulnerabilities/` |
| CVE | 7 | `/api/v1/cve/` |
| Scripts | 7 | `/api/v1/scripts/` |
| Software | 6 | `/api/v1/software/` |
| Third-party | 4 | `/api/v1/thirdparty/` |
| Recordings | 3 | `/api/v1/recordings/` |
| Keys | 5 | `/api/v1/keys/` |
| Access | 3 | `/api/v1/access/` |
| MFA | 4 | `/api/v1/mfa/` |
| Remote | 2 | `/api/v1/remote/` |
| Reports | 5 | `/api/v1/reports/` |

---

## 16. Database

### Schema Migrations (1-24)

| Migration | Table(s) | Purpose |
|-----------|----------|---------|
| 1 | `tenants` | Customer organizations |
| 2 | `users` | User accounts with RBAC |
| 3 | `devices` | Managed endpoints |
| 4 | `permissions` | RBAC permission matrix |
| 5 | RLS policies | Multi-tenant isolation |
| 6 | `enrollment_tokens` | Agent enrollment (legacy) |
| 7 | `audit_log` | Immutable audit trail |
| 8 | `alert_rules` | Alerting rule definitions |
| 9 | `notification_channels` | Notification delivery config |
| 10 | `maintenance_windows` | Scheduled maintenance |
| 11 | `patch_policies` | Patch management policies |
| 12 | `patch_deployments` | Patch deployment tracking |
| 13 | `patch_inventory` | Device software snapshots |
| 14 | `cve_database` + `device_vulnerabilities` | CVE data + device mapping |
| 15 | Seed CVE data | 10 initial CVE records |
| 16 | `mfa_secrets` | TOTP secret store |
| 17 | `session_recordings` | Recording metadata |
| 18 | `cve_sync_state`, `cve_package_ecosystem` | CVE sync tracking |
| 19 | `tenant_encryption_keys` | Per-tenant encryption keys |
| 20 | `user_tenant_access`, `audit_auth` | User scoping + auth audit |
| 21 | `agent_registrations` | Agent registration records |
| 22 | `scripts`, `script_executions` | Scripting engine |
| 23 | `software_packages`, `software_deployments`, targets | Software deployment |
| 24 | `report_schedules`, `generated_reports` | Reporting engine |

---

## 17. CI/CD

### Continuous Integration (`.github/workflows/ci.yml`)

| Step | Tool |
|------|------|
| Lint | golangci-lint |
| Vet | `go vet` |
| Test | `go test -race -cover` |
| Build | Matrix: linux/windows/darwin × amd64/arm64 |
| Security | Trivy + Gosec |

### Release (`.github/workflows/release.yml`)

| Step | Tool |
|------|------|
| Build + archive | GoReleaser |
| Sign binaries | Cosign (keyless, OIDC) |
| SBOM | Syft (SPDX format) |
| Docker build + push | Docker Buildx (multi-arch) |
| Sign Docker image | Cosign |

---

## 18. Testing

### Unit Tests

- **57 Go tests** across all packages
- Agent: update manifest, store, rollout, download
- Storage: MinIO, S3, Local, Mock contract tests
- Auth: TOTP generation/validation, rate limiting, API keys
- Inventory: CVE sync, version parsing, vulnerability matching
- Remote: recording, concurrent writes, multiple sessions
- Encryption: AES-256-GCM encrypt/decrypt, key derivation

### Smoke Test

- `scripts/smoke_test.sh` validates:
  - Health endpoint
  - Agent enrollment
  - CVE database stats
  - MFA enrollment
  - Recording list
  - Access audit, users, permissions
  - Encryption key creation
  - Vulnerability summary

### Load Test

- `scripts/loadtest.sh` simulates:
  - API load via vegeta (100 req/s)
  - NATS agent simulation (500 agents)
  - Alert storm (100 threshold crossings)
  - CVE scan, recording, key rotation, access review
