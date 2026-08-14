# Patch, Software Deployment, and Fleet Durability Alpha Acceptance

This document tracks Issue #147. Completion requires production-boundary evidence for patch/software deployment and agent command durability; structural type tests alone are not sufficient.

## Baseline

- Base branch: `master`
- Starting SHA: `d0a84f7569c4bd6e128f40f4c4b08a5f10ca97d0` (PR #146)
- Primary packages: `internal/patch`, `internal/agent/software`, durable jobs/NATS/agent command paths
- Evidence statuses below intentionally distinguish repository proof from integration or representative-host proof. Implementation presence alone is not Alpha completion.

## Patch lifecycle

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| PF-01 | Canary selection uses authoritative tenant-scoped device storage, not an empty/stub selector | Repository-proven | `Store.GetCanaryDeploymentDevices` resolves deployment targets by joining durable deployment/device ownership and fails closed without the store/DB. Canary selection tests cover deterministic bounded selection. |
| PF-02 | Canary threshold success advances rollout; threshold failure halts rollout fail-closed | Implementation + unit proof; integration pending | `advanceCanaryDeployment` gates on durable job results and persists either failed or broad-rollout state; `CanaryGate` threshold tests cover pass/fail math. Still requires production DB/job-path integration evidence. |
| PF-03 | Deployment state survives orchestrator/agent restart | Partial implementation; restart reconciliation pending | Orchestrator scheduler re-reads persisted pending/canary/deploying deployments at startup. Agent generic-job receipts persist, but a command left `running` across agent restart does not yet have an authoritative outcome-reconciliation path and can remain stranded. Process-restart integration evidence remains required. |
| PF-04 | Duplicate/late device results converge idempotently and cannot overwrite newer terminal state | Implementation present; DB integration pending | Generic durable-job result ingestion binds job/target/agent/attempt/correlation identity and rejects invalid terminal transitions; patch result storage also prevents terminal regression. SQL-backed cross-attempt evidence is still required. |
| PF-05 | Retry counters are bounded and durable | Implementation + unit proof; integration pending | Retry ceiling is derived as initial attempt plus bounded persisted policy retries. Retryable expired rollout targets now re-enter scheduler selection only while `dispatch_attempts + target retry_count` remains below that cap. Exhausted-expiry terminal reconciliation still needs closure and DB-backed restart evidence. |
| PF-06 | `reboot_required` is persisted/surfaced without automatic reboot | Incomplete; representative-host and structured-result work pending | The control plane recognizes `reboot_required` as terminal success, but the endpoint Windows execution path still needs authoritative structured reboot-required detection rather than relying on human-readable output. Windows representative-host evidence remains required. |
| PF-07 | Maintenance windows are enforced before dispatch and after offline-agent recovery | Implementation strengthened; recovery integration pending | Strict `HH:MM-HH:MM` parsing now fails malformed windows closed in both dispatch and durable-job expiry. Retryable expired targets can be selected in a later valid window. Exhausted-expiry terminal reconciliation and offline reconnect integration remain required. |

## Software deployment

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| SF-01 | Create -> target -> dispatch -> agent execution -> result -> aggregate state crosses production NATS/persistence path | Pending end-to-end | Production pieces exist, but a real JetStream + persistence integration harness has not yet proven the entire lifecycle. The patch deployment create route is also still unreachable until its POST route is registered. |
| SF-02 | SHA-256 failure is terminal and cannot execute payload | Implementation present; execution proof pending | Download verification fails on checksum mismatch before installer execution. Add an execution-boundary test proving the payload is never run after mismatch. |
| SF-03 | Download failure, timeout, cancellation, unsupported package, and uninstall failure are bounded/recorded | Implementation present; failure-matrix proof pending | Installer fails closed, bounds timeout/error text, validates package types, and records exit/timeout/cancellation outcomes. The full failure matrix still needs behavioral/integration evidence. |
| SF-04 | Duplicate command delivery is safely idempotent/deduplicated | Repository-proven for concurrency and terminal replay; crash ambiguity now fails closed | Concurrent duplicate receipt tests prove one execution claim and terminal results replay after restart. A receipt found `running` after restart is now converted to a durable terminal `failed` outcome-unknown result instead of re-executing an ambiguous endpoint side effect. Live JetStream redelivery evidence is still required. |
| SF-05 | Agent disconnect during work cannot produce premature success | Pending integration | Agent command ACK occurs only after durable local completion and result publication, but orchestrator software-result ingestion is still core NATS and target/aggregate persistence is not yet one crash-consistent durable boundary. Disconnect/restart integration remains required. |

## Fleet durability

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| FD-01 | Outstanding commands survive agent reconnect | Pending integration | Durable software command consumer identity is stable per tenant/agent and uses explicit ACK semantics, but reconnect behavior has not been exercised end-to-end. Generic job command transport/recovery must also be reconciled against restart behavior. |
| FD-02 | Outstanding commands survive orchestrator/agent restart | Partial durable proof; process/NATS reconciliation pending | Software terminal results survive DB reopen, and ambiguous interrupted software execution now fails closed without duplicate execution. Generic durable-job receipts persist but lack production startup reconciliation for `running` receipts. Full process restart with JetStream/redelivery remains required. |
| FD-03 | ACK/NAK/redelivery converges on one logical command outcome | Implementation present; JetStream proof pending | Software consumer ACKs only after durable completion + result publication, NAKs retryable failures/in-flight duplicates, and terminates malformed/conflicting commands. Orchestrator result consumption must become durable and crash-consistent before this can close. |
| FD-04 | Stale result from an older attempt cannot overwrite newer terminal state | Strong implementation evidence; integration pending | Generic job result ingestion requires exact target, agent, attempt, tenant/client/site, and correlation identity and rejects invalid terminal transitions, preventing a stale result from mutating a replacement target. Cross-attempt SQL integration evidence remains required. |
| FD-05 | Bounded reconnect-storm harness preserves trust boundaries and produces evidence | Pending | No bounded reconnect-storm harness evidence is retained yet. |

## Representative-host evidence

Environment-dependent acceptance must remain explicitly separate from repository-only proof.

- Windows: Windows Update scan/install on representative endpoint, Winget/Chocolatey where available, reboot-required observation.
- Linux: native OS patch scan/install plus DEB/RPM or script deployment on representative endpoint.
- No hosted/representative result may be marked complete without retained evidence tied to an exact candidate SHA.

## Security invariants

- Tenant/MSP/client/site/device scope comes from authoritative host state.
- Agent payload cannot self-select broader scope.
- No automatic reboot in Alpha unless an explicit approved policy/action authorizes it.
- NATS isolation, RLS, approval gates, checksum verification, and maintenance windows may not be weakened for test convenience.

## Immediate blockers before merge

1. Wire `POST /api/v1/patch-deployments` to `s.handleCreatePatchDeployment`; the current unregistered handler is the exact cause of the PR's golangci-lint failure and leaves deployment creation unreachable.
2. Validate selected patch IDs against authoritative tenant-scoped inventory/catalog data and remove patch-ID interpolation from executable PowerShell source.
3. Resolve software device IDs to authoritative agent IDs for NATS command/result routing; tenant-scope deployment detail reads.
4. Make software result ingestion durable and crash-consistent, including aggregate reconciliation on duplicate terminal delivery.
5. Complete generic patch restart reconciliation and exhausted-expiry terminal handling.
6. Add/retain production-boundary integration evidence for canary progression, JetStream redelivery/reconnect, restart recovery, and the bounded FD-05 reconnect-storm harness.
7. Retain Windows and Linux representative-host evidence on the final frozen candidate SHA.
8. Freeze the final head and require every repository workflow to be terminal green on that exact SHA before changing the PR out of draft.

## Merge gate

The implementation PR remains draft until the final head is frozen and focused race/integration tests plus the repository's required workflow matrix are terminal green on that exact SHA. Any head change invalidates prior evidence.
