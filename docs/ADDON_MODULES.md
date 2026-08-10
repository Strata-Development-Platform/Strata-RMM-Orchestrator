# Strata RMM Add-on Modules Framework

## Alpha status

The add-on framework now has a complete **non-executing trust chain** plus the control-plane foundations required for isolated Alpha execution:

1. manifest validation and compatibility checks;
2. lifecycle registry with install/enable/disable/quarantine/uninstall;
3. PostgreSQL persistence with platform-only RLS and append-only audit evidence;
4. scoped short-lived module service identities with shared Redis revocation;
5. brokered module API authorization with MSP/client/site scope enforcement;
6. signed package verification using Ed25519 and SHA-256;
7. bounded `payload.tar.gz` validation;
8. safe immutable filesystem materialization;
9. conservative stale install-lock recovery;
10. atomic active-version metadata and reversible rollback;
11. health-gated activation that preserves the previous active state on failure;
12. a bounded WASI runtime declaration in the manifest.

The framework still **does not execute third-party module code**. A concrete WASI engine, brokered host functions, publisher trust administration, package-install approval UI, uninstall garbage collection, and marketplace/catalog distribution remain incomplete.

## Design rules

- Optional/vendor-specific functionality should use the module boundary instead of increasing core coupling where practical.
- Third-party code must never run inside the orchestrator process by default.
- Modules never receive PostgreSQL superuser credentials, RLS bypass, unrestricted NATS credentials, raw secret-store handles, ambient environment secrets, or arbitrary host filesystem/network access.
- Every capability is explicitly declared, administrator-approved, brokered, tenant-scoped, auditable, and fail closed.
- Disable, quarantine, revocation, or durable-state refresh failures must remove access rather than extend it.
- Documentation may only claim capabilities proven by code and exact-head CI/environment evidence.

## Manifest contract

`internal/modules/manifest.go` is canonical. A module declares identity, version/API compatibility, publisher, requested permissions, event access, namespaced routes, optional UI extensions, and optionally a runtime contract.

Routes must remain beneath `/api/modules/<module-id>/...`, and route permissions must already be present in the manifest permission set.

For executable Alpha modules, `runtime` is optional for backward compatibility but, when present, is intentionally constrained:

```yaml
runtime:
  kind: wasi
  entrypoint: bin/module.wasm
  memory_mib: 128
  timeout_seconds: 30
  max_concurrency: 4
  network: none
```

Current runtime declaration rules:

- only `wasi` is accepted;
- entrypoints must be relative, canonical, contained `.wasm` paths;
- memory is bounded to 16–512 MiB;
- execution timeout is bounded to 1–120 seconds;
- concurrency is bounded to 1–32;
- network is either `none` or `brokered`;
- native-process, absolute/traversal/backslash paths, host-network requests, and unbounded resource declarations are rejected.

This contract does not itself execute WASM.

## Lifecycle and persistence

`internal/modules/registry.go` provides the fail-closed state machine:

```text
install -> installed -> enabled
                     -> disabled -> enabled
                     -> quarantined

enabled -> disabled -> uninstall
```

`internal/modules/persistence.go` and migration `00090_addon_modules` persist manifest/state/reason/timestamps and append-only audit records. The module tables use both `ENABLE ROW LEVEL SECURITY` and `FORCE ROW LEVEL SECURITY`, limited to platform-owner/platform-admin application contexts.

Real PostgreSQL integration tests prove platform-only access, RLS behavior, audit immutability, restart restoration, and quarantine persistence.

The runtime registry is refreshed from durable state. When a due refresh fails, module authorization fails closed rather than trusting stale enabled permissions.

## Module service identity and API authorization

`internal/modules/identity.go` issues distinct `token_use=module` JWTs. Tokens are short-lived, bound to module ID, may be scoped to MSP/client/site, contain only an explicit subset of currently enabled manifest permissions, and have unique IDs for immediate revocation.

Validation rejects tokens when:

- the module is disabled or quarantined;
- a token permission is no longer present in the enabled manifest;
- the token is expired, wrong type, wrong module, or malformed;
- the scope hierarchy is invalid;
- the token ID is revoked;
- the shared revocation backend cannot be checked.

Production revocation uses Redis and fails closed on Redis errors. Modules never receive direct database or unrestricted NATS credentials.

The module HTTP authorization boundary validates module identity, declared permission, and authoritative MSP/client/site resource scope before handler execution. Device ownership is resolved from platform storage rather than trusted caller headers. Sibling and cross-tenant access is denied.

## Signed package verification

`internal/modules/package.go` accepts a ZIP containing exactly:

- `manifest.json`
- `payload.tar.gz`
- `signature.json`

Verification is bounded and fail closed. It rejects duplicate/unexpected members, malformed metadata, unsafe ZIP paths, oversized reads, digest tampering, bad signatures, and untrusted publisher/key IDs. Ed25519 verification binds canonical manifest JSON and the SHA-256 payload digest.

Untrusted packages are never extracted or executed.

## Payload validation

`internal/modules/payload.go` validates the already-verified `payload.tar.gz` in memory before filesystem use. It accepts directories and regular files only and rejects traversal, absolute/non-canonical/backslash paths, duplicates, symlinks/hardlinks/devices/FIFOs, unsafe mode bits, invalid compression, excessive file counts, excessive member sizes, and excessive total expanded bytes.

A private fingerprint binds module ID, version, payload digest, and the exact validated file set so post-validation mutation is rejected by the next boundary.

## Filesystem materialization

`internal/modules/materialize.go` writes only a matching verified/sealed payload beneath a trusted install root. It:

- rejects symlinked/unsafe roots and target components;
- uses an exclusive per-version install lock;
- applies strict directory/file permissions and optional ownership during creation;
- stages in a hidden sibling directory;
- uses exclusive file creation;
- refuses to overwrite an existing version;
- atomically promotes a completed staging tree by same-filesystem rename;
- cleans staging state on failure;
- conservatively reclaims stale install locks only after the configured grace period and identity recheck.

Materialized version directories are immutable to this implementation.

## Active version and rollback

`internal/modules/activation.go` selects only an already-materialized immutable version. Activation is serialized and atomically replaces small metadata rather than modifying the version tree itself.

The state preserves the current active version plus one previous version. Rollback swaps to the previous version and remains reversible. Missing versions, malformed/symlinked state, and activation lock contention fail closed.

This is metadata selection only; it does not launch code.

## Health-gated activation

`internal/modules/activation_health.go` adds the orchestration gate between a candidate version and active metadata. A trusted host-side checker runs under a bounded context before the active state can change.

Health failure, timeout, cancellation, missing candidate, or lock contention leaves the prior `.active.json` state unchanged. A successful check is required before active/previous metadata changes.

The health-check interface is deliberately separated from the future WASI engine so activation semantics remain independently testable.

## Runtime supervisor

`internal/modules/runtime.go` is the brokered invocation supervisor. Before any runtime call, it requires an existing enabled/non-quarantined module, an exact manifest-declared route/method, the exact route permission, and a currently granted permission.

Calls are context-bounded. Repeated execution failures cause quarantine; a successful invocation resets the consecutive-failure count. Quarantined modules are denied before reaching the runtime.

The `Runtime` interface exposes no database handles, unrestricted NATS clients, secret-store handles, or arbitrary core packages.

## Intended WASI execution architecture

The next execution boundary is intentionally WASI-only for Alpha:

```text
Strata Core
  -> Durable Module Registry
  -> Permission / Service Identity Broker
  -> Signed Package Verification
  -> Bounded Payload Validation
  -> Immutable Materialization
  -> Active Version + Health Gate
  -> Runtime Supervisor
  -> WASI Runtime Adapter
       -> no ambient filesystem
       -> no ambient environment/secrets
       -> no raw sockets
       -> bounded memory/time/concurrency
       -> brokered Strata host APIs only
```

The future engine must not infer capabilities from payload contents. It must enforce the manifest runtime contract and expose only explicitly reviewed host functions.

## Testing requirements

Every add-on change must include the narrow tests for its boundary plus the repository-wide exact-head matrix.

Required coverage includes:

- manifest identity/API/permission/route validation;
- runtime-contract rejection for native runtime, unsafe entrypoints, invalid network mode, and resource limits;
- lifecycle install/enable/disable/quarantine/uninstall transitions;
- disabled/quarantined permission and invocation denial;
- real PostgreSQL RLS, audit immutability, and restart restoration;
- service-token permission/scope/revocation/outage negative tests;
- sibling/cross-tenant API authorization denial;
- signed-package digest/signature/trust/path/size adversarial tests;
- archive traversal/link/special-file/mode/count/size/mutation tests;
- materialization containment, modes, symlink, immutable-version, locking, cleanup, and stale-lock recovery tests;
- active-version switch, reversible rollback, malformed state, missing-version, and concurrency tests;
- health success/failure/timeout/cancellation/state-preservation tests;
- supervisor route/method/permission/timeout/failure-threshold/quarantine tests;
- future WASI-engine tests for host-import allowlisting, no ambient filesystem/env/network, memory/time/concurrency enforcement, cancellation, malformed WASM, traps, and brokered identity/scope.

The dedicated `Add-on Modules` workflow plus repository-wide race tests are required. Exact-head CI, Security Gate, Backup/Restore, Observability, Resilience, MSP Lifecycle, and Internal Alpha remain mandatory before merge. If the head changes, prior evidence is stale.

## Current Alpha acceptance boundary

Proven foundations now include lifecycle/persistence/RLS/audit, scoped module identity and brokered API authorization, signed package verification, bounded payload validation, immutable materialization, stale-lock recovery, active-version metadata, reversible rollback, health-gated activation, runtime supervision, and a bounded WASI runtime declaration.

Still incomplete and must remain labeled incomplete:

- a concrete WASI execution engine;
- reviewed brokered WASI host functions;
- runtime memory enforcement evidence beyond manifest validation;
- executable end-to-end reference module evidence;
- publisher onboarding/trust-key rotation administration;
- package-install permission approval UI;
- uninstall garbage collection;
- marketplace/catalog/billing/distribution.

Do not call executable add-ons complete until a real reference WASI module crosses the full signed-package -> validation -> materialization -> health -> runtime -> brokered API boundary under exact-head CI/environment evidence.
