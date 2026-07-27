# Deployment

## Production Topology

```
                         ┌─────────────┐
                         │   Clients   │
                         │ (Web UI /   │
                         │  API)       │
                         └──────┬──────┘
                                │ HTTPS (443)
                                ▼
                        ┌───────────────┐
                        │    nginx      │
                        │  (TLS term,   │
                        │   rate limit, │
                        │   reverse     │
                        │   proxy)      │
                        └───────┬───────┘
                                │ HTTP (8080)
                                ▼
                     ┌───────────────────┐
                     │   RMM API         │
                     │   (Orchestrator)  │
                     │   1+ replicas     │
                     └───┬───────┬───────┘
                         │       │
                ┌────────┘       └────────┐
                ▼                          ▼
        ┌───────────────┐         ┌──────────────────┐
        │    NATS       │         │  PostgreSQL 16 +  │
        │  JetStream    │◄───────►│  TimescaleDB 2.x  │
        │  2.10+        │         │  (relational +    │
        │  (cluster)    │         │   metrics)        │
        └───────┬───────┘         └──────────────────┘
                │
                │ outbound NATS (4222)
                ▼
        ┌───────────────────┐
        │   Agents          │
        │   (Go binary)     │
        │   (Windows/Linux) │
        └───────────────────┘
```

### Optional Components

```
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│   MinIO /    │    │   Redis      │    │   Grafana    │
│   S3         │    │   (cache/    │    │   (dash-     │
│   (object    │    │    sessions) │    │    boards)   │
│    store)    │    └──────────────┘    └──────────────┘
└──────────────┘
```

### Port Layout

| Service | Port | Protocol | Purpose |
|---------|------|----------|---------|
| nginx | 443 | HTTPS | TLS termination, reverse proxy |
| nginx | 80 | HTTP | Redirect to HTTPS |
| Orchestrator | 8080 | HTTP | Internal REST API (northbound) |
| Orchestrator | 8443 | TCP | Tunnel gateway (southbound) |
| NATS | 4222 | TCP | Client connections (agents, services) |
| NATS | 8222 | HTTP | Monitoring dashboard |
| PostgreSQL | 5432 | TCP | Database connections |
| MinIO | 9000 | HTTP | Object storage API |
| MinIO | 9001 | HTTP | Admin console |
| Grafana | 3000 | HTTP | Dashboard UI |

---

## Systemd Service Configuration

### Orchestrator Service (`/etc/systemd/system/strata-rmm.service`)

```ini
[Unit]
Description=Strata RMM Orchestrator
Documentation=https://strata-rmm.io/docs
After=network-online.target nats.service postgresql.service
Requires=nats.service postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=strata-rmm
Group=strata-rmm
ExecStart=/usr/local/bin/strata-rmm orchestrator \
  --nats-url nats://localhost:4222 \
  --timescale-dsn "postgres://strata:${POSTGRES_PASSWORD}@localhost:5432/strata_rmm?sslmode=require" \
  --api-addr :8080 \
  --tunnel-addr :8443 \
  --storage-backend ${STORAGE_BACKEND:-minio} \
  --storage-bucket ${STORAGE_BUCKET:-strata-recordings} \
  --storage-endpoint ${STORAGE_ENDPOINT:-localhost:9000}

# Security hardening
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
NoNewPrivileges=true
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE
ReadWritePaths=/var/lib/strata-rmm /var/log/strata-rmm

# Restart policy
Restart=always
RestartSec=10
StartLimitIntervalSec=60
StartLimitBurst=3

# Logging
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

### NATS Service (`/etc/systemd/system/nats.service`)

```ini
[Unit]
Description=NATS Server
Documentation=https://docs.nats.io/
After=network.target

[Service]
Type=simple
User=nobody
ExecStart=/usr/local/bin/nats-server -js -m 8222
Restart=always
RestartSec=5
LimitNOFILE=1000000

[Install]
WantedBy=multi-user.target
```

### Agent Service (`/etc/systemd/system/strata-rmm-agent.service`)

```ini
[Unit]
Description=Strata RMM Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/strata-rmm agent --config /etc/strata-rmm/agent.yaml
Restart=always
RestartSec=30
StartLimitIntervalSec=300
StartLimitBurst=5

[Install]
WantedBy=multi-user.target
```

---

## Environment Variables

### Orchestrator

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `STRATA_RMM_NATS_URL` | `nats://localhost:4222` | Yes | NATS server URL |
| `STRATA_RMM_TIMESCALE_DSN` | — | Yes | TimescaleDB connection string (`postgres://user:pass@host:5432/db?sslmode=require`) |
| `STRATA_RMM_API_ADDR` | `:8080` | No | API listen address |
| `STRATA_RMM_TUNNEL_ADDR` | `:8443` | No | Tunnel gateway listen address |
| `POSTGRES_PASSWORD` | — | Yes | Database password (injected via secret) |
| `JWT_SECRET` | — | Yes* | HMAC signing key (min 32 bytes) |
| `STRATA_ALLOW_LEGACY_DEPLOYMENT_ENROLLMENT` | `false` | No | Temporary compatibility escape hatch for reusable deployment-ID enrollment; do not enable in production |
| `STORAGE_BACKEND` | `local` | No | Storage type: `minio`, `s3`, `local`, `none` |
| `STORAGE_BUCKET` | `strata-recordings` | No | Storage bucket name |
| `STORAGE_ENDPOINT` | — | No | MinIO/S3 endpoint |
| `STORAGE_REGION` | — | No | AWS region (S3 only) |
| `STORAGE_ACCESS_KEY` | — | No | Storage access key |
| `STORAGE_SECRET_KEY` | — | No | Storage secret key |
| `STORAGE_USE_SSL` | `false` | No | Enable TLS for storage |
| `STORAGE_KMS_KEY_ID` | — | No | KMS key ID for SSE-KMS |
| `STRATA_RMM_LOG_LEVEL` | `info` | No | Log level: `debug`, `info`, `warn`, `error` |

*\* Required in production. Default dev secret must be changed.*

### Agent

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `STRATA_RMM_NATS_URL` | `nats://localhost:4222` | Yes | NATS server URL |
| `STRATA_RMM_DEPLOYMENT_ID` | — | Yes | Deployment/tenant identifier |
| `STRATA_RMM_ENROLLMENT_TOKEN` | — | Yes | Agent enrollment token |
| `STRATA_RMM_DATA_DIR` | `/var/lib/strata-rmm` | No | Agent data directory |
| `STRATA_RMM_LOG_LEVEL` | `info` | No | Log level |
| `STRATA_RMM_COLLECT_INTERVAL` | `60` | No | Metrics collection interval (seconds) |
| `STRATA_RMM_HEARTBEAT_INTERVAL` | `30` | No | Heartbeat interval (seconds) |

---

## Firewall Rules

### Inbound (Platform)

| Source | Dest Port | Protocol | Service | Purpose |
|--------|-----------|----------|---------|---------|
| Internet | 443 | TCP | nginx | HTTPS API + Web UI |
| Internet | 8443 | TCP | Orchestrator | Tunnel gateway (RDP/SSH/VNC) |
| Agents | 4222 | TCP | NATS | Agent NATS connections |
| Internal | 5432 | TCP | PostgreSQL | DB connections (internal only) |
| Internal | 8222 | TCP | NATS | NATS monitoring (admin only) |
| Internal | 9000 | TCP | MinIO | Storage API (internal only) |
| Internal | 9001 | TCP | MinIO | Admin console (internal only) |
| Internal | 3000 | TCP | Grafana | Dashboards (admin only) |

### Outbound (Platform)

| Dest | Port | Protocol | Service | Purpose |
|------|------|----------|---------|---------|
| GitHub | 443 | HTTPS | Orchestrator | Auto-update checks |
| OSV.dev | 443 | HTTPS | Orchestrator | CVE feed sync |
| NVD | 443 | HTTPS | Orchestrator | Optional CVE feed |
| SMTP server | 587/465 | TCP | Orchestrator | Email notifications |
| Slack/Teams | 443 | HTTPS | Orchestrator | Alert webhooks |

### Agent Outbound

| Dest | Port | Protocol | Purpose |
|------|------|----------|---------|
| NATS server | 4222 | TCP | NATS messaging |
| API server | 443/8080 | HTTPS | Registration + file downloads |

---

## Non-Owner PostgreSQL Role Usage

### Current State (Development)
- Database user: `strata` (owner)
- All DDL + DML operations executed as owner
- RLS policies defined but enforced only when `app.tenant_id` is set

### Production Requirement

Create a restricted role for application usage:

```sql
-- Create application role
CREATE ROLE strata_rmm_app WITH LOGIN PASSWORD '<strong-password>';
GRANT CONNECT ON DATABASE strata_rmm TO strata_rmm_app;
GRANT USAGE ON SCHEMA public TO strata_rmm_app;

-- Table-level grants (read/write on data tables)
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO strata_rmm_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO strata_rmm_app;

-- Revoke DDL permissions
REVOKE CREATE ON SCHEMA public FROM strata_rmm_app;

-- Future tables should also be accessible
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO strata_rmm_app;

-- RLS must be enforced for this role
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
-- ... (repeat for all tenant-scoped tables)

-- Set role in application connection
-- Connection string: postgres://strata_rmm_app:password@localhost:5432/strata_rmm?sslmode=require&application_name=strata-rmm
```

### Separation of Duties

| Operation | Role | Purpose |
|-----------|------|---------|
| `strata` (owner) | Schema migrations | DDL operations only during deployments |
| `strata_rmm_app` | Application | All DML during normal operation |
| `strata_rmm_readonly` | Reporting/analytics | SELECT-only access for read replicas |

---

## Backup Procedures

### Database (PostgreSQL + TimescaleDB)

```bash
# Full backup
pg_dump -h localhost -U strata_rmm_app -d strata_rmm \
  --format=custom \
  --compress=9 \
  --file=/backups/strata-rmm-$(date +%Y%m%d-%H%M%S).dump

# TimescaleDB backup (requires --format=custom for parallel restore support)
pg_dump -h localhost -U strata_rmm_app -d strata_rmm \
  --format=custom \
  --compress=9 \
  --file=/backups/strata-timescaledb-$(date +%Y%m%d-%H%M%S).dump
```

### Restoration

```bash
# Restore full backup
pg_restore -h localhost -U strata -d strata_rmm \
  --clean \
  --if-exists \
  --jobs=4 \
  /backups/strata-rmm-20260727-120000.dump

# Verify restoration
psql -U strata_rmm_app -d strata_rmm -c "SELECT COUNT(*) FROM devices;"
psql -U strata_rmm_app -d strata_rmm -c "SELECT COUNT(*) FROM metrics;"
```

### Object Storage (MinIO/S3)

```bash
# MinIO (mc client)
mc mirror /data/minio/strata-recordings s3/backup-bucket/strata-recordings/

# AWS S3 (cross-region replication or versioning)
aws s3 sync s3://strata-recordings s3://strata-recordings-backup/
```

### Schedule

| Backup | Frequency | Retention | Method |
|--------|-----------|-----------|--------|
| Database (full) | Daily | 30 days | pg_dump custom format |
| Database (WAL) | Continuous | 7 days | WAL archiving |
| Object store | Daily | 90 days | S3 sync / mc mirror |
| Configuration | On change | 90 days | Git-tracked |
| NATS streams | Daily | 7 days | `nats stream backup` |

### Automation (crontab)

```cron
# Daily database backup at 2 AM
0 2 * * * /usr/local/bin/backup-db.sh

# Daily object store sync at 3 AM
0 3 * * * /usr/local/bin/backup-storage.sh
```
