# Security Model

## Current verified state

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

While that value is incomplete, the access-control middleware applies a
server-side gate to every authenticated request whose effective grants include
a singleton-platform `platform_owner` or `platform_admin` membership. The exact
authenticated allowlist is:

- `GET /api/v1/auth/me`;
- `POST /api/v1/auth/logout`;
- `GET /api/v2/context`;
- `POST /api/v2/context/switch`;
- `GET /api/v2/platform/provider/profile`;
- `POST /api/v2/platform/provider/setup`; and
- `PATCH /api/v2/platform/provider/profile`.

Public health routes leave access control before the gate is consulted. There
is no authenticated HTTP recovery endpoint in the current route registry. Any
other authenticated route for such an incomplete provider administrator
returns HTTP `428 Precondition Required`, `Cache-Control: no-store`, and:

```json
{
  "error": "provider setup is required before provider administration",
  "code": "provider_setup_required",
  "setup_url": "/provider/setup"
}
```

Setup completion is read from PostgreSQL on each affected request; it is not
accepted from browser state or a JWT. The gate does not apply to users without
an effective singleton-platform owner/admin grant.

The provider-profile routes remain privileged platform routes and apply
layered authorization after the setup-gate allowlist check:

- the selected request scope must be the top-level singleton platform;
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

## Scope-bound authorization

User authorization is represented as one `AuthorizationResult` bound to one
selected scope. It contains the database-proven hierarchy, the exact membership
grants that apply there, and the roles and permissions derived from only those
grants. Login, `/api/v1/auth/me`, `/api/v2/context`, context switching, route
checks, handler checks, and the web console consume that scope-bound result.
They do not union unrelated memberships into a global role list.

In production, JWT role claims are not an authorization source. On every
protected user request the server validates the signed token, confirms that the
user is active and email-verified, resolves the token-selected MSP/client/site
hierarchy from active database rows, and reloads memberships from PostgreSQL.
Only memberships with `status = 'active'`, no elapsed `expires_at`, a legal
role/scope pairing, and an exact applicable scope are retained. Ancestor IDs are
therefore proven from database relationships rather than trusted because they
appeared together in a token.

The inheritance policy is explicitly downward:

- a platform role applies at the platform and may flow to selected MSP, client,
  and site descendants only when its membership uses the singleton platform ID
  `00000000-0000-0000-0000-000000000001`;
- an MSP role applies to that exact MSP and its selected client/site
  descendants;
- a client role applies to that exact client and its selected sites; and
- a site role applies only to that exact site.

A child membership never implies its parent, a sibling, or the platform. An MSP
membership cannot authorize another MSP, and a client/site identifier cannot be
combined with an unrelated ancestor. Platform-global administration additionally
requires that the selected scope itself be the singleton platform and that the
effective grant be `platform_owner` or `platform_admin` from that platform.

Revocation and expiry are effective for outstanding access tokens on their next
protected use because membership is reloaded for every request. The same is
true when a user is disabled or loses email verification, or when an MSP is
suspended/pending, a client is archived, or a site is archived: hierarchy or
identity revalidation fails closed. `POST /api/v1/auth/logout` is stateless and
does not maintain a token revocation list; clients discard local state, while
database revalidation remains the immediate authorization control. Refresh-token
redesign is not part of this remediation.

Agent tokens remain purpose-separated. Their approved registration, device,
tenant, MSP, client, and site relationships and active lifecycle state are also
revalidated from PostgreSQL before an agent route is entered.

## Scoped user provisioning

`POST /api/v1/admin/users` creates an identity and one or more explicit
memberships in the request transaction. The request may use one
`scope_type`/`scope_id`/`role` tuple or a `memberships` array, but not both.
`PUT /api/v1/admin/users/{userID}/memberships` replaces the memberships the
actor is authorized to manage; the legacy
`PUT /api/v1/admin/users/{userID}/tenants` path is an alias with the same
membership payload and behavior. A request contains 1–100 assignments, at most
one role per scope, and every scope ID must resolve to an active hierarchy row.

Role legality is enforced before mutation:

| Scope | Legal roles |
|---|---|
| platform singleton | `platform_owner`, `platform_admin`, `platform_support`, `platform_billing`, `platform_security_auditor`, `platform_viewer` |
| MSP | `msp_owner`, `msp_admin`, `msp_technician`, `msp_viewer` |
| client or site | `client_admin`, `client_viewer` |

Assignment authority is also bound to the actor's selected scope. A global
platform owner/admin may assign within the hierarchy, but only a platform owner
may create another platform owner. An MSP owner/admin can assign within that
exact MSP and its children; creating an `msp_owner` additionally requires an
MSP owner or a platform owner/admin. A client administrator can assign roles
only on its exact client and child sites, and a site administrator only on that
site. These checks prevent a lower-scope administrator from manufacturing a
parent, sibling, platform, or otherwise illegal grant.

Identity insert, membership insert/replacement, compatibility-mirror updates,
and `user.provisioned` or `user.memberships_updated` audit insertion share the
buffered request transaction. A membership, mirror, audit, or commit failure
therefore returns an error without a partial success. Normalized-email conflicts
are deterministic: an active verified identity with the exact visible active
assignment set returns idempotent `status: exists`; any different or unavailable
identity returns `409 email_conflict`. Passwords are bcrypt-hashed and never
included in responses or audit details.

Each request is atomic, but membership replacement is not serialized by a lock
on the target user. Concurrent replacements with different roles for the same
scope can therefore leave both role rows active because the database uniqueness
key includes the role. Operators must avoid concurrent replacement for one user
until target-user serialization and a dedicated regression test are added.

The `memberships` table is the sole authorization source. `users.role`,
`users.tenant_id`, and `user_tenant_access` remain compatibility mirrors for
legacy consumers and display; provisioning keeps them synchronized where a
single legacy representation is possible, but they never create effective
authority. At login, legacy `users.tenant_id` is only a candidate for selecting
an initial client and must be independently proven by the current hierarchy and
an applicable active membership.

Migration 69 (`enforce_scope_bound_authorization`) is forward-safe: it preserves
existing membership and legacy rows, records `invalid_membership`,
`legacy_role_without_membership`, and
`legacy_tenant_access_without_membership` evidence in
`authorization_migration_issues`, and adds the legal role/scope constraint as
`NOT VALID`. Existing ambiguous rows are therefore reportable rather than
silently converted, deleted, or granted authority; new or changed rows must
satisfy the constraint. The migration also installs membership-aware RLS helper
functions and policies for users, memberships, hierarchy rows, devices, and
control-plane audit access.

Migration 69 has an intentionally non-destructive no-op down migration. Its
authorization hardening, ambiguity evidence, and compatibility disposition are
retained because automatically restoring the legacy authority model would
reopen the escalation path. A binary rollback must retain schema 69 and be
reviewed for runtime compatibility; it must not delete issue evidence or
re-enable `users.role`/`user_tenant_access` as authority.

## Provider-approved MSP owner activation

There is no public account-registration route. An active, unexpired
`platform_owner` or `platform_admin` membership on the singleton platform is
required to create an MSP with an owner email or rotate that invitation. Both
the request context and a fresh database authorization query must pass, and any
MSP/client/site-scoped platform session is denied. Tenant-scoped roles cannot
create or resend invitations.

Each invitation uses 32 cryptographically random bytes encoded as unpadded
base64url. The raw value is delivered in
`/activate-account#<token>` and is never stored, logged, returned in an API
response, or copied into audit details. A URL fragment is not included in the
browser's HTTP request or referrer. PostgreSQL stores only the lowercase
hex-encoded SHA-256 digest. The UI reads the fragment into memory, sends it only
in JSON bodies to the inspect/accept APIs, and clears the fragment after
successful acceptance; it does not persist the token in browser storage.

`POST /api/v1/auth/invitations/inspect` and
`POST /api/v1/auth/invitations/accept` are intentionally public so the invited
owner needs no prior account. They share a dedicated per-direct-peer abuse
bucket of one request per second with a burst of five, separate from login and
general API capacity. Inputs use strict JSON decoding and a 16 KiB body limit.
Invalid, expired, revoked, consumed, malformed, or wrong-state invitations use
safe errors and do not disclose the owner address; inspection returns only the
MSP name, a masked email, and expiry.

Invitation access is fail closed in the database. Migration 68 forces RLS on
`account_invitations`; only a platform-admin database context or the exact
digest in the transaction-local invitation context can see or change a row.
The corresponding user RLS permits only the narrowly matched invitation
identity during acceptance. Acceptance locks the invitation row and checks it
is unaccepted, unrevoked, unexpired, attached to an inactive `pending_owner`
MSP, and backed by a suspended entitlement. It refuses an existing normalized
email or existing MSP owner. Concurrent or replayed acceptance therefore has
one winner.

The winning transaction creates a bcrypt password hash and verified active
tenant-neutral identity, grants the first `msp_owner` membership, activates the
MSP and entitlement, records `msp.owner_activated`, and marks the invitation
accepted. Any failure rolls back all of it, and success returns no session.
Resend similarly locks and revokes the old invitation before creating a fresh
one, and refuses to rotate a valid invitation already marked delivered.

Normalized emails are globally unique after migration 68. The migration
reports every duplicate normalized address and aborts before adding the unique
index; it also rejects blank normalized addresses. Existing active users are
backfilled with `email_verified_at`, while login now requires an active identity
with non-null verification. For invited owners, successful possession-based
acceptance supplies that verification. MFA, password recovery, open sign-up,
billing, and refresh-token redesign remain outside this slice.

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
  → credential and email-verification eligibility checks
  → signed access token
  → centralized route classification
  → authenticated identity and membership
  → scoped database transaction
  → authorization and RLS enforcement
```

MFA and password-recovery flows are not implemented by this slice and remain
later security work.

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
