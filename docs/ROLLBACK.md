# Strata RMM — Rollback Reference

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Rollback Overview

Rollback reverts a Strata deployment to a previous version. Two types:

| Type | Scope | Description |
|------|-------|-------------|
| Application rollback | Orchestrator binary | Revert to previous version |
| Schema rollback | PostgreSQL/TimescaleDB | Revert database schema |

---

## 2. Application Rollback

### 2.1 Helm Rollback

```bash
# List releases
helm history strata

# Rollback to previous revision
helm rollback strata 1

# Rollback with specific revision
helm rollback strata 1
```

### 2.2 Docker Rollback

```bash
# Pull previous image
docker pull strata-rmm/orchestrator:v1.0.0

# Restart with previous version
docker-compose up -d --force-recreate --no-deps orchestrator
```

### 2.3 Systemd Rollback

```bash
# Stop service
systemctl stop strata

# Replace binary
cp /opt/strata/strata-v1.0.0 /opt/strata/strata

# Start service
systemctl start strata

# Verify
systemctl status strata
```

---

## 3. Schema Rollback

### 3.1 Migration System

Schema migrations are tracked in `pkg/postgres/schema.go`:

| Table | Description |
|-------|-------------|
| `schema_versions` | Migration version history |
| `deployment_ids` | Deployment tracking |

### 3.2 Rollback Procedure

```bash
# 1. Stop application
systemctl stop strata

# 2. Run schema downgrade
# (Migrations have .down.sql files)
psql -U strata -d strata_rmm -f /path/to/migrations/00XXX.down.sql

# 3. Verify schema
psql -U strata -d strata_rmm -c "SELECT * FROM schema_versions ORDER BY version DESC;"

# 4. Start application
systemctl start strata
```

### 3.3 Schema Downgrade

Each migration has corresponding `.down.sql` files:

```
migrations/
├── 00001_initial.up.sql
├── 00001_initial.down.sql
├── 00002_second.up.sql
├── 00002_second.down.sql
└── ...
```

---

## 4. Policy Rollback

Policies support rollback via the API:

```bash
# Rollback a policy
curl -X POST https://strata.example.com/api/v1/policies/{policyID}/rollback \
  -H "Authorization: Bearer {token}"
```

This creates a new revision and reverts to the previous state.

---

## 5. Patch Rollback

Patch deployments support automatic rollback on canary failure:

```bash
# Canary deployment with automatic rollback
# See: internal/patch/executor_extended.go
```

---

## 6. Agent Rollback

Agent updates support automatic rollback:

1. Download new version
2. Verify signature
3. Install
4. **If verification fails:** Automatic rollback to previous version

---

## 7. Post-Rollback Verification

### 7.1 Health Check

```bash
# Check health
curl https://strata.example.com/health

# Check metrics
curl https://strata.example.com/metrics
```

### 7.2 Data Integrity

```bash
# Verify tenant data
psql -U strata -d strata_rmm -c "SELECT count(*) FROM tenants;"

# Verify device data
psql -U strata -d strata_rmm -c "SELECT count(*) FROM devices;"
```

### 7.3 Service Connectivity

```bash
# Check NATS
nats-top -c nats://localhost:4222

# Check PostgreSQL
psql -U strata -d strata_rmm -c "SELECT 1;"
```

---

## 8. Emergency Rollback

If rollback fails:

1. **Stop** all services
2. **Restore** from latest backup
3. **Reapply** schema migrations up to target version
4. **Restart** services
5. **Verify** data integrity

---

## 9. Limitations

- **Schema rollback:** Not all migrations support downgrade
- **Data loss:** Rollback may lose data created after target version
- **Migration order:** Must rollback in reverse order
- **External dependencies:** NATS, Redis, object storage must be compatible

---

*Last Updated: 2026-08-08*
