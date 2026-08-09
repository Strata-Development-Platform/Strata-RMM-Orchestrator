# Module API Authorization Boundary

## Current slice

Strata add-on service credentials are consumed through a transport-independent authorization broker in `internal/modules/api_authorization.go` and an HTTP adapter in `internal/platform/module_authorization.go`.

This slice does not make module tokens ordinary user or agent tokens. Module traffic remains a distinct security principal and must pass the module identity manager on every request.

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

`WithModuleAuthorization` is intentionally separate from the global user/agent access middleware. A module route must declare:

- the expected module ID;
- the exact required module permission;
- a target resolver that obtains MSP/client/site ownership from the authoritative resource rather than trusting request headers;
- the protected handler.

Invalid/expired service credentials receive an authentication denial. Valid module credentials that attempt permission escalation, module substitution, revoked use, or cross-scope access are denied before the handler runs.

## Security invariants

Module authorization never grants direct PostgreSQL access, RLS bypass, unrestricted NATS access, raw secret-store handles, or imports into core internals. Protected handlers must continue to use normal data-layer ownership/RLS controls. The module principal is an additional service authorization boundary, not a replacement for database isolation.

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
- validated principal propagation only after successful authorization.

## Still incomplete

This contract is ready to protect concrete brokered module endpoints, but it does not by itself provide a third-party process/container launcher, package signing, package installation/update/rollback, marketplace distribution, event-bus credentials, publisher accounts, billing, or resource sandboxing. A real reference module must cross this boundary end-to-end before the executable module runtime is called Alpha-complete.
