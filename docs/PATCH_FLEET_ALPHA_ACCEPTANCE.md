# Patch, Software Deployment, and Fleet Durability Alpha Acceptance

This document tracks Issue #147. Completion requires production-boundary evidence for patch/software deployment and agent command durability; structural type tests alone are not sufficient.

## Baseline

- Base branch: `master`
- Starting SHA: `d0a84f7569c4bd6e128f40f4c4b08a5f10ca97d0` (PR #146)
- Primary packages: `internal/patch`, `internal/agent/software`, generic durable jobs/NATS/agent command paths
- Evidence statuses distinguish repository proof, integration proof, and representative-host proof. Implementation presence alone is not Alpha completion.

## Patch lifecycle

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| PF-01 | Canary selection uses authoritative tenant-scoped device storage | Repository-proven | `Store.GetCanaryDeploymentDevices` joins durable deployment/device ownership and deterministic canary tests cover bounded selection. |
| PF-02 | Canary threshold success advances rollout; threshold failure halts fail-closed | Implementation + unit proof; integration pending | `advanceCanaryDeployment` gates on durable generic-job target results and persists failed or broad-rollout state. Production DB/job-path canary progression evidence is still required. |
| PF-03 | Deployment state survives orchestrator/agent restart | Implementation strengthened; job-integration evidence pending exact head | Scheduler reconciles persisted patch deployments immediately at startup. Generic commands now use stable per-agent JetStream durables. A stale `running` receipt after agent restart fails closed as outcome-unknown instead of re-executing an ambiguous side effect. Reconnect/restart integration tests are retained under the `jobintegration` tag and must pass on the final SHA. |
| PF-04 | Duplicate/late results converge idempotently and cannot overwrite newer state | Strong implementation evidence; SQL cross-attempt proof pending | Generic result ingestion binds target/agent/attempt/client/site/correlation identity and rejects invalid terminal transitions. Cross-attempt SQL integration evidence remains required. |
| PF-05 | Retry counters are bounded, durable, and exhaustion converges terminally | Implementation strengthened; exact-head validation pending | Policy retry cap is persisted/bounded. Retryable maintenance expiry re-enters selection only while budget remains. An expired target whose persisted dispatch+retry budget is exhausted is classified as terminal patch failure so the rollout cannot hang indefinitely. |
| PF-06 | `reboot_required` is persisted/surfaced without automatic reboot | Repository implementation complete; representative Windows proof pending | Windows Update reads the structured `RebootRequired` result and returns it as structured command data; ResultCode success is checked independently. No Alpha path automatically reboots. Representative-host observation remains required. |
| PF-07 | Maintenance windows are enforced before dispatch and after offline recovery | Implementation strengthened; integration pending | Strict `HH:MM-HH:MM` parsing fails malformed windows closed in both dispatch and expiry. Retryable expired work can resume in a later valid window and exhausted work converges to patch failure. Offline/reconnect integration must pass on the final SHA. |

## Software deployment

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| SF-01 | Create -> target -> dispatch -> execution -> result -> aggregate crosses production persistence/NATS path | Partial production-boundary proof; software-specific E2E pending | Creation is transactionally bridged into generic jobs/outbox with authoritative device→agent identity. The generic Postgres+JetStream round trip is integration-tested, but a software-specific create-to-legacy-aggregate E2E case is still required. |
| SF-02 | SHA-256 failure is terminal and cannot execute payload | Repository-proven | Production `executeWithContext` verifies SHA-256 during download before chmod/installer execution. `TestChecksumMismatchNeverExecutesDownloadedPayload` uses a runnable marker script and proves checksum mismatch leaves the marker absent. |
| SF-03 | Download failure, timeout, cancellation, unsupported package, and uninstall failure are bounded/recorded | Implementation + unit proof; failure-matrix integration pending | Installer validates package types, bounds timeout/error text, propagates cancellation/timeout, and records terminal failure. Complete behavioral failure-matrix evidence remains open. |
| SF-04 | Duplicate command delivery is safely idempotent/deduplicated | Repository proof + JetStream integration harness; exact-head pass pending | Terminal receipts persist locally, duplicates replay the persisted result without re-execution, and ambiguous interrupted execution fails closed. Offline/reconnect and duplicate-delivery jobintegration tests must pass on the final SHA. |
| SF-05 | Disconnect/restart cannot produce premature or lost success | Implementation strengthened; integration pending | Generic command delivery is JetStream durable and ACKs only after a terminal local receipt exists. Generic results are durably consumed from `STRATA_CMD_RESULTS`, ACK only after `job_inbox.processed_at` is committed, and the agent retains/replays unacknowledged results. Disconnect-during-work proof remains required. |

## Fleet durability

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| FD-01 | Outstanding commands survive agent reconnect | Integration harness added; exact-head pass pending | Stable tenant/agent JetStream durables retain commands while consumers are offline. `TestCommandPublishedWhileAgentOfflineExecutesAfterReconnect` and the bounded reconnect-storm harness exercise offline publication and reconnect. |
| FD-02 | Outstanding work/results survive orchestrator/agent restart | Integration harness added; exact-head pass pending | Agent terminal receipts persist and replay until platform receipt; stale `running` receipts fail closed without repeat execution. Platform result consumption is JetStream durable and ACK-after-DB-commit. Final exact-head restart/redelivery evidence is required. |
| FD-03 | ACK/NAK/redelivery converges on one logical outcome | Implementation + integration harness; exact-head pass pending | Generic command consumer uses manual explicit ACK/NAK/TERM, in-progress heartbeats, durable receipts, and terminal-result replay. Platform results are idempotently claimed in `job_inbox` and broker-ACKed only after durable processing. |
| FD-04 | Stale older-attempt result cannot overwrite newer terminal state | Strong implementation evidence; integration pending | Result ingestion requires exact target, agent, attempt, tenant/client/site, and correlation identity and rejects invalid terminal transitions. Cross-attempt SQL integration evidence remains open. |
| FD-05 | Bounded reconnect-storm harness preserves trust boundaries and produces evidence | Harness added; exact-head pass pending | `TestBoundedReconnectStormPreservesTenantAgentIsolation` cycles six endpoint identities across two tenants through four offline/reconnect rounds, queues work while offline, verifies per-identity execution, and proves duplicate suppression. |

## Representative-host evidence

Environment-dependent acceptance remains separate from repository-only proof.

- **Windows pending:** Windows Update scan/install on a representative endpoint, structured reboot-required observation, and supported software install/uninstall evidence where available.
- **Linux pending:** native OS patch scan/install plus a representative DEB/RPM or script software deployment.
- No representative result may be marked complete without retained evidence tied to the final candidate SHA.

## Security invariants

- Tenant/MSP/client/site/device scope comes from authoritative host state.
- Agent payload cannot self-select broader scope.
- Patch IDs are validated against the latest tenant/device missing-patch inventory before deployment creation.
- Caller-derived patch identifiers are validated/escaped before Windows command construction.
- No automatic reboot in Alpha unless an explicit approved policy/action authorizes it.
- NATS isolation, RLS, approval gates, checksum verification, and maintenance windows may not be weakened for test convenience.

## Immediate blockers before merge

1. Freeze the candidate SHA and require all required repository workflows, including the real Postgres+JetStream durable-job suite and reconnect-storm harness, to be terminal green on that exact head.
2. Add software-specific end-to-end evidence from durable deployment creation through generic execution/result and legacy deployment aggregate convergence (SF-01).
3. Retain SQL-backed canary progression and cross-attempt stale-result evidence for PF-02/PF-04/FD-04.
4. Complete the software failure-matrix integration evidence for SF-03/SF-05.
5. Retain representative Windows and Linux endpoint evidence on the final candidate SHA.

## Merge gate

PR #148 remains draft until the final head is frozen and focused race/integration tests plus every required repository workflow are terminal green on that exact SHA. Any head change invalidates prior evidence. Representative-host items remain environment gates and must not be self-certified by repository-only tests.
