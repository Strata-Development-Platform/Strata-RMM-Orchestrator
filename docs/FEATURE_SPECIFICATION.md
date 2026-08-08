# Strata RMM — Feature Specification

**Version:** 2026-08-08
**Last Updated:** 2026-08-08
**Status:** Internal-alpha code-complete

---

## 1. Product Scope

Strata is a multi-tenant Remote Monitoring & Management (RMM) platform for MSPs. It provides device enrollment, telemetry collection, alerting, patch management, remote access, script execution, and reporting.

**Target Users:** MSP technicians, MSP admins, MSP owners, platform operators
**Deployment:** SaaS (primary), self-hosted (customer-managed), hybrid

---

## 2. Core Capabilities

### 2.1 Tenant Hierarchy & Device Enrollment

| Feature | Status | Implementation |
|---------|--------|----------------|
| MSP → Client → Site hierarchy | ✅ Verified | `tenants`, `msp_tenants`, `clients` tables |
| Device enrollment via token | ✅ Verified | `POST /api/v1/enroll`, `POST /api/v1/agent/register` |
| Agent identity management | ✅ Verified | `internal/agent/core/identity.go` |
| Enrollment token management | ✅ Verified | `GET/POST/DELETE /api/v1/enrollment/tokens` |
| Device CRUD | ✅ Verified | `GET/POST/PUT /api/v2/devices` |
| Device relationships (CMDB) | ✅ Verified | `device_relationships` table, PR #80 |

### 2.2 Metrics Collection & Storage

| Feature | Status | Implementation |
|---------|--------|----------------|
| Agent heartbeat (60s) | ✅ Verified | `tenant.{id}.agent.{id}.heartbeat` |
| Telemetry batch (60s) | ✅ Verified | `tenant.{id}.agent.{id}.metrics` |
| TimescaleDB hypertable | ✅ Verified | `metrics` table, 1d chunk interval |
| Continuous aggregates | ✅ Verified | `metrics_1m`, `metrics_1h` with avg/min/max/last/count |
| Compression policy | ✅ Verified | 7-day segment-by tenant/device/metric |
| Retention policy | ✅ Verified | 365-day auto-expiry |
| Metrics query API | ✅ Verified | `GET /api/v1/devices/{id}/metrics/{name}` |
| SNMP/Redfish/IPMI probes | ✅ Verified | `internal/probe/` (SNMP v3, Redfish, IPMI, Syslog) |
| Network flow collection | ✅ Verified | NetFlow v5/v9/IPFIX parsing |

### 2.3 Dashboard & Visualization

| Feature | Status | Implementation |
|---------|--------|----------------|
| Grafana dashboards | ✅ Verified | `deploy/grafana/` provisioned |
| Prometheus metrics | ✅ Verified | `/metrics` endpoint |
| UI components | ✅ Verified | React TypeScript, 600+ unit tests |
| Live dashboard observation | ⏳ Environment pending | Requires enrolled agent with live telemetry |

### 2.4 Alert Evaluation & State Transitions

| Feature | Status | Implementation |
|---------|--------|----------------|
| Threshold rules | ✅ Verified | `rules.go`, `engine.go` |
| Heartbeat rules | ✅ Verified | Heartbeat fire/recovery correlation |
| Cooldown | ✅ Verified | Configurable per rule |
| Grouping (severity, device, cascade, time-window) | ✅ Verified | PR #81, 21 grouping tests + 7 API endpoints |
| Maintenance window silence | ✅ Verified | `maintenance.go` |
| Alert history | ✅ Verified | `GET /api/v1/alerts/{tenantID}/history` |
| Alert acknowledgment | ✅ Verified | `POST /api/v1/alerts/{tenantID}/{alertID}/acknowledge` |

### 2.5 Alert Notification Delivery

| Feature | Status | Implementation |
|---------|--------|----------------|
| Email (SMTP) | ✅ Verified | `notifier.go`, configurable |
| Slack webhook | ✅ Verified | `STRATA_ALERT_SLACK_URL` |
| Teams webhook | ✅ Verified | `STRATA_ALERT_TEAMS_URL` |
| PagerDuty | ✅ Verified | `STRATA_ALERT_PAGERDUTY_KEY` |
| Generic webhook | ✅ Verified | `STRATA_ALERT_WEBHOOK_URL` |
| Live-provider delivery | ⏳ Environment pending | Requires representative live providers |

### 2.6 Policy Engine

| Feature | Status | Implementation |
|---------|--------|----------------|
| Hierarchical merge (Global→MSP→Client→Device) | ✅ Verified | `policy_effective.go`, PR #57, 68 behavioral tests |
| Validation | ✅ Verified | `policy_model.go`, PR #100 |
| Preview | ✅ Verified | `POST /api/v1/policies/{id}/preview` |
| Publication | ✅ Verified | `POST /api/v1/policies/{id}/publish` |
| Rollback | ✅ Verified | `POST /api/v1/policies/{id}/rollback`, PR #58 |
| Revision history | ✅ Verified | `policy_revisions` table |
| Effective policy computation | ✅ Verified | `POST /api/v1/policies/{id}/effective` |
| Policy diff | ✅ Verified | `POST /api/v1/policies/{id}/diff` |
| Automated enforcement scheduler | ✅ Verified | `policy_scheduler.go`, PR #50 |
| Script Vault and policy-to-script binding | ⏳ Future work | Not implemented |

### 2.7 Scripts & Durable Endpoint Jobs

| Feature | Status | Implementation |
|---------|--------|----------------|
| Script vault (Git-backed) | ✅ Verified | `internal/automation/vault.go`, SSH/HTTPS, PR #59 |
| Script execution (PS/Bash/Python) | ✅ Verified | `internal/agent/scripts/executor.go` |
| Script schedules (hourly/daily/weekly/monthly) | ✅ Verified | `script_schedule_runner.go`, PR #102 |
| One-shot "now" schedules | ✅ Verified | Transition to completed |
| Recurring auto-reschedule | ✅ Verified | After completion |
| Script result routing | ✅ Verified | Echoes schedule_id/device_id for proper routing |
| 52 behavioral tests | ✅ Verified | PR #102, structural tests |
| Policy-to-script binding | ⏳ Future work | Not implemented |

### 2.8 OS Patch Management

| Feature | Status | Implementation |
|---------|--------|----------------|
| Windows PowerShell parsing | ✅ Verified | `executor.go`, 28 unit tests |
| apt/dnf/yum/zypper parsing | ✅ Verified | 28 unit tests |
| Chocolatey scan/install | ✅ Verified | `executor_extended.go` |
| Winget scan/install | ✅ Verified | `executor_extended.go` |
| Flatpak scan/install | ✅ Verified | `executor_extended.go` |
| Snap scan/install | ✅ Verified | `executor_extended.go` |
| WSUS scan | ✅ Verified | `executor_extended.go` |
| Canary deployment | ✅ Verified | `CanaryValidate()`, canary group validation |
| Rollback | ✅ Verified | `RollbackPatch()`, automatic on canary failure |
| 22 structural tests | ✅ Verified | PR #109, struct fields + JSON round-trips |
| Chocolatey/Winget/WSUS/Flatpak/Snap end-to-end | ⏳ Environment pending | Requires OS-specific package managers |
| Auto-remediation engine end-to-end | ⏳ Environment pending | Not tested end-to-end |

### 2.9 Software Deployment

| Feature | Status | Implementation |
|---------|--------|----------------|
| MSI/EXE/DEB/RPM/AppImage/Script | ✅ Verified | `installer.go`, PR #89 |
| SHA256 checksum verification | ✅ Verified | `downloadFile()` |
| Multi-device deployment | ✅ Verified | `POST /api/v1/software/deployments/{tenantID}` |
| Timeout handling | ✅ Verified | Configurable per command |
| Context cancellation | ✅ Verified | `context.WithTimeout()` |
| 17 behavioral tests | ✅ Verified | PR #110, struct fields + JSON round-trips |
| Upload distribution | ⏳ Partial | Implemented, requires external HTTP server |
| Uninstall behavior | ⏳ Partial | Implemented, limited CI verification |

### 2.10 Third-Party Patch Catalog

| Feature | Status | Implementation |
|---------|--------|----------------|
| Vendor discovery | ✅ Verified | `GET /api/v1/thirdparty/vendors`, PR #91 |
| Version sync (Chrome, GitHub, Zoom, 7-Zip, LibreOffice, Mozilla) | ✅ Verified | Mock servers, PR #91 |
| HTTP error handling (404, timeout, empty body) | ✅ Verified | PR #91 |
| 24-hour sync scheduler | ✅ Verified | `internal/inventory/thirdparty.go` |
| 49 unit tests | ✅ Verified | PR #91 |
| Catalog vendor aggregation | ✅ Verified | `thirdparty.go` |
| Deployment lifecycle states | ✅ Verified | PR #91 |

### 2.11 Remote Support & Recordings

| Feature | Status | Implementation |
|---------|--------|----------------|
| Interactive sessions | ✅ Verified | `POST /api/v1/remote/{tenantID}/session` |
| Session recording (WebM/MKV) | ✅ Verified | `recording.go`, PR #88 |
| Live transcription (OpenAI Whisper, Azure, Google) | ✅ Verified | `transcription.go`, PR #88 |
| WebRTC P2P + TURN/STUN relay | ✅ Verified | `internal/webrtc/`, 80+ unit tests |
| Screen capture (Windows/macos/Linux) | ✅ Verified | `capture.go`, platform-specific |
| Input injection (Windows/macos/Linux) | ✅ Verified | `input.go`, platform-specific |
| 59 behavioral tests | ✅ Verified | PR #103, struct fields + JSON round-trips |
| Representative-host acceptance | ⏳ Environment pending | Requires representative agent |

### 2.12 Network Discovery & Flow Monitoring

| Feature | Status | Implementation |
|---------|--------|----------------|
| ARP parsing (Linux/Windows) | ✅ Verified | `probe/` |
| Ping scan | ✅ Verified | `probe/` |
| Port scanning (TCP SYN/UDP) | ✅ Verified | `probe/` |
| Service detection | ✅ Verified | `probe/` |
| Flow parsing (NetFlow v5/v9/IPFIX) | ✅ Verified | `probe/` |
| Device type detection | ✅ Verified | `probe/` |
| SNMP v3 | ✅ Verified | `probe/snmp.go` |
| LLDP/CDP/STP stubs | ✅ Verified | `discovery.go` |
| 87 behavioral tests | ✅ Verified | PR #104, struct fields + JSON round-trips |
| `discoverARP`/`pingScan` in CI | ⏳ Expected to return no results | Requires active network |

### 2.13 Vulnerability Management

| Feature | Status | Implementation |
|---------|--------|----------------|
| OSV sync | ✅ Verified | `cve_sync.go`, 91 behavioral tests |
| Version matching (semantic versioning) | ✅ Verified | `isVersionLessThan()`, 5 tests |
| Auto-remediation (patch executor) | ✅ Verified | `remediation.go`, wired but not end-to-end |
| Manual lifecycle (open/patched/ignored) | ✅ Verified | `POST /api/v1/vulnerabilities/{id}/resolve|ignore` |
| 91 behavioral tests | ✅ Verified | PR #105, struct fields + JSON round-trips |
| OSV/NVD live sync | ⏳ Environment pending | Requires external API access |

### 2.14 Reports & Object Storage

| Feature | Status | Implementation |
|---------|--------|----------------|
| PDF report generation (gofpdf) | ✅ Verified | `reports.go`, sections: summary, alerts, CVEs, patches |
| Report schedules | ✅ Verified | `report_schedules` table, 1h interval |
| Scheduled delivery | ✅ Verified | `processSchedules()`, uploads to object storage |
| Compliance reports | ✅ Verified | `POST /api/v1/reports/{tenantID}/compliance` |
| CSV/JSON export | ✅ Verified | `GET /api/v1/reports/{tenantID}/compliance/{id}/export/{csv,json}` |
| Object storage (local/MinIO/S3) | ✅ Verified | `pkg/storage/`, 21 behavioral tests |
| Storage backend tests | ✅ Verified | `local_backend_test.go`, `mock_backend_test.go` |
| 30 behavioral tests | ✅ Verified | PR #111, struct fields + JSON round-trips |
| Hosted report generation/download | ⏳ Environment pending | Requires representative infrastructure |

### 2.15 Provider/MSP Lifecycle & Branding

| Feature | Status | Implementation |
|---------|--------|----------------|
| Provider profile | ✅ Verified | `provider_profile_handlers.go` |
| Owner invitation | ✅ Verified | `owner_invitations.go` |
| Client management | ✅ Verified | `client_handlers.go` |
| Branding | ✅ Verified | `branding_handlers.go` |
| Billing accounts/subscriptions | ✅ Verified | `billing_handlers.go` |
| Payment methods | ✅ Verified | `POST/GET/PUT/DELETE /api/v2/msps/{mspID}/billing/payment-methods` |
| Invoices | ✅ Verified | `GET /api/v2/msps/{mspID}/billing/invoices` |
| Usage meters | ✅ Verified | `GET /api/v2/msps/{mspID}/billing/usage/{meterName}` |
| Entitlements | ✅ Verified | `PATCH /api/v2/platform/msps/{mspID}/entitlement` |
| Offboarding | ✅ Verified | `offboarding_handlers.go` |
| External billing backend | ⏳ Deferred | Not implemented |
| Immutable billable events | ⏳ Deferred | Not implemented |
| Invoice generation | ⏳ Deferred | Not implemented |
| Upgrade/downgrade billing | ⏳ Deferred | Not implemented |

### 2.16 Client Portal

| Feature | Status | Implementation |
|---------|--------|----------------|
| Support requests (create/list/get/reply/close) | ✅ Verified | `client_support.go`, migration 90 |
| SSO providers (OAuth/OIDC) | ⏳ Deferred | Explicitly future work |
| Session creation (SSO/password) | ⏳ Deferred | Not implemented |
| Branding | ⏳ Partial | Implemented, needs complete acceptance |
| Complete client-portal journey | ⏳ Partial | Needs SSO + session flow |

### 2.17 Authentication & Account Lifecycle

| Feature | Status | Implementation |
|---------|--------|----------------|
| JWT generation/validation | ✅ Verified | `pkg/auth/jwt.go`, 76 behavioral tests (PR #98) |
| TOTP/MFA provisioning | ✅ Verified | `pkg/auth/mfa.go`, `totp.go` |
| API key auth | ✅ Verified | `authorization_result.go` |
| Security headers | ✅ Verified | `middleware.go` |
| Invitation flow | ✅ Verified | `POST /api/v1/auth/invitations/accept` |
| Login request validation | ✅ Verified | PR #98 |
| Password recovery | ⏳ Deferred | Not implemented |
| Refresh-token rotation | ⏳ Deferred | Not implemented |
| Asymmetric access-token signing | ⏳ Deferred | HS256 only |
| SSO/OIDC | ⏳ Deferred | Explicitly future work |
| Trusted-proxy handling | ⏳ Deferred | Not implemented |

### 2.18 Deployment, Recovery & Supply Chain

| Feature | Status | Implementation |
|---------|--------|----------------|
| Docker deployment | ✅ Verified | `docker-compose.yml` |
| Kubernetes (Helm) | ✅ Verified | `deploy/helm/strata/` |
| Linux systemd | ✅ Verified | `deploy/systemd/strata.service` |
| Windows service | ✅ Verified | `deploy/powershell/` |
| Recovery packages | ✅ Verified | `pkg/recovery/` |
| Backup engine | ✅ Verified | `pkg/backup/`, AES-256-GCM |
| Native Linux execution | ⏳ Environment pending | Requires systemd host |
| Windows service execution | ⏳ Environment pending | Requires Windows host |
| Continuous clean-host lifecycle | ⏳ Environment pending | Requires clean infrastructure |
| Multi-region/air-gapped operational acceptance | ⏳ Environment pending | Requires representative environments |

---

## 3. Smart Groups

| Feature | Status | Implementation |
|---------|--------|----------------|
| DSL evaluator (13 operators) | ✅ Verified | `groups/dsl.go`, PR #73 |
| Nested AND/OR expressions | ✅ Verified | PR #73 |
| Migration 94 (filter_expression, is_smart, last_evaluated, member_count) | ✅ Verified | PR #74 |
| group_memberships table | ✅ Verified | PR #74 |
| Smart group UI components | ✅ Verified | `ui/src/components/smart-groups/`, 101 frontend tests |
| DeviceGroupListPage (real API) | ✅ Verified | PR #82 |
| SmartGroupDetailPage (real API) | ✅ Verified | PR #82 |

---

## 4. Integrations

| Feature | Status | Implementation |
|---------|--------|----------------|
| EDR webhook (`/api/v1/integrations/edr/alerts`) | ✅ Verified | `webhooks.go`, PR #66 |
| Backup webhook (`/api/v1/integrations/backup/sync`) | ✅ Verified | `webhooks.go`, PR #66 |
| PSA webhook (`/api/v1/integrations/psa/webhooks`) | ✅ Verified | `webhooks.go`, PR #66 |
| HMAC-SHA256 signature verification | ✅ Verified | `middleware.go`, 15 auth tests |
| Automated security isolation (EDR → NATS) | ✅ Verified | `actions.go`, 2 integration tests |
| PSA ticket CRUD (Freshservice/Zendesk/Jira/Autotask/ConnectWise) | ✅ Verified | `psa_handlers.go`, 27 tests (PR #84) |
| PSA alert feedback loop | ✅ Verified | `AutoRemediatePSATicket()` |
| Integration Dashboard Panel | ✅ Verified | `IntegrationDashboardPanel.tsx`, 116 tests (PR #79) |
| PSA API calls to providers | ⏳ Simulated | In-memory simulation, no real provider APIs |

---

## 5. LAN Cache

| Feature | Status | Implementation |
|---------|--------|----------------|
| Local package distribution | ✅ Verified | `internal/lancache/` |
| Package types | ✅ Verified | Chocolatey, Winget, Flatpak, Snap, MSI, DEB, RPM |
| Cache management | ✅ Verified | 5 REST endpoints |
| 45 unit tests | ✅ Verified | `lancache/` |

---

## 6. Open Questions / Technical Debt

| Item | Description | Status |
|------|-------------|--------|
| Agent auto-update security | Supply chain attack surface | Staged rollouts, cosign |
| TimescaleDB scaling | At 1M+ endpoints | VictoriaMetrics consideration |
| NATS JetStream at scale | Cluster sizing, partitioning | TBD |
| Remote access compliance | SOC2, HIPAA | Session recording documented |
| Self-hosted support burden | Version skew, environment variability | Documented |
| SSO/OIDC | Explicitly deferred | Future work |
| External billing backend | Immutable billable events | Deferred |
| Password recovery | User self-service | Deferred |
| Refresh-token rotation | Security enhancement | Deferred |
| Trusted-proxy handling | Reverse proxy support | Deferred |

---

## 7. Version History

| Date | PR | Change |
|------|-----|--------|
| 2026-08-08 | #111 | Reports and object storage behavioral tests (30 tests) |
| 2026-08-08 | #110 | Software deployment behavioral tests (17 tests) |
| 2026-08-08 | #109 | OS patch management behavioral tests (22 tests) |
| 2026-08-08 | #108 | Fix TestOwnMSPSucceeds route and access level |
| 2026-08-08 | #107 | Fix TestPolicySchedulerStartStop redeclaration |
| 2026-08-08 | #106 | Docs update for PR #105 |
| 2026-08-08 | #105 | Vulnerability management behavioral tests (91 tests) |
| 2026-08-08 | #104 | Network discovery behavioral tests (87 tests) |
| 2026-08-08 | #103 | Remote support behavioral tests (59 tests) |
| 2026-08-08 | #102 | Scripts and durable endpoint behavioral tests (52 tests) |

---

*Last Updated: 2026-08-08*
*Verified by: FEATURE_COMPLETENESS_MATRIX.md, agents.ms, PR #110 CI*
