# Module API Authorization Boundary

## Current slice

Strata add-on service credentials are consumed through a transport-independent authorization broker in `internal/modules/api_authorization.go` and an HTTP adapter in `internal/platform/module_authorization.go`.

This slice does not make module tokens ordinary user or agent tokens. Module traffic remains a distinct security principal and must pass the module identity manager on every request.

A concrete reference route now crosses this boundary at `GET /api/modules/com.example.backup/devices/{deviceID}`. It requires the `devices.read` permission and resolves the device's MSP/client/site ownership from the `devices` table and its canonical hierarchy before authorization. Request headers and query parameters are not accepted as ownership evidence.

## Authorization sequence

A brokered module request is authorized in this order:

1. validate JWT signature, issuer, audience, lifetime, and `module` token type;
2. verify the module still exists and is enabled;
3. verify every credential permission still exists in the current enabled manifest;
4. verify the token has not been revoked, failing closed if the revocation store cannot be checked;
5. verify the route expects the same module ID as the credential;
6. verify the credential explicitly carries the permission required by the route;
7. resolve the concrete target resource scope from authoritative platform data;
8. verify the credential's MSP/client/site scope contains that target;
9. attach the validated module principal to request context and invoke the handler.

A scoped credential cannot escape upward to an ancestor or sideways to a sibling scope. MSP-scoped identities may reach descendants inside the same MSP; client-scoped identities may reach that client and its sites; site-scoped identities may reach only that site.

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

The authorization tests cover:

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
- fail-closed behavior when no module authorizer has been configured.

## Still incomplete

The HTTP reference endpoint is now wired to the authorization boundary, but production startup still needs to restore the installed module registry, configure a durable/shared revocation store, construct the module identity manager/API authorizer, and inject it with `WithModuleAuthorizer`. Until that runtime wiring exists, the reference route deliberately returns `503` rather than falling back to weaker authorization.

This slice also does not yet provide the third-party process/container launcher, package signing, package installation/update/rollback, marketplace distribution, event-bus credentials, publisher accounts, billing, or resource sandboxing required for an Alpha-complete executable module runtime.
