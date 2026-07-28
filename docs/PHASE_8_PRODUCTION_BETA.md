# Phase 8 — Production Beta Readiness

## Objective

Prepare Strata RMM for a controlled hosted beta in which Strata operates the platform, sells isolated tenancy to MSPs, and allows each MSP to manage branded client and site environments.

Phase 8 does not declare the platform production-ready. It creates and satisfies evidence-based release gates for deployment, recovery, observability, scale, onboarding, security, and operations.

## Entry baseline

- Phase 7 endpoint operations merged at PR #4.
- StrataLabs project contract merged at PR #5.
- MSP → client → site → device ownership model is implemented.
- Durable jobs, approvals, idempotency, maintenance windows, offline lifecycle, capability negotiation, inventory ingestion, audit evidence, and browser acceptance are covered by dedicated CI jobs.
- Public beta hostname: `https://rmm.stratadevplatform.com`.
- Existing API and agent compatibility must be preserved unless an approved migration plan says otherwise.

## Non-negotiable rules

1. Every change uses a focused pull request based on current `master`.
2. No PR is complete until all required GitHub Actions jobs pass on its exact head.
3. Tenant isolation, agent identity, approval separation, audit immutability, and duplicate-execution prevention may not be weakened.
4. Schema changes are additive or include a tested forward-and-rollback migration plan.
5. Secrets and production credentials must never enter source control, logs, test artifacts, or browser bundles.
6. Destructive production operations require explicit confirmation, audit evidence, and a documented rollback.
7. Beta launch requires every mandatory acceptance row in `PHASE_8_ACCEPTANCE_MATRIX.md` to contain linked evidence.

## Phase 8A Complete: Configuration and Startup Hardening

Phase 8A (PR #7, merge commit `9543bbc`) delivered centralized configuration loading, runtime modes, production fail-closed validation, health endpoints, a comprehensive configuration inventory, and full acceptance verification. The tested head is `342bf9e114ec5547333449e33a5e7df3a834d466` with CI run https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30381729001 (21/21 jobs passed). This section summarizes what was implemented and how it affects the remaining workstreams.

### Design Decisions

**Trusted proxies removed and deferred.** The `TRUSTED_PROXIES` env var and `TrustedProxies` field were removed from `HTTPConfig` and the `WithHTTPConfig` builder. Trusted-proxy support (for correct `X-Forwarded-For` handling behind load balancers) is deferred to a later phase when the deployment topology is known and a proxy-aware middleware implementation can be designed against real infrastructure.

### Runtime Modes

Three runtime modes are enforced by `pkg/config/config.go`:

| Mode | Purpose | Validation |
|------|---------|-----------|
| `development` | Local dev, relaxed checks | Basic structural validation only |
| `test` | Isolated test execution | Same as development |
| `production` | Live beta/production | Full validation + production policy checks |

### Production Validation

`ProductionValidate()` enforces fail-closed semantics when `STRATA_RUNTIME_MODE=production`:

- **Public URL** must be HTTPS with no embedded credentials
- **CORS origins** must not use wildcard `*`
- **JWT secret** must not have `dev-` or `test-` prefix
- **Database DSN** must not use `sslmode=disable` or default passwords (postgres, strata, password)
- **NATS** encrypted transport is mandatory in production; URL must use `tls://` or `nats+tls://`; a trusted CA file is required; authentication requires either a token or a complete mTLS certificate/key pair — a token does not make plaintext transport acceptable
- **Dev seeding** (`STRATA_SEED_DEV=true`) is rejected in production

### Required Settings for Production

In addition to the common validation requirements (JWT secret, database DSN, NATS URL, API address), production mode requires explicit configuration of:

1. `STRATA_RUNTIME_MODE=production`
2. `STRATA_PUBLIC_URL` — HTTPS public endpoint
3. `CORS_ORIGINS` — explicit origin list (no wildcard)
4. `NATS_URL` — production NATS cluster address
5. `TIMESCALE_DSN` / `STRATA_DB_DSN` / `DATABASE_URL` — production database with SSL

### Startup Sequence

The orchestrator startup now follows a strict sequence:

1. Load configuration from environment variables with typed parsing
2. Run `Validate()` — structural checks (all modes)
3. Run `ProductionValidate()` — fail-closed policy checks (production only)
4. Log redacted configuration summary (secrets masked)
5. Initialize subsystems (JWT → NATS → database → ingestion → alerting → vulns → storage → dispatcher → API)
6. Health endpoints return `starting` until the full sequence completes

## Workstreams

### 8.0 — Baseline and configuration inventory

Deliverables:

- inventory every service, port, dependency, secret, persistent volume, scheduled worker, external integration, and health endpoint;
- document supported single-node beta and future highly available topology;
- identify development defaults that must fail closed in production;
- create a production configuration schema with startup validation;
- define environment naming, ownership, and change-control rules.

Exit gate: a fresh environment can be configured from documented inputs without undocumented secrets or manual database edits.

### 8.1 — Deployment, upgrade, and rollback

Deliverables:

- deterministic deployment workflow with immutable image and release identifiers;
- preflight checks for schema, dependencies, capacity, secrets, and compatibility;
- migration locking and one-runner semantics;
- health-based rollout, rollback, and post-deploy verification;
- agent upgrade rings with pause, resume, rollback, and compatibility checks;
- deployment audit records and operator-visible status.

Exit gate: clean install, same-version replay, forward upgrade, failed upgrade, and rollback exercises pass without data loss or orphaned workloads.

### 8.2 — Backup and disaster recovery

Deliverables:

- encrypted PostgreSQL backups with retention and restore verification;
- TimescaleDB, object-storage, NATS JetStream, configuration, and signing-key recovery procedures;
- tenant-aware restore rules and evidence handling;
- defined RPO and RTO targets for the beta;
- scheduled restore drills into an isolated environment;
- documented regional or provider-loss procedure.

Initial beta targets:

- PostgreSQL RPO: 15 minutes or better;
- object storage RPO: 24 hours or better;
- control-plane RTO: 4 hours or better;
- quarterly full restore exercise, with automated monthly restore validation where practical.

Exit gate: a destructive recovery drill restores a usable, tenant-isolated environment and produces timestamped evidence.

### 8.3 — Observability and operator response

Deliverables:

- structured logs with correlation, MSP, client, site, device, job, and request identifiers where applicable;
- RED metrics for APIs and workers, USE metrics for infrastructure, and durable-job lifecycle metrics;
- dashboards for availability, latency, errors, queue depth, dispatch age, agent connectivity, database health, and tenant saturation;
- actionable alerts with runbook links, ownership, severity, deduplication, and escalation;
- audit-safe log redaction;
- synthetic health checks for public login, authenticated API, agent messaging, and storage.

Exit gate: injected failures are detected, alerted, diagnosed, and resolved using dashboards and runbooks without direct database guesswork.

### 8.4 — Performance and resilience

Deliverables:

- reproducible load profiles for MSP users, concurrent agents, inventory ingestion, heartbeat traffic, job fan-out, reconnect storms, and browser workflows;
- soak tests that detect leaks, queue growth, lock contention, and retry storms;
- dependency-failure tests for PostgreSQL, TimescaleDB, NATS, object storage, DNS, and outbound providers;
- bounded retries, jitter, circuit breaking, backpressure, and admission control;
- capacity model and beta tenant/device limits.

Exit gate: agreed beta load runs for 24 hours without tenant leakage, unbounded resource growth, lost acknowledged work, duplicate destructive execution, or unsupported error rates.

### 8.5 — MSP onboarding, branding, and entitlements

Deliverables:

- operator workflow to create, suspend, reactivate, and archive MSP tenants;
- MSP workflow to create clients and sites and issue scoped enrollment;
- branded portal validation and agent-download instructions;
- custom-domain readiness behind a provider-neutral interface; Cloudflare remains optional;
- plan, entitlement, quota, grace-period, and suspension enforcement;
- usage accounting with reconciliation and audit evidence;
- safe offboarding and export procedure.

Exit gate: a new MSP can be provisioned, branded, enrolled, limited, suspended, restored, and offboarded without cross-tenant access or manual database changes.

### 8.6 — Security and compliance gate

Deliverables:

- updated threat model and data-flow inventory;
- centralized secret management and rotation procedures;
- authentication/session review, authorization matrix, and route audit;
- dependency, container, SBOM, secret, and static-analysis gates;
- abuse controls for login, enrollment, jobs, remote access, and public endpoints;
- security-event retention and incident-response runbook;
- external penetration-test scope and remediation process.

Exit gate: no unresolved critical or high findings, all privileged routes have negative authorization tests, and incident-response/tabletop evidence is recorded.

### 8.7 — Beta operations and launch decision

Deliverables:

- service catalog, support boundaries, maintenance policy, and beta SLOs;
- operator onboarding, escalation, incident, change, and status-page procedures;
- tenant communication templates for incidents and maintenance;
- beta cohort criteria and rollback/exit conditions;
- final acceptance evidence index and signed go/no-go decision.

Exit gate: every mandatory acceptance criterion passes, residual risks are explicitly accepted, and rollback ownership is assigned.

## PR sequence

1. Phase 8A — Configuration inventory and production-mode startup validation.
2. Phase 8B — Deployment, migration, upgrade, and rollback automation.
3. Phase 8C — Backup, restore, and disaster-recovery evidence.
4. Phase 8D — Observability, synthetic checks, and incident runbooks.
5. Phase 8E — Load, soak, reconnect-storm, and dependency-failure testing.
6. Phase 8F — MSP onboarding, branding, entitlements, usage, and offboarding.
7. Phase 8G — Security gate, beta operations, and final launch review.

Do not combine workstreams merely to reduce PR count when doing so weakens reviewability, rollback, or exact-head evidence.
