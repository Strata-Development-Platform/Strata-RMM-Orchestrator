# Changelog

## [0.2.0-beta] — 2026-07-28

### Added
- Phase 8A: Production configuration and startup hardening
- Runtime modes (development/test/production) with fail-closed validation
- Centralized configuration loading with strict typed parsing
- Liveness and readiness health endpoints with dependency checks
- Configuration inventory and documentation
- Phase 8A acceptance verification — all 9 criteria verified
- Configuration inventory covers all 53 settings across orchestrator, agent, and probe

### Changed
- TrustedProxies removed from HTTPConfig and WithHTTPConfig (deferred to later phase)

## Unreleased

### Added

- Added provider-approved MSP owner onboarding: migration 68 global normalized
  email uniqueness and verification metadata, pending-owner lifecycle,
  digest-only one-time invitations with forced RLS, TLS-only SMTP delivery,
  public hash-fragment activation, atomic first-owner/MSP/entitlement activation,
  fail-closed platform authorization, immutable token-safe audit events, and
  provider-console creation/resend states. This does not add open sign-up, MFA,
  password recovery, billing, or refresh-token redesign.
- Added migration 67, server-authoritative first-login provider business setup,
  later Platform Settings editing, active platform-membership authorization,
  tenant-scope denials, and transactional immutable audit evidence.
- Added validation, authorization, database integration, frontend unit, and
  Chromium acceptance coverage for the provider business-profile lifecycle.
- Completed Phase 7 endpoint operations and technician-console workflows, including approvals, idempotency, offline lifecycle handling, immutable audit evidence, capability negotiation, inventory ingestion, and browser acceptance coverage.
- Added the complete StrataLabs project contract: documentation index, contribution guide, security policy, and website-integration guidance.

### Changed

- Control-plane mutation handlers now fail the request when their audit event
  cannot be recorded, allowing the request transaction to roll back.
- Corrected StrataLabs project metadata and replaced the obsolete roadmap with current SaaS, orchestration, endpoint-operations, beta-hardening, and hosted-service milestones.

- Wired the durable jobs dispatcher into the managed-agent lifecycle.
- Enforced persistence-before-acknowledgement and exact result replay.
- Added server result receipts and agent-side acknowledgement tracking.
- Made inbox deduplication and job state updates transactional.
- Added strict NATS subject, ownership, correlation, and attempt validation.
- Propagated job cancellation, expiry deadlines, and agent shutdown to handlers.
- Corrected Job Center tenant/client filtering and surfaced action failures.
- Removed generated TypeScript build metadata from source control.

### Fixed

- Kept Regional Defaults Continue as a non-submit control with a distinct DOM
  identity so native click activation cannot skip Review, and associated login
  labels with their email and password inputs.

All notable changes to this project should be documented here.

The format follows Keep a Changelog principles, and releases should use semantic versioning where applicable.

## [0.6.3] - 2026-07-27

### Added - Phase 5C/5D: Job Center UI + Operational Dashboard
- **JobsPage** (`ui/src/pages/JobsPage.tsx`): Full Job Center with status/type filters, active/recent job lists, job detail view with per-target results, cancel/retry actions, state badges with color-coded dots, and loading/empty/error states.
- **JobHealthPage** (`ui/src/pages/JobHealthPage.tsx`): Operational dashboard with job status summary cards (failed, expired, running, queued, succeeded, cancelled), alert banner for failed jobs.
- **Navigation**: Jobs and Job Health links added to sidebar. Routes registered in App.tsx.
- **UI verification**: TypeCheck clean, lint 0 errors, production build OK.

## [0.6.2] - 2026-07-27

### Added - Phase 5B: Agent Durable Execution (Complete)
- **Agent ReceiptLedger** (`internal/agent/jobs/ledger.go`): BBolt-backed command receipt tracking with `command_receipts` bucket. Supports `RecordReceipt`, `GetReceipt`, `IsDuplicate`, `MarkRunning`, `MarkComplete`, `GetUnacknowledgedResults` (for result retransmission), and `Cleanup` (bounded retention).
- **Agent HandlerRegistry** (`internal/agent/jobs/handler.go`): Maps `command_type` strings to typed `CommandHandler` functions. Supports `Register()`, `Get()`, and `IsSupported()` checks.
- **Agent JobDispatcher**: NATS subscriber for `tenant.{tid}.cmd.{aid}` commands. Validates envelope, deduplicates via BBolt ledger, publishes typed acknowledgements (`accepted`/`duplicate`/`unsupported`), dispatches to handler registry, publishes typed results. `replayResults()` retransmits unacknowledged completed results on startup.
- **Orchestrator inbox processing** (`internal/platform/dispatcher.go`): NATS subscriptions for `.ack` and `.result` subjects. Inserts into `job_inbox` for deduplication, validates ownership (MSP, job, target, device), applies state transitions, updates aggregate job state.
- **Cancellation handling**: Orchestrator updates target to `cancelled` state; agent dispatcher framework supports context cancellation in handlers.
- **4 ledger tests**: Record/get/duplicate detection, state transitions (received→running→succeeded), unacknowledged result filtering, retention cleanup.
- **9 Go packages**: All tests pass including new `internal/agent/jobs` package.

## [0.6.1] - 2026-07-27

### Added - Phase 5B: Agent Command Protocol
- **CommandEnvelope** (`internal/platform/command.go`): Typed command envelope with `schema_version`, `event_id`, `job_id`, `target_id`, `msp_id`, `client_id`, `site_id`, `device_id`, `agent_id`, `correlation_id`, `attempt`, `issued_at`, `expires_at`, `command_type`, and structured `payload`.
- **Acknowledgement types**: `accepted`, `duplicate`, `rejected`, `expired`, `unsupported` with typed `Acknowledgement` struct.
- **ResultEnvelope**: Typed result with ownership validation and canonical terminal states (`succeeded`, `failed`, `cancelled`, `expired`).
- **Command validation**: `ValidateCommandEnvelope()` rejects malformed JSON, unsupported schema versions, missing identifiers, wrong MSP/agent/device, expired commands, future-issued commands, invalid attempts, and empty command types.
- **Result validation**: `ValidateResultEnvelope()` validates ownership fields and restricts to canonical terminal states.
- **15 protocol tests**: Covering valid envelopes, malformed JSON, unsupported versions, missing fields, wrong tenant/agent/device, expired, future-issued, invalid attempts, empty types, and result status validation.
- **JWT secret fix**: `NewTokenGenerator` no longer silently reads env var; `NewTokenGeneratorOrFail` is used for env-var-based configuration. Login, middleware, and agent handlers all use the fail-returning constructor.

## [0.6.0] - 2026-07-27

### Added - Phase 5: Durable Job Orchestration
- **Job outbox/inbox** (migrations 49-50): Transactional outbox for reliable job event publishing and inbox for deduplicated result processing.
- **Enhanced job schema** (migration 51): `correlation_id`, `version`, `scheduled_for`, `dispatch_count`, `cancelled_by`, per-target `lease_owner`, `lease_expires`, `next_retry_at`, `dispatched_at`, `acknowledged_at`, `exit_code`, `msp_id`.
- **State machine engine** (`internal/platform/statemachine.go`): Strict state transitions: `pending→queued→dispatched→running→succeeded/failed/cancelled/expired`.
- **Dispatcher worker** (`internal/platform/dispatcher.go`): Outbox publisher, dispatch worker with bounded leases, reconciliation worker for expired leases and aggregate state computation.
- **New API endpoints**: `POST /api/v1/jobs/{jobID}/retry` (retry failed targets), `GET /api/v1/devices/{deviceID}/jobs` (device job history), `GET /api/v1/jobs/{jobID}/events` (job event stream).
- **Idempotency**: Tenant-scoped idempotency key support — returns existing job on duplicate key, rejects conflicting payloads.
- **Wrapper script** (`start.sh`): Ensures JWT_SECRET env var is available to the process (systemd Environment= workaround).
- **Membership seed**: Admin user now has `msp_admin` membership in default MSP, enabling authenticated API access through third-party authorization middleware.

### Fixed
- **Auth login with FORCE RLS**: Login handler now uses `set_config()` in a transaction to set app context before querying `client_organizations`.
- **RLS on job creation**: All job handlers use `s.requestDB(r)` to pick up the tenant transaction context.
- **JWT fallback**: No more hardcoded fallback secret — only env var or explicit secret.
- **TestTokenPurposeSeparation**: Sets JWT_SECRET env var for test consistency.

## [0.5.0] - 2026-07-27

### Added
- **SaaS Rewrite Phase 0 — Baseline**: Backup branch `backup/pre-saas-rewrite-20260727`, baseline tag `pre-saas-rewrite-20260727`.
- **Comprehensive baseline inventory**: 17,292 Go LOC, 3,185 TS/TSX LOC, 66 HTTP routes, 23 NATS subjects, 37 database tables.
- **Missing `alerts` table**: Created by migration #26 — all alert queries previously failed with "relation does not exist".
- **Missing UI routes**: 10 page components were imported but not registered in App.tsx. All routes now wired.
- **Missing remote input endpoint**: `POST /api/v1/remote/{tenantID}/session/{sessionID}/input` added.
- **TypeScript fixes**: `tsc -b` now passes cleanly. Fixed 12 errors across 6 files.
- **Documentation**: `MASTER_PLAN.md`, `IMPLEMENTATION_STATUS.md`, `COMPATIBILITY_MATRIX.md`, `SECURITY_MODEL.md`, `SAAS_TENANCY.md`, `API_COMPATIBILITY.md`, `AGENT_PROTOCOL.md`.

### Production Hardening — Phase 7
- **FIXED**: KOTS config — removed hardcoded `strata-rmm-dev-secret` default.
- **FIXED**: Old in-memory EnrollmentManager replaced with DB-backed `enrollment_tokens_v2`.
- **FIXED**: `handleEnroll` (legacy endpoint) now creates persisted enrollment tokens.
- **REMOVED**: `auth.EnrollmentManager` dependency from APIServer struct (fully DB-backed).
- **ADDED**: Non-owner PostgreSQL role `strata_rmm_app` with SELECT/INSERT/UPDATE/DELETE only.
- **ADDED**: Default privileges for future tables.
- **FIXED**: All systemd services enabled for auto-start on boot.

### RMM Vertical Slices — Phase 6
- **ADDED**: `POST /api/v1/maintenance-windows` — create maintenance windows with time range.
- **ADDED**: `GET /api/v1/maintenance-windows` — list maintenance windows by client.
- **ADDED**: `DELETE /api/v1/maintenance-windows/{id}` — delete a maintenance window.
- **ADDED**: `device_groups` table (migration 36) — MSP/client-scoped device groupings.
- **ADDED**: `POST /api/v1/device-groups` — create device groups with member devices.
- **ADDED**: `GET /api/v1/device-groups` — list device groups by client.
- **ADDED**: `DELETE /api/v1/device-groups/{id}` — delete a device group.

### Durable Job Orchestration — Phase 5
- **ADDED**: `jobs` table (migration 34) — unified job tracking with type, status, payload, retries.
- **ADDED**: `job_targets` table (migration 35) — per-device job status, results, error messages.
- **ADDED**: `POST /api/v1/jobs` — create and dispatch a job to devices via NATS.
- **ADDED**: `GET /api/v1/jobs` — list jobs filtered by MSP and client.
- **ADDED**: `GET /api/v1/jobs/{jobID}` — get job details with per-target status.
- **ADDED**: `POST /api/v1/jobs/{jobID}/cancel` — cancel a pending/queued job.
- **ADDED**: `POST /api/v1/jobs/result` — receive job results from agents via NATS callback.
- **ADDED**: Idempotency key support for safe retries.
- **ADDED**: Automatic job completion tracking via target status counts.

### Secure Enrollment — Phase 4
- **ADDED**: `enrollment_tokens_v2` table (migration 33) — DB-backed, MSP/client/site-scoped.
- **ADDED**: `POST /api/v1/enrollment/tokens` — create scoped enrollment token with max uses.
- **ADDED**: `POST /api/v1/enrollment/validate` — validate and consume a token, returns scope.
- **ADDED**: `GET /api/v1/enrollment/tokens` — list tokens for the current MSP.
- **ADDED**: Token generation uses cryptographically random 32-byte hex strings, SHA-256 hashed in DB.
- **ADDED**: Enrollment tokens support max_uses, expiration, and revocation.

### Branding & Domains — Phase 3
- **ADDED**: `branding_profiles` table (migration 31) — MSP display name, logos, colors, portal text.
- **ADDED**: `custom_domains` table (migration 32) — custom subdomain management with verification status.
- **ADDED**: Default branding profile seeded for the Strata Platform MSP.
- **ADDED**: `withBranding` middleware — resolves MSP identity from hostname (platform domain, MSP subdomains, custom domains).
- **ADDED**: `GET /api/v1/branding` — returns the branding profile for the current MSP.
- **ADDED**: `PUT /api/v1/branding` — updates branding profile colors, logos, text.
- **ADDED**: `GET /api/v1/domains` — lists custom domains for the current MSP.

### SaaS — Phase 2
- **ADDED**: `msp_tenants` table (migration 27) — MSP tenant model.
- **ADDED**: `client_organizations` table (migration 28) — client orgs under MSPs.
- **ADDED**: `sites` table (migration 29) — sites/locations under client orgs.
- **ADDED**: Default MSP seed (migration 30) — creates "Strata Platform" MSP.
- **ADDED**: Legacy tenant backfill — existing tenants become client orgs under default MSP.
- **ADDED**: JWT claims now include `mid` (MSP ID), `cid` (client ID), `sid` (site ID).
- **ADDED**: Auth middleware checks cross-MSP and cross-client access denial via query params.
- **UPDATED**: `GenerateUserToken` — includes MSP/client/site scope.

### Security — Phase 1
- **FIXED**: Hardcoded JWT secret `strata-rmm-dev-secret` — replaced with `JWT_SECRET` env var (6 locations).
- **FIXED**: Placeholder password hash `$2a$10$placeholder` — replaced with proper bcrypt hash.
- **ADDED**: Central auth middleware — classifies routes as Public, User, or Admin.
- **ADDED**: Route classification table — admin routes require `admin` role.
- **PENDING**: Non-owner PostgreSQL role for RLS enforcement.
- Enrollment tokens still stored in-memory only (Phase 4).

## [0.4.0] - 2026-07-25

### Added
- **Built-in Remote Control** (`internal/agent/remotecontrol/`): Agent-side screen capture (JPEG frames at 5-10 FPS) with keyboard/mouse input injection via NATS. Platform-specific: Linux X11 (ImageMagick/xdotool), Windows DirectX stub, macOS CGDisplay stub, software test pattern fallback. Web UI canvas viewer with quality/FPS sliders.
- **Remote Control API**: `POST /api/v1/remote/{tid}/session` (start), `DELETE /api/v1/remote/{tid}/session/{sid}` (stop). Returns NATS topics for frame/input/control.
- **Scripting Engine** (`internal/agent/scripts/executor.go`): Run PowerShell, Bash, Python, Batch scripts on endpoints via NATS command dispatch. Parameter interpolation, timeout enforcement, stdout/stderr capture, execution history.
- **Script Management API**: CRUD scripts, run on devices, list/view execution results. 7 endpoints.
- **Software Deployment** (`internal/agent/software/installer.go`): Push MSI/EXE/DEB/RPM packages to endpoints. SHA256 verification, silent install args, per-device deployment tracking.
- **Software Deployment API**: Package CRUD, create/list/detail deployments with per-device status. 6 endpoints.
- **Third-Party Patching** (`internal/inventory/thirdparty.go`): Auto-sync versions for 10 apps (Chrome, Firefox, Adobe Reader, 7-Zip, VLC, Teams, Zoom, Notepad++, LibreOffice). Auto-creates software_packages entries with silent install args. 24h sync loop.
- **Third-Party Patching API**: List apps/sync all/sync single app. 4 endpoints.
- **Reporting Engine** (`internal/reporting/reports.go`): PDF report generation (executive summary with stat boxes, alerts table, CVE table, patch status). Scheduled delivery (daily/weekly/monthly). Storage to MinIO/S3.
- **Reporting API**: List generated reports, create/list/delete schedules, on-demand generation. 5 endpoints.
- **Orchestrator Self-Update** (`internal/update/orchestrator.go`): GitHub releases API check, binary download with SHA256 verification, atomic swap + health check + rollback. Bare metal (systemd restart), Docker (print instructions), K8s detection.
- **Orchestrator Update CLI**: `strata-rmm orchestrator update [--check]` subcommand.
- **Orchestrator Update API**: `GET /api/v1/admin/update/check`, `POST /api/v1/admin/update/apply`.
- **Deployment ID Onboarding**: `POST /api/v1/admin/customers` auto-generates deployment ID. Agent `--deployment-id` flag replaces enrollment token flow. `POST /api/v1/agent/register` for first check-in.
- **User Auth + RBAC**: `POST /api/v1/auth/login` (bcrypt + JWT), `GET /api/v1/auth/me`. Admin user management with tenant scoping.
- **Platform Overview API**: `GET /api/v1/platform/overview` (aggregate stats), `GET /api/v1/platform/customers` (per-customer summaries).
- **Database migrations 20-24**: user_tenant_access, scripts, software_packages, report_schedules, agent_registrations.
- **UI Components**: Toast notifications, skeleton loaders, confirm dialogs, empty states, status badges, dark mode toggle, lucide-react icons, collapsible sidebar.
- **UI Pages**: Login, Dashboard, Customers, Customer Detail (devices/alerts/vulns/recordings/settings tabs), User Management, Scripts, Software, Patch Mgmt, Reports, Admin Settings, My Settings (MFA), Remote Control viewer.
- **Installation Guide** (`docs/INSTALL.md`): Docker Compose, bare metal Linux (NATS + TimescaleDB + MinIO + systemd), Kubernetes Helm, agent install for all platforms.
- **SOC 2 Compliance Evidence** (`docs/SOC2.md`): Controls mapped to TSC criteria (CC6.1-CC7.4) with evidence locations.
- **KOTS Integration** (`deploy/kots/`): Admin console UI config for self-hosted deployments.
- **Windows MSI Installer** (`scripts/wix/`): WiX source for agent MSI with service registration.
- **End-to-End Smoke Test** (`scripts/smoke_test.sh`): Validates health, enrollment, CVE, MFA, recordings, access, encryption.

### Changed
- `cmd/agent/agent.go`: Wired script executor, software installer, remote control manager into startup
- `cmd/orchestrator/orchestrator.go`: Added version parameter, update subcommand, third-party engine, report engine
- `internal/platform/api.go`: 40+ new route registrations, NATS subscriptions for script/software results
- `ui/src/`: Complete restructure with shared components, toast system, theme support
- `main.go`: Version export for orchestrator self-update
- `go.mod`: Added gofpdf/v2, golang.org/x/crypto/bcrypt

## [0.3.0] - 2026-07-25

### Added
- **CVE Feed Sync** (`internal/inventory/cve_sync.go`): OSV.dev batch API for 17 tracked packages, optional NVD sync, auto-upsert with CVSS scoring and fixed versions, sync state tracking
- **Vulnerability Remediation** (`internal/inventory/vulnerability.go`): Auto-close CVEs when device patches, manual resolve/ignore endpoints, tenant vulnerability summary
- **Per-Tenant Encryption Keys** (`pkg/encrypt/keys.go`): AES-256-GCM key management with create/list/rotate/revoke lifecycle, supports local/aws_kms/gcp_kms/azure_kv providers
- **Access Review API**: `GET /api/v1/access/audit/{tenantID}`, `GET /api/v1/access/users/{tenantID}`, `GET /api/v1/access/permissions/{tenantID}`
- **Rate Limiting & Security Headers** (`pkg/auth/ratelimit.go`): Per-IP token bucket, per-endpoint limits (health:100/s, enroll:5/s), CSP, nosniff, 10MB body limit
- **Auto-update wired into agent**: Update client + rollout manager initialized in agent startup
- **Config-driven storage backend**: `--storage-backend` flag / `STORAGE_BACKEND` env var
- **Database migrations 18-19**: cve_sync_state, tenant_encryption_keys

### Changed
- `internal/platform/api.go`: CVE sync, encryption key, access review routes + security middleware

## [0.2.0] - 2026-07-25

### Added
- **StorageBackend abstraction** (`pkg/storage/`): MinIO, AWS S3, Local, Mock implementations
- **Session Recording** (`internal/remote/recorder.go`): Raw byte-level recording with SHA256, MinIO/S3 upload
- **Recording CRUD API**: List, playback (presigned URL), delete
- **Recording retention cleanup**: Scheduled job (24h), configurable retention
- **MFA/TOTP** (`pkg/auth/totp.go`): RFC 6238, HMAC-SHA1, 6-digit codes, QR enrollment
- **Agent Auto-Update** (`internal/agent/update/`): Manifest, staged rollout, rollback, BBolt state
- **Version subcommand**: `strata-rmm version --output=json`
- **GoReleaser + cosign**: Multi-platform builds, keyless signing, SPDX SBOM
- **CI/CD workflows**: Lint, test, matrix build, Trivy, Gosec, Docker multi-arch
- **Helm CRD**: AgentUpdateChannel custom resource
- **GitHub issue templates**: Sprint task, security report, PR template
- **Database migrations 16-17**: session_recordings, mfa_secrets

### Changed
- `main.go`: Version ldflags, version subcommand
- `Makefile`: VERSION/COMMIT/DATE ldflags, goreleaser target
- `go.mod`: Upgraded to Go 1.25, added minio-go/v7, AWS SDK v2

### Fixed
- `internal/alerting/rules.go:81`: Format verb mismatch fix

## [0.1.0] - 2026-07-25

### Added
- Architecture plan (docs/ARCHITECTURE.md)
- Core platform CLI with Cobra
- Agent, Probe, and Orchestrator command structure
- GitHub issue templates (bug report, feature request)
- Project roadmap (.stratalabs/roadmap.json)
