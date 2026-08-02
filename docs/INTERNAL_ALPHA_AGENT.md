# Internal-alpha endpoint agent acceptance status

The endpoint agent is intended for controlled, ephemeral internal-alpha environments only. It is not evidence of hosted or production operational readiness.

## Verified locally by package tests

- Enrollment consumes a database-backed, expiring, bounded-use credential in the registration transaction.
- The issued application credential is bound to a tenant and endpoint, and the generated NATS subject policy contains only endpoint-specific subjects.
- Identity state is stored with owner-only file modes on Unix. Incomplete, corrupt, certificate/key-mismatched, or tenant-mismatched state fails closed instead of silently creating a replacement identity.
- The BoltDB offline queue survives restart, retains entries until publish acknowledgement, and enforces a combined metric/event capacity (`store.queue_max_items`, default `10000`). Capacity exhaustion rejects new entries rather than allowing unbounded growth.
- Production configuration rejects missing, plaintext, credential-bearing, loopback, and container-local advertised NATS URLs.
- Remote session input and stop operations must match the tenant, device, and agent binding established when the session was dispatched. Pending bindings expire after 30 minutes, are removed on stop or failed dispatch, and disappear on orchestrator shutdown/restart. The start response is `pending`; the current HTTP path does not prove that the endpoint accepted or activated the session.
- Linux and Windows installers validate TLS, verify SHA-256 checksums, bound download attempts, preserve the data directory during binary replacement, and fail if enrollment material remains in the runtime configuration.

## Provider first-login slice compatibility

This slice changes the orchestrator database/API and web console only. It does
not change the endpoint-agent protocol, subjects, identity files,
configuration, installers, or release compatibility.

| Criterion | Slice status |
|---|---|
| Migration 67 adds the singleton business fields and immutable setup metadata; clean bootstrap grants the initial platform-owner membership | Implemented with focused schema/bootstrap coverage |
| First top-level provider sign-in is routed through Business, Contact, Regional Defaults, and Review before explicit completion | Implemented with frontend and Chromium acceptance coverage |
| Setup and profile APIs enforce active platform membership and reject tenant-scoped and non-platform roles | Implemented with authorization and PostgreSQL integration coverage |
| Server validation, idempotent identical setup retry, protected completion metadata, partial updates, and persisted context are covered | Implemented with validation, handler, and database integration coverage |
| Completion and effective edits create transactional immutable control-plane audit evidence without profile values | Implemented with focused PostgreSQL integration coverage |
| Exact-head repository CI, review, and the remaining internal-alpha operational gates | Pending; this slice does not satisfy them |

This matrix is limited to provider account setup. It does not claim a complete
white-label licensing system, every dashboard level, hosted acceptance, or full
internal-alpha readiness.

## MSP owner activation slice compatibility

Provider-approved MSP owner activation changes the orchestrator schema, account
mailer, HTTP API, and web console only. It does not change endpoint-agent
protocol versions, NATS subjects, identity files, installer inputs, enrollment,
durable command handling, or release compatibility. Existing enrolled agents
do not need to be replaced when migration 68 is applied.

| Criterion | Slice status |
|---|---|
| Migration 68 adds global normalized-email uniqueness, email verification metadata, nullable legacy `tenant_id`, pending-owner onboarding, and RLS-protected invitation rows | Implemented with a schema contract plus three focused PostgreSQL migration/RLS tests |
| Top-level provider owner/admin creates an inactive MSP and emails its intended owner a digest-backed, expiring link | Implemented with unit, authorization, and focused activation integration coverage; live SMTP was not contacted |
| Public inspect/accept is abuse-isolated, token-safe, one-time, and atomically creates the owner and activates the MSP/entitlement | Implemented with focused service/database tests and React component tests |
| Pending MSPs are excluded from host resolution and workspace switching; tenant-scoped callers cannot create or resend | Implemented with authorization and route contracts |
| Activation-specific browser and hosted email-delivery evidence | Pending; no new Playwright test was added for this slice and SMTP delivery was not exercised against a live provider |
| Exact-head repository CI, review, and all remaining internal-alpha operational gates | Pending; opening a draft PR does not satisfy them |

This slice does not add open sign-up, MFA, password recovery, billing, or
refresh-token redesign. Those items and the mandatory hosted-agent exercises
below remain prerequisites; the activation work alone does not establish
internal-alpha readiness.

## Scope-authorization remediation compatibility

The scope-bound authorization, scoped user provisioning, and provider setup
gate remediation changes the orchestrator schema/API and web console. It does
not change endpoint-agent protocol versions, NATS subjects, enrollment inputs,
identity files, installer behavior, durable command envelopes, or release
artifact formats. Existing enrolled agents do not need to be replaced when
migration 69 is applied. Agent-token validation does continue to fail closed if
the approved registration, device, or active MSP/client/site hierarchy no
longer exists.

| Criterion | Slice status |
|---|---|
| User roles and permissions are derived from active, unexpired memberships applicable to one exact database-validated selected scope | Implemented; exact-head CI and review pending |
| Singleton-platform membership is required for platform roles, and child/sibling membership never implies parent or global authority | Implemented; exact-head CI and review pending |
| Protected requests revalidate active identity, membership, expiry, and hierarchy state instead of trusting JWT role claims | Implemented; stateless logout and the existing access-token design remain |
| Scoped user creation/replacement validates role legality and actor authority, preserves unmanaged memberships, serializes target-user updates, and commits identity, memberships, mirrors, and audit together | Implemented; exact-head database-backed concurrency and scope-preservation evidence pending |
| Incomplete provider administrators are server-gated with an exact recovery/setup allowlist and stable `provider_setup_required` response | Implemented; hosted browser/API exercise pending |
| Migration 69 preserves rows, reports ambiguity, constrains new/changed membership state, and retains hardening on down migration | Implemented; operator review of `authorization_migration_issues` is required after upgrade |
| Exact-head repository CI, security review, hosted adversarial exercises, and all remaining internal-alpha gates | Pending; opening a draft PR does not satisfy them |

This bounded remediation does not establish internal-alpha readiness. It does
not add or accept MFA, password recovery, open public sign-up, billing, or a
refresh-token/session redesign, and it does not replace the mandatory hosted
agent and operational exercises below.

## Supported internal-alpha matrix

| Platform | Architecture | Build coverage | Installer coverage | Status |
| --- | --- | --- | --- | --- |
| Linux | AMD64 | Native CI build | Shell syntax/static contract; isolated systemd execution still required | Partial |
| Linux | ARM64 | Cross-compile CI build | Shell syntax/static contract; ARM64 host execution still required | Partial |
| Windows | AMD64 | Cross-compile CI build | PowerShell parse/static contract; Windows service execution still required | Partial |
| Windows | ARM64 | Cross-compile CI build | PowerShell parse/static contract; Windows ARM64 service execution still required | Partial |
| macOS | AMD64/ARM64 | Cross-compile CI build | No installer | Partial |

## Mandatory hosted-alpha prerequisites

Before promotion, capture exact-head CI evidence and run the end-to-end exercise against ephemeral PostgreSQL/TimescaleDB, NATS with JetStream, and object storage. The exercise must cover enrollment replay, process restarts, a broker outage and queue replay, duplicate ingestion, harmless job dispatch, and cross-tenant/cross-endpoint denials. Linux systemd and Windows service installation must also execute on representative supported hosts. Vulnerability, container, and secret scans must complete successfully.

Release downloads currently resolve the configured GitHub release before being cached and served with a same-origin checksum. Promotion requires pinning and recording immutable release/tag identity in deployment evidence; a mutable `latest` response must not be treated as authoritative provenance.

The frontend audit currently reports advisories in `vite`/`esbuild` (development tooling) and `react-router-dom`/`react-router` (production navigation). npm offers fixes only through semver-major Vite 8 and React Router 7 upgrades, so they are not changed as part of this endpoint-agent remediation. Hosted-alpha review must either validate those migrations separately or formally accept the remaining exposure; the development server must not be exposed to untrusted networks.

## Rollback

Stop and disable the agent service, restore the previously verified agent binary, and retain the data directory and identity directory. Restart only after the prior binary checksum and configuration have been verified. Revoking the endpoint credential and re-enrolling creates a new endpoint identity; do not delete identity state as a routine binary rollback step.
