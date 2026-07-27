# Implementation Status

## Feature Matrix

### Core Platform

| Feature | Status | Notes |
|---------|--------|-------|
| HTTP API server | ✅ Verified | 66 routes in `api.go` |
| PostgreSQL schema | ✅ Verified | 30 tables, 26 migrations |
| TimescaleDB schema | ✅ Verified | 7 hypertables, 2 migrations |
| NATS messaging | ✅ Verified | 12 wildcard subscriptions, 23 subject patterns |
| Multi-tenant data isolation | ⚠️ Partial | RLS enabled on some tables, single ambiguous `tenant_id` |
| Agent enrollment | ⚠️ Partial | In-memory enrollment tokens, not persisted |
| JWT auth (user) | ✅ Verified | HS256, hardcoded secret |
| JWT auth (agent) | ✅ Verified | Same HS256 secret |
| MFA / TOTP | ✅ Verified | RFC 6238, 30s window |
| Rate limiting | ✅ Verified | Token bucket, 30 req/60s default |
| Security headers | ✅ Verified | Via `auth.SecurityHeaders()` middleware |

### Agent

| Feature | Status | Notes |
|---------|--------|-------|
| Go agent binary | ✅ Verified | Cross-platform (Linux/Windows/macOS) |
| NATS communication | ✅ Verified | 6 subject patterns per agent |
| Metrics collection | ✅ Verified | CPU, memory, disk, net via gopsutil |
| Heartbeat | ✅ Verified | Regular interval |
| Script execution | ✅ Verified | Runs scripts, returns results |
| Software install/uninstall | ✅ Verified | MSI, EXE, DEB, RPM |
| OS patching | ⚠️ Partial | Linux (apt/dnf/zypper), Windows (WU API) |
| Remote desktop | ❌ Stub | `capture_stub.go` — not implemented |
| Auto-update | ✅ Verified | Manifest-based, signed |
| BBolt local store | ✅ Verified | Durable local queue |

### Inventory

| Feature | Status | Notes |
|---------|--------|-------|
| Device registration | ✅ Verified | Via agent enrollment |
| Device CRUD | ✅ Verified | PostgreSQL `devices` table |
| Software inventory | ✅ Verified | dpkg / Win32_Product |
| CVE/NVD sync | ⚠️ Stub | 10 seeded CVEs, no actual NVD sync |
| Vulnerability matching | ⚠️ Partial | Matches CVEs by package+version |
| Third-party app sync | ⚠️ Stub | Returns empty data |

### Monitoring and Alerting

| Feature | Status | Notes |
|---------|--------|-------|
| Metrics ingestion | ✅ Verified | NATS → TimescaleDB batch writer |
| Events ingestion | ✅ Verified | NATS → TimescaleDB |
| Heartbeat tracking | ✅ Verified | Per-device heartbeat |
| Alert rules CRUD | ✅ Verified | Rules in `alert_rules` table |
| Alert evaluation | ✅ Verified | Threshold + heartbeat rules |
| Alert notifications | ✅ Verified | Slack, webhook (email/teams/pagerduty pluggable) |
| Grafana dashboards | ✅ Verified | Provisioned in `deploy/grafana/` |

### Remote Access

| Feature | Status | Notes |
|---------|--------|-------|
| Session creation | ✅ Verified | NATS-relayed |
| NATS tunnel | ✅ Verified | Bidirectional streaming |
| Screen capture | ❌ Stub | `capture_stub.go` — no-op on most platforms |
| Keyboard/mouse input | ✅ Verified | Via NATS subject |
| Session recording | ✅ Verified | Byte-level recording to storage |
| Multi-monitor | ❌ Not implemented | — |
| Clipboard | ❌ Not implemented | — |
| File transfer | ❌ Not implemented | — |

### Software Deployment

| Feature | Status | Notes |
|---------|--------|-------|
| Package CRUD | ✅ Verified | MSI, EXE, DEB, RPM, AppImage |
| Deployment creation | ✅ Verified | Per-tenant, multi-device |
| Deployment tracking | ✅ Verified | Per-target status |
| Patch policies | ✅ Verified | CRUD on patch policies |
| Patch deployment | ⚠️ Partial | Linux and Windows executors |

### Scripting

| Feature | Status | Notes |
|---------|--------|-------|
| Script CRUD | ✅ Verified | Per-tenant scripts |
| Script execution | ✅ Verified | NATS-dispatched to agents |
| Execution history | ✅ Verified | Per-execution results |
| Multi-device targeting | ✅ Verified | Run on selected devices |

### Reporting

| Feature | Status | Notes |
|---------|--------|-------|
| Report generation | ✅ Verified | PDF via gofpdf |
| Report schedules | ✅ Verified | Daily/weekly/monthly |
| Report history | ✅ Verified | List of generated reports |

### Encryption Keys

| Feature | Status | Notes |
|---------|--------|-------|
| Key CRUD | ✅ Verified | AES-256-GCM per tenant |
| Key rotation | ✅ Verified | Active key designation |
| Key revocation | ✅ Verified | Per-key deletion |

### Network Probe

| Feature | Status | Notes |
|---------|--------|-------|
| SNMP polling | ✅ Verified | v1/v2c/v3 |
| NetFlow/IPFIX/sFlow | ✅ Verified | Collection and parsing |
| Network discovery | ✅ Verified | ARP + SNMP discovery |
| Topology edges | ✅ Verified | From discovery results |

### UI

| Feature | Status | Notes |
|---------|--------|-------|
| Login page | ✅ Verified | Email/password form |
| Dashboard | ✅ Verified | Platform overview with stats |
| Customers list | ✅ Verified | Tenant listing with device/alert counts |
| Customer detail | ✅ Verified | Devices, alerts, vulns, recordings |
| Scripts | ✅ Verified | CRUD, execution, history |
| Software | ✅ Verified | Packages, deployments |
| Third-party patching | ✅ Verified | App listing, sync |
| Reports | ✅ Verified | Generated reports, schedules |
| Settings | ✅ Verified | MFA enrollment |
| Admin users | ✅ Verified | User CRUD, tenant assignment |
| Admin settings | ✅ Verified | CVE sync, platform overview |
| Remote desktop | ✅ Verified | Canvas with WebSocket frames |

### Security

| Issue | Severity | Location |
|-------|----------|----------|
| Hardcoded JWT secret `strata-rmm-dev-secret` | 🔴 CRITICAL | 6 locations in Go + KOTS config |
| Placeholder password hash in seed | 🔴 CRITICAL | `$2a$10$placeholder` |
| Hardcoded MinIO credentials | 🟡 HIGH | Docker Compose + Helm values |
| Hardcoded PostgreSQL password | 🟡 HIGH | Docker Compose + Grafana |
| In-memory only enrollment tokens | 🟡 HIGH | Not persisted to DB |
| Symmetric JWT signing | 🟡 MEDIUM | HS256 — no public key verification |
| No refresh tokens | 🟡 MEDIUM | Session management limited |
| Missing auth on some routes | 🟡 MEDIUM | Need to audit every handler |
| Single tenant_id for all | 🟡 MEDIUM | No MSP/client/site hierarchy |

### Testing

| Area | Status | Notes |
|------|--------|-------|
| Go unit tests | ✅ Passing | 6 packages, all OK |
| Go race detection | ❌ Not run | Need to add to CI |
| Frontend type check | ✅ Clean | `tsc -b` passes |
| Frontend lint | ❌ Missing deps | ESLint deps not installed |
| API contract tests | ❌ Not implemented | — |
| Cross-tenant isolation tests | ❌ Not implemented | — |
| E2E tests | ❌ Not implemented | — |
| Load tests | ⚠️ Partial | `scripts/loadtest.sh` exists |

### Deployment

| Method | Status | Notes |
|--------|--------|-------|
| Docker Compose | ✅ Verified | `deploy/docker/docker-compose.yml` |
| Helm chart | ✅ Verified | `deploy/helm/strata-rmm/` |
| KOTS | ✅ Verified | `deploy/kots/` |
| Manual Linux install | ✅ Verified | `scripts/install-platform.sh` |
| CI (GoReleaser) | ✅ Verified | `.goreleaser.yml` |
| Grafana dashboards | ✅ Verified | `deploy/grafana/` |
