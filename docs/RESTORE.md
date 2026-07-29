# Phase 8C Restore Procedures

This document describes restore procedures for Phase 8C of the Strata RMM Orchestrator.

## Overview

Phase 8C implements comprehensive restore capabilities with:

- **Authenticated encryption verification**: AES-256-GCM before decryption
- **Integrity verification**: SHA-256 digest before restore
- **Multi-component support**: PostgreSQL, TimescaleDB, NATS JetStream, object storage
- **State machine-driven recovery**: 15-state recovery coordinator

## Prerequisites

- PostgreSQL 16+ with `pg_restore` available
- NATS JetStream 2.11+ (optional)
- Object storage (S3, GCS, Azure Blob, or local file system)
- Encryption key configured in `tenant_encryption_keys` table
- Valid backup ID from `backups` table

## Prerequisites Check

```bash
# Verify PostgreSQL connection
psql $STRATA_POSTGRES_DSN -c "SELECT 1"

# Verify encryption key exists
psql $STRATA_POSTGRES_DSN -c "SELECT id, status FROM tenant_encryption_keys WHERE status = 'active'"

# List available backups
curl http://localhost:8080/api/v1/backup \
  -H "Authorization: Bearer <token>"
```

## PostgreSQL/TimescaleDB Restore

### Full Database Restore

```bash
# Restore using the API
curl -X POST http://localhost:8080/api/v1/restore/database \
  -H "Authorization: Bearer <token>" \
  -d '{"backup_id": "backup_abc123"}'
```

### Verify Restore

```bash
# Check database connectivity
psql $STRATA_POSTGRES_DSN -c "SELECT COUNT(*) FROM some_table"

# Verify data integrity
psql $STRATA_POSTGRES_DSN -c "SELECT * FROM some_table WHERE id = 1"
```

### Manual Restore (if needed)

```bash
# Extract encrypted data from backup table
psql $STRATA_POSTGRES_DSN -c "SELECT data FROM backups WHERE id = 'backup_abc123'"

# Note: Manual restore requires decryption using encryption key
# This is handled automatically by the restore API
```

## NATS JetStream Restore

### Restore JetStream Configuration

```bash
# Restore JetStream streams and consumers
curl -X POST http://localhost:8080/api/v1/restore/jetstream \
  -H "Authorization: Bearer <token>" \
  -d '{"backup_id": "backup_abc123"}'
```

### Verify Restore

```bash
# List streams
curl http://localhost:8222/v1/stream \
  -H "Authorization: Bearer <token>"

# List consumers
curl http://localhost:8222/v1/consumers/STREAM_NAME \
  -H "Authorization: Bearer <token>"
```

## Object Storage Restore

### Restore Object Storage

```bash
# Restore objects from backup
curl -X POST http://localhost:8080/api/v1/restore/objectstorage \
  -H "Authorization: Bearer <token>" \
  -d '{"backup_id": "backup_abc123"}'
```

### Verify Restore

```bash
# List objects
curl http://localhost:8080/api/v1/objectstorage/objects \
  -H "Authorization: Bearer <token>"
```

## Full Disaster Recovery

### Automated Recovery

```bash
# Initiate full disaster recovery
curl -X POST http://localhost:8080/api/v1/recovery \
  -H "Authorization: Bearer <token>" \
  -d '{"backup_id": "backup_abc123", "components": ["database", "jetstream", "objectstorage"]}'

# Check recovery status
curl http://localhost:8080/api/v1/recovery/status \
  -H "Authorization: Bearer <token>"
```

### Recovery States

The recovery coordinator follows a 15-state state machine:

1. **Idle**: Initial state
2. **Discovery**: Discover backup locations
3. **PreFlight**: Execute pre-flight checks
4. **BackupDatabase**: Backup current database state
5. **BackupJetStream**: Backup current JetStream state
6. **BackupObjectStorage**: Backup current object storage state
7. **VerifyIntegrity**: Verify backup integrity
8. **RestoreDatabase**: Restore database from backup
9. **RestoreJetStream**: Restore JetStream from backup
10. **RestoreObjectStorage**: Restore object storage from backup
11. **PostRestoreValidation**: Validate restore results
12. **HealthCheck**: Verify system health
13. **Verification**: Final verification
14. **Rollback**: Rollback on failure
15. **Completed**: Recovery complete

## Encryption Verification

### Automatic Verification

Before restore, the system automatically:

1. Verify integrity digest (SHA-256)
2. Decrypt using encryption key (AES-256-GCM)
3. Validate decrypted data

### Manual Verification

```bash
# Extract encrypted data
psql $STRATA_POSTGRES_DSN -c "SELECT data, integrity_digest FROM backups WHERE id = 'backup_abc123'"

# Calculate SHA-256
echo "<encrypted_data_base64>" | base64 -d | sha256sum

# Compare with stored digest
# Must match exactly for restore to proceed
```

## Rollback Procedures

### Automatic Rollback

If restore fails, the coordinator automatically:

1. Detect failure
2. Transition to `Rollback` state
3. Attempt to restore previous state
4. Log failure details

### Manual Rollback

```bash
# Initiate rollback
curl -X POST http://localhost:8080/api/v1/recovery/rollback \
  -H "Authorization: Bearer <token>"
```

## RPO/RTO Metrics

### RPO (Recovery Point Objective)

- **Data Loss Window**: 1 hour (configurable)
- **Last Backup Time**: Timestamp of last successful backup
- **Max Acceptable RPO**: 24 hours

### RTO (Recovery Time Objective)

- **Total Recovery Time**: From start to completion
- **Phase Times**: Per-phase timing breakdown

### Get RPO/RTO Metrics

```bash
# Get current metrics
curl http://localhost:8080/api/v1/recovery/metrics \
  -H "Authorization: Bearer <token>"
```

## Troubleshooting

### Restore Failed

1. Verify backup exists:
   ```bash
   curl http://localhost:8080/api/v1/backup/backup_abc123 \
     -H "Authorization: Bearer <token>"
   ```

2. Verify encryption key:
   ```sql
   SELECT * FROM tenant_encryption_keys WHERE id = '<key_reference>';
   ```

3. Check disk space:
   ```bash
   df -h
   ```

4. Verify network connectivity:
   ```bash
   curl http://localhost:8080/api/health
   ```

### Integrity Verification Failed

If integrity verification fails:

1. **Backup is corrupted**
2. **Do not restore** - use earlier backup
3. **Investigate storage medium**
4. **Re-run backup procedure**

### Encryption Key Not Found

```sql
# List available keys
SELECT id, status, created_at FROM tenant_encryption_keys;

# Create new key
INSERT INTO tenant_encryption_keys (tenant_id, key_alias, kms_type, encryption, key_material)
VALUES ('system', 'primary', 'local', 'aes-256-gcm', '<32-byte-key>');
```

## Security Considerations

1. **Authentication**: All restore endpoints require valid bearer token
2. **Authorization**: Restore requires admin privileges
3. **Encryption**: All data encrypted at rest and in transit
4. **Audit logging**: All restore operations logged
5. **Verification**: Integrity verified before any data modified

## Related Documentation

- [Backup Procedures](BACKUP.md)
- [Disaster Recovery](DISASTER_RECOVERY.md)
- [Security Model](SECURITY_MODEL.md)
