# Patch, Software Deployment, and Fleet Durability Alpha Acceptance

This document tracks Issue #147. Completion requires production-boundary evidence for patch/software deployment and agent command durability; structural type tests alone are not sufficient.

## Baseline

- Base branch: `master`
- Starting SHA: `d0a84f7569c4bd6e128f40f4c4b08a5f10ca97d0` (PR #146)
- Primary packages: `internal/patch`, `internal/agent/software`, durable jobs/NATS/agent command paths

## Patch lifecycle

| ID | Proof | Status |
|---|---|---|
| PF-01 | Canary selection uses authoritative tenant-scoped device storage, not an empty/stub selector | Pending |
| PF-02 | Canary threshold success advances rollout; threshold failure halts rollout fail-closed | Pending |
| PF-03 | Deployment state survives orchestrator restart | Pending |
| PF-04 | Duplicate/late device results converge idempotently and cannot overwrite newer terminal state | Pending |
| PF-05 | Retry counters are bounded and durable | Pending |
| PF-06 | `reboot_required` is persisted/surfaced without automatic reboot | Pending |
| PF-07 | Maintenance windows are enforced before dispatch and after offline-agent recovery | Pending |

## Software deployment

| ID | Proof | Status |
|---|---|---|
| SF-01 | Create -> target -> dispatch -> agent execution -> result -> aggregate state crosses production NATS/persistence path | Pending |
| SF-02 | SHA-256 failure is terminal and cannot execute payload | Pending |
| SF-03 | Download failure, timeout, cancellation, unsupported package, and uninstall failure are bounded/recorded | Pending |
| SF-04 | Duplicate command delivery is safely idempotent/deduplicated | Pending |
| SF-05 | Agent disconnect during work cannot produce premature success | Pending |

## Fleet durability

| ID | Proof | Status |
|---|---|---|
| FD-01 | Outstanding commands survive agent reconnect | Pending |
| FD-02 | Outstanding commands survive orchestrator restart | Pending |
| FD-03 | ACK/NAK/redelivery converges on one logical command outcome | Pending |
| FD-04 | Stale result from an older attempt cannot overwrite newer terminal state | Pending |
| FD-05 | Bounded reconnect-storm harness preserves trust boundaries and produces evidence | Pending |

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

## Merge gate

The implementation PR remains draft until the final head is frozen and focused race/integration tests plus the repository's required workflow matrix are terminal green on that exact SHA. Any head change invalidates prior evidence.
