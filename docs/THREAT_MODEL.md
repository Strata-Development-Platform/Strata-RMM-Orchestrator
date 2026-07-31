# Threat Model and Data-Flow Inventory

## Scope and trust boundaries

This model covers the hosted control plane, browser UI, enrolled agents,
PostgreSQL/TimescaleDB, NATS JetStream, object storage, deployment and recovery
automation, custom-domain routing, and third-party update/vulnerability sources.
It is an engineering review for internal alpha; it is not a penetration-test
report or compliance certification.

| Boundary | Data crossing it | Authentication | Authorization and containment | Principal threats |
|---|---|---|---|---|
| Browser → API | credentials, JWT, tenant-scoped requests | password/MFA login and user JWT | active memberships, route policy, scoped transaction, PostgreSQL RLS | credential stuffing, token theft, IDOR, cross-MSP access |
| Agent → API/NATS | enrollment token, identity, inventory, job results | single-use enrollment then agent JWT/NATS credentials | agent/device ownership, subject and attempt constraints | fake enrollment, replay, command/result substitution |
| API/workers → PostgreSQL | identity context, customer state, durable work, audit | restricted database role | transaction-local scope and forced RLS | owner bypass, scope omission, destructive query |
| API/workers → JetStream | jobs, attempts, results, correlation metadata | TLS plus token or mTLS | account permissions, durable consumer state, idempotency | plaintext credentials, message replay, duplicate execution |
| API → object storage | recordings and reports | storage credentials | object ownership metadata and API authorization | object-key guessing, content disclosure, integrity loss |
| Operator → deployment/recovery | release identity, migrations, encrypted backups | operator-controlled host and secret store | preflight, locks, immutable identities, audit evidence | supply-chain substitution, leaked secrets, unsafe rollback |
| Internet → public endpoints | health, login, enrollment validation, downloads | endpoint-dependent | body limits and per-source abuse buckets | flooding, enumeration, oversized payloads |
| Support → MSP scope | time-limited support request | platform user JWT | explicit grant, scope, expiry, audit | standing privilege, grant reuse, tenant confusion |

## Protected assets

- customer identity, device inventory, scripts, command payloads, recordings,
  reports, audit evidence, credentials, signing/encryption material, durable-job
  state, billing/entitlement data, and release/recovery evidence;
- availability of the API, database, messaging, dispatcher, storage, and
  enrollment paths;
- integrity of tenant boundaries, approvals, immutable audit records, release
  artifacts, and migration history.

## Required invariants

1. Unknown routes under privileged namespaces fail closed as platform-admin.
2. User and agent tokens are never interchangeable; inactive identities and
   revoked memberships fail on request validation.
3. Client-provided scope identifiers cannot expand a principal's database scope.
4. Destructive endpoint work requires capability, policy, approval, correlation,
   attempt, expiry, and idempotency checks.
5. Secrets and customer payloads do not enter logs, metrics, exports, SBOMs, or
   deployment artifacts.
6. A deployment, migration, recovery, or offboarding failure cannot be reported
   as success and must release bounded resources safely.

## STRIDE review and mitigations

| Threat | Existing mitigation | Phase 8G verification | Residual work |
|---|---|---|---|
| Spoofing | signed purpose-specific tokens, active identity lookup, TLS | algorithm/lifetime/rotation and purpose tests | external identity-provider/refresh-session design remains a post-alpha decision |
| Tampering | TLS, hashes, signed tokens, immutable audit tables | CI static/image/SBOM gates and recovery integrity tests | external pentest required before production beta |
| Repudiation | append-only audit and deployment/recovery evidence | security event and incident procedure audit | hosted retention/export exercise pending |
| Information disclosure | RLS, scoped handlers, redaction, bounded MSP export | privileged-route and cross-scope negative suites | hosted object-store authorization exercise pending |
| Denial of service | bounded contexts/body sizes, readiness, isolated abuse buckets | limiter contracts and resilience harness | environment load/soak/fault gates A8-14–A8-17 pending |
| Elevation of privilege | centralized route policy, memberships, support grants, RLS | all inventoried admin routes deny non-platform admins; privileged prefixes fail closed | external route/API penetration test pending |

## Review triggers

Update this document when a new external integration, privileged route, token
type, storage class, background consumer, deployment path, custom-domain trust
rule, or customer-data category is introduced. The change must identify its
runtime consumer and add negative authorization/security tests.
