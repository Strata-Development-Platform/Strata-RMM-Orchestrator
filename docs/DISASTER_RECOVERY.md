# Disaster Recovery Procedures

## Overview

The Strata RMM disaster recovery system implements a 20-state recovery workflow with:

- **Environment-scoped advisory locking** (`pg_try_advisory_xact_lock`) to prevent concurrent recoveries
- **Real service quiescing** before backup/restore operations
- **Encrypted backup/restore** for all data stores
- **Integrity verification** at every phase
- **Automatic rollback** on failure
- **RPO/RTO measurement** for compliance reporting

## Recovery Workflow States

The 20-state workflow consists of the following phases:

### Backup Phase
1. **Idle** → **Discovery**: Initialize recovery session
2. **Discovery** → **PreFlight**: Validate prerequisites (database connectivity, NATS availability)
3. **PreFlight** → **Quiesce**: Stop accepting new work
4. **Quiesce** → **BackupDatabase**: Encrypted pg_dump with AES-256-GCM
5. **BackupDatabase** → **BackupJetStream**: NATS stream/consumer/message backup
6. **BackupJetStream** → **BackupObjectStorage**: Object storage inventory backup
7. **BackupObjectStorage** → **VerifyIntegrity**: SHA-256 checksum verification
8. **VerifyIntegrity** → **PreRestoreValidation**: All backups verified

### Restore Phase
9. **PreRestoreValidation** → **RestoreDatabase**: Encrypted pg_restore to target
10. **RestoreDatabase** → **RestoreJetStream**: NATS stream/consumer/message restore
11. **RestoreJetStream** → **RestoreObjectStorage**: Object storage restore
12. **RestoreObjectStorage** → **PostRestoreValidation**: Validate restored data integrity

### Verification Phase
13. **PostRestoreValidation** → **HealthCheck**: Service health verification
14. **HealthCheck** → **Verification**: Data consistency checks
15. **Verification** → **RPOValidation**: Verify data loss window within RPO target
16. **RPOValidation** → **RTOValidation**: Verify total recovery time within RTO target

### Completion Phase
17. **RTOValidation** → **Cleanup**: Cleanup temporary resources
18. **Cleanup** → **Completed**: Recovery finished successfully

### Rollback Path
- Any phase can transition to **Rollback** → **Cleanup** → **Completed** on failure
- Rollback reverses all changes made during the recovery process

## Advisory Locking

Recovery operations use PostgreSQL advisory locks to ensure exclusive access:

```sql
SELECT pg_try_advisory_xact_lock(0x535452415441524D);
```

The lock ID is configurable via `STRATA_BACKUP_ADVISORY_LOCK_ID`. The default is `0x535452415441524D` ("STRATARM").

If the lock cannot be acquired, the recovery fails immediately with `ErrLockNotAcquired`.

## Dry-Run Mode

Run a recovery through all state transitions without executing side effects:

```bash
strata-rmm orchestrator recovery --backup-id <id> --dry-run
```

In dry-run mode:
- Advisory lock is NOT acquired
- Database dumps/restores are skipped
- State transitions still execute
- All events are logged
- Returns full `RecoveryResult` with event history

## Executing Recovery

### Restore from Backup

```bash
strata-rmm orchestrator recovery <backup-id> --timeout 2h --dry-run=false
```

### Backup Only

```bash
strata-rmm orchestrator backup --database-type timescaledb
```

### Recovery with Custom Timeout

```bash
strata-rmm orchestrator recovery <backup-id> --timeout 1h30m
```

## RPO and RTO Targets

| Metric | Target | Description |
|---|---|---|
| **RPO** | 15 minutes | Maximum acceptable data loss window |
| **RTO** | 4 hours | Maximum acceptable recovery time |

These targets are validated in the `RPOValidation` and `RTOValidation` states. Recovery is marked as failed if RPO or RTO targets cannot be met.

## Failure Handling

If any phase fails:

1. The state transitions to **Rollback**
2. Rollback reverses object storage changes
3. Rollback reverses JetStream changes
4. Rollback reverses database changes
5. State transitions to **Cleanup**
6. Cleanup completes and recovery finishes with `Success: false`

All failure events are recorded in `recovery_operations` and `backup_audit_log`.

## Recovery Audit Trail

Every recovery operation creates entries in:

- **`recovery_operations`**: State transitions, operations performed, timestamps
- **`backup_audit_log`**: Backup creation, verification, restore, rollback events
- **`backup_records`**: Backup metadata and integrity digests

To view the audit trail:

```sql
SELECT * FROM backup_audit_log ORDER BY timestamp DESC LIMIT 20;
SELECT * FROM recovery_operations WHERE recovery_id = '<recovery-id>' ORDER BY id;
```
