# Strata RMM Orchestrator

A horizontally-scalable, multi-tenant Remote Monitoring & Management platform with cross-platform agents (Go), supporting both SaaS and self-hosted deployments.

## Architecture Overview

Built on polyglot microservices with:
- **NATS JetStream** - Message backbone with multi-tenant subject isolation
- **TimescaleDB** - Time-series metrics with compression & continuous aggregates
- **PostgreSQL** - Relational data with Row Level Security for multi-tenancy
- **Redis** - Caching, sessions, distributed locks
- **Kubernetes** - Elastic scaling, self-healing, GitOps deployment

## Components

| Component | Language | Description |
|-----------|----------|-------------|
| **Agent** | Go | Cross-platform monitoring agent (Windows/Linux/macOS) |
| **Probe** | Go | Agentless network collector (SNMP, NetFlow, synthetics) |
| **Orchestrator** | Go | Platform services (API, Inventory, Monitoring, Alerting, Remote Access) |
| **API Gateway** | Kong/Traefik | TLS, rate limiting, tenant routing, WebSocket tunnels |

## Quick Start

### Prerequisites
- Go 1.23+
- NATS JetStream cluster
- PostgreSQL 15+ with TimescaleDB extension
- Redis 7+

### Build
```bash
go build -o bin/strata-rmm .
```

### Run Agent
```bash
./bin/strata-rmm agent \
  --tenant-id=<TENANT_UUID> \
  --enrollment-token=<TOKEN> \
  --nats-url=nats://localhost:4222
```

### Run Network Probe
```bash
./bin/strata-rmm probe \
  --tenant-id=<TENANT_UUID> \
  --nats-url=nats://localhost:4222
```

### Run Orchestrator (Platform Services)
```bash
./bin/strata-rmm orchestrator \
  --config=config.yaml
```

## Project Structure

```
├── cmd/
│   ├── agent/        # Monitoring agent entry point
│   ├── probe/        # Network probe entry point
│   └── orchestrator/ # Platform services entry point
├── internal/
│   ├── agent/        # Agent core (identity, comms, collectors, executors)
│   ├── platform/     # Platform services
│   ├── monitoring/   # Metrics ingestion & check evaluation
│   ├── alerting/     # Alert engine & notifications
│   ├── inventory/    # Asset CMDB & discovery
│   ├── remoteaccess/ # Reverse tunnel & protocol proxies
│   └── patch/        # Patch/software management
├── pkg/
│   ├── nats/         # NATS JetStream client helpers
│   ├── timescale/    # TimescaleDB client & schemas
│   ├── postgres/     # PostgreSQL client & migrations
│   └── auth/         # OIDC/mTLS authentication
├── deploy/
│   ├── helm/         # Helm charts for K8s
│   ├── kots/         # KOTS manifests for self-hosted
│   └── docker/       # Docker Compose for dev
├── docs/
│   └── ARCHITECTURE.md
├── scripts/          # Build & deployment scripts
└── tests/            # Integration & e2e tests
```

## Development Phases

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full 12-month roadmap:

- **Phase 1** (Months 1-3): Foundation - Tenant/Auth, DB, NATS, Agent core
- **Phase 2** (Months 3-5): Monitoring Core - Ingestion, Alerting, SNMP, Dashboards
- **Phase 3** (Months 5-7): Remote Access & Automation - Tunnels, Patching
- **Phase 4** (Months 7-10): Advanced - ML Anomaly, NetFlow, IP Phones, Synthetics
- **Phase 5** (Months 10-12): Hardening - Self-hosted distro, Air-gap, Security audit

## Multi-Tenancy

- **Shared (Default)**: Single cluster, RLS policies, subject isolation `tenant.{id}.*`
- **Dedicated (Premium)**: Per-tenant namespace/cluster, data residency

## Deployment Models

1. **SaaS** - Multi-region K8s (EKS/GKE/AKS), GitOps via ArgoCD/Flux
2. **Self-Hosted** - Helm + KOTS, air-gapped bundles, license validation
3. **Hybrid** - Control plane (SaaS) + Data plane (customer-hosted)

## Security

- **Agent ↔ Platform**: mTLS with auto-rotated certs (or JWT enrollment)
- **Data**: AES-256 at rest, TLS 1.3 in transit
- **Secrets**: HashiCorp Vault / Sealed Secrets
- **Audit**: Immutable append-only logs

## License

Proprietary - Strata Development Platform

---

## Contributing

Internal project - not accepting external contributions at this time.