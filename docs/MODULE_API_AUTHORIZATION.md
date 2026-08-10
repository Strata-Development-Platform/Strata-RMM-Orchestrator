# Module API Authorization Boundary

## Current slice

Strata add-on service credentials are consumed through a transport-independent authorization broker in `internal/modules/api_authorization.go` and an HTTP adapter in `internal/platform/module_authorization.go`.

This slice does not make module tokens ordinary user or agent tokens. Module traffic remains a distinct security principal and must pass the module identity manager on every request.

A concrete reference route crosses this boundary at `GET /api/modules/com.example.backup/devices/{deviceID}`. It requires the `devices.read` permission and resolves the device's MSP/client/site ownership from the `devices` table and its canonical hierarchy before authorization. Request headers and query parameters are not accepted as ownership evidence.

The production authorization runtime restores installed module state from PostgreSQL under the platform-admin RLS scope and uses Redis for shared credential revocation. It never falls back to process-local revocation when Redis is missing or unavailable.

## Authorization sequence

A brokered module request is authorized in this order:

1. refresh the durable module registry when its refresh interval has elapsed, failing closed if durable state cannot be re-read;
2. validate JWT signature, issuer, audience, lifetime, and `module` token type;
3. verify the module still exists and is enabled;
4. verify every credential permission still exists in the current enabled manifest;
5. verify the token has not been revoked, failing closed if the revocation store cannot be checked;
6. verify the route expects the same module ID as the credential;
7. verify the credential explicitly carries the permission required by the route;
8. resolve the concrete target resource scope from authoritative platform data;
9. verify the credential's MSP/client/site scope contains that target;
10. attach the validated module principal to request context and invoke the handler.

A scoped credential cannot escape upward to an ancestor or sideways to a sibling scope. MSP-scoped identities may reach descendants inside the same MSP; client-scoped identities may reach that client and its sites; site-scoped identities may reach only that site.

## Durable registry refresh

The module identity manager keeps a pointer to a concurrency-safe registry. The API runtime periodically reads a complete durable snapshot from `addon_modules` in a read-only transaction with the platform-admin RLS context, validates the entire snapshot, and atomically replaces the registry contents.

This means install, disable, quarantine, manifest permission changes, and uninstall state persisted by the module lifecycle control plane can take effect without restarting the API server. A malformed snapshot is rejected before the live registry is mutated. A database/transaction/validation failure causes module authorization to return `503` during the retry window rather than trusting stale enabled state.

## HTTP integration contract

`WithModuleAuthorization` is intentionally separate from the ordinary user/agent principal path. Module routes receive an explicit `AccessModule` classification so they can reach the dedicated module-service authorization boundary without attempting to validate module JWTs as user or agent credentials. Unregistered `/api/modules/` routes remain fail closed.

A module route must declare:

- the expected module ID;
- the exact required module permission;
- a target resolver that obtains MSP/client/site ownership from the authoritative resource rather than trusting request headers;
- the protected handler.

Invalid/expired service credentials receive an authentication denial. Valid module credentials that attempt permission escalation, module substitution, revoked use, or cross-scope access are denied before the handler runs.

For the reference device route, the protected read repeats the device lookup using the previously authorized MSP/client/site tuple. If ownership changes between authorization and the read, the request fails closed instead of returning a device from the new scope.

## Security invariants

Module authorization never grants direct PostgreSQL access, RLS bypass, unrestricted NATS access, raw secret-store handles, or imports into core internals. Protected handlers must continue to use resource-bound data access. The module principal is an additional service authorization boundary, not a replacement for database isolation.

## Tests

The authorization and registry tests cover:

- valid descendant access within an MSP scope;
- sibling MSP, client, and site denial;
- site-to-client and site-to-MSP ancestor escape denial;
- permission escalation denial;
- module-ID substitution denial;
- revoked credential denial;
- malformed target hierarchy denial;
- missing bearer denial;
- fail-closed middleware misconfiguration;
- validated principal propagation only after successful authorization;
- explicit classification of the registered reference route;
- fail-closed handling for unregistered module routes and wrong HTTP methods;
- reference-route sibling-site, sibling-client, and cross-MSP denial;
- fail-closed behavior when module authorization dependencies are unavailable;
- atomic durable registry snapshot replacement;
- immediate permission removal after a disabled-state snapshot is applied;
- rejection of malformed or duplicate snapshots without mutating the live registry.

## Still incomplete

The authorization runtime can now restore and refresh durable lifecycle state, but the platform still needs operator-facing lifecycle APIs/workflows that install, enable, disable, quarantine, and uninstall modules through audited transactions.

The executable module runtime also still needs the third-party process/container launcher, package signing and verification, package installation/update/rollback, marketplace distribution, event-bus credentials, publisher accounts, billing, and resource sandboxing required for an Alpha-complete add-on ecosystem.
