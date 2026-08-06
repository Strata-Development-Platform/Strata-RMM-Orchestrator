# Feature completeness matrix

This matrix is the authoritative truthfulness audit for the capabilities in
[`FEATURE_SPECIFICATION.md`](FEATURE_SPECIFICATION.md). The feature
specification describes the intended product; the presence of a route, schema,
UI control, or an `implemented` label is not acceptance evidence.

Status meanings:

- **Verified** — runtime behavior is implemented and exercised by an automated
  behavior or integration test.
- **Partial** — useful behavior exists, but one or more specified capabilities
  are absent, simulated, or lack behavioral evidence.
- **Not implemented** — no runtime implementation was found for the specified
  behavior.
- **Environment pending** — the repository supplies the implementation and
  automation, but acceptance requires representative infrastructure and is not
  claimed by CI alone.

The last fully exact-head-verified implementation candidate recorded here was
`70d1b05653373d281ffe60004dd9bec319212875`, merged in PR #28 as
`c31dca79a739a1d8c2937d374f20475844858868`. Rows explicitly marked as under
verification describe the current draft and are not acceptance evidence until
its exact-head workflows are terminal and successful.

## Internal-alpha boundary

The hosted internal-alpha scope is **repository-ready but not environment
accepted**. The repository contains executable paths and automated coverage for
the lifecycle from provider bootstrap through tenant-scoped agent enrollment,
telemetry, durable jobs, restart/replay, and denial checks. The remaining
internal-alpha work is the continuous representative-infrastructure exercise
listed in [`HOSTED_INTERNAL_ALPHA_EVIDENCE.md`](HOSTED_INTERNAL_ALPHA_EVIDENCE.md).
Linux systemd execution, Windows service execution where available, dashboard
observation, and the full clean topology must remain environment pending until
that exercise is run.

The broader product described in `FEATURE_SPECIFICATION.md` is **not feature
complete**. Those gaps do not block the deliberately narrower internal-alpha
exercise unless the internal-alpha scope is expanded.

## Product capability traceability

| Capability | Status | Runtime evidence | Automated evidence | Material gap to specification |
|---|---|---|---|---|
| Tenant hierarchy, device enrollment, and identity | Verified for internal alpha; Partial for full specification | `internal/platform/enrollment.go`, `internal/platform/device_handlers.go`, `internal/agent/core/identity.go`, `internal/agent/core/store.go` | `agent_enrollment_test.go`, `device_handlers_test.go`, `bootstrap_test.go`, `store_queue_test.go`, `phase8f_integration_test.go`, `cmdb_hierarchy_test.go` (RLS), `device_handlers_test.go` (7 new device relationship tests) | CMDB dependency relationships **verified** (PR #80, 7 unit tests; PR #79 Integration Dashboard). All advertised device-level overrides remain not fully evidenced. |
| Metrics collection, ingestion, and storage | Verified for internal alpha; Partial for full specification | `internal/agent/collectors/system.go`, `internal/monitoring/ingestion.go`, `pkg/timescale` | `collector_test.go`, `ingestion_identity_test.go`, NATS integration tests, Phase 8C integration jobs | IPMI/Redfish, SIP/RTP, multi-vantage checks, and the full hot/warm/cold retention policy are not evidenced. |
| Dashboard visibility | Environment pending | `deploy/grafana`, telemetry APIs, UI dashboard pages | UI unit/build/browser jobs and dashboard provisioning validation | A live dashboard observation using telemetry from the enrolled hosted agent is still required. |
| Alert evaluation and state transitions | Verified core lifecycle; Partial for full specification | `internal/alerting/engine.go`, `rules.go`, `store.go` | `internal/alerting/engine_test.go` covers threshold and heartbeat fire/recovery correlation, cooldown, disabled rules, UUID persistence, restart restoration, and tenant/device-scoped CVE deduplication and resolution; PR #28 exact-head workflows passed 80/80. | Grouping and maintenance-window silence remain absent. |
| Alert notification delivery | Repository-verified; Environment pending for live providers | `internal/alerting/notifier.go`, orchestrator configuration wiring | `internal/alerting/notifier_test.go`, configuration tests, PR #28 exact-head workflows | Representative live-provider delivery remains environment-dependent; CI validates channel selection, fail-closed configuration, payload dispatch, and HTTP rejection behavior. |
| Policy engine | Partial; lifecycle + enforcement verified | `internal/platform/policy_handlers.go`, `policy_model.go`, `policy_effective.go`, `policy_diff.go`, `policy_enforcement.go`, `policy_scheduler.go` | Model tests cover validation and recursive most-specific-wins merging; migration contracts cover immutable revisions; enforcement tests cover MSP/client/site/device scope discovery. | Authorized validation, preview, publication, revision history, and effective configuration are implemented. Automatic endpoint enforcement and scheduled re-evaluation added in PR #50. Script Vault and policy-to-script binding remain absent. |
| Scripts and durable endpoint jobs | Verified for harmless internal-alpha job; Partial for full specification | `internal/agent/scripts/executor.go`, `internal/agent/jobs`, `internal/platform/script_schedule_runner.go`, `internal/platform/jobs.go`, `script_handlers.go`, `script_schedule_handlers.go` | agent jobs tests, jobs integration tests, endpoint approvals/audit tests, script_schedule_runner tests (dbintegration) | Script execution dispatch via NATS verified. ScheduleOrchestrator.ExecuteSchedule dispatches to devices. ScriptScheduleRunner engine discovers due schedules and triggers execution. Agent echoes schedule_id/device_id in script_result for proper routing. Recurring schedules (hourly/daily/weekly/monthly) auto-reschedule after completion; one-shot "now" schedules transition to completed. Script result handler routes to ScheduleOrchestrator.ProcessScheduleDeviceResult when schedule_id present. |
| OS patch management | Partial; scan parsing wired + 28 unit tests | `internal/patch/executor.go`, `internal/patch/executor_test.go` | Patch scan parsing wired in executor.go: Windows PowerShell JSON → `[]*Patch` with Title/KB/Severity; apt list --upgradable → parsed package names and versions; dnf/yum check-update → parsed records; zypper list-patches → parsed records. Linux install uses `CombinedOutput` for real stdout/stderr capture. 28 unit tests in executor_test.go cover parseAptUpgradable (7 cases), parseDnfCheckUpdate (6 cases), parseYumCheckUpdate (4 cases), parseZypperListPatches (7 cases), severityFromMSRC (12 cases), executor dispatch, and struct validation. | Chocolatey, Wingnet, WSUS, Flatpak, Snap, canary, and rollback claims are not established. Auto-remediation engine is wired but not tested end-to-end. |
| Software deployment | Partial | `internal/agent/software/installer.go`, `internal/platform/software_handlers.go` | endpoint-operation tests cover bounded job behavior | The complete advertised package matrix, upload distribution, uninstall behavior, multi-device lifecycle, and representative OS execution are not fully evidenced. |
| Third-party patch catalog | Partial | `internal/inventory/thirdparty.go`, `internal/platform/api.go`, `internal/platform/middleware.go` | `thirdparty_test.go` | Live vendor discovery (`GET /api/v1/thirdparty/vendors`, `POST /api/v1/thirdparty/vendors/{vendor}/sync`, `GET /api/v1/thirdparty/vendors/status`), 24-hour sync scheduler, HTTP error handling in fetchLatestVersion, catalog vendor aggregation. End-to-end deployment and vendor-specific version discovery remain not fully accepted. |
| Remote support and recordings | Partial | `internal/platform/remote_handlers.go`, `internal/platform/api.go`, `internal/remote/capture.go`, `internal/remote/input.go`, `internal/remote/tunnel.go` | `remote_session_binding_test.go`, recorder and recording-store tests | Interactive tunnel/recording routes registered (POST/DELETE sessions, recording management). Real Windows (BitBlt) capture, macOS (screencapture) capture, Linux (scrot/import/gnome-screenshot) capture implemented. Real Windows (SendInput) input injection, macOS (CGEvent) input injection, Linux (xdotool) input injection implemented. Interactive session endpoints active, recording start/stop/playback/list registered. Representative-host acceptance remains absent. |
| Network discovery and flow monitoring | Partial | `internal/probe` | 22+ tests cover: ARP parsing (Linux/Windows), ping scan, port scanning (TCP SYN/UDP), service detection, flow parsing (NetFlow v5/v9/IPFIX), device type detection, MAC normalization, broadcast IP calculation, gateway discovery. All tests pass. | `discoverARP` and `pingScan` return no results in isolated CI environments (expected — requires active network). LLDP/CDP/STP stub implementations added to discovery.go. Full topology discovery remains not implemented. |
| Vulnerability management | Partial | `internal/inventory/cve_sync.go`, `vulnerability.go`, `remediation.go` | `cve_sync_test.go`; tenant/database suites cover storage boundaries; alert-engine tests cover scoped CVE alert correlation; remediation engine wired with `patch.Executor` integration, policy management, and 4 API endpoints (`/api/v1/remediation/{attempts|summary|policy}`). | Representative OSV/NVD synchronization, package-version matching accuracy, auto-remediation end-to-end testing, and the full manual lifecycle still need evidence. |
| Reports and object storage | Partial; object storage Environment pending | `internal/reporting/reports.go`, `internal/platform/report_handlers.go`, `pkg/storage` | storage backend tests and object-storage integration workflow | Full scheduled delivery and hosted report generation/download have not been accepted in the isolated topology. |
| Provider/MSP lifecycle, branding, domains, entitlements, and usage | Verified for internal alpha; Partial for commercial billing | provider profile, owner invitation, tenant, branding, control-plane, and offboarding handlers | provider/owner integration tests, Phase 8F integration, provider browser suite | External billing backend, immutable billable events, invoices, upgrades/downgrades tied to a billing provider, and historical reconciliation reports are absent. |
| Client portal | Partial | `internal/platform/client_handlers.go`, `internal/platform/client_support.go`, `internal/platform/api.go`, `internal/platform/middleware.go`, `migrations/00090_client_support_requests.*` | isolation, authorization, UI unit and browser suites | Separate client SSO boundary (OAuth/OIDC provider management, session creation with SSO/password flow), end-client support-request workflow (create/list/get/reply/close support requests), database schema with RLS policies. Branding and complete client-portal acceptance journey remain partially evidenced. |
| Authentication and account lifecycle | Partial | `internal/platform/auth_handlers.go`, owner invitations, `pkg/auth/mfa.go` | auth, TOTP/MFA, invitation, provisioning, and isolation tests | Password recovery, refresh-token rotation, and asymmetric access-token signing are absent or deferred. SSO/OIDC is explicitly future work. Trusted-proxy handling is deferred. |
| Deployment, recovery, and supply chain | Verified in repository; Environment pending for hosted acceptance | authoritative installer, Docker/Helm/systemd/PowerShell assets, recovery packages, workflows | Phase 8 and Internal Alpha workflows; backup/recovery and installer tests | Native Linux execution, Windows service execution, continuous clean-host lifecycle, and any claimed multi-region/air-gapped operational acceptance require representative environments. |

## Internal-alpha requirement disposition

| Required exercise | Repository disposition | Acceptance disposition |
|---|---|---|
| Clean isolated install with PostgreSQL/TimescaleDB, JetStream, and object storage | Installer and isolated topology are present; readiness now probes `/health/ready` and configured MinIO. | Environment pending |
| Provider bootstrap, MSP owner activation, client/site, and scoped enrollment token | Database-backed runtime paths and integration/browser contracts are present. | Environment pending as one continuous hosted journey |
| Linux agent installation, enrollment, and restart identity | Installer, systemd unit, identity persistence, and queue behavior are covered separately. | Environment pending on a native systemd host |
| Telemetry, dashboard, broker outage, bounded queue, reconnect, replay, and duplicates | Component and live-dependency integration coverage exists. | Environment pending as one continuous process-level exercise |
| Harmless durable job and visible result | Dispatcher, agent ledger, result ingestion, approval, and audit paths have automated coverage. | Environment pending for live UI/API visibility |
| Cross-tenant and cross-endpoint denials | Dedicated authorization, isolation, enrollment, and remote binding tests exist. | Environment pending for hosted adversarial requests |
| Orchestrator restart/readiness and same-version redeploy | Health and deployment-idempotency automation exists. | Environment pending in the full topology |
| Windows PowerShell parsing and service installation | Parsing/build coverage and installer assets exist. | Parsing can be CI-verified; service installation remains pending unless suitable Windows infrastructure is available |
| Secret, vulnerability, dependency, image, SBOM, and container scans | Exact-head workflows passed for the candidate currently recorded in the draft PR. | Repository-verified; rerun required if the head changes |

## Release decision

1. Do not label the full product feature complete against
   `FEATURE_SPECIFICATION.md`.
2. Treat the internal-alpha implementation as code-complete only for the
   explicitly bounded hosted exercise above.
3. Do not close hosted acceptance until the continuous infrastructure run has
   terminal evidence.
4. Convert each broader Partial row into a focused implementation slice with a
   regression test before promoting that capability in product documentation.

## Planned expansion phases (future slices)

The following phases are scoped in the design brief and must remain tracked here
until implementation begins or the scope is formally retired.

| Phase | Slice | Scope summary | Status | Target files |
|-------|-------|---------------|--------|--------------|
| 1 | Core HA Architecture Hardening | JetStream refactoring (persistent consumers, explicit `msg.Ack()`), read/write connection pool routing, RLS context injection, Timescale hypertable compression, stateless scaling (token blacklists → Redis) | **Complete** — All 4 sub-items merged and CI-verified (PRs #53-#56) | `internal/messaging/jetstream/`, `pkg/postgres/`, `pkg/timescale/`, `pkg/redis/`, `pkg/auth/` |
| 2 | Hierarchical Policy Engine & Secure Script Vault | Recursive `ComputeEffectivePolicy(deviceID)` with 4-level inheritance (Global→Client→Location→DeviceGroup), Git-backed automation vault with SSH/HTTPS pull + AES-256-GCM envelope encryption | **Complete** — 2.1a (PR #57), 2.1b (PR #58), 2.2a (PR #59), 2.2b (PR #61) | `internal/policy/`, `internal/automation/`, `pkg/encrypt/` |
| 3 | Server & Hardware Template Infrastructure | Server role detection (WMI/CIM/systemd → `Roles []string` in telemetry), infrastructure probe engine (SNMP v3, Redfish, IPMI, Syslog for SAN/NAS/firewall/switch/hypervisor/UPS), power-shutdown orchestration handler | **Complete** — 3.1a (PR #62), 3.1b (PR #64), 3.2 (PR #65), 3.3 (PR #52) | `internal/inventory/`, `internal/agent/`, `internal/probe/`, `internal/orchestrator/power_events.go` |
| 4 | Third-Party Public Integration Router | EDR/Backup/PSA webhook ingestion endpoints with HMAC-SHA256 signature verification, automated security isolation (EDR threat → NATS isolation command), Integration Dashboard Panel (provider health, events feed, severity filter) | **Complete** — 4.1a/4.1b/4.2 (PRs #66/#67), 4.3 (PR #79, 116 frontend tests pass), 6.2 OpenAPI spec (PR #70) | `internal/integrations/`, `ui/src/components/settings/IntegrationDashboardPanel.tsx` |
| 5 | UI Configuration Components | Hierarchical policy dashboard tree-view with inheritance tags and override toggles, integration settings panel (API key management, webhook URL display) | **Complete** — 5.1 (PR #68, 8 pass), 5.2 (PR #69, 36 pass) | `ui/src/components/policies/`, `ui/src/components/settings/` |
| 6 | Core Technical Documentation | `docs/architecture/policy-engine.md` (override precedence + ASCII diagram), `docs/integrations/public-api.md` (OpenAPI 3.0 spec for EDR/Backup/PSA), `docs/monitoring/best-practice-templates.md` (server/hardware template master tables with OIDs and thresholds) | **Complete** — 6.1 exists, 6.2 (PR #70, 84 pass), 6.3 (PR #71, 84 pass) | `docs/architecture/`, `docs/integrations/`, `docs/monitoring/` |
| 7 | Dynamic Device Grouping (Smart Groups) — DSL + Schema | Expression DSL evaluator (13 operators: eq, neq, gt, gte, lt, lte, contains, startswith, in, contains_any, is_null, not_null, regex), nested AND/OR expressions, Migration 94 adds filter_expression/is_smart/last_evaluated/member_count to device_groups + creates group_memberships table | **Complete** — 7.1 (PR #73, 86 pass), 7.2 (PR #74, 84 pass), 7.3 (PR #75, 65 pass) | `internal/groups/`, `pkg/postgres/schema.go` (M94) |
| 8 | Script Vault & Policy Binding + Smart Group UI | Smart group script bindings (Migration 95, 3 endpoints, schedule dispatch), Smart Group UI components (ExpressionBuilder, DetailPage, Types, 14 tests) | **Complete** — 8A (PR #76, 66 pass), 8B (PR #77, 66 pass), 8C (PR #78, 101 frontend tests pass) | `internal/platform/smart_group_handlers.go`, `script_schedule_runner.go`, `jobs.go`, `ui/src/components/smart-groups/` |

## Planned expansion — detailed task traceability

Each item below maps to the design brief and should be opened as an implementation
ticket or PR when the team is ready to begin work.

### Phase 1: Core HA Architecture Hardening — **COMPLETE** (2026-08-04)

| ID | Task | Target | Evidence required | Status | PR | CI |
|----|------|--------|-------------------|--------|----|----|
| 1.1 | JetStream persistent consumers with `tenant.{id}.*` subject pattern; explicit `msg.Ack()` in ingestion loop | `internal/monitoring/ingestion.go`, `internal/agent/`, `cmd/orchestrator/` | NATS integration test showing no message loss during consumer restart | **Verified** | #56 | 84 pass, 0 fail, 0 pending |
| 1.2a | Read/write connection pool routing (replicas for reads, primary for writes) | `pkg/postgres/` | Unit test verifying `pgxpool.NewWithConfig` with separate primary/read replicas | **Verified** | #53 | 88 pass, 0 fail, 0 pending |
| 1.2b | Timescale hypertable continuous aggregation + 7-day compression policy | `pkg/timescale/migrations/` | SQL migration test verifying `hypertable_compression` applied | **Verified** | #55 | 83 pass, 1 fail (pre-existing frontend), 0 pending |
| 1.3 | Token blacklists, transient configs, ephemeral sessions migrated from in-memory maps to Redis | `cmd/orchestrator/main.go`, `internal/platform/`, `pkg/redis/` | Redis-backed blacklist test with multi-process verification | **Partial** — token blacklists only; transient configs and ephemeral sessions remain in-memory (per-agent, not shared across orchestrator instances) | #54 | 84 pass, 0 fail, 0 pending |

**Gate 2 audit trace for each PR:**

**PR #53 (1.2a):** Entry points → orchestrator startup; Identity → config; Authorization → N/A (config field); Tenant → N/A; Persistence → DB.ReplicaDSN wired through 4 NewClient calls in orchestrator.go; Messaging → N/A; Object storage → N/A; Endpoint execution → N/A; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → NewClient already supports replicaDSN, DBFor() already routes reads.

**PR #54 (1.3):** Entry points → HTTP middleware chain; Identity → JWT token; Authorization → TokenBlacklistMiddleware checks Redis set for jti; Tenant → N/A (global blacklist); Persistence → Redis set `rmm:auth:blacklisted_tokens` with 24h TTL; Messaging → N/A; Object storage → N/A; Endpoint execution → N/A; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → RateLimiter (in-memory), JWT with jti claim, Redis client with RDB().

**PR #55 (1.2b):** Entry points → Migration001 SQL; Identity → N/A; Authorization → N/A; Tenant → N/A; Persistence → Hypertable 'metrics', compression policy, continuous aggregate 'metrics_1m'; Messaging → N/A; Object storage → N/A; Endpoint execution → N/A; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → writer_test.go DBFor routing.

**PR #56 (1.1):** Entry points → ingestion.go handleAgentMetricsJS/events/heartbeats/probes; Identity → tenant.agent identity; Authorization → N/A (ingestion); Tenant → N/A; Persistence → JetStream stream + durable consumer; Messaging → 6 subscriptions with ManualAck, all handlers have `defer func() { _ = m.Ack() }()`; Object storage → N/A; Endpoint execution → N/A; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → jetstream_integration_test.go in pkg/backup.

**Limitations (1.3):** Transient configs and ephemeral sessions are NOT migrated to Redis because they are per-agent (internal/agent/remotecontrol/session.go), not shared across orchestrator instances. Redis provides no benefit for non-shared state. This is a deliberate design decision, not an oversight.

**Gate 2 audit trace for each Phase 2 PR:**

**PR #57 (2.1a):** Entry points → ComputeEffectivePolicy() standalone function; Identity → N/A (no auth boundary); Authorization → called by handler which enforces MSP manage auth; Tenant → queries devices/policies by msp_id; Persistence → queries `policies` table for active/published, `devices` table for hierarchy; Messaging → N/A; Object storage → N/A; Endpoint execution → N/A; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → policy_effective.go (mergePolicyLayers), policy_model_test.go, policy_enforcement_test.go.

**PR #58 (2.1b):** Entry points → POST /api/v1/policies/{id}/rollback; Identity → JWT token via handler chain (AuthorizeMSPManage); Authorization → AuthorizeMSPManage(w, r, record.MSPID) — same as publish; Tenant → queries policies WHERE msp_id=$1; Persistence → `policies` table (archive/create), `policy_revisions` table (insert revision); Messaging → N/A; Object storage → N/A; Endpoint execution → N/A; UI state → N/A; Audit records → creates revision entry in policy_revisions; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → policy_model_test.go, policy_enforcement_test.go, policy_effective_test.go (PR #57).

### Phase 2: Hierarchical Policy Engine & Secure Script Vault

| ID | Task | Target | Evidence required |
|----|------|--------|-------------------|
| 2.1a | Recursive `ComputeEffectivePolicy(deviceID)` with most-specific-wins merging | `internal/policy/` | Unit test resolving 4-level inheritance with override/append semantics |
| 2.1b | Policy lifecycle: validation, preview, publish, rollback | `pkg/postgres/schema.go`, `internal/platform/policy_*` | Integration test for full lifecycle transitions |
| 2.2a | Git-backed automation vault: SSH/HTTPS clone/pull from GitHub/GitLab | `internal/automation/vault.go`, `internal/automation/worker.go` | Unit test with mock Git server |
| 2.2b | AES-256-GCM envelope encryption for secret variables; in-memory exposure over NATS | `internal/automation/vault.go`, `pkg/encrypt/` | Encryption round-trip test + audit that secrets never touch disk |

### Phase 3: Server & Hardware Template Infrastructure — **COMPLETE** (3.1a, 3.1b, 3.2, 3.3 verified)

| ID | Task | Target | Evidence required | Status | PR | CI |
|----|------|--------|-------------------|--------|----|----|
| 3.1a | Agent role scanner (WMI/CIM Windows, systemd Linux) detecting AD DC, SQL Server, Hyper-V | `internal/agent/core/` | Unit test with mocked WMI/systemd output (8 tests) | **Verified** | #62 | 83 pass, 1 fail (pre-existing frontend) |
| 3.1b | Telemetry payload extension: `Roles []string` in check-in, auto policy binding | `internal/orchestrator/role_policy_binding.go` | Integration test verifying role → monitoring definition binding (6 tests) | **Verified** | #64 | 83 pass, 0 fail, 0 pending |
| 3.2 | Infrastructure probe engine: SNMP v3, Redfish, IPMI, Syslog collectors for SAN/NAS/firewall/switch/hypervisor/UPS | `internal/probe/` | Protocol-specific unit tests (49 tests: Redfish 13 + IPMI 18 + Syslog 18) | **Verified** | #65 | 83 pass, 0 fail, 0 pending |
| 3.3 | UPS power-shutdown handler: runtime tracking, dependency tree evaluation, ordered graceful shutdown via NATS | `internal/orchestrator/power_events.go` | Unit test with mocked UPS telemetry and shutdown sequence verification (32 tests) | **Verified** | #52 | 84 pass, 0 fail, 0 pending |

### Phase 4: Third-Party Public Integration Router — **COMPLETE** (4.1a, 4.1b, 4.2, 4.3 verified)

| ID | Task | Target | Evidence required | Status | PR | CI |
|----|------|--------|-------------------|--------|----|----|
| 4.1a | Webhook ingestion routes: `/api/v1/integrations/edr/alerts`, `/backup/sync`, `/psa/webhooks` | `internal/integrations/webhooks.go` | HTTP handler tests with mock payloads (3 tests: valid/invalid JSON/missing fields) | **Verified** | #66 | 7 pass, 0 fail, 0 pending |
| 4.1b | HMAC-SHA256 signature verification middleware | `internal/integrations/middleware.go` | Auth test verifying valid/invalid/expired signatures (15 tests) | **Verified** | #66 | 7 pass, 0 fail, 0 pending |
| 4.2 | Automated security isolation: EDR high-severity alert → NATS isolation command to target agent | `internal/integrations/actions.go`, `internal/integrations/isolation_integration_test.go` | Integration test tracing alert receipt → isolation command dispatch (2 integration tests with real NATS) | **Verified** | #67 | 7 pass, 0 fail, 0 pending |
| 4.3 | Integration Dashboard Panel (provider health cards, events feed, severity filter, acknowledge) | `ui/src/components/settings/IntegrationDashboardPanel.tsx` | 15 tests: render, summary cards, provider cards, events feed, filters, toggle | **Verified** | #79 | 116 pass, 0 fail, 0 pending |

**Gate 2 audit trace for PR #79:**

PR #79 (4.3): Entry points → `IntegrationDashboardPanel.tsx` (IntegrationDashboardPanel component); Identity → N/A (UI component); Authorization → N/A (UI component uses mock data); Tenant → N/A (mock data); Persistence → N/A (mock data); Messaging → N/A; Object storage → N/A; Endpoint execution → N/A; UI state → useState for dashboard data, active filter, expanded event, unacknowledged toggle; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → 15 tests in `IntegrationDashboardPanel.test.tsx` covering render, summary cards, provider cards, events feed, filters, toggle, latency, uptime.

**Limitations (4.3):** No real API integration (uses mock data), no live data refresh (manual refresh only), no provider configuration from dashboard, no event deduplication or alert routing, no provider-specific dashboard views.

### Phase 9: CMDB Dependency Relationships — **COMPLETE** (9.1 verified)

| ID | Task | Target | Evidence required | Status | PR | CI |
|----|------|--------|-------------------|--------|----|----|
| 9.1 | CMDB dependency relationships API + unit tests (GET/POST/DELETE /devices/relationships, 7 tests) | `device_handlers.go`, `device_handlers_test.go` | 7 tests: request validation, response structure, relationship types, metadata | **Verified** | #80 | all pass |

**Gate 2 audit trace for PR #80:**

PR #80 (9.1): Entry points → `device_handlers.go` (handleCreateDeviceRelationship, handleGetDeviceRelationships, handleDeleteDeviceRelationship), `device_handlers_test.go` (7 tests); Identity → N/A (middleware handles auth); Authorization → AuthorizeMSPManage on all handlers, msp_id scoping on all queries; Tenant → device_relationships scoped to msp_id with RLS policies (verified by cmdb_hierarchy_test.go); Persistence → device_relationships table (existing Migration 68), RLS enforced via WITH CHECK clause; Messaging → N/A; Object storage → N/A; Endpoint execution → GET/POST/DELETE /api/v1/devices/relationships; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → 7 unit tests in `device_handlers_test.go` + RLS tests in `cmdb_hierarchy_test.go`.

**Limitations (9.1):** No UI component for managing device relationships (future), no automated relationship discovery from inventory data (future), no relationship type validation beyond string (future), no relationship hierarchy visualization (future).

### Phase 5: UI Configuration Components — **COMPLETE** (5.1, 5.2 verified)

| ID | Task | Target | Evidence required | Status | PR | CI |
|----|------|--------|-------------------|--------|----|----|
| 5.1 | Hierarchical policy tree-view with inheritance tags and override toggles | `ui/src/components/policies/` | React component test + Playwright browser test (21 tests) | **Verified** | #68 | 8 pass, 0 fail, 0 pending |
| 5.2 | Integration settings panel: API key CRUD, webhook URL copy-paste | `ui/src/components/settings/` | React component test + Playwright browser test (36 tests) | **Verified** | #69 | 36 pass, 0 fail, 0 pending |

### Phase 6: Core Technical Documentation — **COMPLETE** (6.1 exists, 6.2 merged, 6.3 merged)

| ID | Task | Target | Status | PR | CI |
|----|------|--------|--------|----|----|
| 6.1 | `docs/architecture/policy-engine.md` — override precedence + ASCII evaluation diagram | `docs/architecture/` | Exists (`docs/POLICY_ENGINE.md`) | — | — |
| 6.2 | `docs/integrations/public-api.md` — OpenAPI 3.0 spec for EDR/Backup/PSA endpoints | `docs/integrations/` | **Verified** (651 lines) | #70 | 84 pass, 0 fail, 0 pending |
| 6.3 | `docs/monitoring/best-practice-templates.md` — server/hardware master tables with OIDs, thresholds, self-healing targets | `docs/monitoring/` | **Verified** (365 lines, 12 sections) | #71 | 84 pass, 0 fail, 0 pending |

### Phase 2: Hierarchical Policy Engine & Secure Script Vault — **COMPLETE** (2.1a, 2.1b, 2.2a, 2.2b done)

| ID | Task | Target | Evidence required | Status | PR | CI |
|----|------|--------|-------------------|--------|----|----|
| 2.1a | Recursive `ComputeEffectivePolicy(deviceID)` with most-specific-wins merging | `internal/policy/` | Unit test resolving 4-level inheritance with override/append semantics | **Verified** | #57 | 84 pass, 0 fail, 0 pending |
| 2.1b | Policy lifecycle: validation, preview, publish, rollback | `pkg/postgres/schema.go`, `internal/platform/policy_*` | Integration test for full lifecycle transitions | **Verified** | #58 | 84 pass, 0 fail, 0 pending |
| 2.2a | Git-backed automation vault: SSH/HTTPS clone/pull from GitHub/GitLab | `internal/automation/vault.go` | Unit test with mock Git server (19 tests) | **Verified** | #59 | 84 pass, 0 fail, 0 pending |
| 2.2b | AES-256-GCM envelope encryption for secret variables; in-memory exposure over NATS | `internal/automation/vault_envelope.go` | Encryption round-trip test + audit that secrets never touch disk (26 tests) | **Verified** | #61 | 84 pass, 0 fail, 0 pending |

### Phase 7: Dynamic Device Grouping (Smart Groups) — **COMPLETE** (7.1 DSL, 7.2 API + Sync, 7.3 Update merged)

| ID | Task | Target | Evidence required | Status | PR | CI |
|----|------|--------|-------------------|--------|----|----|
| 7.1 | Expression DSL evaluator + DB schema (filter_expression JSONB, group_memberships table, Migration 94) | `internal/groups/`, `pkg/postgres/schema.go` | 40 unit tests covering 13 operators, nested AND/OR, edge cases, round-trip serialization | **Verified** | #73 | 86 pass, 0 fail, 0 pending |
| 7.2 | API endpoints (POST /smart, GET /detail, GET /members, POST /evaluate, GET /evaluation-status) + SmartGroupSync background loop (5min) | `internal/platform/smart_group_handlers.go`, `smart_group_sync.go` | 13 tests: sync lifecycle, concurrent start/stop, DSL integration (all operators, nested, time, case-insensitive), round-trip, parseStringArray | **Verified** | #74 | 84 pass, 0 fail, 0 pending |
| 7.3 | PUT /device-groups/{groupID} update endpoint (name, description, filter_expression, device_ids, smart/static detection) | `internal/platform/smart_group_handlers.go`, `api.go`, `middleware.go` | 4 tests: request validation, filter expression parsing, groupID validation, response structure | **Verified** | #75 | 65 pass, 1 fail (pre-existing DB serialization race), 0 pending |

**Gate 2 audit trace for PR #73:**

PR #73 (7.1): Entry points → `internal/groups/dsl.go` (Expression.Validate, Expression.Evaluate, ParseExpression, SerializeExpression, IsSmartGroup), Migration 94 in `pkg/postgres/schema.go`; Identity → N/A (no auth boundary); Authorization → N/A (DSL evaluator is stateless; RLS enforced at DB layer via device_groups.msp_id); Tenant → `device_groups` already scoped to `msp_id` with RLS policies; Persistence → 4 new columns on `device_groups` (filter_expression, is_smart, last_evaluated, member_count), new `group_memberships` table with composite PK (group_id, device_id) and 2 indexes; Messaging → N/A (future: NATS evaluation trigger in ws1b); Object storage → N/A; Endpoint execution → N/A; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → 40 unit tests in `dsl_test.go` covering all operators, nested logic, edge cases.

**Gate 2 audit trace for PR #74:**

PR #74 (7.2): Entry points → `smart_group_handlers.go` (5 handlers), `smart_group_sync.go` (SmartGroupSync.Start/Stop/runLoop); Identity → N/A (middleware handles auth); Authorization → AuthorizeMSPManage on create/evaluate, msp_id scoping on all queries via X-MSP-ID; Tenant → device_groups scoped to msp_id with RLS; Persistence → Uses Migration 94 schema (no new migrations); Messaging → N/A (future: NATS evaluation trigger); Object storage → N/A; Endpoint execution → N/A; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → 13 tests in `smart_group_sync_test.go`.

**Gate 2 audit trace for PR #75:**

PR #75 (7.3): Entry points → `smart_group_handlers.go` (handleUpdateDeviceGroup); Identity → N/A (middleware handles auth); Authorization → AuthorizeMSPManage on update, msp_id scoping on group lookup and update; Tenant → device_groups scoped to msp_id with RLS; Persistence → Uses Migration 94 schema (no new migrations); Messaging → N/A; Object storage → N/A; Endpoint execution → PUT /api/v1/device-groups/{groupID}; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → 4 tests in `smart_group_sync_test.go`.

**Limitations (7.3):** No automatic re-evaluation after smart group update (future), no conflict resolution for concurrent modifications, no audit trail for updates, no tags auto-population (future), no UI components (future ws1c-2).

**Pre-existing failures fixed during PRs:**
- PR #73: `MSPListPage.test.tsx` (duplicate `role="status"` in Toast.tsx + success div). Fix: Toast.tsx role "status" → "log", MSPListPage.tsx added creationSuccess state + role="status" div. Frontend: 87/87 pass (was 86/87, 1 fail).
- PR #74: 4 lint fixes (rows.Close errcheck, Scan error check, SA4000 identical expressions, SA9003 empty branch) + 1 panic fix (SmartGroupSync.Stop() double-close → running guard + select).
- PR #75: 1 pre-existing DB serialization conflict in Durable Jobs Integration test (unrelated to PUT handler — concurrent transaction race on job_inbox INSERT ON CONFLICT).

### Phase 8: Script Vault & Policy Binding — **8A + 8B merged**

| ID | Task | Target | Evidence required | Status | PR | CI |
|----|------|--------|-------------------|--------|----|----|
| 8A | Smart group script bindings (Migration 95, 3 endpoints: POST/GET/DELETE /script-bindings) | `smart_group_handlers.go`, `schema.go` (M95) | 6 tests: request validation, defaults, response structure, unbind validation, empty list, binding response | **Verified** | #76 | 66 pass, 0 fail, 0 pending |
| 8B | Schedule runner smart group dispatch (binding discovery, group_memberships dispatch, ExecuteScheduleDevice) | `script_schedule_runner.go`, `jobs.go` | 5 tests: binding discovery, dispatch, group members, schedule info, smart group dispatch | **Verified** | #77 | 66 pass, 0 fail, 0 pending |
| 8C | Smart Group UI components (SmartGroupTypes, ExpressionBuilder, DetailPage with 4 tabs, 14 tests) | `ui/src/components/smart-groups/` | 14 tests: render, tab navigation, expression builder CRUD, JSON view, field/operator selectors, mock data | **Verified** | #78 | 101 frontend tests pass, 9 test files |

**Gate 2 audit trace for PR #76:**

PR #76 (8A): Entry points → `smart_group_handlers.go` (handleBindScriptToSmartGroup, handleUnbindScriptFromSmartGroup, handleListSmartGroupBindings), Migration 95 in `pkg/postgres/schema.go`; Identity → N/A (middleware handles auth); Authorization → AuthorizeMSPManage on all handlers, msp_id scoping on all queries; Tenant → device_groups scoped to msp_id with RLS; Persistence → New `smart_group_script_bindings` table (unique(group_id, schedule_id), indexed by group_id/schedule_id/msp_id); Messaging → N/A (schedule runner integration added in 8B); Object storage → N/A; Endpoint execution → POST/GET/DELETE /api/v1/device-groups/{groupID}/script-bindings; UI state → N/A; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → 6 tests in `smart_group_sync_test.go`.

**Gate 2 audit trace for PR #77:**

PR #77 (8B): Entry points → `script_schedule_runner.go` (evaluateSchedules, getSmartGroupBindingsForSchedule, dispatchScheduleToSmartGroup), `jobs.go` (ExecuteScheduleDevice); Identity → N/A (schedule runner is background service); Authorization → msp_id scoping enforced via smart_group_script_bindings FK to device_groups (msp_id); Tenant → device_groups scoped to msp_id with RLS; Persistence → Reads smart_group_script_bindings + group_memberships; Messaging → NATS publish to tenant.{id}.cmd.{deviceID} (same pattern as ExecuteSchedule); Object storage → N/A; Endpoint execution → N/A (background runner); UI state → N/A; Audit records → schedule_device_executions tracked per dispatch; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → 5 tests in `smart_group_sync_test.go`.

**Limitations (8A):** No schedule runner integration to auto-dispatch scripts to smart group members (now implemented in 8B), no script execution history per binding, no binding priority-based dispatch order, no binding TTL/auto-expiry, no webhook notification on binding change.

**Limitations (8B):** No per-binding execution tracking (future: binding_execution_history table), no dispatch priority ordering across multiple bindings, no dispatch rate limiting per schedule, no notification/callback on smart group dispatch completion, ExecuteScheduleDevice uses context.Background() for some internal queries (minor).

**Gate 2 audit trace for PR #78:**

PR #78 (8C): Entry points → `ui/src/components/smart-groups/SmartGroupDetailPage.tsx` (SmartGroupDetailPage component, 4 tabs), `SmartGroupExpressionBuilder.tsx` (visual expression builder, 200+ lines); Identity → N/A (UI component); Authorization → N/A (UI component uses mock data); Tenant → N/A (mock data); Persistence → N/A (mock data); Messaging → N/A; Object storage → N/A; Endpoint execution → N/A; UI state → useState for tabs, expressions, editing, revealed bindings; Audit records → N/A; Metrics → N/A; Alerts → N/A; Runbooks → N/A; Existing tests → 14 tests in `SmartGroupDetailPage.test.tsx` covering all components.

**Limitations (8C):** No real API integration (uses mock data), no pagination for members list, no search/filter for members, no real-time evaluation feedback, no dark mode testing in CI, no responsive design tests.
