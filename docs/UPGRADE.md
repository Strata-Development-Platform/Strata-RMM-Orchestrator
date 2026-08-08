# Strata RMM — Upgrade Reference

**Version:** 2026-08-08
**Last Updated: 2026-08-08

---

## 1. Upgrade Overview

Strata supports zero-downtime upgrades with automatic schema migrations. Schema changes run during application startup.

---

## 2. Upgrade Paths

| From | To | Supported | Notes |
|------|----|-----------|-------|
| v1.0.x | v1.1.x | ✅ Yes | In-place upgrade |
| v1.0.x | v2.0.x | ✅ Yes | Requires manual review |

---

## 3. Upgrade Procedure

### 3.1 Helm Upgrade

```bash
# Check for new version
helm search repo strata

# Backup database
./strata backup run

# Upgrade
helm upgrade strata deploy/helm/strata/ -f values.yaml

# Verify
kubectl get pods -l app=strata
kubectl logs -l app=strata --tail=100
```

### 3.2 Docker Upgrade

```bash
# Backup database
./strata backup run

# Pull new image
docker pull strata-rmm/orchestrator:latest

# Restart
docker-compose up -d --force-recreate --no-deps orchestrator

# Verify
docker logs orchestrator --tail=100
```

### 3.3 Systemd Upgrade

```bash
# Backup database
./strata backup run

# Replace binary
cp /opt/strata/strata-v1.1.0 /opt/strata/strata
chmod +x /opt/strata/strata

# Restart service
systemctl restart strata

# Verify
systemctl status strata
journalctl -u strata --tail=100
```

---

## 4. Schema Migrations

### 4.1 Automatic Migration

Schema migrations run automatically on startup:

```go
// pkg/postgres/upgrade.go
func Upgrade(ctx context.Context, db *sql.DB) error {
    // Run pending migrations in order
    // Rollback on failure
}
```

### 4.2 Migration Tracking

Migrations are tracked in `schema_versions` table:

```sql
CREATE TABLE schema_versions (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 4.3 Manual Migration

If automatic migration fails:

```bash
# Check pending migrations
psql -U strata -d strata_rmm -c "SELECT * FROM schema_versions ORDER BY version DESC;"

# Run migration manually
psql -U strata -d strata_rmm -f migrations/00XXX.up.sql

# Verify
psql -U strata -d strata_rmm -c "SELECT * FROM schema_versions ORDER BY version DESC;"
```

---

## 5. Pre-Upgrade Checklist

- [ ] Backup database
- [ ] Review migration notes
- [ ] Test upgrade in staging
- [ ] Notify users of maintenance window
- [ ] Verify rollback plan

---

## 6. Post-Upgrade Verification

### 6.1 Health Check

```bash
# Check health
curl https://strata.example.com/health

# Check metrics
curl https://strata.example.com/metrics
```

### 6.2 Data Integrity

```bash
# Verify tenant data
psql -U strata -d strata_rmm -c "SELECT count(*) FROM tenants;"
psql -U strata -d strata_rmm -c "SELECT count(*) FROM devices;"

# Verify migration status
psql -U strata -d strata_rmm -c "SELECT * FROM schema_versions ORDER BY version DESC;"
```

### 6.3 Service Connectivity

```bash
# Check NATS
nats-top -c nats://localhost:4222

# Check PostgreSQL
psql -U strata -d strata_rmm -c "SELECT 1;"
```

---

## 7. Upgrade Notes

### 7.1 Breaking Changes

| Version | Change | Action Required |
|---------|--------|-----------------|
| v1.1.0 | JWT secret min length 32 chars | Update if using shorter secret |
| v1.1.0 | Production TLS required | Enable NATS TLS |

### 7.2 Deprecations

| Version | Deprecated | Replaced By |
|---------|------------|-------------|
| v1.2.0 | (planned) | (planned) |

---

## 8. Rollback

If upgrade fails:

1. **Stop** application
2. **Restore** from backup
3. **Reapply** schema migrations up to target version
4. **Restart** application

---

*Last Updated: 2026-08-08*
