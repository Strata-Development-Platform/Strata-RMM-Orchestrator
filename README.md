# Strata RMM Orchestrator

A horizontally-scalable, multi-tenant Remote Monitoring & Management platform with cross-platform agents (Go), supporting both SaaS and self-hosted deployments.

## Quick Start

```bash
# Prerequisites: Go 1.22+, Docker, NATS 2.10+, TimescaleDB 2.15+

# Build everything
make build

# Start local dev environment
docker compose -f deploy/docker/docker-compose.yml up -d nats postgres

# Start orchestrator
make run-orch

# In another terminal - run an agent
TENANT_ID="00000000-0000-0000-0000-000000000001" ENROLLMENT_TOKEN="dev-token" make run-agent
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `strata-rmm agent` | Cross-platform monitoring agent |
| `strata-rmm probe` | Agentless network probe (SNMP, flow, discovery) |
| `strata-rmm orchestrator` | Platform services (API, ingestion, alerting, patching, tunnels) |

## Architecture

```
Agent (Go) ──NATS──► Orchestrator ──► TimescaleDB (metrics)
  │                        │                    │
  │                    PostgreSQL (RLS)      Grafana
  │                        │
Probe (SNMP/Flow) ──NATS──┘
```

- **Messaging**: NATS Core with tenant subject isolation (`tenant.{id}.agent.{id}.{metrics,events,heartbeat,cmd}`)
- **Time-Series**: TimescaleDB hypertables with compression (7d) + continuous aggregates (1m, 1h)
- **Relational**: PostgreSQL with Row-Level Security for tenant isolation
- **API**: Go 1.22 stdlib `net/http` (method-based routing, no framework)

## Features

### ✅ Phase 1 — Foundation
- Go cross-platform agent (Linux/Windows/macOS, single binary ~15MB)
- NATS communication with reconnect backoff + store-and-forward (BBolt)
- Metrics ingestion pipeline (NATS → TimescaleDB batch writer)
- JWT auth + enrollment tokens
- REST API (health, metrics query, heartbeat, enrollment)
- PostgreSQL relational schema with RLS (tenants, devices, users, audit)
- Docker + Makefile for local dev

### ✅ Phase 2 — Monitoring Core
- **Alerting Engine**: Threshold rules (gt/gte/lt/lte/eq/neq), heartbeat monitoring, cooldowns, auto-resolution
- **Notifications**: Slack, webhook, email/teams/pagerduty (pluggable)
- **Network Probe**: SNMP v1/v2c/v3 polling, NetFlow v9/IPFIX/sFlow, ARP/SNMP discovery
- **Patch Management**: Windows (PowerShell/WU API) + Linux (apt/dnf/zypper), policy CRUD, deployment scheduling
- **Remote Access**: NATS-relayed RDP/SSH/VNC tunnels with bidirectional streaming
- **Grafana Dashboards**: System metrics + alerts overview (provisioned)

### ✅ Phase 3 — Advanced Features
- Software inventory (dpkg/Win32_Product)
- CVE vulnerability correlation (10 seeded CVEs, automatic device matching, 6h scan loop)

### ✅ Phase 5 — Platform Hardening
- Helm chart (deployment, HPA, PDB, network policies, ingress, air-gapped, multi-region)
- Load testing script (vegeta API + NATS agent simulation, 500 agents)

## Multi-Tenancy

- **Default**: Shared schema with RLS + NATS subject isolation
- **Premium**: Dedicated schema/database per tenant
- Tenant onboarding: Create tenant → run migrations → generate enrollment token

## Deployment Models

1. **SaaS**: Multi-region K8s via Helm, GitOps (ArgoCD/Flux), NATS supercluster
2. **Self-Hosted**: Helm charts, air-gapped bundles, license validation
3. **Hybrid**: SaaS control plane + customer-hosted agents/probes

## Security

- **Agent ↔ Platform**: mTLS or JWT enrollment → short-lived certs
- **Data**: AES-256 at rest, TLS 1.3 in transit
- **RBAC**: Admin/Technician/Viewer roles per tenant
- **Audit**: Immutable append-only log

## Project Structure

```
├── cmd/
│   ├── agent/          # Agent CLI
│   ├── probe/          # Network probe CLI
│   └── orchestrator/   # Platform services CLI
├── internal/
│   ├── agent/          # Agent core (config, identity, BBolt, collectors)
│   ├── alerting/       # Alert engine + notifications
│   ├── collectors/     # Software inventory collector
│   ├── monitoring/     # Metrics/events/heartbeat ingestion
│   ├── inventory/      # Device CRUD + vulnerability engine
│   ├── patch/          # Patch management + executors
│   ├── platform/       # API server + routes
│   ├── probe/          # SNMP, flow, discovery
│   └── remote/         # Tunnel relay gateway
├── pkg/
│   ├── auth/           # JWT generation/validation
│   ├── postgres/       # Relational schema migrations
│   └── timescale/      # TimescaleDB client + migrations
├── deploy/
│   ├── docker/         # Docker Compose local dev
│   ├── grafana/        # Provisioned dashboards
│   └── helm/           # K8s Helm chart
├── docs/
│   ├── ARCHITECTURE.md # Full architecture plan
│   └── RUNBOOK.md      # Operations runbook
└── scripts/
    ├── install.sh      # Linux agent installer
    └── loadtest.sh     # Performance test suite
```

## Development

```bash
# Build
make build          # All platforms
make build-linux    # Linux amd64
make build-windows  # Windows amd64
make build-arm64    # Linux arm64

# Test
make test
make lint
make coverage

# Local dev
make dev            # Start services + orchestrator
make docker-up      # Start all containers
make docker-down    # Stop all containers
```

See [docs/RUNBOOK.md](docs/RUNBOOK.md) for operations guide and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full plan.
