# Feature completeness matrix

**Audit date:** 2026-08-30  
**Authority:** This file is the repository's feature-completeness truth source.  
**Related gates:** #167 (Gate D source audit), #15 (hosted internal-alpha validation)

A route, schema, UI component, mock, structural test, or successful build is not by itself proof that a capability is complete. This matrix separates repository behavior from deferred product scope and from behavior that can only be accepted on representative infrastructure.

## Status definitions

- **Verified** — the required repository behavior is implemented and exercised by automated behavior/integration/security tests.
- **Partial / deferred** — useful behavior exists, but part of the intended product scope is absent, simulated, deliberately deferred, or lacks representative acceptance.
- **Not implemented** — the named behavior is absent or intentionally a stub.
- **Environment pending** — repository implementation/automation exists, but acceptance requires representative hosts, providers, networks, or continuous topology and is not inferred from CI.

## Beta boundary

The repository implements the core multi-tenant RMM control plane, endpoint identity/enrollment, telemetry, alerting, durable endpoint jobs, scripts, tenant authorization, backup/recovery, deployment assets, and provider/MSP lifecycle needed for the planned internal-alpha exercise. Gate D has additionally remediated concrete source defects in script dispatch durability (#168), script tenant boundaries (#170, #172), broker result identity (#174), scheduled execution correlation (#175), scheduled timeout accounting (#178), interactive remote-session lifecycle (#181), and crash-durable cancellation (#183).

This does **not** mean the full product specification is feature-complete. Representative hosted acceptance remains governed by #15. Deliberately deferred commercial/SSO/full-topology capabilities are not beta acceptance evidence and are not hidden by phase-level `Complete` labels.

## Capability matrix

| Capability | Repository status | What is established | Remaining/deferred acceptance |
|---|---|---|---|
| Tenant hierarchy, enrollment, endpoint identity | **Verified** for repository/internal-alpha scope | MSP/client/site scoping, enrollment tokens, device/agent identity, RLS/authorization and negative isolation paths are exercised in CI. | One continuous hosted provider→MSP→client/site→endpoint journey is **Environment pending** under #15. |
| Telemetry and metrics ingestion | **Verified** for repository scope | Agent heartbeat/metrics identity, broker ingestion, Timescale persistence and replay/duplicate protections have automated coverage. | Live enrolled-host observation and long-running reconnect/load behavior are **Environment pending**. |
| Core dashboards / observability | **Partial + Environment pending** | Grafana/Prometheus assets, telemetry APIs and dashboard build/tests exist. | Live dashboard observation requires #15. The settings **Integration Dashboard** is currently mock-backed and is not accepted as a real provider-health/event dashboard. |
| Alert evaluation and notifications | **Verified core; Environment pending live providers** | Threshold/heartbeat lifecycle, cooldown/grouping/maintenance behavior, persistence and configured notifier paths are covered. | Slack/Teams/PagerDuty/SMTP/webhook delivery against representative live providers remains **Environment pending**. |
| Policy engine | **Partial / deferred full scope** | Validation, hierarchy/effective policy, revision/publication/rollback and enforcement scheduling exist with automated coverage. | Policy-to-script binding remains deferred/not implemented as a product expansion. |
| Scripts and durable endpoint jobs | **Verified** for repository scope | Durable outbox dispatch, script execution/schedules/results, tenant+agent result identity, current-attempt correlation, timeout accounting, cancellation durability and stale-result protections are covered. | Native representative-host execution and soak/reconnect behavior remain **Environment pending**. |
| OS patch management | **Partial** | Windows/Linux parsing and package-manager execution paths exist with automated unit/contract coverage. | Representative Chocolatey/Winget/WSUS/Flatpak/Snap/apt/dnf/yum/zypper execution, canary/rollback and full remediation journeys remain **Environment pending** unless specifically covered by source tests. |
| Software deployment | **Partial** | MSI/EXE/DEB/RPM/AppImage/script command paths, checksum verification, timeouts and deployment lifecycle code exist. | Representative OS install/uninstall/distribution behavior remains **Environment pending**. |
| Third-party patch catalog | **Partial** | Vendor/version discovery, sync scheduling, error handling and catalog/deployment state logic are implemented/tested with controlled sources. | Live vendor APIs and real endpoint deployment remain **Environment pending**. |
| Remote support / recordings | **Partial + Environment pending** | Session authorization/lifecycle, WebRTC/TURN-STUN configuration, recording/transcription plumbing and platform capture/input code exist; Gate D fixed interactive binding lifecycle in #181. | Representative Windows/Linux/macOS interactive sessions, TURN behavior and recording/transcription providers remain **Environment pending**. |
| Network discovery / flow monitoring | **Partial** | ARP/ping/port/service/flow/SNMP parsing and discovery primitives exist. | LLDP/CDP/STP topology functions in `internal/probe/discovery.go` are explicit stubs; full topology discovery is **Not implemented**. Real-network discovery is **Environment pending**. |
| Vulnerability management | **Partial** | OSV-oriented records/version matching/lifecycle and remediation wiring exist with automated coverage. | Live external-feed behavior and representative end-to-end remediation remain **Environment pending**. |
| Reports and object storage | **Partial + Environment pending** | Report generation/export/schedule and local/S3-compatible storage paths have repository coverage. | Hosted generation/download/delivery and provider-specific behavior remain **Environment pending**. |
| Provider/MSP lifecycle, branding, entitlement | **Verified** for internal-alpha control-plane scope; **Partial** commercial scope | Provider setup, owner invitation, MSP/client/site lifecycle, branding/entitlements/offboarding and tenant isolation are exercised by Phase 8F/browser/integration coverage. | External billing backend, immutable billable events, provider-backed invoice generation and reconciliation are deferred. |
| Client portal | **Partial / deferred** | Tenant-scoped support-request/runtime pieces exist. | Full end-client authentication/session journey and SSO/OIDC are deferred. |
| Authentication/account lifecycle | **Partial / deferred full scope** | JWT, TOTP/MFA, API-key and invitation/authz paths are implemented and security-tested. | Password recovery, refresh-token rotation, asymmetric access-token signing, SSO/OIDC and trusted-proxy expansion remain deferred unless separately promoted into beta scope. |
| Deployment, upgrade, recovery, supply chain | **Verified in repository; Environment pending hosted** | Replay-safe installer behavior, signed/digest-pinned Docker lifecycle, fail-closed unsupported modes, migrations, backup/restore, rollback, SBOM/vulnerability/secret scans and deployment templates are covered by CI/Phase 8 workflows. | Native Linux/Windows clean-host lifecycle, continuous upgrade/rollback, RPO/RTO drill, multi-region/air-gapped claims and 24-hour soak are **Environment pending** under #15. |
| EDR / backup / PSA integration surfaces | **Partial + Environment pending** | Webhook/authentication/provider adapter contracts are present where implemented. | Representative third-party provider interoperability is not accepted from mocks alone. |

## Explicit deferred or not-implemented product scope

The following are intentionally visible rather than being counted as completed beta features: SSO/OIDC; password recovery; refresh-token rotation; asymmetric token signing; external commercial billing backend; immutable billable events; provider-backed invoice generation/reconciliation; policy-to-script binding; and full LLDP/CDP/STP topology discovery. Moving any of these into the beta must-have boundary requires a focused issue, implementation evidence, and exact-head CI.

## Hosted internal-alpha acceptance

The following remain **Environment pending** and are tracked by #15: clean isolated install; provider bootstrap and tenant creation; Linux AMD64 plus Windows AMD64 endpoint enrollment/service execution; live telemetry/dashboard observation; broker/database/object-storage fault injection; reconnect storms; harmless and destructive remote operations with ambiguous-outcome handling; baseline load; 24-hour soak; backup restore with measured RPO/RTO; and signed owner go/no-go evidence. Repository CI cannot substitute for these exercises.

## Gate D source-remediation ledger

| Issue | Source defect remediated |
|---|---|
| #168 | Interactive script request bypassed durable outbox. |
| #170 | Script get/delete ignored route tenant boundary. |
| #172 | Script execution detail ignored route tenant boundary. |
| #174 | Script result persistence did not bind trusted broker tenant/agent identity. |
| #175 | Scheduled rerun dispatch identity diverged from persisted execution identity. |
| #178 | Scheduled timeout results bypassed retry/completion accounting. |
| #181 | Interactive remote sessions were not bound to lifecycle state. |
| #183 | Job cancellation delivery was best-effort instead of crash-durable. |
| #185 | Reconciles unsupported/stale feature-completion claims across status documents. |

## Evidence rule

No timeless `all CI green` statement is authoritative. A merge decision must inspect the exact PR head and require the repository's mandated workflows to be terminal successful. After merge, hosted acceptance remains separate and can only be closed by #15 evidence.
