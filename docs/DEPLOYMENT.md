# Deployment Guide (Phase 8B)

## Prerequisites

| Component | Version | Notes |
|-----------|---------|-------|
| Go | 1.25.x | Build toolchain for source installs |
| PostgreSQL | 16.x | Relational database |
| TimescaleDB | 2.28.x | Time-series extension (installed as PostgreSQL extension) |
| NATS | 2.10+ | Messaging backbone with JetStream |

### Optional Dependencies

| Component | Version | Purpose |
|-----------|---------|---------|
| MinIO | latest | Self-hosted S3-compatible object storage for recordings |
| Redis | 7.x | Session cache (future) |
| Grafana | 11.x | Observability dashboards |

### Network Requirements

- Port 8080 (orchestrator API, internal)
- Port 8443 (tunnel gateway, agent-facing)
- Port 4222 (NATS client, agent-facing)
- Port 5432 (PostgreSQL, internal)

---

## Authoritative Deployment Path

Two supported methods — choose **one**:

### A. Binary + systemd (bare metal / VM — production)

1. Download the release binary or build from source.
2. Place the binary at `/usr/local/bin/strata-rmm`.
3. Create system user, config directory, data directory.
4. Deploy systemd unit files from `deploy/`.
5. Set environment variables (see Configuration).
6. Start the orchestrator.

### B. Docker Compose (single-host / staging)

Use `deploy/docker/docker-compose.yml`:

```bash
docker compose -f deploy/docker/docker-compose.yml up -d
```

For production, override with a `.env` file or environment variables.

---

## Configuration

All configuration is via environment variables. See `docs/CONFIGURATION.md` for the complete inventory.

### Production Example

```bash
# Mode
STRATA_RUNTIME_MODE=production

# NATS
NATS_URL=nats://nats.example.com:4222
NATS_TOKEN=<secure-token>

# Database
TIMESCALE_DSN=postgres://strata_rmm_app:<password>@db.example.com:5432/strata_rmm?sslmode=require

# API
STRATA_API_ADDR=:8080
STRATA_TUNNEL_ADDR=:8443
STRATA_PUBLIC_URL=https://rmm.example.com
CORS_ORIGINS=https://rmm.example.com

# JWT
JWT_SECRET=<64-char-hex-secret>

# Storage
STORAGE_BACKEND=s3
STORAGE_BUCKET=strata-recordings-prod
STORAGE_REGION=us-east-1

# Performance
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=10
DB_CONN_MAX_LIFETIME=10m
```

### Agent Example (`agent.yaml`)

```yaml
agent:
  tenant_id: "<uuid>"
  log_level: info
  data_dir: /var/lib/strata-rmm
nats:
  urls:
    - nats://nats.example.com:4222
  token: "<secure-token>"
collect:
  interval: 60s
  enable_system: true
  enable_hardware: true
  enable_software: true
  enable_network: true
  enable_services: true
store:
  type: bbolt
  path: /var/lib/strata-rmm/agent.db
update:
  enabled: true
  check_interval: 24h
  channel: stable
```

---

## Preflight Validation

Before deploying, run the `preflight` subcommand to validate configuration, database, and NATS connectivity:

```bash
# Validate configuration, database, and NATS
strata-rmm orchestrator preflight

# Check NATS connectivity
nats ping -s nats://localhost:4222

# Check database connectivity
psql "$TIMESCALE_DSN" -c "SELECT extversion FROM pg_extension WHERE extname='timescaledb';"

# Verify binary integrity
sha256sum /usr/local/bin/strata-rmm
```

The orchestrator's `preflight` subcommand performs:

1. Load and validate configuration via `LoadOrchestratorConfig()`
2. Run `Validate()` — mode-independent requirement validation
3. Run `ProductionValidate()` — production policy enforcement (when `STRATA_RUNTIME_MODE=production`)
4. Ping the database to verify connectivity
5. Connect to NATS to verify messaging connectivity

---

## Clean Installation Steps

### 1. System Preparation

```bash
# Create system user
sudo useradd -r -s /bin/false strata-rmm

# Create directories
sudo mkdir -p /var/lib/strata-rmm /etc/strata-rmm
sudo chown strata-rmm:strata-rmm /var/lib/strata-rmm /etc/strata-rmm
sudo chmod 700 /var/lib/strata-rmm
```

### 2. Deploy Binary

```bash
sudo cp strata-rmm /usr/local/bin/strata-rmm
sudo chmod 755 /usr/local/bin/strata-rmm
```

### 3. Set Up Database

```sql
CREATE USER strata_rmm_app WITH LOGIN PASSWORD '<strong-password>';
CREATE DATABASE strata_rmm OWNER strata_rmm_app;
GRANT ALL PRIVILEGES ON DATABASE strata_rmm TO strata_rmm_app;

-- Connect to strata_rmm
\c strata_rmm
CREATE EXTENSION IF NOT EXISTS timescaledb;
GRANT ALL ON SCHEMA public TO strata_rmm_app;
```

### 4. Configure Environment

Create `/etc/strata-rmm/orchestrator.env`:

```bash
STRATA_RUNTIME_MODE=production
NATS_URL=nats://localhost:4222
NATS_TOKEN=<token>
TIMESCALE_DSN=postgres://strata_rmm_app:<password>@localhost:5432/strata_rmm?sslmode=require
STRATA_API_ADDR=:8080
STRATA_TUNNEL_ADDR=:8443
STRATA_PUBLIC_URL=https://rmm.example.com
CORS_ORIGINS=https://rmm.example.com
JWT_SECRET=<64-char-secret>
```

### 5. Deploy Systemd Service

```bash
sudo cp deploy/strata-rmm-agent.service /etc/systemd/system/strata-rmm.service
sudo systemctl daemon-reload
sudo systemctl enable strata-rmm
sudo systemctl start strata-rmm
```

### 6. Apply Migrations

```bash
sudo strata-rmm orchestrator --apply-migrations
```

### 7. Verify

```bash
curl http://localhost:8080/health
# Expected: {"status":"ok","ready":"true"}
```

---

## Same-Version Reapplication (Idempotent)

Reapplying the same release produces no duplicate resources or destructive drift.

### Binary Reapplication

```bash
# Replace binary (atomic copy)
sudo cp strata-rmm /usr/local/bin/strata-rmm.tmp
sudo mv /usr/local/bin/strata-rmm.tmp /usr/local/bin/strata-rmm

# Restart service
sudo systemctl restart strata-rmm

# Verify no migration re-execution
journalctl -u strata-rmm -n 20 | grep -i migration
# Expected: "migrations already applied, skipping"
```

### Docker Reapplication

```bash
docker compose -f deploy/docker/docker-compose.yml pull orchestrator
docker compose -f deploy/docker/docker-compose.yml up -d --no-deps orchestrator
```

### Idempotency Guarantees

- **Migrations**: Each migration has a unique ID recorded in `schema_migrations`. Reapplying skips already-applied migrations.
- **NATS streams**: Created with `max_msgs` / `max_age` — re-declaring the same config is a no-op.
- **Systemd units**: Deploying the same unit file overwrites without duplication.
- **Data directories**: Pre-existing directories are reused.

---

## Health Verification

### Liveness

```bash
curl http://localhost:8080/health?liveness=1
# {"status":"alive"}
```

### Readiness

```bash
curl http://localhost:8080/health
# {"status":"ok","ready":"true"}
```

### Full Diagnostic

```bash
curl http://localhost:8080/health?mode=full
# Includes: DB status, NATS status, migrations, storage, JetStream
```

---

## Troubleshooting

| Symptom | Likely Cause | Action |
|---------|-------------|--------|
| Service fails to start | Missing `JWT_SECRET` or invalid DSN | Check `journalctl -u strata-rmm -n 50` |
| NATS connection refused | NATS not running or wrong URL | `systemctl status nats`; verify URL |
| Database connection failed | Wrong password or SSL mode | Verify `TIMESCALE_DSN`, check `sslmode` |
| Migrations not applied | DB user lacks DDL permissions | Use owner role for migrations |
| Health returns 503 | One or more dependencies unhealthy | Check `?mode=full` diagnostic |
| CORS errors in browser | `CORS_ORIGINS` mismatch | Verify origin matches exactly |
| Agents not connecting | NATS URL or token mismatch | Regenerate enrollment token |
| Tunnel connections fail | Firewall blocking port 8443 | Check security group / iptables |
