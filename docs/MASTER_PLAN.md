# Strata RMM Orchestrator — Master Plan

## Objective

Transform Strata RMM Orchestrator into a secure, functional, multi-tenant SaaS RMM platform operated by Strata, with MSP tenancy, delegated client/site administration, branded portals, and managed endpoint operations.

## Architecture

- **Module**: `github.com/strata-rmm/strata-rmm-orchestrator`
- **Language**: Go 1.25+
- **Database**: PostgreSQL 16 + TimescaleDB 2.x
- **Messaging**: NATS JetStream 2.10+
- **UI**: React 18 + Vite + Tailwind CSS
- **Auth**: JWT (HS256); asymmetric signing and session hardening remain Phase 8 work
- **Storage**: S3-compatible (MinIO / AWS S3)
- **Proxying**: nginx currently; custom-domain providers, including optional Cloudflare integration, remain provider-neutral

## Deployment hierarchy

```text
Platform Operator
  └─ MSP Tenant
       └─ Client Organization
            └─ Site / Location
                 └─ Managed Device
```

## Delivery phases

| Phase | Focus | Status |
|---|---|---|
| 0 | Baseline, backup, feature matrix, documentation | Complete |
| 1 | Security containment — auth, isolation, secrets | Complete |
| 2 | SaaS ownership — MSP/client/site model | Complete |
| 3 | Branding and custom-domain foundation | Complete |
| 4 | Secure enrollment and agent identity | Complete |
| 5 | Durable job orchestration | Complete |
| 6 | RMM vertical slices and technician workflows | Substantially implemented |
| 7 | Endpoint operations, approvals, lifecycle, audit, browser acceptance | Complete — PR #4 |
| 8 | Production beta readiness | In progress |

“Complete” means the planned phase scope was merged and verified; it does not by itself mean the entire platform is ready for unrestricted production use.

## Current phase

Phase 8 establishes evidence-based hosted-beta gates across deployment, recovery, observability, resilience, MSP lifecycle, security, and operations:

- [Production beta plan](PHASE_8_PRODUCTION_BETA.md)
- [Risk register](PHASE_8_RISK_REGISTER.md)
- [Acceptance matrix](PHASE_8_ACCEPTANCE_MATRIX.md)

Implementation follows focused PRs 8A–8G. A controlled beta may launch only after every mandatory acceptance criterion has durable evidence and residual risk has explicit ownership.

## Baseline

- **Starting commit**: `d4c4ab1`
- **Backup branch**: `backup/pre-saas-rewrite-20260727`
- **Baseline tag**: `pre-saas-rewrite-20260727`
- **Phase 7 merge**: `8f00d81894b0fa466af76336bbbb297f4e6e2218`
- **Production hostname**: `https://rmm.stratadevplatform.com`
