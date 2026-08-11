# WASI Alpha Acceptance Evidence

This document tracks the executable add-on Alpha gate defined by issue #145. It is intentionally stricter than unit- or contract-level evidence: completion requires a real signed reference module to cross the production package, activation, runtime, broker, and response boundaries on one exact PR head SHA.

## Candidate baseline

- Base branch: `master`
- Starting SHA: `31e10495a6430b58a2f9c45613c51dd323a6d606` (PR #144)
- Runtime engine: wazero
- Guest response ABI: schema version 1
- Required broker proof: `devices.get`

## Production path to prove

```text
signed package
  -> publisher/key trust verification
  -> manifest validation
  -> payload validation
  -> immutable materialization
  -> durable install
  -> enable
  -> health-gated activation
  -> RuntimeSupervisor
  -> WASIRuntime / wazero
  -> strata_broker host ABI
  -> authoritative devices.get resolution
  -> strict WASI response ABI
  -> InvocationResult
```

No direct registry mutation, direct database insertion, hand-written active-version metadata, bypassed signature verification, or mock runtime may be counted as full-path evidence.

## Positive acceptance rows

| ID | Proof | Status | Evidence |
|---|---|---|---|
| WA-01 | Deterministic reference WASI module source/fixture can be reproduced in CI | Pending | |
| WA-02 | Reference package is signed and accepted only by configured publisher/key trust | Pending | |
| WA-03 | Package passes production manifest/payload validation and immutable materialization | Pending | |
| WA-04 | Module installs and enables through the production lifecycle path | Pending | |
| WA-05 | Candidate activation uses the real health gate | Pending | |
| WA-06 | Real wazero execution succeeds with no ambient host capability | Pending | |
| WA-07 | Brokered invocation reaches `devices.get` with authoritative scope | Pending | |
| WA-08 | Sibling/cross-tenant device substitution is denied | Pending | |
| WA-09 | Guest output is accepted only through WASI response ABI v1 | Pending | |
| WA-10 | Signed upgrade to a second immutable version executes successfully | Pending | |
| WA-11 | Rollback restores the prior signed version and executes successfully | Pending | |

## Runtime hostile-path rows

| ID | Proof | Status | Evidence |
|---|---|---|---|
| WR-01 | Malformed/truncated WASM fails closed without orchestrator crash | Pending | |
| WR-02 | Undeclared host imports fail instantiation | Pending | |
| WR-03 | Host filesystem is unavailable by default | Pending | |
| WR-04 | Host environment, process arguments, and secrets are not inherited | Pending | |
| WR-05 | `network:none` exposes no raw socket/network capability | Pending | |
| WR-06 | `network:brokered` exposes reviewed broker calls only, never raw sockets | Pending | |
| WR-07 | Execution timeout terminates boundedly | Pending | |
| WR-08 | Caller cancellation terminates boundedly | Pending | |
| WR-09 | Guest trap is surfaced as runtime failure and participates in quarantine accounting | Pending | |
| WR-10 | Manifest memory limit is enforced by wazero | Pending | |
| WR-11 | Max concurrency is enforced under race testing | Pending | |
| WR-12 | Disable/quarantine/revocation blocks later invocation without restart | Pending | |
| WR-13 | stdout/stderr and surfaced runtime errors remain bounded/sanitized | Pending | |
| WR-14 | Repeated success/failure/timeout cycles show no unbounded runtime resource growth in CI-observable resources | Pending | |

## CI gate

The final candidate SHA must remain unchanged while all required workflows reach terminal green:

- Add-on Modules
- CI
- Phase 8G Security Gate
- Internal Alpha Agent
- Phase 8C Backup, Restore, and Disaster Recovery
- Phase 8D Observability and Synthetics
- Phase 8E Resilience Validation
- Phase 8F MSP Lifecycle and Unified Dashboards

A documentation-only or test-only head change invalidates earlier exact-head workflow evidence.

## Claim boundary

Passing these rows proves the executable add-on path at the repository/CI integration boundary. It does not by itself complete marketplace/catalog/billing, general publisher onboarding administration, or hosted Phase 8 environment exercises such as soak/load/DR go-no-go evidence.
