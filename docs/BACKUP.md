# Phase 8C Backup Procedures

This document describes backup procedures for Phase 8C of the Strata RMM Orchestrator.

## Overview

Phase 8C implements comprehensive backup capabilities with:

- **Authenticated encryption**: AES-256-GCM for all backups
- **Integrity verification**: SHA-256 digest for data integrity
- **Multi-component support**: PostgreSQL, TimescaleDB, NATS JetStream, object storage
- **Key management**: Key references stored only (not encryption keys)

## Prerequisites

- PostgreSQL 16+ with `pg_dump` available
- NATS JetStream 2.11+ (optional)
- Object storage (S3, GCS, Azure Blob, or local file system)
- Encryption key configured in `tenant_encryption_keys` table

## Configuration

### Environment Variables

```bash
# Database connection
STRATA_POSTGRES_DSN="postgres://user:password@host:5432/db?sslmode=enable"

# Encryption key reference (optional)
STRATA_ENCRYPTION_KEY_REF="key-id"

# Object storage (optional)
AWS_ACCESS_KEY_ID="..."
AWS_SECRET_ACCESS_KEY="..."
AWS_REGION="us-east-1"
STRATA_OBJECT_STORAGE_BUCKET="strata-backups"
```

## PostgreSQL/TimescaleDB Backup

### Full Database Backup

```bash
# Create a full backup using the backup API
curl -X POST http://localhost:8080/api/v1/backup/database \
  -H "Authorization: Bearer <token>" \
  -d '{"database_type": "postgresql"}'
```

### Backup Location

Backups are stored in the `backups` table with:

- `id`: Unique backup identifier
- `timestamp`: Backup creation time
- `database_type`: `postgresql` or `timescaledb`
- `data`: Encrypted backup data (AES-256-GCM)
- `integrity_digest`: SHA-256 hash for verification

### Backup Metadata

Each backup includes:

- **Table count**: Number of tables backed up
- **Row estimate**: Estimated row count
- **Data size**: Encrypted data size in bytes
- **Compression**: Currently `none` (compression not implemented)
- **Encryption scheme**: `aes-256-gcm` or `none`
- **Key reference**: ID of encryption key used

## NATS JetStream Backup

### JetStream Streams

All JetStream streams are backed up including:

- Stream configuration (subjects, retention, max age, etc.)
- Consumer configurations (ack policy, delivery, filter, etc.)

### Backup Command

```bash
curl -X POST http://localhost:8080/api/v1/backup/jetstream \
  -H "Authorization: Bearer <token>"
```

## Object Storage Backup

### Supported Providers

- AWS S3
- Google Cloud Storage
- Azure Blob Storage
- Local file system (for testing)

### Backup Command

```bash
curl -X POST http://localhost:8080/api/v1/backup/objectstorage \
  -H "Authorization: Bearer <token>"
```

## Encryption Details

### Algorithm: AES-256-GCM

- **Key size**: 256 bits (32 bytes)
- **Nonce**: 12 bytes per backup (randomly generated)
- **Authentication tag**: 16 bytes

### Key Management

- Keys stored in `tenant_encryption_keys` table
- Key material never stored in backups
- Key reference (UUID) stored in backup metadata

### Example Encryption Flow

```
Backup Data → AES-256-GCM (with nonce) → Ciphertext → SHA-256 Digest
```

## Backup Scheduling

### Cron Job Example

```bash
# Daily at 2:00 AM
0 2 * * * curl -X POST http://localhost:8080/api/v1/backup/database -H "Authorization: Bearer <token>"
```

### Kubernetes CronJob

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: strata-daily-backup
spec:
  schedule: "0 2 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: strata-rmm:latest
            command: ["curl", "-X", "POST", "http://localhost:8080/api/v1/backup/database"]
          restartPolicy: Never
```

## Backup Verification

### Verify Integrity

```bash
# Get backup metadata
curl http://localhost:8080/api/v1/backup/backup_abc123 \
  -H "Authorization: Bearer <token>"

# Check integrity digest
# Verify: SHA-256(encrypted_data) == stored_integrity_digest
```

### Manual Verification

```bash
# Extract encrypted data from backup
# Calculate SHA-256 hash
sha256sum backup_data.bin

# Compare with stored digest
grep integrity_digest backups_metadata.json
```

## Backup Retention

### Automatic Cleanup

Backups can be deleted after retention period:

```bash
curl -X DELETE http://localhost:8080/api/v1/backup/backup_abc123 \
  -H "Authorization: Bearer <token>"
```

### Manual Cleanup

```bash
# Delete backups older than 30 days
DELETE FROM backups WHERE timestamp < NOW() - INTERVAL '30 days';
```

## Troubleshooting

### Backup Failed

1. Check PostgreSQL connection:
   ```bash
   psql $STRATA_POSTGRES_DSN -c "SELECT 1"
   ```

2. Verify encryption key:
   ```sql
   SELECT * FROM tenant_encryption_keys WHERE status = 'active';
   ```

3. Check disk space:
   ```bash
   df -h
   ```

### Integrity Check Failed

If integrity verification fails:

1. Backup is corrupted
2. Restore from earlier backup
3. Investigate storage medium
4. Re-run backup

## Security Considerations

1. **Encrypt at rest**: All backups encrypted with AES-256-GCM
2. **Key rotation**: Regularly rotate encryption keys
3. **Access control**: Restrict backup API access
4. **Audit logging**: Log all backup operations
5. **Offsite storage**: Store backups in separate location

## Related Documentation

- [Restore Procedures](RESTORE.md)
- [Disaster Recovery](DISASTER_RECOVERY.md)
- [Security Model](SECURITY_MODEL.md)
