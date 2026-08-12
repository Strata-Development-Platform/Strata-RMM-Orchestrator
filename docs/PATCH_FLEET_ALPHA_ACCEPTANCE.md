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
| PF-03 | Deployment state survives orchestrator restart | Implementation present; restart proof pending | Scheduler reconciles persisted pending/canary/deploying deployments immediately at startup. A process-restart integration test against the durable store/job path is still required. |
| PF-04 | Duplicate/late device results converge idempotently and cannot overwrite newer terminal state | Implementation present; DB integration pending | `ApplyDevicePatchResult` uses scoped durable upsert logic, deduplicates identical state/error pairs, bounds attempts, and prevents terminal installed/reboot-required regression. Requires SQL-backed integration evidence. |
| PF-05 | Retry counters are bounded and durable | Implementation + unit proof; integration pending | Retry ceiling is derived as initial attempt plus bounded policy retries and persisted in durable patch/job state. Helper tests cover bounds; restart/redelivery integration evidence remains required. |
| PF-06 | `reboot_required` is persisted/surfaced without automatic reboot | Implementation present; representative-host proof pending | `reboot_required` is accepted as a terminal patch result and treated as successful completion; no Alpha path authorizes an automatic reboot. Windows representative-host observation remains required. |
| PF-07 | Maintenance windows are enforced before dispatch and after offline-agent recovery | Implementation + unit proof; recovery integration pending | Pending, canary, and broad dispatch paths re-evaluate the maintenance window; malformed windows fail closed. Offline recovery/redelivery must still be proven through the durable job path. |

## Software deployment

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| SF-01 | Create -> target -> dispatch -> agent execution -> result -> aggregate state crosses production NATS/persistence path | Pending end-to-end | Production pieces exist, but a real JetStream + persistence integration harness has not yet proven the entire lifecycle. The create-deployment POST route must also be wired before this can close. |
| SF-02 | SHA-256 failure is terminal and cannot execute payload | Implementation present; execution proof pending | Download verification fails on checksum mismatch before installer execution. Add an execution-boundary test proving the payload is never run after mismatch. |
| SF-03 | Download failure, timeout, cancellation, unsupported package, and uninstall failure are bounded/recorded | Implementation present; failure-matrix proof pending | Installer now fails closed, bounds timeout/error text, validates package types, and records exit/timeout/cancellation outcomes. The full failure matrix still needs behavioral/integration evidence. |
| SF-04 | Duplicate command delivery is safely idempotent/deduplicated | Repository-proven at durable-ledger boundary; JetStream proof pending | Concurrent duplicate receipt test proves one execution claim; terminal results replay after restart and conflicting payload substitution is rejected. JetStream redelivery integration evidence is still required for Alpha closure. |
| SF-05 | Agent disconnect during work cannot produce premature success | Pending integration | ACK occurs only after durable result persistence and result publication; publish/persistence failures NAK. A disconnect-during-work integration test is still required. |

## Fleet durability

| ID | Proof | Status | Current evidence / remaining gate |
|---|---|---|---|
| FD-01 | Outstanding commands survive agent reconnect | Pending integration | Durable JetStream consumer identity is stable per tenant/agent and commands use explicit ACK semantics, but reconnect behavior has not been exercised end-to-end. |
| FD-02 | Outstanding commands survive orchestrator restart | Durable-ledger unit proof; process/NATS proof pending | bbolt tests prove terminal replay and interrupted execution resumption across DB reopen. Full process restart with JetStream redelivery remains required. |
| FD-03 | ACK/NAK/redelivery converges on one logical command outcome | Implementation present; JetStream proof pending | Handler ACKs only after durable completion + result publication, NAKs retryable failures/in-flight duplicates, and terminates malformed/conflicting commands. Requires live JetStream redelivery evidence. |
| FD-04 | Stale result from an older attempt cannot overwrite newer terminal state | Patch implementation present; integration pending | Patch result upsert prevents regression of installed/reboot-required terminal state. Cross-attempt persistence test remains required; software command conflicts are fingerprint-rejected. |
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
2. Add/retain production-boundary integration evidence for canary progression, durable result idempotency, JetStream redelivery/reconnect, and restart recovery.
3. Add the bounded reconnect-storm harness required by FD-05.
4. Retain Windows and Linux representative-host evidence on the final frozen candidate SHA.
5. Freeze the final head and require every repository workflow to be terminal green on that exact SHA before changing the PR out of draft.

## Merge gate

The implementation PR remains draft until the final head is frozen and focused race/integration tests plus the repository's required workflow matrix are terminal green on that exact SHA. Any head change invalidates prior evidence.
