# Strata RMM Add-on Modules Framework

## Alpha scope

Strata's extension boundary is a versioned, least-privilege module contract. Alpha establishes the manifest and compatibility rules first. Runtime sandboxing, signed package distribution, marketplace/catalog services, billing for commercial modules, and publisher accounts are later phases and must not be represented as complete until implemented and tested.

## Design goals

- Keep optional and vendor-specific functionality out of the core where practical.
- Never load untrusted third-party code directly into the orchestrator process by default.
- Require explicit administrator approval for module permissions.
- Preserve MSP/client/site/tenant authorization and audit semantics.
- Make incompatible modules fail closed before activation.
- Allow modules to be disabled or removed without destabilizing core RMM workflows.

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

## Required runtime architecture

The intended runtime is out-of-process by default:

```text
Strata Core
  -> Module Registry
  -> Permission Broker
  -> Event/API Bridge
  -> Lifecycle + Health Manager
       -> isolated module process/container
```

Modules must receive scoped service identities, not database superuser credentials. Direct unrestricted PostgreSQL, RLS bypass, unrestricted NATS wildcards, raw secret-store access, or arbitrary core package imports are prohibited extension patterns.

## Lifecycle requirements

A future runtime implementation must enforce:

1. package checksum/signature verification;
2. manifest schema validation;
3. module API compatibility validation;
4. explicit administrator permission review;
5. installation in disabled state;
6. health check before activation;
7. auditable enable/disable/update/uninstall operations;
8. bounded startup/execution time and resource use;
9. failed-upgrade rollback;
10. emergency quarantine/disable.

## Testing requirements

Module framework work must include:

- valid-manifest positive tests;
- invalid ID/version/API rejection;
- unknown and duplicate permission rejection;
- route namespace escape rejection;
- undeclared route permission rejection;
- cross-tenant and cross-scope negative authorization tests when runtime identities are added;
- install/update/rollback/uninstall lifecycle tests when package management is added;
- module crash/timeout/dependency failure tests when execution is added;
- an end-to-end reference module in the Alpha environment before the runtime framework is called complete.

## Alpha acceptance boundary

The current foundation proves the manifest/permission/namespace contract only. It does not yet prove runtime execution isolation or third-party package installation. Those capabilities remain explicitly incomplete until their implementation and environment evidence exist.
