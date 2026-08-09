# Strata RMM Add-on Modules Framework

## Alpha scope

Strata's extension boundary is a versioned, least-privilege module contract plus a fail-closed lifecycle registry, runtime supervisor, and durable platform-control-plane persistence layer. Alpha currently includes manifest validation, compatibility checking, permission allowlisting, namespaced module routes, install/enable/disable/quarantine/uninstall state, deterministic listing, permission enforcement, declared-route invocation checks, bounded runtime calls, health supervision, automatic quarantine after repeated execution failures, PostgreSQL-backed lifecycle persistence, restart restoration, platform-only RLS, and append-only lifecycle audit evidence.

Signed package distribution, marketplace/catalog services, billing for commercial modules, publisher accounts, tenant-scoped module service identities, and a concrete third-party process/container launcher remain later phases and must not be represented as complete until implemented and tested.

## Design goals

- Keep optional and vendor-specific functionality out of the core where practical.
- Never load untrusted third-party code directly into the orchestrator process by default.
- Require explicit administrator approval for module permissions.
- Preserve MSP/client/site/tenant authorization and audit semantics.
- Make incompatible modules fail closed before activation.
- Allow modules to be disabled, quarantined, or removed without destabilizing core RMM workflows.

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

This is durable lifecycle persistence, not yet a signed package manager. Package acquisition, verification, update/rollback distribution, publisher trust, and executable installation remain separate work.

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
  -> Permission Broker
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

Items 2, 3, 5, 7 (lifecycle audit foundation), 8 (execution timeout portion), 10, and 12 now have code-level foundations. Item 4 is enforced by the current platform-admin-only persistence boundary but does not yet include a complete package-install approval UI. The remaining items are not complete merely because persistence and the supervisor exist.

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
- cross-tenant and cross-scope negative authorization tests when runtime identities are added;
- install/update/rollback/uninstall tests when package management is added;
- an end-to-end reference module in the Alpha environment before the runtime execution framework is called complete.

The dedicated `Add-on Modules` GitHub Actions workflow executes the PostgreSQL persistence/RLS/restart test with race detection. A build-tagged integration test that is not exercised in CI does not count as durable evidence.

## Alpha acceptance boundary

The current foundation proves manifest validation, lifecycle/permission state, fail-closed invocation supervision, durable platform-controlled module state, restart-safe quarantine, platform-only RLS enforcement, and append-only lifecycle audit behavior. It does **not** yet prove a real third-party executable/process/container, signed package installation or update rollback, tenant-scoped module service identities, resource sandboxing, or marketplace distribution. Those capabilities remain explicitly incomplete until their implementation and environment evidence exist.
