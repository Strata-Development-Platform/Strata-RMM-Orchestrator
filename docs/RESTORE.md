# Phase 8C Restore Procedures

## Overview

Phase 8C implements comprehensive restore capabilities with:

- **Real pg_restore execution**: Uses system binary, not SQL execution
- **Mandatory authenticated encryption**: AES-256-GCM before decryption
- **Integrity verification**: SHA-256 digest verified before restore
- **Multi-component support**: PostgreSQL, TimescaleDB, NATS JetStream, object storage
- **Target DSN required**: Restore requires explicit target database DSN

## PostgreSQL/TimescaleDB Restore

### Full Database Restore

Restore uses the system `pg_restore` binary with:
- `--no-owner --no-privileges --clean --if-exists` flags
- 30-minute timeout enforced via context
- Target DSN must be explicitly provided
- Backup data piped via stdin

### Integrity Verification

Before restore, the system automatically:
1. Verify integrity digest (SHA-256) - fails if corrupted
2. Decrypt using encryption key (AES-256-GCM)
3. Run pg_restore with the decrypted data

### Encryption Requirements

- Backup must have non-"none" encryption scheme
- Encryption key must be available in tenant_encryption_keys
- Restore fails if key is missing or scheme is "none"

## NATS JetStream Restore

Restore JetStream streams, consumers, and durable messages:
- Streams created via JetStream API
- Consumers created via CreateOrUpdateConsumer
- Messages re-published to their original subjects

## Object Storage Restore

Objects are restored with content verification:
- SHA-256 digest compared before upload
- Content decoded from base64
- Digest mismatch causes restore failure

## Rollback Procedures

### Automatic Rollback

If any phase fails, the coordinator automatically:
1. Detect failure
2. Transition to Rollback state
3. Log failure details
4. Transition to Cleanup state
5. Report final status

## Security Considerations

1. **Authentication**: All restore endpoints require valid bearer token
2. **Authorization**: Restore requires admin privileges
3. **Encryption**: All data encrypted (no plaintext backups accepted)
4. **Integrity verification**: SHA-256 checked before any data modification
5. **Target isolation**: Restore requires explicit target DSN
