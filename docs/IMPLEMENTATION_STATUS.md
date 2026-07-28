# Implementation Status — Historical Phase 0 Baseline

> **Status note (2026-07-28):** This file preserves the original pre-rewrite inventory and is not the authoritative description of current `master`. Several findings below—including the hardcoded JWT fallback, in-memory enrollment, incomplete tenant hierarchy, missing centralized authorization, and missing cross-tenant/browser coverage—were addressed in Phases 1–7. Use the [master plan](MASTER_PLAN.md), [Phase 8 plan](PHASE_8_PRODUCTION_BETA.md), [risk register](PHASE_8_RISK_REGISTER.md), and [acceptance matrix](PHASE_8_ACCEPTANCE_MATRIX.md) for current delivery status and release gates. Phase 7 was verified and merged in PR #4 at `8f00d81894b0fa466af76336bbbb297f4e6e2218`.

## Original feature matrix

The tables below are retained as the Phase 0 discovery record. “Verified” means observed in the starting prototype; it is not a production-readiness assertion.

### Core platform

| Feature | Phase 0 status | Historical note |
|---|---|---|
| HTTP API server | Verified | 66 routes in `api.go` |
| PostgreSQL schema | Verified | 30 tables, 26 migrations |
| TimescaleDB schema | Verified | 7 hypertables, 2 migrations |
| NATS messaging | Verified | 12 wildcard subscriptions, 23 subject patterns |
| Multi-tenant data isolation | Partial | Ambiguous single `tenant_id`; subsequently remediated through SaaS hierarchy and isolation work |
| Agent enrollment | Partial | In-memory tokens; subsequently replaced with persisted scoped enrollment |
| JWT auth | Verified with critical defect | Hardcoded fallback; subsequently removed |
| MFA / TOTP | Verified | RFC 6238 |
| Rate limiting and security headers | Verified | Prototype controls present |

### Endpoint and RMM capabilities

| Area | Phase 0 status | Historical note |
|---|---|---|
| Cross-platform agent | Verified | Linux, Windows, macOS |
| Metrics, heartbeat, inventory | Verified/partial | Expanded through later endpoint operations work |
| Script and software execution | Verified | Later integrated with durable jobs, approvals, idempotency, and audit |
| OS patching | Partial | Platform coverage remained incomplete |
| Remote desktop capture | Stub | Production remote-access hardening remains gated |
| CVE/NVD and third-party catalog sync | Stub/partial | Seed/empty-provider behavior observed |
| Alerting and dashboards | Verified | Production observability still requires Phase 8 evidence |
| Reports and schedules | Verified | Prototype capability |
| Network probe | Verified | SNMP, flow collection, discovery |
| Tenant encryption keys | Verified | Operational key custody/restore remains a Phase 8 gate |

### User interface

The prototype included login, dashboard, customers, scripts, software, reports, settings, administration, and remote-desktop views. Phases 5–7 added durable Job Center and technician-console workflows. Browser acceptance for Phase 7 is part of the verified baseline; production accessibility, performance, and operational monitoring remain Phase 8 concerns.

### Historical security findings

| Finding at Phase 0 | Original severity | Current disposition |
|---|---:|---|
| Hardcoded JWT fallback | Critical | Removed; startup fails without required secret |
| Placeholder seed password | Critical | Remediated in containment work |
| In-memory enrollment tokens | High | Replaced with DB-backed scoped enrollment |
| Owner-bypassed RLS | High | Restricted-role and cross-scope tests added |
| Missing centralized route authorization | Medium | Central route classification/middleware added |
| HS256 and no refresh-token rotation | Medium | Still open for Phase 8 session/signing hardening |
| Incomplete MSP/client/site hierarchy | Medium | Implemented in SaaS ownership phases |

### Historical testing baseline

At Phase 0, Go unit tests and frontend type checking passed, while race, API contract, isolation, browser E2E, and full load coverage were absent or incomplete. Later phases added dedicated tenant-isolation, database, integration, security, durable-job, endpoint-operation, and browser acceptance jobs. Phase 8 now requires exact-head CI plus recovery, fault, load, soak, and operational evidence defined in the acceptance matrix.

### Deployment baseline

Docker Compose, Helm, KOTS, manual Linux installation, GoReleaser, and Grafana resources existed in the prototype. Their existence is not proof of a repeatable or recoverable hosted production deployment; Phase 8A–8E provide those gates.
