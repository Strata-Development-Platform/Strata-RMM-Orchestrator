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

Phase 8A acceptance criteria and verification status:

| ID | Acceptance Criterion | Status |
|---|---|---|
| A8-01 | Centralized configuration with runtime modes — `LoadOrchestratorConfig()` parses all settings from env vars with typed validation | Verified |
| A8-02 | Production fail-closed validation — `ProductionValidate()` rejects insecure settings in production mode | Verified |
| A8-03 | Liveness/readiness with dependency checks — health endpoints return correct status during startup and runtime | Verified |
| A8-04 | Staged startup — orchestrator initializes subsystems in order, health returns `starting` until complete | In progress |
| A8-05 | Secret redaction — `RedactedSummary()` masks sensitive values before logging | Verified |
| A8-06 | CLI compatibility — existing CLI flags and env vars continue to work with the new configuration layer | Verified |
| A8-07 | UUID/text defect correction — configuration parsing uses strict typed parsing | Verified |
| A8-08 | Configuration inventory — exhaustive inventory table documented in `docs/CONFIGURATION.md` | Verified |
| A8-09 | Expanded test coverage — configuration loading, validation, and redaction have unit tests | Verified |

## Evidence index

Populate this table during implementation.

| Acceptance ID | Evidence URL | Commit/release | Date | Owner | Result |
|---|---|---|---|---|---|---|
| A8-01…A8-26 | Pending | Pending | Pending | Pending | Pending |
