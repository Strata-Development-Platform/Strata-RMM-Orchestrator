# Strata RMM Add-on Modules Framework

## Alpha scope

Strata's extension boundary is a versioned, least-privilege module contract plus a fail-closed lifecycle registry, runtime supervisor, durable platform-control-plane persistence layer, scoped module service-identity broker, signed-package verifier, and bounded payload-archive validator. Alpha currently includes manifest validation, compatibility checking, permission allowlisting, namespaced module routes, install/enable/disable/quarantine/uninstall state, deterministic listing, permission enforcement, declared-route invocation checks, bounded runtime calls, health supervision, automatic quarantine after repeated execution failures, PostgreSQL-backed lifecycle persistence, restart restoration, platform-only RLS, append-only lifecycle audit evidence, short-lived module JWT credentials whose permissions cannot exceed the enabled manifest, Ed25519 package verification, SHA-256 payload integrity checks, and non-executable validation of signed `payload.tar.gz` contents.

Marketplace/catalog services, billing for commercial modules, publisher-account management, filesystem materialization, update/rollback installation, a concrete third-party process/container launcher, resource sandboxing, and end-to-end module API/event middleware remain later phases and must not be represented as complete until implemented and tested.

## Design goals

- Keep optional and vendor-specific functionality out of the core where practical.
- Never load untrusted third-party code directly into the orchestrator process by default.
- Require explicit administrator approval for module permissions.
- Preserve MSP/client/site/tenant authorization and audit semantics.
- Make incompatible modules fail closed before activation.
- Allow modules to be disabled, quarantined, or removed without destabilizing core RMM workflows.
- Never give a module database-superuser credentials, unrestricted NATS credentials, or ambient access beyond its declared capability set.

## Manifest contract

The canonical Go contract is `internal/modules/manifest.go`. Each module declares:

- stable lowercase module ID;
- display name, semantic version, publisher, and supported Strata module API version;
- requested permissions from the platform allowlist;
- event subscriptions/publications;
- API routes under `/api/modules/<module-id>/...`;
- optional UI navigation extensions.

A module cannot invent permissions or escape its API namespace. Route permissions must also be present in the manifest's declared permission set.

Example:

```yaml
id: com.example.backup
name: Example Backup
version: 1.0.0
api_version: v1
publisher: Example Inc.
permissions:
  - devices.read
  - alerts.write
routes:
  - path: /api/modules/com.example.backup/status
    methods: [GET]
    permission: devices.read
```

## Lifecycle registry

`internal/modules/registry.go` provides the current Alpha lifecycle state machine:

```text
install -> installed -> enabled
                     -> disabled -> enabled
                     -> quarantined

enabled -> disabled -> uninstall
```

The registry is fail closed:

- an installed-but-not-enabled module receives no permission;
- undeclared permissions are denied;
- quarantined modules cannot be re-enabled through the normal enable operation;
- enabled modules must be disabled before uninstall;
- invalid or duplicate manifests are rejected.

`internal/modules/persistence.go` and migration `00090_addon_modules` add the durable control-plane backing for this state machine. The SQL store preserves manifest, state, reason, install/update timestamps, and lifecycle audit records, and can reconstruct the in-memory registry after orchestrator restart without implicitly clearing quarantine.

Persistence is intentionally platform controlled in the current Alpha scope. Both `addon_modules` and `addon_module_audit` use PostgreSQL `ENABLE ROW LEVEL SECURITY` plus `FORCE ROW LEVEL SECURITY`; the policies permit only `platform_owner` and `platform_admin` application contexts. The store does not open an elevated connection or manufacture `app.role`: callers must provide the authorization-scoped database transaction already carrying the platform's `SET LOCAL app.*` context.

Lifecycle evidence is append-only. Application roles cannot mutate visible audit history through the RLS policy, and a database trigger rejects UPDATE/DELETE even for a connection that can bypass RLS. The module integration test verifies both layers independently with a real non-superuser PostgreSQL role.

This durable lifecycle state remains intentionally separate from package acquisition and executable installation. Lifecycle installation records an approved manifest; it does not itself fetch, unpack, launch, or trust third-party code.

## Signed package verification

`internal/modules/package.go` defines the cryptographic package-verification boundary. A package ZIP must contain exactly `manifest.json`, `payload.tar.gz`, and `signature.json`. Verification is bounded and fail closed before a `VerifiedPackage` is returned.

The verifier:

- validates safe ZIP entry names and rejects directories, duplicates, and unexpected entries;
- bounds manifest, signature, payload, and overall package reads;
- decodes manifest and signature metadata with strict JSON handling;
- validates the manifest through the canonical module contract;
- verifies the payload SHA-256 digest;
- resolves a publisher/key identifier through a trust-store interface;
- verifies an Ed25519 signature bound to canonical manifest JSON plus the payload digest.

`VerifyPackage` does not extract `payload.tar.gz`, write files, install a module, or execute code. Publisher-account enrollment, trust-key rotation/distribution, marketplace acquisition, and update/rollback orchestration are still separate work.

## Payload archive validation

`internal/modules/payload.go` is the next non-executable boundary after signed-package verification. It accepts only the opaque `payload.tar.gz` bytes already present in a `VerifiedPackage` and validates the archive before any future filesystem or runtime adapter is allowed to consume it.

The validator:

- accepts directories and regular files only;
- rejects absolute paths, traversal, non-canonical paths, Windows separators, duplicate paths, symlinks, hardlinks, devices, and FIFOs;
- rejects unsupported permission bits such as setuid/setgid/sticky metadata;
- bounds regular-file count, individual file size, and total expanded bytes;
- requires at least one regular file;
- returns validated file bytes in memory in archive order.

This stage deliberately performs **no filesystem writes and no execution**. Filesystem ownership, atomic materialization, immutable install directories, process/container launch, resource controls, service-identity injection, health-gated activation, failed-update rollback, and cleanup remain later trust boundaries.

## Module service identity

`internal/modules/identity.go` defines the credential boundary intended for future out-of-process modules. It reuses the platform JWT signer but gives modules a distinct `token_use=module` identity rather than impersonating a user or agent.

A module service token contains only:

- the module ID;
- a short expiration (five minutes by default, fifteen minutes maximum);
- optional MSP/client/site scope, with hierarchy validation;
- an explicit permission subset that must already be present in the enabled module manifest;
- a unique token ID used for immediate revocation.

Validation is deliberately dynamic. A correctly signed token is still rejected if the module is disabled or quarantined, if a permission in the token is no longer granted by the current manifest, or if the token ID has been revoked. Revocation-store errors fail closed.

`RedisRevocationStore` provides the shared production-oriented revocation backend so all API replicas can observe a revocation immediately. The in-memory revocation store exists only for tests and single-process development and must not be used as production revocation evidence.

The service-identity broker does **not** hand modules PostgreSQL credentials, NATS wildcard credentials, secret-store handles, or core package access. Those capabilities must be brokered through authenticated Strata APIs/events and independently authorize the token's module ID, scope, and declared permission.

## Runtime supervisor

`internal/modules/runtime.go` defines the narrow execution boundary between the Strata control plane and a future out-of-process module runtime.

Before a runtime call is allowed, the supervisor requires all of the following:

1. the module exists;
2. the module is enabled and not quarantined;
3. the requested path exactly matches a manifest-declared module route;
4. the HTTP-style method is declared on that route;
5. the invocation permission exactly matches the route permission;
6. the permission is currently granted by the enabled module manifest.

Runtime calls are bounded by a context timeout. Consecutive execution failures are counted per module; once the configured threshold is reached, the registry quarantines the module. A successful invocation resets the consecutive-failure counter. Quarantined modules do not reach the runtime again.

The `Runtime` interface intentionally does not expose database handles, unrestricted NATS clients, secret-store handles, or imports from core internal packages. A future process/container adapter must authenticate the module with a scoped service identity and preserve this brokered boundary.

## Required runtime architecture

The intended runtime remains out-of-process by default:

```text
Strata Core
  -> Persistent Module Registry
  -> Permission / Service Identity Broker
  -> Signed Package Verification
  -> Bounded Payload Validation
  -> Event/API Bridge
  -> Runtime Supervisor
       -> authenticated isolated module process/container
```

Modules must receive scoped service identities, not database superuser credentials. Direct unrestricted PostgreSQL, RLS bypass, unrestricted NATS wildcards, raw secret-store access, or arbitrary core package imports are prohibited extension patterns.

## Runtime lifecycle requirements

The complete runtime implementation must enforce:

1. package checksum/signature verification;
2. manifest schema validation;
3. module API compatibility validation;
4. explicit administrator permission review;
5. installation in a non-enabled state;
6. health check before activation;
7. auditable enable/disable/update/uninstall operations;
8. bounded startup/execution time and resource use;
9. failed-upgrade rollback;
10. emergency quarantine/disable;
11. tenant-aware service identity and API authorization;
12. persistent state that survives orchestrator restart without implicitly re-enabling quarantined modules.

Items 1, 2, 3, 5, 7 (lifecycle audit foundation), 8 (archive bounds and execution-timeout foundations), 10, 11 (service-token foundation), and 12 now have code-level foundations. Item 4 is enforced by the current platform-admin-only persistence boundary but does not yet include a complete package-install approval UI. Item 1 does not yet include marketplace acquisition, publisher onboarding/key-rotation workflows, or update/rollback installation. Item 8 does not yet include process/container CPU, memory, filesystem, syscall, or network sandbox enforcement. Item 11 is not complete end-to-end until API/event middleware consumes the module identity and proves cross-tenant negative authorization with real infrastructure.

## Testing requirements

Module framework work must include:

- valid-manifest positive tests;
- invalid ID/version/API rejection;
- unknown and duplicate permission rejection;
- route namespace escape rejection;
- undeclared route permission rejection;
- lifecycle tests covering install/enable/disable/quarantine/uninstall;
- permission denial for disabled and quarantined modules;
- runtime denial for disabled/quarantined modules before any execution call;
- runtime route, method, and permission-escalation negative tests;
- timeout/crash/failure-threshold and quarantine tests;
- real PostgreSQL tests for RLS, platform-only persistence, audit immutability, and restart restoration;
- module service-token tests for excessive permission requests, invalid scope hierarchy, disable/quarantine invalidation, immediate revocation, wrong token type, short lifetime, and fail-closed revocation-store outage;
- signed-package tests for digest/signature tampering, untrusted publisher keys, malformed metadata, path abuse, duplicate/unexpected ZIP members, and byte limits;
- payload-archive tests for traversal, absolute/non-canonical/backslash paths, duplicates, links/special files, unsafe modes, invalid compression, empty archives, entry-size bounds, file-count bounds, and total expanded-size bounds;
- cross-tenant and sibling-scope negative API authorization tests when module middleware is wired;
- install/update/rollback/uninstall tests when filesystem/package installation is added;
- an end-to-end reference module in the Alpha environment before the runtime execution framework is called complete.

The dedicated `Add-on Modules` GitHub Actions workflow executes the PostgreSQL persistence/RLS/restart test with race detection. Module package, payload, service-identity, registry, and supervisor unit tests also run under the repository-wide race suite. A build-tagged integration test that is not exercised in CI does not count as durable evidence.

## Alpha acceptance boundary

The current foundation proves manifest validation, lifecycle/permission state, fail-closed invocation supervision, durable platform-controlled module state, restart-safe quarantine, platform-only RLS enforcement, append-only lifecycle audit behavior, issuance/verification/revocation semantics for narrowly scoped short-lived module credentials, cryptographic verification of bounded signed packages, and non-executable validation of bounded payload archives. It does **not** yet prove filesystem installation/materialization, a real third-party executable/process/container, update rollback, end-to-end module-authenticated API/event authorization, process/container resource sandboxing, publisher onboarding/key lifecycle, or marketplace distribution. Those capabilities remain explicitly incomplete until their implementation and environment evidence exist.
