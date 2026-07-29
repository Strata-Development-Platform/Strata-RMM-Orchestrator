# Phase 8C Disaster Recovery

This document describes disaster recovery procedures for Phase 8C of the Strata RMM Orchestrator.

## Overview

Phase 8C implements comprehensive disaster recovery with:

- **State machine-driven recovery**: 15-state coordinator
- **RPO/RTO measurement**: Quantifiable recovery metrics
- **Multi-component support**: PostgreSQL, TimescaleDB, NATS JetStream, object storage
- **Automated rollback**: Automatic recovery on failure

## Disaster Recovery Scenarios

### Scenario 1: Complete Database Failure

**Symptoms**:
- PostgreSQL unresponsive
- All queries fail
- Connection pool exhausted

**Recovery Steps**:
1. Assess damage
2. Identify last known good backup
3. Initiate recovery from backup
4. Verify data integrity
5. Resume operations

### Scenario 2: JetStream Data Loss

**Symptoms**:
- NATS JetStream unavailable
- Messages lost
- Consumer offsets reset

**Recovery Steps**:
1. Identify affected streams
2. Restore from JetStream backup
3. Verify stream configuration
4. Restart consumers
5. Verify message flow

### Scenario 3: Object Storage Corruption

**Symptoms**:
- Object storage inaccessible
- Files corrupted
- Upload failures

**Recovery Steps**:
1. Identify affected objects
2. Restore from object storage backup
3. Verify object integrity
4. Resume operations

## Recovery Coordinator

### State Machine

The recovery coordinator implements a 15-state state machine:

```
Idle → Discovery → PreFlight → BackupDatabase → BackupJetStream → BackupObjectStorage → VerifyIntegrity → RestoreDatabase → RestoreJetStream → RestoreObjectStorage → PostRestoreValidation → HealthCheck → Verification → Completed
```

### Recovery Phases

1. **Discovery**: Locate backup files
2. **PreFlight**: Validate prerequisites
3. **Backup**: Create emergency backups (if needed)
4. **Integrity**: Verify backup integrity
5. **Restore**: Apply backups to systems
6. **Verification**: Confirm system health

### API Endpoints

#### Start Recovery

```bash
curl -X POST http://localhost:8080/api/v1/recovery \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "backup_id": "backup_abc123",
    "components": ["database", "jetstream", "objectstorage"],
    "options": {
      "verify_integrity": true,
      "rollback_on_failure": true
    }
  }'
```

#### Check Status

```bash
curl http://localhost:8080/api/v1/recovery/status \
  -H "Authorization: Bearer <token>"
```

#### Get Metrics

```bash
curl http://localhost:8080/api/v1/recovery/metrics \
  -H "Authorization: Bearer <token>"
```

## RPO/RTO Measurement

### RPO (Recovery Point Objective)

**Definition**: Maximum acceptable amount of data loss measured in time

**Metrics**:
- **Data Loss Window**: Time since last backup
- **Last Backup Time**: Timestamp of last successful backup
- **Max Acceptable RPO**: Configured maximum (default: 24 hours)

**Example**:
```json
{
  "rpo": {
    "data_loss_window": "3600s",
    "last_backup_time": "2024-01-01T00:00:00Z",
    "max_acceptable_rpo": "86400s"
  }
}
```

### RTO (Recovery Time Objective)

**Definition**: Maximum acceptable time to restore operations

**Metrics**:
- **Recovery Start Time**: When recovery began
- **Recovery End Time**: When recovery completed
- **Total Recovery Time**: End - Start
- **Phase Times**: Per-phase timing breakdown

**Example**:
```json
{
  "rto": {
    "recovery_start_time": "2024-01-01T01:00:00Z",
    "recovery_end_time": "2024-01-01T01:30:00Z",
    "total_recovery_time": "1800s",
    "phase_times": {
      "discovery": "60s",
      "restore": "1200s",
      "verification": "240s"
    }
  }
}
```

### Monitor RPO/RTO

```bash
# Get current RPO/RTO metrics
curl http://localhost:8080/api/v1/recovery/metrics \
  -H "Authorization: Bearer <token>"
```

## Automated Recovery

### Full Recovery

```bash
# Initiate automated recovery
curl -X POST http://localhost:8080/api/v1/recovery/automated \
  -H "Authorization: Bearer <token>" \
  -d '{
    "backup_id": "backup_abc123",
    "components": ["all"]
  }'
```

### Partial Recovery

```bash
# Recover only specific components
curl -X POST http://localhost:8080/api/v1/recovery/partial \
  -H "Authorization: Bearer <token>" \
  -d '{
    "backup_id": "backup_abc123",
    "components": ["database"]
  }'
```

## Rollback Procedures

### Automatic Rollback

If any phase fails, the coordinator automatically:

1. Detect failure
2. Log error details
3. Transition to rollback state
4. Attempt to restore previous state
5. Report final status

### Manual Rollback

```bash
# Initiate rollback
curl -X POST http://localhost:8080/api/v1/recovery/rollback \
  -H "Authorization: Bearer <token>"
```

## Testing Disaster Recovery

### Test Recovery Plan

```bash
# Test full recovery without affecting production
curl -X POST http://localhost:8080/api/v1/recovery/test \
  -H "Authorization: Bearer <token>" \
  -d '{
    "backup_id": "backup_abc123",
    "test_mode": true,
    "isolate_environment": true
  }'
```

### Verify Recovery

```bash
# Check recovery test results
curl http://localhost:8080/api/v1/recovery/test/verify \
  -H "Authorization: Bearer <token>"
```

## Monitoring and Alerts

### Alert Configuration

Monitor recovery metrics:

```yaml
alerts:
  - name: Recovery Timeout
    condition: recovery_duration > 2h
    severity: critical
  - name: Data Loss Exceeds RPO
    condition: data_loss_window > 24h
    severity: high
  - name: Recovery Failure
    condition: recovery_status == "failed"
    severity: critical
```

### Dashboard

Track recovery metrics:

- **RPO**: Data loss window over time
- **RTO**: Recovery time over time
- **Success Rate**: % of successful recoveries
- **Failure Reasons**: Common failure causes

## Pre-Flight Checks

Before recovery, the system validates:

1. **Database connectivity**: Can connect to PostgreSQL
2. **Encryption key**: Key available and valid
3. **Disk space**: Sufficient space for restore
4. **Network connectivity**: All components reachable
5. **Backup integrity**: SHA-256 digest verified

### Run Pre-Flight

```bash
curl -X POST http://localhost:8080/api/v1/recovery/preflight \
  -H "Authorization: Bearer <token>"
```

## Best Practices

1. **Regular Testing**: Test recovery monthly
2. **Multiple Backups**: Keep multiple backup copies
3. **Offsite Storage**: Store backups offsite
4. **RPO/RTO Review**: Review metrics quarterly
5. **Documentation**: Keep procedures updated

## Related Documentation

- [Backup Procedures](BACKUP.md)
- [Restore Procedures](RESTORE.md)
- [Security Model](SECURITY_MODEL.md)
