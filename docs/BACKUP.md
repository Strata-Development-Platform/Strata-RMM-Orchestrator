# Phase 8C Backup Procedures

## Overview

Phase 8C implements comprehensive backup capabilities with:

- **Real pg_dump/pg_restore execution**: Uses system binaries, not SQL simulation
- **Mandatory authenticated encryption**: AES-256-GCM for all backups (no plaintext fallback)
- **Integrity verification**: SHA-256 digest for data integrity
- **Multi-component support**: PostgreSQL, TimescaleDB, NATS JetStream, object storage
- **Key management**: Key references stored only (not encryption keys)

## Prerequisites

- PostgreSQL 16+ with `pg_dump` and `pg_restore` binaries in PATH
- NATS JetStream 2.11+ (optional)
- Object storage (S3, GCS, Azure Blob, or local file system)
- Encryption key configured in `tenant_encryption_keys` table (mandatory)

## PostgreSQL/TimescaleDB Backup

### Binary Requirements

Backup uses the system `pg_dump` binary with:
- `--no-owner --no-privileges --format=custom` flags
- 30-minute timeout enforced via context
- DSN derived from database connection string

### Full Database Backup

Backups are stored in the `backups` table with:
- `id`: Unique backup identifier (format: `backup_<base64>`)
- `timestamp`: Backup creation time
- `database_type`: `postgresql` or `timescaledb`
- `data`: Encrypted backup data (AES-256-GCM, mandatory)
- `integrity_digest`: SHA-256 hash for verification
- `scheme`: Encryption scheme (never "none")
- `key_reference`: ID of encryption key used (never "none")

## NATS JetStream Backup

All JetStream streams are backed up including:
- Stream configuration (subjects, retention, max age, etc.)
- Consumer configurations (ack policy, delivery, filter, etc.)
- Durable messages (up to 1000 per stream, base64-encoded)

## Object Storage Backup

All object contents are backed up including:
- Object key, content (base64-encoded), length, and SHA-256 digest
- Content verified on restore with digest comparison

## Encryption Details

### Algorithm: AES-256-GCM (Mandatory)

- **Key size**: 256 bits (32 bytes)
- **Nonce**: 12 bytes per backup (randomly generated)
- **Authentication tag**: 16 bytes
- Backup fails if encryption key is unavailable

### Key Management

- Keys stored in `tenant_encryption_keys` table
- Key material never stored in backups
- Key reference (UUID) stored in backup metadata
- No plaintext fallback: encryption is mandatory

## Security Considerations

1. **Encrypt at rest**: All backups encrypted with AES-256-GCM
2. **No plaintext fallback**: Missing key causes backup failure
3. **Key rotation**: Regularly rotate encryption keys
4. **Access control**: Restrict backup API access
5. **Integrity verification**: SHA-256 digest checked on restore
