# RMM Platform Architecture Plan

## Executive Summary

A horizontally-scalable, multi-tenant Remote Monitoring & Management platform with cross-platform agents (Go), supporting both SaaS and self-hosted deployments. Built on polyglot microservices with NATS JetStream for messaging, TimescaleDB for metrics, and PostgreSQL for relational data.

---

## 1. High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              PLATFORM LAYER                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │   API GW     │  │   AuthZ      │  │   Tenant     │  │   Billing/   │   │
│  │  (Kong/      │  │   (OAuth2/   │  │   Manager    │  │   Usage      │   │
│  │   Traefik)   │  │   OIDC)      │  │              │  │              │   │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │
│         │                 │                 │                 │           │
│  ┌──────▼─────────────────▼─────────────────▼─────────────────▼───────┐   │
│  │                    NATS JetStream Cluster                            │   │
│  │  (Subjects: tenant.{id}.agent.>, tenant.{id}.cmd.>, platform.>)    │   │
│  └──────┬────────────────────────────────────────────────────────────┘   │
│         │                                                                │
│  ┌──────▼──────┐ ┌──────▼──────┐ ┌──────▼──────┐ ┌──────▼──────┐       │
│  │  Inventory  │ │  Monitoring │ │  Alerting   │ │  Remote     │       │
│  │  Service    │ │  Service    │ │  Service    │ │  Access     │       │
│  │  (Go)       │ │  (Go/Rust)  │ │  (Go)       │ │  (Go)       │       │
│  └──────┬──────┘ └──────┬──────┘ └──────┬──────┘ └──────┬──────┘       │
│         │               │               │               │               │
│  ┌──────▼───────────────▼───────────────▼───────────────▼───────┐       │
│  │                    DATA LAYER                                  │       │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────┐ │       │
│  │  │ PostgreSQL  │ │ TimescaleDB │ │   Redis     │ │ Object  │ │       │
│  │  │ (Relational │ │ (Metrics)   │ │ (Cache/     │ │ Store   │ │       │
│  │  │  + Tenant   │ │             │ │  Sessions)  │ │ (S3/    │ │       │
│  │  │  隔离)      │ │             │ │             │ │  MinIO) │ │       │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────┘ │       │
│  └────────────────────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┼───────────────┐
                    ▼               ▼               ▼
            ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
            │   Agent      │ │   Agent      │ │  Network     │
            │  (Windows)   │ │  (Linux)     │ │  Probe       │
            │  (Go)        │ │  (Go)        │ │  (Go)        │
            └──────────────┘ └──────────────┘ └──────────────┘
```

---

## 2. Core Services

### 2.1 API Gateway (Kong/Traefik)
- TLS termination, rate limiting, request routing
- Tenant resolution from subdomain/header/JWT
- WebSocket upgrade for remote access tunnels

### 2.2 Auth & Tenancy Service
- **OIDC Provider**: Keycloak or custom (Go)
- **Multi-tenancy**: 
  - Shared schema with `tenant_id` column + RLS policies (default)
  - Optional dedicated schema/database per tenant (premium)
- **RBAC**: Role-based (Admin, Technician, Viewer) + resource-level permissions
- **API Keys**: For agent bootstrap and integrations

### 2.3 Inventory Service (Go)
- Asset CRUD: devices, network gear, software, services
- Discovery: SNMP, ARP, NDP, mDNS, WS-Discovery
- Hardware/Software inventory collection via agent
- CMDB relationships (dependency mapping)
- **API**: REST + GraphQL for flexible queries

### 2.4 Monitoring Service (Go/Rust)
- **Metrics Ingestion**: NATS → TimescaleDB (batch writes)
- **Check Types**:
  - System: CPU, RAM, Disk, Net, Processes, Services
  - Network: ICMP, SNMP (v2c/v3), NetFlow/sFlow, IPMI/Redfish
  - Application: HTTP, TCP, DNS, Custom scripts
  - IP Phones: SIP registration, RTP quality, vendor APIs (Cisco, Yealink, Poly)
- **Collection Modes**:
  - Agent-based (high-fidelity, high-frequency)
  - Agentless (SNMP/ICMP/API) for network gear, printers, IP phones
- **Retention**: Hot (7d, 1s resolution) → Warm (90d, 1m) → Cold (7y, 1h)

### 2.5 Alerting Engine (Go)
- **Rule Types**: Threshold, Anomaly (ML-based), Composite, Heartbeat
- **Notification Channels**: Email, SMS, Slack, Teams, PagerDuty, Opsgenie, Webhook
- **Escalation Policies**: Time-based, on-call rotations
- **Alert Deduplication**: Correlation, grouping, auto-resolution
- **Silencing/Inhibitions**: Maintenance windows, dependency-aware

### 2.6 Remote Access Service (Go)
- **Tunnel Architecture**: 
  - Agent registers reverse tunnel (WebSocket over NATS)
  - Gateway proxies RDP/SSH/VNC through authenticated tunnel
  - No inbound ports required on agent side
- **Protocols**: 
  - RDP (Windows) via FreeRDP/WebRDP
  - SSH (Linux) via native Go SSH server
  - VNC via websockify proxy
  - Custom TCP port forwarding
- **Session Recording**: Optional audit logs, video recording
- **MFA**: Required for interactive sessions

### 2.7 Patch/Software Management (Go + Python)
- **Windows**: WSUS/Windows Update API, Chocolatey, Winget
- **Linux**: apt/dnf/zypper/pacman, Flatpak, Snap
- **Third-party**: Custom repositories, vulnerability scanning (CVE)
- **Deployment**: Staged rollouts, maintenance windows, rollback

### 2.8 Network Probe (Go)
- Lightweight agentless collector for network segments
- SNMP polling (bulk walks, parallel)
- Flow collection (NetFlow v5/v9, IPFIX, sFlow)
- Synthetic monitoring (HTTP, DNS, TCP, ICMP from multiple vantage points)
- Network topology discovery (LLDP, CDP, STP, ARP)

---

## 3. Agent Architecture (Go)

### 3.1 Design Principles
- Single static binary (~15-25MB)
- Zero dependencies, runs as SYSTEM/root or unprivileged
- Self-update with signature verification
- Modular: core + pluggable collectors

### 3.2 Communication
```
Agent ←→ NATS JetStream (tenant.{id}.agent.{agent_id})
  ├── Metrics:   Periodic batch publish (configurable interval, default 60s)
  ├── Events:    Real-time (heartbeat, alerts, inventory changes)
  ├── Commands:  Subscribe to tenant.{id}.cmd.{agent_id} (request/response)
  └── Bulk:      File transfer, scripts, patches via object store presigned URLs
```
- **Reliability**: 
  - Local persistence (BBolt/SQLite) during disconnect
  - Automatic replay on reconnect
  - QoS: at-least-once for metrics, exactly-once for commands
- **NAT Traversal**: Outbound-only, works behind CGNAT/firewalls

### 3.3 Module System
```
core/
  ├── config       # TOML/JSON, remote config via NATS KV
  ├── identity     # Cert-based (mTLS) or token-based auth
  ├── comms        # NATS client, reconnection, backoff
  ├── update       # Self-update, rollback, staging
  └── health       # Self-monitoring, watchdog

collectors/
  ├── system       # CPU, mem, disk, net, processes (gopsutil)
  ├── hardware     # SMART, IPMI, BIOS, chassis (via WMI/Linux sysfs)
  ├── software     # Installed packages, vulnerabilities
  ├── services     # systemd, Windows Services, Docker, Podman
  ├── network      # Interfaces, connections, listening ports
  └── custom       # Plugin interface (WASM or native .so/.dll)

executors/
  ├── shell        # PowerShell, Bash, CMD
  ├── script       # Python, custom interpreters
  ├── patch        # OS/third-party update orchestration
  └── remote       # Tunnel endpoint for remote access
```

### 3.4 Platform Support
| Platform | Arch | Notes |
|----------|------|-------|
| Windows 10/11 | amd64, arm64 | MSI, EXE, Winget |
| Windows Server 2016+ | amd64, arm64 | Service install |
| Ubuntu 20.04+ | amd64, arm64 | DEB, systemd |
| Debian 11+ | amd64, arm64 | DEB |
| RHEL/Rocky/Alma 8+ | amd64, arm64 | RPM |
| Fedora 38+ | amd64, arm64 | RPM |
| Alpine 3.18+ | amd64, arm64 | APK, OpenRC |
| macOS 12+ | amd64, arm64 | PKG, LaunchDaemon (optional) |

---

## 4. Data Layer

### 4.1 PostgreSQL (Relational + Tenant Isolation)
```sql
-- Core tables with tenant_id + RLS
CREATE TABLE tenants (...);
CREATE TABLE devices (...);
CREATE TABLE alerts (...);
CREATE TABLE users (...);
CREATE TABLE roles_permissions (...);

-- Row Level Security
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON devices USING (tenant_id = current_tenant_id());
```

**Dedicated Tenant Option**: Separate schema/database per tenant, managed by Tenant Manager service.

### 4.2 TimescaleDB (Metrics)
```sql
-- Hypertable for metrics
CREATE TABLE metrics (
    time        TIMESTAMPTZ       NOT NULL,
    tenant_id   UUID              NOT NULL,
    device_id   UUID              NOT NULL,
    metric_name TEXT              NOT NULL,
    value       DOUBLE PRECISION  NOT NULL,
    tags        JSONB             DEFAULT '{}'
);
SELECT create_hypertable('metrics', 'time', chunk_time_interval => INTERVAL '1 day');

-- Compression
ALTER TABLE metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'device_id, metric_name');
SELECT add_compression_policy('metrics', INTERVAL '7 days');

-- Continuous aggregates for downsampling
CREATE MATERIALIZED VIEW metrics_1m WITH (timescaledb.continuous) AS
SELECT time_bucket('1 minute', time) AS bucket, tenant_id, device_id, metric_name,
       AVG(value) AS avg, MIN(value) AS min, MAX(value) AS max, COUNT(*) AS count
FROM metrics GROUP BY bucket, tenant_id, device_id, metric_name;
```

### 4.3 Redis (Cache/Sessions)
- Session store, rate limiting, distributed locks
- NATS JetStream KV for agent config/state (alternative)

### 4.4 Object Store (S3/MinIO)
- Agent binaries, patches, scripts
- Session recordings, exported reports
- Bulk command payloads (>1MB)

---

## 5. Multi-Tenancy Implementation

### 5.1 Shared Infrastructure (Default)
- Single Kubernetes cluster / VM fleet
- PostgreSQL: RLS + connection pooling (PgBouncer per tenant)
- NATS: Subject isolation `tenant.{id}.*`
- TimescaleDB: `tenant_id` column + compression by tenant
- Cost-efficient, easier operations

### 5.2 Dedicated Tenant (Optional)
- Namespace/cluster per tenant
- Separate PostgreSQL, TimescaleDB, NATS streams
- Data residency compliance
- Premium tier, higher cost

### 5.3 Tenant Onboarding Flow
```
1. Create tenant record → 2. Provision DB schema/RLS → 3. Create NATS streams/KV
   → 4. Generate agent enrollment token → 5. Optional: dedicated resources
```

---

## 6. Deployment Models

### 6.1 SaaS (Primary)
- **Platform**: Kubernetes (EKS/GKE/AKS or self-managed)
- **Regions**: Multi-region for latency/data residency
- **Scaling**: HPA/VPA on all services, NATS cluster scaling
- **Operations**: GitOps (ArgoCD/Flux), observability stack

### 6.2 Self-Hosted (Customer-Managed)
- **Distribution**: Helm charts + KOTS (Replicated) or Docker Compose
- **Requirements**: K8s 1.27+ or Docker Swarm, PostgreSQL 15+, TimescaleDB, NATS, Redis, S3-compatible
- **Air-gapped**: Full offline install bundle
- **Updates**: Semantic versioning, automated migration jobs
- **License**: License key validation (phone-home optional)

### 6.3 Hybrid
- Control plane (SaaS) + Data plane (customer-hosted agents/probes)
- Customer data never leaves their network

---

## 7. Security Architecture

### 7.1 Agent ↔ Platform
- **mTLS**: Platform CA → Agent certificates (auto-rotated)
- **Alternative**: JWT enrollment token → short-lived certs
- **NAT Traversal**: Outbound-only, no port forwarding

### 7.2 Data Protection
- **At Rest**: AES-256 (disk encryption + DB TDE)
- **In Transit**: TLS 1.3 everywhere
- **Secrets**: HashiCorp Vault or Sealed Secrets (K8s)

### 7.3 Access Control
- **Zero Trust**: Every request authenticated + authorized
- **Audit Log**: Immutable, tamper-evident (append-only + WORM storage)
- **Pen Testing**: Annual third-party, bug bounty program

---

## 8. Observability (Platform Self-Monitoring)

- **Metrics**: Prometheus + Grafana (internal tenant)
- **Logs**: Loki/Elasticsearch
- **Traces**: Tempo/Jaeger
- **SLOs**: Defined per service, alerting on burn rate
- **Chaos Engineering**: Regular GameDays

---

## 9. Development Phases

### Phase 1: Foundation (Months 1-3)

#### 1.1 Agent Core (Months 1-1.5) ✅
- [x] Config management (YAML/TOML, remote config via NATS KV)
- [x] Identity management (self-generated ECDSA P256 certs, UUID agent IDs)
- [x] Local persistence (BBolt store with metrics/events/state/queue buckets)
- [x] Agent lifecycle (start/stop, health API, state machine)
- [x] Logger abstraction (zap-backed, structured logging)
- [x] System collector: CPU, RAM, disk, net, load, host info via gopsutil v3

#### 1.2 NATS JetStream Communication (Months 1.5-2) ✅
- [x] NATS connection manager (TLS/mTLS, reconnect with backoff, error handling)
- [x] Tenant subject isolation: `tenant.{id}.agent.{agent_id}` / `.cmd.` / `.events.` / `.heartbeat.`
- [x] Metrics batch publish with BBolt-queued replay on reconnect
- [x] Heartbeat loop (30s interval) with status reporting
- [x] Event publishing with store-and-forward
- [x] Command subscription helpers (Subscribe, QueueSubscribe, Request)

#### 1.3 Metrics Ingestion Pipeline (In Progress)
- [ ] NATS subscription service (platform-side consumer)
- [ ] TimescaleDB hypertable schema + migrations
- [ ] Metrics batch writer (hypertable insert with compression)
- [ ] Continuous aggregate views (1m, 1h downsampling)

#### 1.4 Remaining Foundation
- [ ] Tenant/Auth service + API Gateway (Kong/Traefik)
- [ ] PostgreSQL relational schema + RLS policies
- [ ] Inventory API + UI skeleton
- [ ] Agent service installer (systemd, Windows Service)

### Phase 2: Monitoring Core (Months 3-5)
- [ ] Metrics ingestion pipeline (NATS → TimescaleDB)
- [ ] Alerting engine (threshold, heartbeat)
- [ ] SNMP/ICMP collector (agentless)
- [ ] Network Probe (SNMP, discovery)
- [ ] Dashboard/visualization (Grafana or custom)

### Phase 3: Remote Access & Automation (Months 5-7)
- [ ] Tunnel infrastructure (WebSocket/NATS)
- [ ] RDP/SSH/VNC proxy
- [ ] Script execution framework
- [ ] Patch management (Windows + Linux)
- [ ] Software inventory + vulnerability correlation

### Phase 4: Advanced Features (Months 7-10)
- [ ] Anomaly detection (ML-based)
- [ ] NetFlow/sFlow ingestion
- [ ] IP Phone monitoring (SIP, vendor APIs)
- [ ] Synthetic monitoring
- [ ] Reporting engine

### Phase 5: Platform Hardening (Months 10-12)
- [ ] Self-hosted distribution (Helm/KOTS)
- [ ] Air-gapped install
- [ ] Multi-region SaaS
- [ ] Performance/load testing
- [ ] Security audit + penetration test
- [ ] Documentation + runbooks

---

## 10. Technical Decisions Summary

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Agent Language | Go | Cross-compile, single binary, performance, NATS client |
| Platform Services | Polyglot | Best tool per domain (Go for data plane, Python for ML, etc.) |
| Message Bus | NATS JetStream | Lightweight, streaming, KV, multi-tenant subjects |
| Time-Series DB | TimescaleDB | PostgreSQL-compatible, compression, continuous aggregates |
| Relational DB | PostgreSQL | RLS, JSONB, mature, multi-tenant patterns |
| Cache | Redis | Sessions, rate limits, distributed locks |
| API Gateway | Kong/Traefik | Plugins, TLS, rate limiting, WebSocket |
| Auth | Keycloak/OIDC | Standards-based, federation, MFA |
| Container Orch | Kubernetes | Elastic scaling, self-healing, GitOps |
| Self-Hosted Distro | Helm + KOTS | Enterprise-grade, license management, air-gap |

---

## 11. Open Questions / Risks

1. **Agent Auto-Update Security**: Supply chain attack surface → Sigstore/cosign verification, staged rollouts
2. **TimescaleDB Scaling**: At 1M+ endpoints, consider distributed hypertables or VictoriaMetrics
3. **NATS JetStream at Scale**: Cluster sizing, subject partitioning strategy
4. **Remote Access Compliance**: SOC2, HIPAA requirements for session recording
5. **Self-Hosted Support Burden**: Version skew, customer environment variability
6. **IP Phone Vendor APIs**: Proprietary, may need reverse engineering or partnerships

---

## 12. Next Steps

1. **Validate**: Review this plan with stakeholders
2. **Prototype**: Build agent core + NATS comms + TimescaleDB ingest (2 weeks)
3. **Decide**: Finalize self-hosted distribution method (KOTS vs custom)
4. **Staff**: Define team structure (Platform, Agent, UI, DevOps, Security)
5. **Budget**: Infrastructure costs at scale (SaaS multi-region)
