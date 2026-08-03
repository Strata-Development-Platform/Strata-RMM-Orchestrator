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
| Tenant hierarchy, device enrollment, and identity | Verified for internal alpha; Partial for full specification | `internal/platform/enrollment.go`, `internal/platform/device_handlers.go`, `internal/agent/core/identity.go`, `internal/agent/core/store.go` | `agent_enrollment_test.go`, `device_handlers_test.go`, `bootstrap_test.go`, `store_queue_test.go`, `phase8f_integration_test.go` | CMDB dependency relationships and all advertised device-level overrides are not evidenced. |
| Metrics collection, ingestion, and storage | Verified for internal alpha; Partial for full specification | `internal/agent/collectors/system.go`, `internal/monitoring/ingestion.go`, `pkg/timescale` | `collector_test.go`, `ingestion_identity_test.go`, NATS integration tests, Phase 8C integration jobs | IPMI/Redfish, SIP/RTP, multi-vantage checks, and the full hot/warm/cold retention policy are not evidenced. |
| Dashboard visibility | Environment pending | `deploy/grafana`, telemetry APIs, UI dashboard pages | UI unit/build/browser jobs and dashboard provisioning validation | A live dashboard observation using telemetry from the enrolled hosted agent is still required. |
| Alert evaluation and state transitions | Verified core lifecycle; Partial for full specification | `internal/alerting/engine.go`, `rules.go`, `store.go` | `internal/alerting/engine_test.go` covers threshold and heartbeat fire/recovery correlation, cooldown, disabled rules, UUID persistence, restart restoration, and tenant/device-scoped CVE deduplication and resolution; PR #28 exact-head workflows passed 80/80. | Grouping and maintenance-window silence remain absent. |
| Alert notification delivery | Repository-verified; Environment pending for live providers | `internal/alerting/notifier.go`, orchestrator configuration wiring | `internal/alerting/notifier_test.go`, configuration tests, PR #28 exact-head workflows | Representative live-provider delivery remains environment-dependent; CI validates channel selection, fail-closed configuration, payload dispatch, and HTTP rejection behavior. |
| Policy engine | Partial; lifecycle foundation under exact-head verification | `internal/platform/policy_handlers.go`, `policy_model.go`, policy schema | Focused model tests cover validation and recursive most-specific-wins merging; migration contracts cover immutable revisions. | Authorized validation, preview, publication, revision history, and effective configuration are implemented in this draft. Automatic endpoint enforcement and scheduled re-evaluation remain absent. |
| Scripts and durable endpoint jobs | Verified for harmless internal-alpha job; Partial for full specification | `internal/agent/scripts/executor.go`, `internal/agent/jobs`, `internal/platform/jobs.go`, `script_handlers.go` | agent jobs tests, jobs integration tests, endpoint approvals/audit tests | Full recurring schedule behavior and complete multi-device UI workflow are not established by the internal-alpha evidence. |
| OS patch management | Partial; scan parsing wired + 28 unit tests | `internal/patch/executor.go`, `internal/patch/executor_test.go` | Patch scan parsing wired in executor.go: Windows PowerShell JSON → `[]*Patch` with Title/KB/Severity; apt list --upgradable → parsed package names and versions; dnf/yum check-update → parsed records; zypper list-patches → parsed records. Linux install uses `CombinedOutput` for real stdout/stderr capture. 28 unit tests in executor_test.go cover parseAptUpgradable (7 cases), parseDnfCheckUpdate (6 cases), parseYumCheckUpdate (4 cases), parseZypperListPatches (7 cases), severityFromMSRC (12 cases), executor dispatch, and struct validation. | Chocolatey, Wingnet, WSUS, Flatpak, Snap, canary, and rollback claims are not established. Auto-remediation engine is wired but not tested end-to-end. |
| Software deployment | Partial | `internal/agent/software/installer.go`, `internal/platform/software_handlers.go` | endpoint-operation tests cover bounded job behavior | The complete advertised package matrix, upload distribution, uninstall behavior, multi-device lifecycle, and representative OS execution are not fully evidenced. |
| Third-party patch catalog | Partial | `internal/inventory/thirdparty.go` | No focused provider/catalog behavior tests were found. | Live vendor discovery, all advertised applications, automatic 24-hour synchronization, and end-to-end deployment are not accepted. |
| Remote support and recordings | Partial | `internal/platform/remote_handlers.go`, `internal/agent/remotecontrol`, `internal/remote` | `remote_session_binding_test.go`, recorder and recording-store tests | A start response is only `pending` and does not prove endpoint acceptance. Windows and macOS capture return blank frames and input injectors are no-ops. Interactive tunnel/viewer and representative-host acceptance remain absent. |
| Network discovery and flow monitoring | Partial | `internal/probe` | 22+ tests cover: ARP parsing (Linux/Windows), ping scan, port scanning (TCP SYN/UDP), service detection, flow parsing (NetFlow v5/v9/IPFIX), device type detection, MAC normalization, broadcast IP calculation, gateway discovery. All tests pass. | `discoverARP` and `pingScan` return no results in isolated CI environments (expected — requires active network). LLDP/CDP/STP topology discovery is not implemented. |
| Vulnerability management | Partial | `internal/inventory/cve_sync.go`, `vulnerability.go`, `remediation.go` | `cve_sync_test.go`; tenant/database suites cover storage boundaries; alert-engine tests cover scoped CVE alert correlation; remediation engine wired with `patch.Executor` integration, policy management, and 4 API endpoints (`/api/v1/remediation/{attempts|summary|policy}`). | Representative OSV/NVD synchronization, package-version matching accuracy, auto-remediation end-to-end testing, and the full manual lifecycle still need evidence. |
| Reports and object storage | Partial; object storage Environment pending | `internal/reporting/reports.go`, `internal/platform/report_handlers.go`, `pkg/storage` | storage backend tests and object-storage integration workflow | Full scheduled delivery and hosted report generation/download have not been accepted in the isolated topology. |
| Provider/MSP lifecycle, branding, domains, entitlements, and usage | Verified for internal alpha; Partial for commercial billing | provider profile, owner invitation, tenant, branding, control-plane, and offboarding handlers | provider/owner integration tests, Phase 8F integration, provider browser suite | External billing backend, immutable billable events, invoices, upgrades/downgrades tied to a billing provider, and historical reconciliation reports are absent. |
| Client portal | Partial | tenant-scoped workspace, report/device APIs, branding profiles | isolation, authorization, UI unit and browser suites | No separate client authentication/SSO boundary or end-client support-request workflow was found. Branding exists, but a complete client-portal acceptance journey is not evidenced. Platform support-access grants are not client support tickets. |
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
