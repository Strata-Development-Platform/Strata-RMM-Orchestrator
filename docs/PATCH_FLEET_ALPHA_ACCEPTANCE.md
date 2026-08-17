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
| PF-02 | Canary threshold success advances rollout; threshold failure halts fail-closed | Integration-proven | SQL-backed `TestPatchCanaryTerminalGateAdvancesOrStopsBroadRollout` proves successful canary progression creates broad rollout while failed canary state halts broad dispatch. |
| PF-03 | Deployment state survives orchestrator/agent restart | Integration-proven; representative-host observation pending | Scheduler reconciles persisted patch deployments at startup. Generic commands use stable per-agent JetStream durables. Restart/reconnect jobintegration coverage passes on the candidate head; host evidence still needs to show real endpoint convergence. |
| PF-04 | Duplicate/late results converge idempotently and cannot overwrite newer state | Integration-proven | Generic result ingestion binds target/agent/attempt/client/site/correlation identity. SQL-backed cross-attempt coverage proves a stale attempt cannot overwrite the current target while the current attempt can converge. |
| PF-05 | Retry counters are bounded, durable, and exhaustion converges terminally | Repository + integration validation | Policy retry cap is persisted/bounded. Retryable maintenance expiry re-enters selection only while budget remains. Exhausted work converges terminally so rollout cannot hang indefinitely. |
| PF-06 | `reboot_required` is persisted/surfaced without automatic reboot | Repository implementation complete; representative Windows proof pending | Windows Update returns structured reboot-required state and Alpha does not automatically reboot. A real Windows endpoint must still demonstrate the surfaced state when applicable. |
| PF-07 | Maintenance windows are enforced before dispatch and after offline recovery | Repository + integration validation | Strict `HH:MM-HH:MM` parsing fails malformed windows closed. Retryable expired work can resume only in a later valid window and reconnect/offline coverage passes in jobintegration. |

## Software deployment

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| SF-01 | Create -> target -> dispatch -> execution -> result -> aggregate crosses production persistence/NATS path | Integration-proven | `TestDurableSoftwareDeploymentLifecycleWithRealPostgresAndJetStream` creates through the HTTP handler, persists Postgres state, dispatches via JetStream, executes the real agent installer, verifies the host-side marker, ingests the durable result, and converges the legacy aggregate. |
| SF-02 | SHA-256 failure is terminal and cannot execute payload | Repository-proven | Production `executeWithContext` verifies SHA-256 before execution. `TestChecksumMismatchNeverExecutesDownloadedPayload` proves a runnable mismatched payload never executes. |
| SF-03 | Download failure, timeout, cancellation, unsupported package, and uninstall failure are bounded/recorded | Repository-proven; representative-host observation pending | `TestSoftwareExecutionFailureMatrixIsTerminalAndBounded` covers download failure, unsupported type, timeout, cancellation and uninstall nonzero exit. Representative endpoints still need to prove supported real package behavior. |
| SF-04 | Duplicate command delivery is safely idempotent/deduplicated | Integration-proven | Terminal receipts persist locally; duplicates replay persisted terminal results without re-execution; reconnect and duplicate-delivery jobintegration coverage passes. |
| SF-05 | Disconnect/restart cannot produce premature or lost success | Integration-proven | Command delivery is JetStream durable and ACKs only after terminal local receipt. Platform result ingestion ACKs only after durable processing; restart/offline result-retention integration coverage passes. |

## Fleet durability

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| FD-01 | Outstanding commands survive agent reconnect | Integration-proven | Stable tenant/agent JetStream durables retain commands while consumers are offline; offline publication executes after reconnect. |
| FD-02 | Outstanding work/results survive orchestrator/agent restart | Integration-proven | Terminal receipts persist and replay until platform receipt; stale ambiguous running receipts fail closed; platform result consumption is durable. |
| FD-03 | ACK/NAK/redelivery converges on one logical outcome | Integration-proven | Generic command consumer uses explicit broker acknowledgment, durable receipts and terminal replay; platform results are idempotently claimed and ACKed after commit. |
| FD-04 | Stale older-attempt result cannot overwrite newer terminal state | Integration-proven | SQL cross-attempt evidence rejects the stale attempt and allows the authoritative attempt to converge. |
| FD-05 | Bounded reconnect-storm harness preserves trust boundaries and produces evidence | Integration-proven | `TestBoundedReconnectStormPreservesTenantAgentIsolation` cycles multiple identities through offline/reconnect rounds and proves per-identity execution plus duplicate suppression. |

## Representative-host evidence

Environment-dependent acceptance remains separate from repository-only proof. Follow `docs/REPRESENTATIVE_HOST_ALPHA_ACCEPTANCE.md` and use the platform-specific collectors.

- **Windows pending:** Windows Update scan/install on a representative enrolled endpoint, structured reboot-required observation when applicable, proof no automatic reboot occurs, and supported software install/uninstall evidence.
- **Linux pending:** native OS patch scan/install plus a representative DEB/RPM or script software deployment on an enrolled endpoint.
- Evidence collectors: `scripts/alpha-host-evidence-windows.ps1` and `scripts/alpha-host-evidence-linux.sh`.
- No representative result may be marked complete without retained before/after evidence tied to the final candidate SHA and corresponding Strata deployment/job/target identifiers.

## Security invariants

- Tenant/MSP/client/site/device scope comes from authoritative host state.
- Agent payload cannot self-select broader scope.
- Patch IDs are validated against the latest tenant/device missing-patch inventory before deployment creation.
- Caller-derived patch identifiers are validated/escaped before Windows command construction.
- No automatic reboot in Alpha unless an explicit approved policy/action authorizes it.
- NATS isolation, RLS, approval gates, checksum verification, and maintenance windows may not be weakened for test convenience.

## Immediate blockers before merge

1. Freeze the final candidate SHA and require every required repository workflow to be terminal green on that exact head.
2. Execute and retain the representative Windows host acceptance package tied to that SHA.
3. Execute and retain the representative Linux host acceptance package tied to that SHA.
4. Reconcile the retained host evidence into this ledger without self-certifying environment behavior from repository tests.

## Merge gate

PR #148 remains draft until the final head is frozen, every required repository workflow is terminal green on that exact SHA, and both representative-host evidence packages are retained. Any head change invalidates earlier exact-head repository and representative-host evidence.
