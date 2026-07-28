# Rollback Procedures (Phase 8B)

## When to Rollback

Initiate a rollback when any of the following occur after a deployment or upgrade:

| Condition | Severity | Action |
|-----------|----------|--------|
| Health endpoint returns non-200 for >2 consecutive checks | Critical | Immediate rollback |
| Smoke test suite reports any FAIL | High | Rollback within 15 min |
| Agent connectivity drops >20% of fleet | High | Rollback within 30 min |
| Alert engine fails to load rules | Medium | Rollback within 1 hour |
| Database migration errors | Critical | Halt deployment, rollback immediately |
| NATS stream loss or corruption | Critical | Rollback and restore from backup |
| Performance regression >50% latency increase | Medium | Investigate, rollback if confirmed |

---

## Rollback Procedure

### Step 1: Stop Candidate

```bash
# Binary
sudo systemctl stop strata-rmm

# Docker
docker compose -f deploy/docker/docker-compose.yml stop orchestrator
```

### Step 2: Restore Previous Binary / Image

```bash
# Binary
sudo mv /usr/local/bin/strata-rmm.pre-upgrade /usr/local/bin/strata-rmm

# If no backup binary exists, redeploy from known-good artifact
# wget https://github.com/.../releases/download/v0.2.0-beta/strata-rmm-linux-amd64

# Docker — pin the previous image tag in docker-compose.override.yml
```

### Step 3: Restore Previous Configuration

Production configuration must be preserved — restore the known-good config from backup:

```bash
# Restore config files
sudo cp /backups/strata-rmm-config-pre-upgrade/* /etc/strata-rmm/

# Restore environment file
sudo cp /backups/strata-rmm-config-pre-upgrade/orchestrator.env /etc/strata-rmm/orchestrator.env
```

### Step 4: Verify Schema Compatibility

```bash
# Run preflight to verify config, database, and NATS connectivity
sudo /usr/local/bin/strata-rmm orchestrator preflight

# Verify migration state is consistent with the restored binary
psql -U strata_rmm_app -d strata_rmm -c "SELECT MAX(id) FROM schema_migrations;"
```

### Step 5: Start and Wait for Readiness

```bash
# Start service
sudo systemctl start strata-rmm

# Wait for readiness (retry up to 30 seconds)
for i in $(seq 1 30); do
  if curl -sf http://localhost:8080/health | jq -e '.ready == true' > /dev/null 2>&1; then
    echo "READY"
    break
  fi
  echo "waiting... ($i)"
  sleep 1
done

# Run smoke test
./scripts/smoke_test.sh

# Check agents reconnected
curl http://localhost:8080/api/v1/overview | jq '.online_devices'
```

---

## Data Preservation During Rollback

| Data | Preserved? | Mechanism |
|------|-----------|-----------|
| All tenant/device/metric data | Yes | Database unchanged; only binary rollback |
| Migration state | Conditional | Reversed only if rolling back schema changes |
| Session recordings | Yes | Object storage unaffected by code rollback |
| Audit log | Yes | Append-only table preserved |
| Alert history | Yes | Stored in database, unchanged |
| Job queue | Yes | NATS JetStream persists across restarts |
| CVE cache | Yes | Consider re-sync if schema changed |

**Important**: Configuration secrets (JWT_SECRET, NATS_TOKEN) must match between the restored config and running state. If secrets were rotated as part of the upgrade, the rollback must restore the old secrets.

---

## Migration Compatibility

### Additive Migrations (Safe to rollback)

- `CREATE TABLE` — simply drop the table
- `ADD COLUMN` — existing code ignores unknown columns
- `CREATE INDEX` — index persists but unused by old code; safe

### Destructive Migrations (Require backup restore)

- `DROP TABLE` — full schema restore from backup
- `ALTER COLUMN TYPE` — full restore if data reformatted
- `RENAME COLUMN` — full restore if old code references old name

### Rollback SQL Examples

```sql
-- Revert migration 25 (additive)
DROP TABLE IF EXISTS phase_8b_feature CASCADE;

-- Revert migration 26 (additive column)
ALTER TABLE devices DROP COLUMN IF EXISTS new_column;

-- Remove migration records
DELETE FROM schema_migrations WHERE id IN (25, 26);

-- Verify
SELECT id, name, applied_at FROM schema_migrations ORDER BY id;
```

---

## Emergency Rollback Procedure

Use this when the standard rollback fails or when the system is in a degraded state that prevents normal operation.

### Emergency: Binary won't start

```bash
# 1. Force stop
sudo systemctl kill -s SIGKILL strata-rmm 2>/dev/null || true

# 2. Remove suspect binary
sudo rm -f /usr/local/bin/strata-rmm

# 3. Restore backup binary
sudo cp /backups/strata-rmm.pre-upgrade /usr/local/bin/strata-rmm

# 4. Clear any stale PID/lock files
sudo rm -f /var/lib/strata-rmm/*.lock /tmp/strata-rmm-*.pid 2>/dev/null || true

# 5. Verify configuration and start
sudo /usr/local/bin/strata-rmm orchestrator preflight && sudo systemctl start strata-rmm
```

### Emergency: Database migration half-applied

```bash
# 1. Check migration state
psql -U strata -d strata_rmm -c "SELECT * FROM schema_migrations ORDER BY id;"

# 2. Rollback partially-applied migration
psql -U strata -d strata_rmm -c "DROP TABLE IF EXISTS partially_applied_table CASCADE;"
psql -U strata -d strata_rmm -c "DELETE FROM schema_migrations WHERE id = 25;"

# 3. Restore from backup if data corrupted
pg_restore -h localhost -U strata -d strata_rmm --clean --if-exists \
  /backups/pre-upgrade-*.dump
```

### Emergency: NATS data loss

```bash
# 1. Stop all services
sudo systemctl stop strata-rmm nats

# 2. Restore NATS JetStream data
sudo cp -a /backups/nats-data/* /var/lib/nats/

# 3. Restart services
sudo systemctl start nats
sudo systemctl start strata-rmm

# 4. Verify streams
nats stream list
nats stream report
```

### Emergency: Full system restore from backup

```bash
# 1. Stop all services
docker compose -f deploy/docker/docker-compose.yml down

# 2. Restore database
pg_restore -h localhost -U strata -d strata_rmm --clean --if-exists \
  --jobs=4 /backups/pre-upgrade-*.dump

# 3. Restore configuration
cp /backups/strata-rmm-config-pre-upgrade/* /etc/strata-rmm/

# 4. Restore previous Docker image
docker load -i /backups/strata-rmm-image-v0.2.0-beta.tar

# 5. Restart stack
docker compose -f deploy/docker/docker-compose.yml up -d

# 6. Verify
curl http://localhost:8080/health
```

---

## Verification Matrix

| Check | Command | Expected |
|-------|---------|----------|
| Health | `curl localhost:8080/health` | `{"status":"ok"}` |
| Migrations | `psql -c "SELECT MAX(id) FROM schema_migrations"` | Matches pre-rollback |
| Agents | `curl localhost:8080/api/v1/overview` | Device count matches |
| NATS | `curl localhost:8222/connz` | Connections > 0 |
| Storage | `curl localhost:8080/health?mode=full` | Storage backend OK |
| Smoke | `./scripts/smoke_test.sh` | All PASS |
