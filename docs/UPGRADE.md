# Upgrade Procedure (Phase 8B)

## Supported Upgrade Paths

| From | To | Migration Required |
|------|----|-------------------|
| v0.1.x | v0.2.0-beta | Yes |
| v0.2.0-beta (same minor) | v0.2.0-beta (newer build) | No (if schema unchanged) |

Unsupported (full reinstall required):
- v0.0.x (pre-alpha) → any
- Cross-major without documented migration

## Migration Locking

When an upgrade involves database schema changes, the orchestrator acquires a **migration lock**:

1. A row is inserted into `schema_migrations_lock` with the migration ID.
2. If another process attempts to apply the same migration, it blocks or fails.
3. The lock is released after the migration completes (success or rollback).
4. A failed migration prevents the orchestrator from starting.

```sql
-- Migration lock table (auto-managed)
SELECT * FROM schema_migrations_lock;
```

---

## Upgrade Steps

### 1. Backup

```bash
# Full database backup
pg_dump -h localhost -U strata_rmm_app -d strata_rmm \
  --format=custom --compress=9 \
  --file=/backups/pre-upgrade-$(date +%Y%m%d-%H%M%S).dump

# Backup configuration
cp -r /etc/strata-rmm /backups/strata-rmm-config-pre-upgrade/

# Backup binary
cp /usr/local/bin/strata-rmm /usr/local/bin/strata-rmm.pre-upgrade
```

### 2. Pull New Binary / Image

**Binary:**

```bash
# Download release
wget https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/download/v0.2.0-beta/strata-rmm-linux-amd64
sudo mv strata-rmm-linux-amd64 /usr/local/bin/strata-rmm
sudo chmod 755 /usr/local/bin/strata-rmm
```

**Docker:**

```bash
docker compose -f deploy/docker/docker-compose.yml pull orchestrator
```

### 3. Run Preflight

```bash
# Validate new binary
/usr/local/bin/strata-rmm orchestrator --validate-config

# Check schema compatibility
/usr/local/bin/strata-rmm orchestrator --check-migrations

# Dry-run migrations
/usr/local/bin/strata-rmm orchestrator --dry-run-migrations
```

### 4. Apply Migration

```bash
# Apply pending migrations
sudo /usr/local/bin/strata-rmm orchestrator --apply-migrations
```

Expected output:

```
2026/07/28 10:00:00 INFO applying migration id=25 name="phase-8b"
2026/07/28 10:00:01 INFO migration applied id=25
2026/07/28 10:00:01 INFO all migrations applied (current: 25)
```

### 5. Start New Version

**Binary:**

```bash
sudo systemctl restart strata-rmm
```

**Docker:**

```bash
docker compose -f deploy/docker/docker-compose.yml up -d --no-deps orchestrator
```

### 6. Verify Health

```bash
# Check health endpoint
curl http://localhost:8080/health
# {"status":"ok","ready":"true","version":"v0.2.0-beta"}

# Run smoke tests
./scripts/smoke_test.sh

# Verify migration state
psql -U strata_rmm_app -d strata_rmm -c "SELECT MAX(id), COUNT(*) FROM schema_migrations;"

# Check agents reconnected
curl http://localhost:8080/api/v1/overview | jq '.online_devices'
```

---

## Rollback During Upgrade

If the upgrade fails at any step:

### Before migration applied

```bash
# Stop the new version
sudo systemctl stop strata-rmm

# Restore previous binary
sudo mv /usr/local/bin/strata-rmm.pre-upgrade /usr/local/bin/strata-rmm

# Restart
sudo systemctl start strata-rmm

# Verify
curl http://localhost:8080/health
```

### After migration applied

```bash
# 1. Stop the new version
sudo systemctl stop strata-rmm

# 2. Restore previous binary
sudo mv /usr/local/bin/strata-rmm.pre-upgrade /usr/local/bin/strata-rmm

# 3. Revert database migrations (run each in reverse order)
psql -U strata_rmm_app -d strata_rmm -c "DROP TABLE IF EXISTS phase_8b_tables CASCADE;"
psql -U strata_rmm_app -d strata_rmm -c "DELETE FROM schema_migrations WHERE id IN (25,26);"

# 4. Restore previous configuration
cp /backups/strata-rmm-config-pre-upgrade/* /etc/strata-rmm/

# 5. Start previous version
sudo systemctl start strata-rmm

# 6. Verify
curl http://localhost:8080/health
```

### Docker rollback

```bash
# Tag current image
docker tag strata-rmm-orchestrator:latest strata-rmm-orchestrator:failed-upgrade

# Set image to previous version in docker-compose.override.yml
docker compose -f deploy/docker/docker-compose.yml up -d --no-deps orchestrator
```

---

## Data Preservation Guarantees

| Data Type | Preserved During Upgrade | Preserved During Rollback |
|-----------|------------------------|--------------------------|
| Tenant records | Yes | Yes |
| Device records | Yes | Yes |
| Metrics/timeseries | Yes | Yes |
| Audit log | Yes | Yes |
| Session recordings | Yes | Yes |
| Alert history | Yes | Yes |
| Job queue | Yes (drained before upgrade) | Yes (re-queued) |
| Agent enrollment tokens | Yes | Yes |
| CVE database | Yes | Yes |

### What is NOT preserved

- In-flight NATS messages not yet consumed (re-published by agents on reconnect)
- Ephemeral cache state (e.g., in-memory rate limiter counters)

---

## Migration Compatibility

- All migrations are **additive** (CREATE TABLE, ADD COLUMN, CREATE INDEX).
- No destructive DDL (DROP, ALTER COLUMN TYPE) is included in supported upgrade paths.
- Old agent binaries remain compatible with the new orchestrator (backward-compatible NATS protocol).
- New agent binaries require the orchestrator at v0.2.0-beta or later.
