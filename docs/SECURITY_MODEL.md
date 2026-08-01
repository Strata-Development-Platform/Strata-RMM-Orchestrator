# Security Model

## Current verified state after Phase 7

The original prototype findings below drove Phases 1–7. The following controls are now part of the verified baseline:

- the hardcoded JWT-secret fallback was removed and required configuration fails closed;
- route classification and authentication/authorization middleware are centralized;
- MSP, client, site, device, and agent scope is propagated through authenticated requests and database transactions;
- PostgreSQL restricted-role/RLS and cross-tenant negative tests exercise isolation without relying on an owner bypass;
- enrollment tokens are cryptographically secure, database-backed, scoped, expiring, revocable, and single-use;
- authenticated agent transactions establish tenant scope before accessing RLS-protected state;
- endpoint operations require capability, policy, approval, lifecycle, and idempotency checks;
- immutable endpoint audit evidence is insert/select-only for application roles;
- GitHub CI includes dedicated database, isolation, integration, security, durable-job, endpoint-operation, and browser acceptance coverage.

This is a verified engineering baseline, not a declaration of unrestricted production readiness.

## First-login provider setup

Provider setup uses the authenticated UI, but the UI redirect is not a security
boundary. Login, `/api/v1/auth/me`, and `/api/v2/context` derive
`setup_complete` from `setup_completed_at` in the singleton platform row. The
API remains authoritative for every read and mutation.

The provider-profile routes are classified as privileged platform routes and
apply layered authorization:

- the access token must carry `platform_owner` or `platform_admin`;
- MSP, client, and site scope must all be empty;
- the database must confirm a current active, unexpired owner/admin membership
  on the exact singleton platform ID; and
- the request executes inside the restricted-role database transaction with its
  user, role, scope, and write intent set as local security context.

An MSP, client, technician, agent, expired member, or context-switched platform
session therefore receives a denial even if it calls the API directly. Provider
mutations always target the constant singleton platform ID and never accept a
tenant or platform identifier from the request. The profile is platform-wide
rather than tenant-owned; its full contents are not available through MSP or
client APIs. Only the display name is included in ordinary authenticated
workspace context.

Setup and updates use strict JSON decoding, full server-side normalization and
validation, a row lock, and a single transaction. Protected completion fields
are not accepted as input. Setup completion time and actor are written once;
identical completion retries are no-ops, different retries conflict, and PATCH
is unavailable before completion.

The same transaction appends `provider.setup_completed` or
`provider.profile_updated` to `control_plane_audit`. Audit details contain only
the profile schema version or changed field names, never business-profile
values. Migration 67 installs a trigger that rejects control-plane audit UPDATE
and DELETE operations. Control-plane mutation handlers treat an audit append
failure as a request failure, so the surrounding request transaction is rolled
back rather than returning an unaudited success.

Contact, address, and tax-identifier data are business-sensitive. They must not
be logged, placed in audit details, or exposed to tenant-scoped roles. This
slice does not implement licensing enforcement, logo/theme white-labeling, or a
general multi-provider hierarchy.

## Phase 8G security review

Phase 8G adds the following internal-alpha controls:

1. the [threat model](THREAT_MODEL.md) inventories trust boundaries and data flows;
2. privileged route namespaces fail closed, with a table-driven role contract;
3. login, enrollment, remote, privileged-mutation, download, probe, and general
   traffic use isolated abuse-control buckets based on the direct peer address;
4. HMAC signing supports one bounded previous-secret overlap for rotation; newly
   issued tokens always use the current secret;
5. exact-head CI gates authorization/race tests, dependency and static analysis,
   container scanning, an SPDX SBOM, secret scanning, frontend checks, and the
   existing browser suite;
6. [incident response](INCIDENT_RESPONSE.md) and an external
   [penetration-test scope](PENETRATION_TEST_SCOPE.md) are defined.

The current access-token design remains symmetric HS256 without refresh-token
rotation. Its internal-alpha compensating controls are a minimum 256-bit secret,
strict issuer/audience/purpose/lifetime validation, active membership lookup,
bounded two-key rotation overlap, TLS, and secret-store-only deployment. An
approved asymmetric/session design—or explicit signed risk acceptance—remains a
production-beta requirement.

Production beta also still requires centralized secret-store access evidence,
a signed tabletop, external penetration-test remediation/re-test, hosted security
exercises, and closure of every launch-blocking item in the
[risk register](PHASE_8_RISK_REGISTER.md) and
[acceptance matrix](PHASE_8_ACCEPTANCE_MATRIX.md).

## Authentication flow

```text
User login
  → credential and MFA verification
  → signed access token
  → centralized route classification
  → authenticated identity and membership
  → scoped database transaction
  → authorization and RLS enforcement
```

Agent enrollment establishes a persistent, scoped agent identity. Subsequent agent traffic must authenticate that identity and match MSP, client, site, device, agent, subject, correlation, attempt, and expiry constraints.

## Authorization and isolation invariants

- Platform, MSP, client, site, device, and agent scopes are explicit; request path identifiers are never trusted alone.
- Application queries run under restricted database roles with RLS scope set inside the transaction.
- Background workers and messaging consumers carry the same ownership context as synchronous API requests.
- Cross-scope reads and writes fail closed and are covered by negative tests.
- Privileged endpoint work requires policy evaluation and, where applicable, an unexpired approval from a different authorized actor.
- Duplicate or replayed destructive work cannot execute twice.
- Audit evidence may be appended and read by authorized roles but not mutated or deleted through application paths.
- Secrets, tokens, credentials, sensitive command payloads, and customer content must not appear in logs or artifacts.

## Historical pre-rewrite findings

The following findings describe the starting prototype and are retained for traceability:

1. a hardcoded development JWT fallback appeared in multiple locations;
2. seed data contained a placeholder password hash;
3. enrollment tokens were kept only in memory;
4. HS256 signing had no public-key verification or rotation;
5. refresh-token/session rotation was absent;
6. handlers performed inconsistent per-route checks;
7. client-supplied tenant identifiers could be trusted too early;
8. RLS coverage was incomplete and owner connections could bypass it.

Items 1–3 and the route/RLS containment portions of 6–8 were remediated in
completed phases. Phase 8G adds bounded symmetric-key rotation for item 4, but
asymmetric verification remains deferred. Item 5 and the external production
audit remain launch gates.
