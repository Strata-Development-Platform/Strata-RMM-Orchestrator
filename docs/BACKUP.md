# Backup Procedures

## Overview

Strata RMM provides encrypted backup capabilities for PostgreSQL/TimescaleDB, NATS JetStream, and object storage. All backup data is encrypted using AES-256-GCM with integrity verification via SHA-256 checksums.

## Configuration

| Environment Variable | Description | Default | Required |
|---|---|---|---|
| `STRATA_BACKUP_ENABLED` | Enable backup functionality | `false` | no |
| `STRATA_BACKUP_SCHEDULE` | Cron schedule for automated backups | — | no |
| `STRATA_BACKUP_RETENTION_DAYS` | Number of days to retain backups | `30` | no |
| `STRATA_BACKUP_ENCRYPTION_KEY_ID` | Encryption key identifier | — | no |
| `STRATA_BACKUP_DATABASE_TYPE` | Database type: `postgresql` or `timescaledb` | `timescaledb` | no |
| `STRATA_BACKUP_TARGET_DSN` | Target DSN for restore operations | — | conditional |
| `STRATA_BACKUP_DIRECTORY` | Local backup storage directory | `/var/lib/strata-rmm/backups` | no |
| `STRATA_BACKUP_ENCRYPTION_SCHEME` | Encryption scheme: `aes-256-gcm` or `aes-256-cbc` | `aes-256-gcm` | no |
| `STRATA_BACKUP_RPO` | Recovery Point Objective (target data loss window) | `15m` | no |
| `STRATA_BACKUP_RTO` | Recovery Time Objective (target recovery duration) | `4h` | no |
| `STRATA_BACKUP_ADVISORY_LOCK_ID` | PostgreSQL advisory lock ID (hex) | — | no |
| `STRATA_BACKUP_EXTERNAL_BUCKET` | External S3 bucket for backups | — | no |
| `STRATA_BACKUP_EXTERNAL_REGION` | External S3 region | — | no |
| `STRATA_BACKUP_EXTERNAL_ENDPOINT` | External S3 endpoint URL | — | no |
| `STRATA_BACKUP_EXTERNAL_ACCESS_KEY` | External S3 access key | — | yes (if bucket set) |
| `STRATA_BACKUP_EXTERNAL_SECRET_KEY` | External S3 secret key | — | yes (if bucket set) |

## Creating Backups

### Manual Backup

Use the CLI to create an immediate backup:

```bash
strata-rmm orchestrator backup --database-type timescaledb --dry-run
```

For a real backup (requires encryption key and pg_dump):

```bash
strata-rmm orchestrator backup --database-type timescaledb
```

### Scheduled Backups

Configure `STRATA_BACKUP_SCHEDULE` with a cron expression:

```bash
export STRATA_BACKUP_ENABLED=true
export STRATA_BACKUP_SCHEDULE="0 */6 * * *"  # Every 6 hours
```

### Backup Components

Each backup includes:

1. **Database**: Full PostgreSQL/TimescaleDB dump via `pg_dump --format=custom`, encrypted with AES-256-GCM
2. **JetStream**: NATS stream definitions, consumer configurations, and message sequences
3. **Object Storage**: Inventory of all objects with metadata and integrity checksums

### Backup Metadata

Each backup is recorded in the `backup_records` table with:

- Unique backup ID (UUID)
- Database type and version
- Table count and row estimate
- Encrypted data size
- Encryption scheme and key reference
- SHA-256 integrity digest
- Creation timestamp and completion status

## Encryption

All backups are encrypted using AES-256-GCM via the `pkg/encrypt` package. Key material is stored in the `tenant_encryption_keys` table and retrieved via the `KeyStore` using the system tenant ID.

The encryption process:
1. Generate AES-256-GCM cipher from 32-byte key material
2. Create random nonce (12 bytes)
3. Seal plaintext data with nonce
4. Encode ciphertext as base64

Integrity is verified by computing SHA-256 hash of the encrypted data and storing the digest in the backup record.

## Retention

Backups are retained for `STRATA_BACKUP_RETENTION_DAYS` (default: 30 days). The cleanup process:

1. Queries `backup_records` for records older than retention period
2. Verifies integrity before deletion
3. Removes backup data from storage
4. Removes metadata from `backup_records` table
5. Records deletion in `backup_audit_log`

## Verification

After each backup, the system:

1. Computes SHA-256 integrity digest
2. Records the digest in `backup_records.integrity_digest`
3. Verifies digest matches the encrypted payload
4. Logs verification in `backup_audit_log`

To verify a specific backup:

```bash
# Check backup record integrity
psql -c "SELECT id, status, integrity_digest, created_at FROM backup_records WHERE id = '<backup-id>';"
```
