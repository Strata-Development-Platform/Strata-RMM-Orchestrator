# Restore Procedures

## Overview

Restore operations reverse the backup process, recovering PostgreSQL/TimescaleDB, NATS JetStream, and object storage from encrypted backup data.

## Prerequisites

Before restoring:

1. **Identify the backup ID** from `backup_records`:
   ```sql
   SELECT id, database_type, created_at, status, integrity_digest
   FROM backup_records
   WHERE status = 'completed'
   ORDER BY created_at DESC
   LIMIT 10;
   ```

2. **Verify backup integrity** using the stored SHA-256 digest:
   ```bash
   strata-rmm orchestrator recovery <backup-id> --dry-run
   ```

3. **Ensure target DSN** is available if restoring to a different instance:
   ```bash
   export STRATA_BACKUP_TARGET_DSN="postgres://user:pass@target-host:5432/strata_rmm"
   ```

## Restore Workflow

### Step 1: Pre-Restore Validation

The coordinator checks:
- Backup ID exists in `backup_records`
- Backup status is `completed`
- Advisory lock can be acquired
- Target database is reachable

### Step 2: Quiesce

All services are quiesced to prevent new work during restore:
- Job workers stop processing
- NATS consumers pause
- API endpoints return maintenance status

### Step 3: Database Restore

1. Decrypt backup data using AES-256-GCM
2. Run `pg_restore` to the target database
3. Verify schema version matches expected
4. Validate row counts and data integrity

### Step 4: JetStream Restore

1. Recreate stream definitions from backup metadata
2. Recreate consumer configurations
3. Replay messages from backup data
4. Verify message counts match

### Step 5: Object Storage Restore

1. Restore object inventory metadata
2. Verify ETag checksums match
3. Restore object content from backup
4. Verify content-length matches

### Step 6: Post-Restore Validation

1. Run database health checks
2. Verify NATS connectivity
3. Test API endpoints
4. Validate tenant data isolation

## Restore Commands

### Full Restore

```bash
strata-rmm orchestrator recovery <backup-id>
```

### Dry-Run Validation

```bash
strata-rmm orchestrator recovery <backup-id> --dry-run --timeout 1h
```

### Force Restore (bypass some validations)

```bash
strata-rmm orchestrator recovery <backup-id> --force
```

### Restore with Custom Timeout

```bash
strata-rmm orchestrator recovery <backup-id> --timeout 3h
```

## Rollback

If restore fails at any phase, the system automatically rolls back:

```
Rollback → Cleanup → Completed (failure)
```

Rollback reverses:
1. Object storage changes
2. JetStream changes
3. Database changes

## Verification

After restore completes, verify:

### Database
```sql
SELECT COUNT(*) FROM tenants;
SELECT COUNT(*) FROM devices;
SELECT version FROM schema_migrations ORDER BY id DESC LIMIT 1;
```

### JetStream
```bash
nats --server=nats://localhost:4222 stream info STRATA_JOURNALS
nats --server=nats://localhost:4222 consumer info STRATA_JOURNALS default
```

### API
```bash
curl -s http://localhost:8080/health | jq .
```

### Audit Log
```sql
SELECT * FROM backup_audit_log
WHERE action IN ('restore_started', 'restore_completed')
ORDER BY timestamp DESC;
```

## RPO/RTO Reporting

The recovery coordinator reports RPO and RTO metrics:

```
Recovery ID: <uuid>
Final State: Completed
RPO Data Loss Window: 5m23s (within 15m target)
RTO Total Recovery Time: 1h42m (within 4h target)
```
