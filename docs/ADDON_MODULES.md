# Strata RMM Add-on Modules Framework

## Alpha scope

Strata's extension boundary is a versioned, least-privilege module contract plus a fail-closed lifecycle registry and runtime supervisor. Alpha currently includes manifest validation, compatibility checking, permission allowlisting, namespaced module routes, install/enable/disable/quarantine/uninstall state, deterministic listing, permission enforcement, declared-route invocation checks, bounded runtime calls, health supervision, and automatic quarantine after repeated execution failures.

Signed package distribution, marketplace/catalog services, billing for commercial modules, publisher accounts, durable registry persistence, tenant-scoped module service identities, and a concrete third-party process/container launcher remain later phases and must not be represented as complete until implemented and tested.

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

The current registry is in-memory and is a core contract layer, not yet a persistent package manager. Database-backed persistence, audit-event wiring, signed packages, and package installation are still required before the complete distribution lifecycle exists.

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

Items 2, 3, 5, 8 (execution timeout portion), and 10 now have code-level foundations. The remaining items are not complete merely because the supervisor interface exists.

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
- cross-tenant and cross-scope negative authorization tests when runtime identities are added;
- install/update/rollback/uninstall tests when package management is added;
- restart/persistence tests when the registry becomes durable;
- an end-to-end reference module in the Alpha environment before the runtime execution framework is called complete.

## Alpha acceptance boundary

The current foundation proves manifest validation, lifecycle/permission state, and fail-closed invocation supervision. It does **not** yet prove a real third-party executable/process/container, persistent module state, signed package installation, audit wiring, tenant-scoped module service identities, resource sandboxing, or marketplace distribution. Those capabilities remain explicitly incomplete until their implementation and environment evidence exist.
