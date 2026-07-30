# Phase 8C Disaster Recovery

## Overview

Phase 8C implements comprehensive disaster recovery with:

- **20-state state machine**: Recovery lifecycle with real transitions
- **Environment-scoped locking**: pg_try_advisory_lock for exclusive recovery
- **Dry-run mode**: Full state machine walkthrough without side effects
- **RPO/RTO measurement**: Quantifiable recovery metrics from durable timestamps
- **Multi-component support**: PostgreSQL, TimescaleDB, NATS JetStream, object storage
- **Automated rollback**: Automatic recovery on failure with cleanup phase

## Recovery Coordinator

### 20-State State Machine

```
Idle (0) → Discovery (1) → PreFlight (2) → Quiesce (3) → BackupDatabase (4) →
BackupJetStream (5) → BackupObjectStorage (6) → VerifyIntegrity (7) →
PreRestoreValidation (8) → RestoreDatabase (9) → RestoreJetStream (10) →
RestoreObjectStorage (11) → PostRestoreValidation (12) → HealthCheck (13) →
Verification (14) → RPOValidation (15) → RTOValidation (16) →
Rollback (17) → Cleanup (18) → Completed (19)
```

### Recovery Phases

1. **Discovery**: Locate backup files, measure data loss window
2. **PreFlight**: Validate pg_dump/pg_restore available, DB connectivity
3. **Quiesce**: Quiesce services for backup
4. **Backup**: Create encrypted backups of all components
5. **Integrity**: Verify SHA-256 integrity digest
6. **PreRestoreValidation**: Ensure backup data is loadable
7. **Restore**: Apply backups to systems via real binaries
8. **PostRestoreValidation**: Validate DB connectivity after restore
9. **HealthCheck**: Ping all components
10. **Verification**: Final integrity re-verification
11. **RPO/RTOValidation**: Measure and report metrics
12. **Cleanup**: Release advisory locks
13. **Rollback**: Transition on any failure

### Locking Behavior

- Advisory lock acquired via `pg_try_advisory_lock` at start of recovery
- Lock released during Cleanup phase
- Bounded acquisition with 30-second timeout with 100ms polling
- Concurrent recovery attempts receive ErrLockNotAcquired

### Dry-Run Mode

When dry-run is enabled:
- All state transitions execute normally
- All phase execution methods return without side effects
- Backup/restore operations are skipped
- State machine reaches Completed state

## RPO/RTO Measurement

### RPO (Recovery Point Objective)
- Query MAX(timestamp) from backups table
- Data Loss Window = time.Since(lastBackupTime)
- Max Acceptable RPO = 24 hours (configurable)

### RTO (Recovery Time Objective)
- RecoveryStartTime: when Recover() was called
- RecoveryEndTime: in deferred function
- TotalRecoveryTime: end - start
- PhaseTimes: per-phase timing in map[string]Duration

## Pre-Flight Checks

Before recovery, the system validates:
1. **Database connectivity**: Ping PostgreSQL
2. **pg_dump/pg_restore**: Binary availability in PATH
3. **Encryption key**: Key available and valid
4. **Backup data**: Backup loadable from database
