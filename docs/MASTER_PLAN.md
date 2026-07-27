# Strata RMM Orchestrator — Master Plan

## Objective

Transform the existing Strata RMM Orchestrator prototype into a secure, functional, multi-tenant SaaS RMM platform capable of competing with commercial products such as Kaseya and Datto.

## Architecture

- **Module**: `github.com/strata-rmm/strata-rmm-orchestrator`
- **Language**: Go 1.25+
- **Database**: PostgreSQL 16 + TimescaleDB 2.x
- **Messaging**: NATS JetStream 2.10+
- **UI**: React 18 + Vite + Tailwind CSS
- **Auth**: JWT (HS256) → Future: asymmetric keys + refresh tokens
- **Storage**: S3-compatible (MinIO / AWS S3)
- **Proxying**: nginx (current) → Future: Caddy or Traefik

## Deployment Hierarchy

```
Platform Operator
  └─ MSP Tenant
       └─ Client Organization
            └─ Site / Location
                 └─ Managed Device
```

## Delivery Phases

| Phase | Focus | Status |
|-------|-------|--------|
| 0 | Baseline, backup, feature matrix, documentation | ✅ Complete |
| 1 | Security containment — auth, isolation, secrets | ⏳ Next |
| 2 | SaaS ownership — MSP/client/site model | 🔲 |
| 3 | Branding and custom domains | 🔲 |
| 4 | Secure enrollment and agent identity | 🔲 |
| 5 | Durable job orchestration | 🔲 |
| 6 | RMM vertical slices (inventory, alerts, scripts, patching, remote) | 🔲 |
| 7 | Production hardening (load testing, DR, SSO, billing) | 🔲 |

## Baseline

- **Starting commit**: `d4c4ab1`
- **Backup branch**: `backup/pre-saas-rewrite-20260727`
- **Baseline tag**: `pre-saas-rewrite-20260727`
- **Production hostname**: `https://rmm.stratadevplatform.com`
