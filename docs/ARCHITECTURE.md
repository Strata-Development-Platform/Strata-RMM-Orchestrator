# Strata RMM Platform — Architecture

**Version:** 2026-08-08 (after PR #110)
**Last Updated:** 2026-08-08
**Status:** Internal-alpha code-complete

---

## 1. Platform Overview

Strata is a single-process orchestrator (Go 1.22+) with a cross-platform agent (Go). The orchestrator is a monolithic HTTP API server backed by PostgreSQL/TimescaleDB, NATS JetStream for messaging, and optional Redis for token blacklisting. All platform services run in the same process — no microservice decomposition.

**Architecture:** Monolith with modular internal packages
**Language:** Go (stdlib net/http, no framework)
**Deployment:** Docker container, Kubernetes (Helm), bare metal (systemd)
**Agent:** Cross-platform binary (Windows, Linux, macOS)

---

## 2. Process Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     ORCHESTRATOR (Go)                           │
│                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │  HTTP Server │  │  NATS Client │  │  Postgres    │         │
│  │  (std lib)   │  │  (JetStream) │  │  + Timescale │         │
│  │              │  │              │  │              │         │
│  │  Routes:     │  │  Streams:    │  │  Tables:     │         │
│  │  - /health   │  │  - agent     │  │  - tenants   │         │
│  │  - /api/v1   │  │  - metrics   │  │  - devices   │         │
│  │  - /api/v2   │  │  - events    │  │  - alerts    │         │
│  │  - /releases │  │  - cmds      │  │  - users     │         │
│  │              │  │  - platform  │  │  - policies  │         │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘         │
│         │                 │                 │                 │
│  ┌──────▼─────────────────▼─────────────────▼─────────────────▼──┐│
│  │                    PACKAGES (internal/)                       ││
│  │                                                                 ││
│  │  platform/     - HTTP handlers, middleware, route registration ││
│  │  alerting/     - Rule engine, grouping, notification           ││
│  │  automation/   - Git-backed vault, AES-256-GCM encryption     ││
│  │  collectors/   - System metrics (CPU, mem, disk, net)          ││
│  │  groups/       - Smart groups DSL evaluator                    ││
│  │  integrations/ - Webhooks (EDR, Backup, PSA)                   ││
│  │  inventory/    - CMDB, third-party catalog, CVE/vulnerability  ││
│  │  messaging/    - JetStream consumer/manager, NATS subjects     ││
│  │  monitoring/   - Telemetry ingestion pipeline                  ││
│  │  observability/- HTTP health/synthetics                        ││
│  │  orchestrator/ - Power events, role-based policy binding       ││
│  │  patch/        - OS patch management (Chocolatey, apt, etc.)   ││
│  │  probe/        - Network discovery (SNMP, ARP, ping, scan)     ││
│  │  remote/       - Session management, capture, input injection  ││
│  │  reporting/    - PDF report generation (gofpdf)                ││
│  │  resilience/   - Circuit breakers, retry logic                 ││
│  │  synthetic/    - Synthetic monitoring checks                   ││
│  │  webrtc/       - WebRTC session management, recording, transcr ││
│  │  lancache/     - Local package distribution cache              ││
│  │                                                                 ││
│  │  agent/      - Agent-side modules (core, scripts, software)    ││
│  │                                                                 ││
│  │  pkg/auth/     - JWT, TOTP/MFA, API keys, token blacklisting   ││
│  │  pkg/backup/   - Backup engine, quiescer, coordinator          ││
│  │  pkg/config/   - Configuration loading/validation              ││
│  │  pkg/encrypt/  - AES-256-GCM envelope encryption               ││
│  │  pkg/postgres/ - Connection pool, schema migration, upgrade    ││
│  │  pkg/recovery/ - Disaster recovery key management              ││
│  │  pkg/redis/    - Redis client, agent registry                  ││
│  │  pkg/storage/  - S3/MinIO/Local backend interface              ││
│  │  pkg/timescale/ - Hypertable management, batch writer          ││
│  │  pkg/repository/ - Filesystem/S3 repository abstraction        ││
│  └─────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────┘
```

---

## 3. Data Flow

### 3.1 Agent → Platform (Heartbeat + Telemetry)

```
Agent → NATS JetStream → Platform Consumer → TimescaleDB/PostgreSQL
  │
  ├── heartbeat.{tenant}.{agent}   (every 60s)
  ├── metrics.{tenant}.{agent}     (every 60s, batch)
  ├── events.{tenant}.{agent}      (on occurrence)
  └── script_result.{tenant}.{agent}  (after script execution)
```

### 3.2 Platform → Agent (Commands)

```
Platform → NATS JetStream → Agent Subscribe
  │
  └── cmd.{tenant}.{agent}  (request/response pattern)
```

### 3.3 Remote Access Flow

```
Platform ↔ NATS WebRTC signaling ↔ Agent (WebRTC peer)
  │                                    │
  ├── Session creation (POST /webrtc/sessions)
  ├── ICE candidate exchange
  ├── P2P media (or TURN relay if P2P fails)
  └── Recording/transcription (optional)
```

### 3.4 Snapshot/Video Capture

```
Agent → NATS (capture frames) → Platform (relay to browser)
  │
  ├── Capture (scrot/magicavoxel/screencapture depending on OS)
  ├── Input injection (xdotool/CGEvent/SendInput depending on OS)
  └── Session recording (WebM/MKV, stored in object storage)
```

---

## 4. Services & Modules

### 4.1 Platform API (`internal/platform/`)

HTTP API with 310+ routes organized by domain:

| Domain | Routes | Description |
|--------|--------|-------------|
| Auth | `/api/v1/auth/*` | Login, logout, JWT tokens, TOTP/MFA, invitations |
| Enrollment | `/api/v1/enroll`, `/api/v1/agent/*` | Agent registration, config, tokens |
| Devices | `/api/v1/devices/*`, `/api/v2/devices/*` | Device CRUD, inventory, packages, dependencies |
| Tenants | `/api/v1/tenants/*`, `/api/v2/clients/*` | Tenant management, client support |
| MSP | `/api/v2/msps/*`, `/api/v2/platform/msps/*` | MSP lifecycle, billing, memberships, offboarding |
| Platform | `/api/v1/platform/*`, `/api/v2/platform/*` | Overview, customers, provider profile, domains |
| Alerts | `/api/v1/alerts/*` | Rule CRUD, alert list/history/grouping/resolve |
| Policies | `/api/v1/policies/*` | Policy CRUD, validation, preview, publish, effective, diff |
| Smart Groups | `/api/v1/device-groups/*` | Group CRUD, evaluation, members, script bindings |
| Scripts | `/api/v1/scripts/*`, `/api/v1/tenants/*/scripts/*` | Script CRUD, execution, schedules |
| Patch | `/api/v1/patch-*` | Patch policy CRUD, deployments, inventory |
| Vulnerability | `/api/v1/vulnerabilities/*`, `/api/v1/cve/*` | CVE sync, package management, vulnerability listing |
| Remote | `/api/v1/remote/*`, `/api/v1/webrtc/*` | Interactive sessions, recording, WebRTC sessions |
| Third-party | `/api/v1/thirdparty/*` | App/package/vendor sync, version discovery |
| Maintenance | `/api/v1/maintenance-windows/*` | Window CRUD |
| Retention | `/api/v1/retention/*`, `/api/v1/tenants/*/retention` | Retention policy CRUD |
| Reports | `/api/v1/reports/*` | PDF generation, schedules, compliance |
| Remediation | `/api/v1/remediation/*` | Remediation history, summary, policy |
| Jobs | `/api/v1/jobs/*` | Durable job CRUD, events, cancel, retry |
| Integrations | `/api/v1/integrations/*` | Webhook endpoints (EDR, Backup, PSA) |
| Software | `/api/v1/software/*` | Package CRUD, deployments |
| Admin | `/api/v1/admin/*` | User CRUD, customer CRUD, update management |
| Health | `/health`, `/health/live`, `/health/ready`, `/metrics` | Health checks, Prometheus metrics |

### 4.2 Alerting Engine (`internal/alerting/`)

- **Rule Types**: Threshold, Composite, Heartbeat
- **Grouping**: Severity, Device, Cascade, Time-window (PR #81)
- **Notifications**: Email (SMTP), Slack, Teams, PagerDuty, Webhook
- **Maintenance Windows**: Device/tenant-level silence periods
- **Storage**: PostgreSQL (rules, alerts, groups)

### 4.3 Policy Engine (`internal/policy/`, `internal/platform/policy_*`)

- **Hierarchical**: Global → MSP → Client → Device scope (PR #57)
- **Lifecycle**: Create → Validate → Preview → Publish → Rollback (PR #58)
- **Effective Policy**: Recursive merge with most-specific-wins (PR #57)
- **Scheduler**: Automated enforcement on intervals (PR #50)
- **DSL**: Expression evaluator for smart groups (PR #73, 13 operators)

### 4.4 Script Vault & Automation (`internal/automation/`)

- **Git-backed**: SSH/HTTPS clone/pull from GitHub/GitLab (PR #59)
- **Encryption**: AES-256-GCM envelope for secret variables (PR #61)
- **Script Engine**: PS/Bash/Python/Batch execution via NATS (PR #102)
- **Scheduling**: Recurring schedules (hourly/daily/weekly/monthly)

### 4.5 Smart Groups (`internal/groups/`)

- **DSL Evaluator**: 13 operators (eq, neq, gt, gte, lt, lte, contains, startswith, in, contains_any, is_null, not_null, regex)
- **Nested Expressions**: AND/OR combinations
- **Evaluation**: On-demand and scheduled
- **Memberships**: Cached in `group_memberships` table (PR #74)

### 4.6 Network Probe (`internal/probe/`)

- **Discovery**: ARP, ping scan, port scan (TCP SYN/UDP), service detection
- **Protocols**: SNMP v3, NetFlow v5/v9/IPFIX, Redfish, IPMI, Syslog
- **Topology**: LLDP/CDP/STP stubs
- **Device Detection**: Vendor/model/OS fingerprinting

### 4.7 Vulnerability Management (`internal/inventory/`)

- **CVE Sync**: OSV.dev (primary), NVD API (optional) (PR #105)
- **Version Matching**: Semantic version comparison
- **Remediation Engine**: Auto-remediation via patch executor
- **Compliance Reports**: CSV/JSON export (PR #111)

### 4.8 Remote Support (`internal/remote/`, `internal/webrtc/`)

- **WebRTC**: P2P video sessions with TURN/STUN relay (PR #88)
- **Session Recording**: WebM/MKV, stored in object storage
- **Live Transcription**: OpenAI Whisper, Azure Speech, Google Speech
- **Screen Capture**: Windows (BitBlt), macOS (screencapture), Linux (scrot/import/gnome-screenshot)
- **Input Injection**: Windows (SendInput), macOS (CGEvent), Linux (xdotool)
- **API**: 15 REST endpoints, 80+ unit tests

### 4.9 LAN Cache (`internal/lancache/`)

- **Package Distribution**: Chocolatey, Winget, Flatpak, Snap, MSI, DEB, RPM
- **Local Cache**: Reduces bandwidth for multi-device deployments
- **5 REST endpoints**, 45 unit tests

### 4.10 Patch Management (`internal/patch/`)

- **Platforms**: Windows (PowerShell/WSUS/Chocolatey/Winget), Linux (apt/dnf/yum/zypper)
- **Canary Deployment**: Small subset first, validate, then full rollout
- **Rollback**: Automatic on canary failure
- **Policy Binding**: Severity-based, maintenance windows

### 4.11 Software Deployment (`internal/agent/software/`)

- **Package Types**: MSI, EXE, DEB, RPM, AppImage, Script (PR #89)
- **Checksum Verification**: SHA256
- **Multi-device Deployment**: Lifecycle with timeout handling

### 4.12 CMDB (`internal/inventory/`)

- **Device Relationships**: Parent/child dependency mapping (PR #80)
- **Third-party Catalog**: Vendor discovery, version sync (PR #91)
- **Software Inventory**: Package tracking per device

### 4.13 Billing & MSP Lifecycle (`internal/platform/`)

- **Provider Profile**: MSP registration, branding
- **Billing**: Accounts, subscriptions, payment methods, invoices, usage meters
- **Entitlements**: Feature flags, plan tiers
- **Offboarding**: Client/MSP archival, data preservation
- **Client Portal**: Support requests, SSO providers (future)

### 4.14 Backup & Disaster Recovery (`pkg/backup/`)

- **Engine**: PostgreSQL backup with AES-256-GCM encryption
- **Repository**: Filesystem or S3-compatible (MinIO, AWS S3)
- **Recovery**: Full restore with NATS reconnection
- **Quiescer**: Graceful service shutdown during backup

---

## 5. Agent Architecture

### 5.1 Design

- **Single static binary**: Cross-compiled for Windows/Linux/macOS
- **Zero external dependencies**: Standalone execution
- **Self-update**: Manifest-based with cosign keyless signing
- **Modular**: Core + pluggable collectors

### 5.2 Communication

```
Agent ↔ NATS JetStream (outbound-only)
  ├── heartbeat.{tenant}.{agent}     (60s interval)
  ├── metrics.{tenant}.{agent}       (60s interval, batch)
  ├── events.{tenant}.{agent}        (on occurrence)
  ├── cmd.{tenant}.{agent}           (subscribe, receive commands)
  └── software_result.{tenant}.{agent} (after execution)
```

### 5.3 Agent Modules

| Module | Path | Description |
|--------|------|-------------|
| Core | `internal/agent/core/` | Identity, config, store, role scanner |
| Comms | `internal/agent/comms/` | NATS client, reconnect, backoff |
| Scripts | `internal/agent/scripts/` | Script execution (PS/Bash/Python) |
| Software | `internal/agent/software/` | MSI/EXE/DEB/RPM/AppImage install |
| Jobs | `internal/agent/jobs/` | Durable job handler, ledger |
| Update | `internal/agent/update/` | Self-update, manifest, rollout |
| Remote | `internal/agent/remotecontrol/` | Capture, input injection per platform |

### 5.4 Collectors

| Collector | Path | Description |
|-----------|------|-------------|
| System | `internal/agent/collectors/system.go` | CPU, RAM, disk, net (gopsutil) |
| Software | `internal/collectors/software.go` | Installed packages, versions |
| Helpers | `internal/collectors/helpers.go` | Shared utility functions |

### 5.5 Platform Support

| Platform | Arch | Package |
|----------|------|---------|
| Windows 10/11 | amd64, arm64 | EXE, MSI |
| Windows Server 2016+ | amd64, arm64 | Service install |
| Ubuntu 20.04+ | amd64, arm64 | DEB, systemd |
| Debian 11+ | amd64, arm64 | DEB |
| RHEL/Rocky/Alma 8+ | amd64, arm64 | RPM |
| Fedora 38+ | amd64, arm64 | RPM |
| Alpine 3.18+ | amd64, arm64 | APK, OpenRC |
| macOS 12+ | amd64, arm64 | PKG, LaunchDaemon |

---

## 6. Data Layer

### 6.1 PostgreSQL/TimescaleDB

| Table Category | Tables | Description |
|---------------|--------|-------------|
| Tenancy | `tenants`, `msp_tenants`, `clients`, `users`, `memberships` | Multi-tenant hierarchy |
| Devices | `devices`, `device_groups`, `group_memberships`, `device_packages` | Device inventory, smart groups |
| Auth | `auth_tokens`, `enrollment_tokens`, `invitations`, `mfa_secrets` | Authentication, enrollment |
| Alerts | `rules`, `alerts`, `alert_groups`, `alert_group_members` | Alert rules and instances |
| Policies | `policies`, `policy_revisions`, `patch_policies`, `patch_deployments` | Policy management |
| Scripts | `scripts`, `script_schedules`, `script_schedule_devices`, `script_executions` | Script vault and execution |
| Jobs | `jobs`, `job_events`, `job_device_results` | Durable job system |
| Maintenance | `maintenance_windows` | Maintenance windows |
| CMDB | `device_relationships`, `device_packages` | CMDB dependency relationships |
| Billing | `billing_accounts`, `subscriptions`, `invoices`, `payment_methods`, `usage_events` | Billing and subscription management |
| Remote | `remote_sessions`, `remote_recording`, `webrtc_sessions` | Remote access sessions |
| Reports | `report_schedules`, `generated_reports`, `compliance_reports` | Report scheduling and generation |
| Integrations | `integration_events`, `psa_tickets` | Third-party integrations |
| Audit | `audit_log` | Immutable audit trail |
| Versions | `schema_versions`, `deployment_ids` | Schema versioning, deployment tracking |

**Row-Level Security (RLS)**: Enabled on all tenant-scoped tables with policies for `tenant_id`, `msp_id` scoping.

### 6.2 NATS JetStream

| Stream | Subjects | Description |
|--------|----------|-------------|
| Agent | `tenant.{id}.agent.{agent_id}.heartbeat` | Agent heartbeats |
| Agent | `tenant.{id}.agent.{agent_id}.metrics` | Telemetry metrics |
| Agent | `tenant.{id}.agent.{agent_id}.events` | Agent events |
| Agent | `tenant.{id}.agent.{agent_id}.software.result` | Software install results |
| Agent | `tenant.{id}.agent.{agent_id}.patch.result` | Patch results |
| Commands | `tenant.{id}.cmd.{agent_id}` | Command dispatch to agents |
| Platform | `platform.alerts` | Alert events |
| Platform | `platform.recovery` | Recovery events |

### 6.3 Redis (Optional)

| Use | Description |
|-----|-------------|
| Token Blacklist | JWT jti blacklisting (24h TTL) |
| Rate Limiting | Per-IP, per-endpoint token bucket |
| Agent Registry | Active agent tracking (ephemeral) |

### 6.4 Object Storage (Optional)

| Backend | Description |
|---------|-------------|
| Local | Filesystem-based (for self-hosted) |
| MinIO | Self-hosted S3-compatible |
| AWS S3 | Cloud object storage |

| Content | Description |
|---------|-------------|
| Recordings | WebM/MKV remote session recordings |
| Reports | Generated PDF reports |
| Agent Binaries | Release artifacts |
| Backups | Encrypted database backups |

---

## 7. Authentication & Authorization

### 7.1 Token Types

| Type | Scope | Lifetime | Usage |
|------|-------|----------|-------|
| JWT (user) | User + MSP/Client | Configurable | API access |
| JWT (agent) | Agent + Tenant | Enrollment token | Agent registration |
| Enrollment Token | Agent | Time-bound, single-use | One-time agent enrollment |
| API Key | User | Persistent | Programmatic access |

### 7.2 Access Levels

| Level | Description | Routes |
|-------|-------------|--------|
| `msp_owner` | MSP owner, full control | `/api/v2/msps/*` |
| `msp_admin` | MSP admin | `/api/v1/platform/*`, `/api/v2/msps/*` |
| `client_admin` | Client admin | `/api/v2/clients/*` |
| `agent` | Agent identity | `/api/v1/enroll`, `/api/v1/agent/*` |

### 7.3 Multi-Tenancy

- **Shared schema**: Single PostgreSQL database with `tenant_id` column
- **RLS policies**: Database-enforced tenant isolation
- **Subject isolation**: NATS subjects prefixed with tenant ID
- **Admin view**: Platform-level operators can see all tenants

---

## 8. Configuration

### 8.1 Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `STRATA_RUNTIME_MODE` | `development` | No | `development`, `test`, `production` |
| `TIMESCALE_DSN` / `STRATA_DB_DSN` / `DATABASE_URL` | `postgres://localhost:5432/strata_rmm` | Yes | PostgreSQL connection string |
| `DB_REPLICA_DSN` / `TIMESCALE_REPLICA_DSN` | — | No | Read replica connection string |
| `NATS_URL` | `nats://localhost:4222` | Yes | NATS connection URL |
| `NATS_ADVERTISE_URLS` | — | No | Agent-reachable NATS URLs (production) |
| `NATS_TOKEN` | — | No | NATS authentication token |
| `NATS_TLS_ENABLED` | `false` | No | Enable NATS TLS |
| `NATS_TLS_CERT` | — | No | NATS client certificate |
| `NATS_TLS_KEY` | — | No | NATS client key |
| `NATS_TLS_CA` | — | No | NATS CA certificate |
| `REDIS_URL` | — | No | Redis connection URL (optional) |
| `STORAGE_BACKEND` | `local` | No | `local`, `minio`, `s3` |
| `STORAGE_BUCKET` | `strata-recordings` | No | Object storage bucket |
| `JWT_SECRET` | — | Yes | JWT signing secret (min 32 chars) |
| `STRATA_METRICS_TOKEN` | — | No | Prometheus metrics token |
| `STRATA_SMTP_HOST` | — | No | SMTP host for email alerts |
| `STRATA_SMTP_PORT` | — | No | SMTP port |
| `STRATA_SMTP_USERNAME` | — | No | SMTP username |
| `STRATA_SMTP_PASSWORD` | — | No | SMTP password |
| `STRATA_SMTP_FROM` | — | No | SMTP from address |
| `STRATA_ALERT_SLACK_URL` | — | No | Slack webhook URL |
| `STRATA_ALERT_TEAMS_URL` | — | No | Teams webhook URL |
| `STRATA_ALERT_WEBHOOK_URL` | — | No | Generic webhook URL |
| `STRATA_ALERT_PAGERDUTY_KEY` | — | No | PagerDuty integration key |
| `STRATA_ALERT_EMAIL_RECIPIENTS` | — | No | Comma-separated email recipients |
| `STRATA_API_ADDR` | `:8080` | No | HTTP API listen address |
| `STRATA_TUNNEL_ADDR` | — | No | Raw tunnel gateway (not production-safe) |
| `STRATA_PUBLIC_URL` | — | No | Public-facing URL (required in production) |
| `CORS_ORIGINS` | — | No | Comma-separated CORS origins |
| `STRATA_SEED_DEV` | `false` | No | Seed dev tenant on startup |
| `STRATA_DEV_ADMIN_EMAIL` | — | No | Dev admin email |
| `STRATA_DEV_ADMIN_PASSWORD_HASH` | — | No | Dev admin password hash |

### 8.2 Production Requirements

When `STRATA_RUNTIME_MODE=production`:
- `STRATA_PUBLIC_URL` must be HTTPS
- NATS TLS must be enabled with CA file
- NATS token or mTLS certificate required
- `STRATA_METRICS_TOKEN` required
- No wildcard CORS origins
- No plaintext NATS scheme
- No `sslmode=disable` in DB DSN
- No default passwords in DB DSN
- JWT secret must not have dev/test prefix
- NATS advertise URLs must be non-localhost

---

## 9. Security Architecture

### 9.1 Agent ↔ Platform
- **Enrollment**: Token-based one-time agent registration
- **NATS**: Outbound-only, no inbound ports on agent
- **TLS**: Optional mTLS for NATS (production required)
- **JWT**: HS256 tokens with tenant/role claims

### 9.2 Data Protection
- **At Rest**: AES-256-GCM for backups, disk encryption optional
- **In Transit**: TLS 1.3 for API, optional for NATS
- **Secrets**: Environment variables or secret files (absolute paths)
- **JWT Secret**: Min 32 chars, no default/dev prefixes in production

### 9.3 Access Control
- **RBAC**: MSP owner, MSP admin, client admin, agent roles
- **RLS**: Database row-level security on all tenant-scoped tables
- **Audit**: Immutable audit log table, tamper-evident
- **Rate Limiting**: Per-IP token bucket, 10min stale cleanup

### 9.4 Supply Chain
- **CI**: GoReleaser, cosign keyless signing, SPDX SBOM
- **Agent Update**: Manifest-based, staged rollout, automatic rollback
- **CVE Feed**: OSV.dev (primary), NVD API (optional)

---

## 10. Observability

### 10.1 Internal Monitoring

- **Prometheus Metrics**: `/metrics` endpoint, token-authenticated
- **Health Checks**: `/health` (ready), `/health/live` (liveness)
- **Grafana Dashboards**: Deployment templates for metrics visualization

### 10.2 Synthetic Monitoring (`internal/synthetic/`)

- **HTTP Checks**: URL availability, response time, status codes
- **TCP Checks**: Port reachability
- **DNS Checks**: Resolution verification
- **ICMP Checks**: Ping reachability
- **Multi-vantage**: Multiple check locations for regional coverage

### 10.3 Logging

- **Structured**: zap JSON logging
- **Levels**: Debug, Info, Warn, Error, Fatal
- **Context**: Request ID, tenant ID, agent ID in log fields

---

## 11. Deployment Models

### 11.1 Docker (Development/Production)

```bash
docker-compose up -d  # NATS + TimescaleDB + orchestrator
```

### 11.2 Kubernetes (Helm)

```bash
helm install strata deploy/helm/strata/ -f values.yaml
```

- Deployments with HPA, PDB, network policies
- Ingress with TLS termination
- ConfigMaps/Secrets for configuration
- PVC for persistent storage

### 11.3 Bare Metal (systemd)

```bash
./install.sh install  # Deploy binary, configure systemd
systemctl start strata
```

- Systemd unit with restart policy
- Log journal integration
- Uninstall support

### 11.4 Windows Service

- PowerShell installer script
- Service management (start/stop/status)

---

## 12. Test Coverage

| Module | Behavioral Tests | Unit Tests | Total |
|--------|-----------------|------------|-------|
| `internal/platform/` | ~300 | ~200 | ~500 |
| `internal/agent/` | ~50 | ~100 | ~150 |
| `internal/patch/` | 22 | 33 + 39 | 94 |
| `internal/inventory/` | 91 | ~30 | ~121 |
| `internal/probe/` | 87 | 22 | 109 |
| `internal/alerting/` | 21 | ~80 | ~101 |
| `internal/groups/` | ~30 | ~50 | ~80 |
| `internal/reporting/` | 15 | ~10 | ~25 |
| `pkg/storage/` | 21 | ~30 | ~51 |
| `ui/src/` | ~600 (frontend) | N/A | ~600 |
| **Total** | **~637 behavioral** | **~650+ unit** | **~1,300+** |

---

## 13. Open Items / Technical Debt

1. **Agent Auto-Update Security**: Supply chain attack surface → Cosign verification, staged rollouts
2. **TimescaleDB Scaling**: At 1M+ endpoints, consider distributed hypertables or VictoriaMetrics
3. **NATS JetStream at Scale**: Cluster sizing, subject partitioning strategy
4. **Remote Access Compliance**: SOC2, HIPAA requirements for session recording
5. **Self-Hosted Support Burden**: Version skew, customer environment variability
6. **SSO/OIDC**: Explicitly deferred (future work)
7. **External Billing Backend**: Immutable billable events, invoices, upgrades/downgrades

---

*Last Updated: 2026-08-08*
*PR #111: Reports and object storage behavioral tests (30 tests)*
*PR #110: Software deployment behavioral tests (17 tests)*
*PR #109: OS patch management behavioral tests (22 tests)*
*PR #108: Fix TestOwnMSPSucceeds route and access level*
*PR #107: Fix TestPolicySchedulerStartStop redeclaration*
*PR #106: Docs update for PR #105*
*PR #105: Vulnerability management behavioral tests (91 tests)*
*PR #104: Network discovery behavioral tests (87 tests)*
*PR #103: Remote support behavioral tests (59 tests)*
*PR #102: Scripts and durable endpoint behavioral tests (52 tests)*
