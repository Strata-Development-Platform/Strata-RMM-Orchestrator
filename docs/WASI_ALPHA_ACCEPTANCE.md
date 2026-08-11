# WASI Alpha Acceptance Evidence

This document tracks the executable add-on Alpha gate defined by issue #145. Completion requires real signed reference modules and hostile fixtures to cross the production package, activation, runtime, broker, and response boundaries on one exact PR head SHA.

## Candidate baseline

- Base branch: `master`
- Starting SHA: `31e10495a6430b58a2f9c45613c51dd323a6d606` (PR #144)
- Runtime engine: wazero
- Guest response ABI: schema version 1
- Required broker proof: `devices.get`
- Candidate PR: #146

## Production path

```text
signed package
  -> publisher/key trust verification
  -> manifest validation
  -> payload validation
  -> immutable materialization
  -> install / enable lifecycle
  -> health-gated activation
  -> RuntimeSupervisor
  -> WASIRuntime / wazero
  -> strata_broker host ABI
  -> authoritative devices.get resolution
  -> strict WASI response ABI
  -> InvocationResult
```

Direct database insertion, hand-written active-version metadata, bypassed package verification, or mock-only runtime behavior do not count as full-path evidence.

## Positive acceptance rows

| ID | Proof | Status | Evidence |
|---|---|---|---|
| WA-01 | Deterministic reference WASI fixtures are reproducible in CI | Proven | `wasi_alpha_broker_test.go` constructs deterministic WASM bytes in Go; no opaque compiler artifact required. |
| WA-02 | Reference packages are Ed25519 signed and verified against configured publisher/key trust | Proven | `wasi_alpha_reference_test.go` / `signedReferencePayload`; production `VerifyPackage`. |
| WA-03 | Package passes manifest/payload validation and immutable materialization | Proven | `wasi_alpha_reference_test.go`; production `ValidatePayload` and `MaterializePayloadRetrySafe`. |
| WA-04 | Module installs and enables through lifecycle authorization state | Proven at registry/runtime integration boundary | Reference/broker tests use production `Registry.Install` and `Registry.Enable`; hosted API-path evidence remains Phase 8 environment work. |
| WA-05 | Candidate activation uses the real health gate | Proven | `HealthCandidate` plus `ActivateMaterializedVersionWithHealth`; malformed candidate cannot replace active version. |
| WA-06 | Real wazero execution succeeds with no ambient host capability | Proven | signed reference health/invoke plus process/filesystem isolation probes. |
| WA-07 | Brokered invocation reaches `devices.get` with authoritative scope | Proven | `TestSignedWASIReferenceCallsDevicesGetThroughBrokerABI`. |
| WA-08 | Sibling/cross-scope device substitution is denied | Proven | `TestSignedWASIReferenceBrokerDeniesCrossScopeDevice`. |
| WA-09 | Invocation output is constrained by the strict response ABI | Proven with combined runtime/ABI evidence | Reference runtime invocation crosses `decodeWASIInvocationResponse`; strict non-empty envelope validation remains covered by `wasi_response_test.go` from PR #144. |
| WA-10 | Signed upgrade to a second immutable version executes successfully | Proven | signed upgrade execution test on PR #146. |
| WA-11 | Rollback restores the prior signed version and executes successfully | Proven | rollback execution test on PR #146. |

## Runtime hostile-path rows

| ID | Proof | Status | Evidence |
|---|---|---|---|
| WR-01 | Malformed/truncated WASM fails closed without orchestrator crash | Proven | malformed signed candidate activation test and existing `wasi_runtime_test.go`. |
| WR-02 | Undeclared host imports fail instantiation | Proven | unavailable raw-socket import and `network:none` broker-import tests. |
| WR-03 | Host filesystem is unavailable by default | Proven | guest `fd_prestat_get` probe sees no preopened host directory. |
| WR-04 | Host environment/process arguments are not inherited | Proven | guest-visible WASI argv/environment probe on PR #146; runtime module config supplies neither `WithEnv` nor `WithArgs`. |
| WR-05 | `network:none` exposes no raw host network capability | Proven at host-import boundary | broker import is absent and deliberately unavailable raw socket-style import cannot resolve. |
| WR-06 | `network:brokered` exposes reviewed broker calls rather than raw socket handles | Proven at host-import/broker boundary | deterministic guest imports only `strata_broker.call`; direct raw socket host import unavailable. |
| WR-07 | Execution timeout terminates boundedly | Proven | existing real wazero infinite-loop deadline test. |
| WR-08 | Caller cancellation terminates boundedly | Proven | real infinite-loop guest cancellation test through runtime/supervisor. |
| WR-09 | Guest trap participates in supervisor quarantine accounting | Proven | real `wasmTrap` executions quarantine at threshold. |
| WR-10 | Manifest memory limit is enforced by wazero | Proven | `TestWASIRuntimeEnforcesManifestMemoryLimit`. |
| WR-11 | Max concurrency is enforced with real simultaneous guest execution | Proven pending exact-head race CI | real limit=1 blocking test plus parallel success test; Add-on Modules/CI must run with repository race requirements. |
| WR-12 | Disable/quarantine revocation blocks later broker invocation without restart | Proven | broker reauthorizes registry state on every guest host call. |
| WR-13 | stdout/stderr and surfaced runtime errors remain bounded/sanitized | Proven | bounded runtime error helper, strict stdout cap, and guest stderr-flood overflow test. |
| WR-14 | Repeated executions show no unbounded CI-observable runtime growth | Proven at CI-observable goroutine boundary | repeated real health/invoke cleanup test; hosted soak/resource profiling remains Phase 8 environment evidence. |

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

A documentation-only or test-only head change invalidates earlier exact-head workflow evidence. The SHA containing this ledger reconciliation is the freeze candidate; do not append cosmetic commits after green evidence is collected.

## Claim boundary

Passing these rows proves executable add-on behavior at the repository/CI integration boundary. It does not by itself complete marketplace/catalog/billing, publisher onboarding administration, or hosted Phase 8 environment exercises such as load, soak, disaster-recovery and final go/no-go evidence.
