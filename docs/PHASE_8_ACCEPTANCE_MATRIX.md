# Phase 8 Acceptance Matrix

Every mandatory row must link to durable evidence: a GitHub Actions run, test report, restore record, dashboard snapshot, security review, or signed operational checklist.

| ID | Gate | Acceptance criterion | Required proof | Mandatory |
|---|---|---|---|---|
| A8-01 | Configuration | Production mode rejects missing, default, malformed, or conflicting critical configuration | Unit/integration tests and startup logs | Yes |
| A8-02 | Clean deployment | Documented automation creates a healthy isolated beta environment from approved inputs | Deployment workflow run and smoke report | Yes |
| A8-03 | Idempotent deployment | Reapplying the same release produces no duplicate resources or destructive drift | Repeat deployment evidence | Yes |
| A8-04 | Upgrade | Supported previous release upgrades with migrations and mixed-agent compatibility | Upgrade workflow and compatibility report | Yes |
| A8-05 | Rollback | Injected failed rollout returns to the prior healthy release without data loss | Automated rollback exercise | Yes |
| A8-06 | PostgreSQL recovery | Encrypted backup restores schema, tenant data, jobs, approvals, and audit evidence | Isolated restore report | Yes |
| A8-07 | Messaging recovery | NATS restart/outage preserves durable work and resumes without duplicate destructive execution | Integration/fault test | Yes |
| A8-08 | Object recovery | Required reports/recordings restore with integrity verification and authorization intact | Restore and hash report | Yes |
| A8-09 | RPO/RTO | Recovery drill meets documented beta RPO and RTO | Timestamped drill record | Yes |
| A8-10 | API observability | Availability, latency, traffic, errors, and saturation are visible and alertable | Dashboard and alert test | Yes |
| A8-11 | Job observability | Queue depth, dispatch age, retries, failures, expiry, and reconnect state are visible | Dashboard and injected-failure evidence | Yes |
| A8-12 | Synthetic coverage | Public health, login, authenticated API, agent path, and storage are independently checked | Synthetic configuration and alert test | Yes |
| A8-13 | Log safety | Secrets, tokens, credentials, sensitive payloads, and customer content are redacted | Automated redaction tests | Yes |
| A8-14 | Baseline load | Agreed MSP, technician, and agent concurrency meets latency/error objectives | Versioned load report | Yes |
| A8-15 | Soak | Twenty-four-hour run shows no unbounded memory, goroutine, connection, lock, or queue growth | Soak report and metrics | Yes |
| A8-16 | Reconnect storm | Fleet reconnect uses jitter/backpressure and returns to steady state without data loss | Fault/load report | Yes |
| A8-17 | Dependency failure | PostgreSQL, NATS, object storage, DNS, and outbound-provider failures degrade safely and recover | Fault-injection matrix | Yes |
| A8-18 | MSP onboarding | Operator creates an MSP; MSP creates client/site, branding, and scoped enrollment without DB edits | Browser/API acceptance test | Yes |
| A8-19 | Entitlements | Quotas, grace periods, suspension, reactivation, and cached-state expiry are enforced centrally | API/database/browser tests | Yes |
| A8-20 | Custom domains | Verified host routing is tenant-correct and fails closed; provider integration is optional | Host-routing tests | Yes |
| A8-21 | Offboarding | Export, credential revocation, retention, and deletion workflow is authorized and auditable | Offboarding exercise | Yes |
| A8-22 | Authorization | Every privileged route has role and cross-scope negative tests | Authorization coverage report | Yes |
| A8-23 | Security pipeline | Dependency, image, secret, SBOM, static-analysis, and browser checks pass on exact head | GitHub Actions run | Yes |
| A8-24 | Incident response | Tabletop covers tenant breach, control-plane outage, key loss, and malicious endpoint command | Signed tabletop record | Yes |
| A8-25 | Operations | Alerts link to tested runbooks with an owner and escalation path | Alert/runbook audit | Yes |
| A8-26 | Beta decision | All mandatory evidence is indexed; residual risks and rollback owners are signed off | Go/no-go record | Yes |

## Per-PR verification

Every Phase 8 implementation PR must:

1. start from current `master`;
2. document its threat, tenancy, compatibility, migration, and rollback impact;
3. add focused automated tests that fail before the fix where practical;
4. run formatting, lint, unit, race, frontend, integration, security, build, and applicable browser/fault/load jobs;
5. monitor GitHub Actions to terminal state;
6. record the exact head SHA and workflow URL;
7. remain unmerged until all required checks pass and review requirements are satisfied.

## Phase 8A — Configuration and Startup Hardening

Phase 8A status is intentionally separated from the program-wide A8-01…A8-26 gates above. "Implemented" is not "accepted" until exact-head CI and the linked evidence are terminal and green.

| ID | Acceptance criterion | Current status |
|---|---|---|
| 8A-C01 | Typed centralized configuration and runtime modes | Verified — PR #7 merge 9543bbc |
| 8A-C02 | Production rejects insecure public URL, CORS, database, JWT, seeding, and NATS settings | Verified — PR #7 merge 9543bbc |
| 8A-C03 | NATS encrypted transport mandatory; tls:// or nats+tls:// URL; trusted CA required; token or mTLS auth | Verified — PR #7 merge 9543bbc |
| 8A-C04 | Liveness and readiness report meaningful DB, NATS, JetStream, migrations, dispatcher, and configured-storage checks | Verified — ingestion subscription telemetry deferred |
| 8A-C05 | Staged startup fails with active stage; does not silently disable configured storage | Verified — lifecycle fault-injection coverage partial |
| 8A-C06 | Secret-bearing URLs and DSNs are redacted from configuration summaries | Verified — PR #7 merge 9543bbc |
| 8A-C07 | CLI aliases and environment precedence remain compatible | Verified — PR #7 merge 9543bbc |
| 8A-C08 | Durable job reconciliation handles native identifier types | Verified — PR #7 merge 9543bbc |
| 8A-C09 | Configuration inventory and operator guidance match runtime consumers | Verified — PR #7 merge 9543bbc |
| 8A-C10 | Future agent work follows evidence-first delegation, review, CI, and PR discipline | Standard documented in `docs/AGENT_ENGINEERING_STANDARD.md` |

## Phase 8B — CI and Deployment Documentation

| ID | Acceptance criterion | Current status |
|---|---|---|
| 8B-C01 | CI validates deployment script syntax (shellcheck, systemd, YAML) | Implemented — `phase8b-static-validation` job |
| 8B-C02 | CI verifies deployment/upgrade/rollback docs exist | Implemented — `phase8b-docs-check` job |
| 8B-C03 | Deployment guide covers prerequisites, authoritative path, config, preflight, clean install, idempotency, health, troubleshooting | Implemented — `docs/DEPLOYMENT.md` |
| 8B-C04 | Upgrade guide covers supported paths, migration locking, steps with backup, verification, rollback during upgrade | Implemented — `docs/UPGRADE.md` |
| 8B-C05 | Rollback guide covers when-to-rollback criteria, procedure, data preservation, migration compatibility, emergency procedure | Implemented — `docs/ROLLBACK.md` |
| 8B-C06 | CONFIGURATION.md updated with Phase 8B deployment settings | Implemented — new env vars and CLI flags |
| 8B-C07 | Configuration documentation references deployment/upgrade/rollback docs | Implemented — cross-reference in CONFIGURATION.md |

## Evidence index

Populate this table during implementation.

| Acceptance ID | Evidence URL | Commit/release | Date | Owner | Result |
|---|---|---|---|---|---|
| 8A-C01…8A-C10 | [CI run 30381729001](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30381729001) | `342bf9e114ec5547333449e33a5e7df3a834d466` | 2026-07-28 | Coordinator | 21/21 passed; merged PR #7 |
| 8B-C01…8B-C07 | [CI run 30474346350](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30474346350) | `9ce0a607447bfb4ea811a09c16dc785b5085c2cc` | 2026-07-29 | Coordinator | 36/36 passed; merged PR #8 |
| A8-06…A8-08 / Phase 8C | [Phase 8C run 30602575852](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30602575852) and [CI run 30602575803](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30602575803) | `4e2a3bc00172adb7363b880c03077b61589eafcd` | 2026-07-31 | Coordinator | 20/20 Phase 8C and 30/30 repository CI passed |
| A8-09 | Timestamped isolated recovery drill | Pending | Pending | Operations | Not accepted |
| A8-18…A8-21 / Phase 8F | [Phase 8F run 30608673091](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30608673091) and [CI run 30608673085](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30608673085) | `a0dd106ccb95d32487f5cd9193469708cba6eeb0` | 2026-07-31 | Coordinator | 3/3 Phase 8F and 30/30 repository CI passed; hosted lifecycle exercise still required |

## Phase 8C — Backup and Disaster Recovery

| ID | Acceptance criterion | Current status |
|---|---|---|
| 8C-C01 | Independent key provider and external repository survive source loss | Verified |
| 8C-C02 | PostgreSQL backup restores schema, tenant data, durable jobs, and audit evidence into a distinct target | Verified |
| 8C-C03 | JetStream streams, consumer progress, headers, subjects, and messages restore into a distinct target | Verified |
| 8C-C04 | Object bytes, content type, metadata, and digests restore into a distinct target | Verified |
| 8C-C05 | Backup quiescing blocks database mutations and concurrent recovery | Verified |
| 8C-C06 | Restore verifies every artifact before target mutation and always performs bounded cleanup | Verified |
| 8C-C07 | CLI uses the real runtime path and has no validation bypass | Verified |
| 8C-C08 | Recovery documentation matches runtime behavior and limitations | Verified |
| 8C-C09 | Beta RPO/RTO is proven by a timestamped isolated drill | Not accepted; operational drill required |

## Phase 8D — Observability, Synthetics, and Operator Response

Implementation evidence is maintained in `docs/PHASE_8D_EVIDENCE.md`. These
rows remain partial until exact-head CI and the environment-level delivery
exercise are recorded.

| ID | Acceptance criterion | Current status |
|---|---|---|
| A8-10 | API traffic, errors, latency, and saturation are observable and alertable | CI verified; environment alert delivery pending |
| A8-11 | Durable-job state, retries, age, failures, and telemetry failure are observable | CI and PostgreSQL integration verified |
| A8-12 | Independent public, login, authenticated API, agent, and storage synthetics | Implemented; environment exercise pending |
| A8-13 | Metrics, synthetic output, and request logs exclude credentials and raw resource identifiers | CI verified; broader Phase 8G audit remains |
| A8-25 | Phase 8D alerts have owners, severities, and tested runbook links | Implemented; paging delivery exercise pending |

## Phase 8E — Resilience Validation

Phase 8E adds a bounded load runner and reconnect dispersion policy, then
defines the evidence boundary between short CI contracts and mandatory
environment exercises.

| ID | Acceptance criterion | Current status |
|---|---|---|
| A8-14 | Agreed MSP, technician, API, and agent workload meets latency/error objectives | Harness and CI threshold contract verified; environment baseline pending |
| A8-15 | Twenty-four-hour soak has no unbounded resource or queue growth | Evidence contract implemented; 24-hour exercise pending |
| A8-16 | Fleet reconnect is dispersed and returns to steady state without loss or duplicate destructive work | Jitter policy and wiring CI verified; environment storm pending |
| A8-17 | Required dependency failures degrade safely and recover | CI readiness contract verified; environment matrix pending |

Detailed procedure and evidence: [Resilience testing](RESILIENCE_TESTING.md) and
[Phase 8E evidence](PHASE_8E_EVIDENCE.md).

## Phase 8F — MSP Lifecycle and Unified Dashboard

| ID | Acceptance criterion | Current status |
|---|---|---|
| A8-18 | Provision, brand, enroll, and operate an MSP without direct database edits | CI/API/browser contracts verified; hosted exercise pending |
| A8-19 | Entitlements, quotas, grace, suspension, reactivation, and expiry are centrally enforced | CI and PostgreSQL integration verified |
| A8-20 | Platform, subdomain, and verified custom-domain routing are tenant-correct and fail closed | CI host-routing contracts verified; provider exercise pending |
| A8-21 | Bounded export, credential revocation, retention, and deletion approval are authorized and auditable | CI/PostgreSQL verified; hosted offboarding exercise pending |

## Phase 8G — Security and Alpha Gate

Implementation evidence is maintained in [Phase 8G evidence](PHASE_8G_EVIDENCE.md).

| ID | Acceptance criterion | Current status |
|---|---|---|
| A8-22 | Every privileged route has role and cross-scope negative proof | CI verified at `ebeee2de6f3ddb354ebb41fef2959ceeefaf5082`; hosted adversarial review remains recommended |
| A8-23 | Dependency, image, secret, SBOM, static-analysis, frontend, and browser gates pass | 69/69 exact-head jobs passed; see Phase 8G evidence |
| A8-24 | Four-scenario incident-response tabletop is signed | Not accepted; procedure/template only |
| A8-25 | Alert/runbook owners and escalation paths are exercised | Partial; hosted delivery audit pending |
| A8-26 | Mandatory evidence and residual risks receive signed go/no-go | Not accepted |

## Provider first-login business-profile slice

This bounded slice adds provider account setup and later profile management. It
does not change the existing program-wide acceptance status above.

| ID | Acceptance criterion | Current status |
|---|---|---|
| PB-01 | Migration 67 adds provider fields and immutable completion metadata; secure bootstrap grants the first platform owner | Implemented; focused schema, bootstrap, and PostgreSQL coverage added |
| PB-02 | An incomplete top-level provider administrator must pass Business, Contact, Regional Defaults, and Review before explicit submission | Implemented; frontend and Chromium acceptance coverage added |
| PB-03 | The API, rather than client routing, enforces active singleton-platform owner/admin authorization and denies tenant-scoped/non-platform access | Implemented; handler authorization and PostgreSQL negative coverage added |
| PB-04 | Server validation, strict input, concurrency locking, identical-retry idempotency, and protected completion metadata are enforced | Implemented; validation, handler, and database integration coverage added |
| PB-05 | Setup and effective profile edits atomically append immutable control-plane audit evidence without copying profile values | Implemented; focused PostgreSQL integration coverage added |
| PB-06 | Completed profile fields can be edited in Platform Settings while completion actor/time remain unchanged | Implemented; frontend and browser persistence coverage added |
| PB-07 | Exact-head CI, review, hosted exercises, and all internal-alpha launch gates are complete | Not accepted; broader A8-22…A8-26 status remains unchanged |

PB-01…PB-06 do not represent a white-label licensing system, all dashboard
levels, or complete internal-alpha readiness.
