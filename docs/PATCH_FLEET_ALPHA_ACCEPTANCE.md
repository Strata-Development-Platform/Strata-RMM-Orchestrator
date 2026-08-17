# Patch, Software Deployment, and Fleet Durability Alpha Acceptance

This document tracks Issue #147. Completion requires production-boundary evidence for patch/software deployment and agent command durability; structural type tests alone are not sufficient.

## Baseline

- Base branch: `master`
- Starting SHA: `d0a84f7569c4bd6e128f40f4c4b08a5f10ca97d0` (PR #146)
- Last implementation SHA fully validated before this ledger-only update: `679a496601570da845c8ebf751eca4a999209f0b`
- On that SHA all eight required pull-request workflows were terminal green, including CI and the real Postgres+JetStream **Durable Jobs Integration** job.
- Primary packages: `internal/patch`, `internal/agent/software`, generic durable jobs/NATS/agent command paths.
- Evidence statuses distinguish repository proof, integration proof, and representative-host proof. Implementation presence alone is not Alpha completion.

## Patch lifecycle

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| PF-01 | Canary selection uses authoritative tenant-scoped device storage | Repository-proven | `Store.GetCanaryDeploymentDevices` joins durable deployment/device ownership and deterministic canary tests cover bounded selection. |
| PF-02 | Canary threshold success advances rollout; threshold failure halts fail-closed | Integration-proven | `TestPatchCanaryTerminalGateAdvancesOrStopsBroadRollout` uses the production Postgres schema and generic-job target state to prove successful canary progression creates broad rollout while failed canary state halts fail-closed. Passed in Durable Jobs Integration on `679a4966`. |
| PF-03 | Deployment state survives orchestrator/agent restart | Integration-proven | Scheduler reconciliation, stable per-agent JetStream durables, offline command retention, receipt replay, and restart/reconnect harnesses passed in Durable Jobs Integration on `679a4966`. Ambiguous stale `running` receipts fail closed rather than repeating side effects. |
| PF-04 | Duplicate/late results converge idempotently and cannot overwrite newer state | Integration-proven | Result ingestion binds target/agent/attempt/client/site/correlation identity. `TestStaleOlderAttemptCannotOverwriteCurrentTarget` proves an older SQL-backed attempt cannot mutate the current target and the current attempt can still converge. Passed on `679a4966`. |
| PF-05 | Retry counters are bounded, durable, and exhaustion converges terminally | Repository + integration validated | Policy retry cap is persisted/bounded. Retryable maintenance expiry re-enters selection only while budget remains; exhausted work converges terminally. Exact-head CI and durable integration were green on `679a4966`. |
| PF-06 | `reboot_required` is persisted/surfaced without automatic reboot | Repository implementation complete; representative Windows proof pending | Windows Update reads structured `RebootRequired`, surfaces it independently from success, and no Alpha path automatically reboots. Representative-host observation remains required. |
| PF-07 | Maintenance windows are enforced before dispatch and after offline recovery | Repository + integration validated | Strict `HH:MM-HH:MM` parsing fails malformed windows closed. Retryable expired work resumes only inside a valid later window and exhausted work converges terminally. Offline/reconnect integration passed on `679a4966`. |

## Software deployment

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| SF-01 | Create -> target -> dispatch -> execution -> result -> aggregate crosses production persistence/NATS path | Integration-proven | `TestDurableSoftwareDeploymentLifecycleWithRealPostgresAndJetStream` creates through the HTTP handler, persists Postgres deployment/job state, dispatches via JetStream, executes the real agent installer, verifies the filesystem side effect, ingests the durable result, converges the legacy target/deployment aggregate, and persists the result receipt. Passed on `679a4966`. |
| SF-02 | SHA-256 failure is terminal and cannot execute payload | Repository-proven | Production `executeWithContext` verifies SHA-256 before chmod/installer execution. `TestChecksumMismatchNeverExecutesDownloadedPayload` proves checksum mismatch leaves the runnable marker absent. |
| SF-03 | Download failure, timeout, cancellation, unsupported package, and uninstall failure are bounded/recorded | Behavioral proof complete for CI-supported POSIX fixture | `TestSoftwareExecutionFailureMatrixIsTerminalAndBounded` covers download failure, unsupported type before download, bounded timeout, parent cancellation, and uninstall nonzero exit. Representative Windows package behavior remains an environment item. |
| SF-04 | Duplicate command delivery is safely idempotent/deduplicated | Integration-proven | Terminal receipts persist locally; duplicates replay persisted results without re-execution; interrupted ambiguous execution fails closed. JetStream duplicate/reconnect integration passed on `679a4966`. |
| SF-05 | Disconnect/restart cannot produce premature or lost success | Integration-proven for repository harness; representative-host observation pending | Command delivery is JetStream durable and ACKs only after terminal local receipt. Results are durably consumed and ACKed only after DB commit, while the agent retains/replays unacknowledged results. Restart/offline-result retention integration passed on `679a4966`. |

## Fleet durability

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| FD-01 | Outstanding commands survive agent reconnect | Integration-proven | Stable tenant/agent JetStream durables retain commands while consumers are offline. Offline publication/reconnect tests passed on `679a4966`. |
| FD-02 | Outstanding work/results survive orchestrator/agent restart | Integration-proven | Agent terminal receipts persist/replay until platform receipt; platform result consumption is JetStream durable and ACK-after-DB-commit. Restart/redelivery tests passed on `679a4966`. |
| FD-03 | ACK/NAK/redelivery converges on one logical outcome | Integration-proven | Generic command consumer uses explicit ACK/NAK/TERM, in-progress heartbeats, durable receipts, terminal replay, and idempotent `job_inbox` claiming. Durable Jobs Integration passed on `679a4966`. |
| FD-04 | Stale older-attempt result cannot overwrite newer terminal state | Integration-proven | `TestStaleOlderAttemptCannotOverwriteCurrentTarget` provides SQL-backed cross-attempt proof and passed on `679a4966`. |
| FD-05 | Bounded reconnect-storm harness preserves trust boundaries and produces evidence | Integration-proven | `TestBoundedReconnectStormPreservesTenantAgentIsolation` cycles six endpoint identities across two tenants through four offline/reconnect rounds, queues work offline, verifies identity-scoped execution, and proves duplicate suppression. Passed on `679a4966`. |

## Exact-head workflow evidence

Implementation SHA `679a496601570da845c8ebf751eca4a999209f0b` passed all eight required PR workflows:

1. CI
2. Internal Alpha Agent
3. Phase 8G Security Gate
4. Phase 8E Resilience Validation
5. Phase 8F MSP Lifecycle and Unified Dashboards
6. Phase 8D Observability and Synthetics
7. Phase 8C Backup, Restore, and Disaster Recovery
8. Release Packaging

The CI run also passed **Durable Jobs Integration**, `go test ./... -count=1 -race`, vet, lint, security scanning, frontend validation, database isolation, SaaS control-plane tests, deployment/upgrade validation, and durable-job preservation.

Because this acceptance-ledger reconciliation is itself a commit, the resulting documentation-only successor SHA must also receive terminal-green required workflows before merge under the project's exact-head rule.

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

## Remaining blockers before merge

1. Require every required workflow to be terminal green on the new ledger-update head SHA.
2. Retain representative Windows endpoint evidence for patch/install/reboot-required behavior.
3. Retain representative Linux endpoint evidence for native patching and software deployment.
4. Keep PR #148 draft until those environment gates are satisfied or the project explicitly separates representative-host acceptance from the PR merge gate.

## Merge gate

PR #148 remains draft until the final candidate head is frozen and every required repository workflow is terminal green on that exact SHA. Any head change invalidates earlier exact-head workflow status. Representative-host items remain environment gates and must not be self-certified by repository-only tests.
