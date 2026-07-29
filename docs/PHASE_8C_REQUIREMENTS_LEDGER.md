# Phase 8C Requirements Ledger

## Overview

This ledger documents all requirements for Phase 8C recovery gates (A8-06 through A8-09), organized by mandatory acceptance criteria. Each row represents a specific requirement with associated design, implementation, testing, and documentation artifacts.

## Acceptance Gates

| Gate | Description | Status |
|------|-------------|--------|
| A8-06 | PostgreSQL recovery | In scope |
| A8-07 | Messaging recovery | In scope |
| A8-08 | Object recovery | In scope |
| A8-09 | Recovery coordinator state machine | In scope |

## Requirements Ledger

| ID | Failure/Threat | Design | Runtime Consumer | Owner | Files | Negative Test | Integration Test | CI Job | Documentation | Status |
|---|---|---|---|---|---|---|---|---|---|---|
| A8-06-01 | Unencrypted or corrupted backup data renders PostgreSQL unrecoverable | Encrypted backup using AES-256-GCM or KMS; SHA-256 checksum for integrity; tenant-scoped isolation | `pkg/postgres/backup.go` (new), `pkg/encrypt/keys.go` | A | `pkg/postgres/backup.go`, `pkg/encrypt/keys.go` | TestUnencryptedBackupRejected, TestCorruptedChecksumRejected | TestPostgreSQLRestoreIntegrity, TestTenantDataIsolation | phase8c-postgres-restore-integrity | docs/PHASE_8C_POSTGRES_RECOVERY.md | Pending |
| A8-06-02 | Restore fails to preserve schema version and checksum | Version store (schema_migrations) preserved in backup; checksum validated post-restore | `pkg/postgres/rollback.go`, `pkg/postgres/version_store.go` | B | `pkg/postgres/rollback.go`, `pkg/postgres/version_store.go`, `pkg/postgres/backup.go` | TestSchemaVersionNotRestoredRejected | TestSchemaVersionChecksumValidated | phase8c-postgres-restore-integrity | docs/PHASE_8C_POSTGRES_RECOVERY.md | Pending |
| A8-06-03 | Tenant data leakage during restore (cross-tenant isolation breach) | RLS policies enforced during restore; tenant UUID binding in backup metadata | `pkg/postgres/statepreserve.go`, `pkg/postgres/schema.go` | C | `pkg/postgres/statepreserve.go`, `pkg/postgres/schema.go`, `pkg/postgres/backup.go` | TestCrossTenantRestoreDenied, TestRLSPolicyBypassRejected | TestTenantDataRestoreIsolation | phase8c-postgres-restore-integrity | docs/PHASE_8C_POSTGRES_RECOVERY.md | Pending |
| A8-06-04 | Restore executes without idempotency, causing duplicate resources | Idempotent restore via checksum comparison; state snapshot comparison before/after | `pkg/postgres/statepreserve.go`, `pkg/postgres/rollback.go` | A | `pkg/postgres/statepreserve.go`, `pkg/postgres/rollback.go` | TestDuplicateRestoreOverwritesData | TestIdempotentRestoreIdempotency | phase8c-postgres-restore-integrity | docs/PHASE_8C_POSTGRES_RECOVERY.md | Pending |
| A8-06-05 | Restore consumes excessive time exceeding RTO | Parallel restore of tenant data; incremental restore from checkpoint; timeout enforcement | `pkg/postgres/rollback.go`, `internal/recovery/worker.go` (new) | D | `pkg/postgres/rollback.go`, `internal/recovery/worker.go` | TestRestoreExceedsTimeoutCancelled | TestRestoreTimeoutEnforcement | phase8c-postgres-restore-performance | docs/PHASE_8C_POSTGRES_RECOVERY.md | Pending |
| A8-06-06 | Backup/restore operation lacks audit trail for compliance | Audit events recorded for backup creation, restore initiation, and completion; immutable ledger | `internal/platform/audit.go`, `pkg/postgres/rollback.go` | E | `internal/platform/audit.go`, `pkg/postgres/rollback.go` | TestAuditNotRecordedRejected | TestBackupRestoreAuditTraced | phase8c-postgres-restore-audit | docs/PHASE_8C_POSTGRES_RECOVERY.md | Pending |
| A8-07-01 | NATS/JetStream restart causes duplicate message delivery and destructive execution | Receipt ledger deduplication; idempotent command execution; state machine ensures single execution | `internal/agent/jobs/ledger.go`, `internal/platform/statemachine.go` | A | `internal/agent/jobs/ledger.go`, `internal/platform/statemachine.go` | TestDuplicateReceiptOverwritesResult | TestIdempotentExecutionSingleResult | phase8c-messaging-idempotency | docs/PHASE_8C_MESSAGING_RECOVERY.md | Pending |
| A8-07-02 | JetStream stream loss during outage causes work queue corruption | Mirrored streams; snapshot recovery; sequence gap detection and replay | `internal/agent/jobs/ledger.go`, `internal/agent/comms/nats.go` | B | `internal/agent/jobs/ledger.go`, `internal/agent/comms/nats.go` | TestMissingStreamCausesPanic | TestStreamRecoveryFromSnapshot | phase8c-messaging-recovery | docs/PHASE_8C_MESSAGING_RECOVERY.md | Pending |
| A8-07-03 | Agent reconnect storm after NATS restart causes resource exhaustion | Backpressure; exponential backoff with jitter; connection concurrency limits | `internal/agent/comms/nats.go`, `internal/agent/core/agent.go` | C | `internal/agent/comms/nats.go`, `internal/agent/core/agent.go` | TestNoBackpressureCausesOOM | TestBackpressureThrottlesReconnect | phase8c-messaging-recovery | docs/PHASE_8C_MESSAGING_RECOVERY.md | Pending |
| A8-07-04 | Outstanding jobs lost during NATS outage without recovery | Job state persisted in PostgreSQL; JetStream acks preserved; reconciliation on reconnect | `internal/agent/jobs/ledger.go`, `internal/agent/jobs/handler.go` | D | `internal/agent/jobs/ledger.go`, `internal/agent/jobs/handler.go` | TestPendingJobsDeletedOnOutage | TestOutstandingJobReconciliation | phase8c-messaging-recovery | docs/PHASE_8C_MESSAGING_RECOVERY.md | Pending |
| A8-07-05 | NATS TLS misconfiguration causes unencrypted message exposure | TLS mandatory; cert validation; fallback rejection | `internal/agent/comms/nats.go`, `pkg/config/config.go` | E | `internal/agent/comms/nats.go`, `pkg/config/config.go` | TestUnencryptedNATSAccepted | TestTLSFallbackRejected | phase8c-messaging-security | docs/PHASE_8C_MESSAGING_RECOVERY.md | Pending |
| A8-08-01 | Object storage restore returns corrupted data without detection | SHA-256 checksum validation; ETag verification; content-length check | `pkg/storage/backend.go`, `pkg/storage/s3_backend.go` | A | `pkg/storage/backend.go`, `pkg/storage/s3_backend.go`, `pkg/storage/local_backend.go` | TestCorruptedObjectAccepted | TestObjectIntegrityValidation | phase8c-object-restore-integrity | docs/PHASE_8C_OBJECT_RECOVERY.md | Pending |
| A8-08-02 | Authorization tokens expired or revoked during restore | Presigned URL with short expiry; token refresh; access control list validation | `pkg/auth/jwt.go`, `pkg/storage/s3_backend.go` | B | `pkg/auth/jwt.go`, `pkg/storage/s3_backend.go` | TestExpiredTokenAccepted | TestAuthorizationEnforcedOnRestore | phase8c-object-restore-security | docs/PHASE_8C_OBJECT_RECOVERY.md | Pending |
| A8-08-03 | Object metadata loss during restore breaks audit trail | Metadata preserved in backup manifest; tenant/device binding in keys | `pkg/storage/backend.go`, `pkg/storage/s3_backend.go` | C | `pkg/storage/backend.go`, `pkg/storage/s3_backend.go` | TestMissingMetadataCausesSilentFailure | TestMetadataPreservedInRestore | phase8c-object-restore-integrity | docs/PHASE_8C_OBJECT_RECOVERY.md | Pending |
| A8-08-04 | Large object restore exceeds RTO due to sequential download | Parallel downloads; chunked transfer; progress checkpointing | `pkg/storage/backend.go`, `pkg/storage/s3_backend.go` | D | `pkg/storage/backend.go`, `pkg/storage/s3_backend.go` | TestLargeObjectRestoreTimeout | TestParallelDownloadPerformance | phase8c-object-restore-performance | docs/PHASE_8C_OBJECT_RECOVERY.md | Pending |
| A8-08-05 | Object storage region mismatch causes cross-tenant data exposure | Bucket region validation; multi-region backup replication; geo-fencing | `pkg/storage/backend.go`, `pkg/config/config.go` | E | `pkg/storage/backend.go`, `pkg/config/config.go` | TestWrongRegionBucketAccepted | TestRegionEnforcementOnRestore | phase8c-object-restore-security | docs/PHASE_8C_OBJECT_RECOVERY.md | Pending |
| A8-09-01 | Recovery coordinator state machine accepts invalid transitions | Valid state transitions enforced; state machine rejects transitions not in allowed set | `internal/platform/statemachine.go` | A | `internal/platform/statemachine.go` | TestInvalidTransitionAccepted | TestStateMachineTransitionValidation | phase8c-recovery-coordinator | docs/PHASE_8C_RECOVERY_COORDINATOR.md | Pending |
| A8-09-02 | Recovery coordinator fails to persist state across restarts | State stored in PostgreSQL; WAL journal; atomic updates | `internal/platform/statemachine.go`, `pkg/postgres/rollback.go` | B | `internal/platform/statemachine.go`, `pkg/postgres/rollback.go` | TestStateLostOnRestart | TestStatePersistenceAcrossRestart | phase8c-recovery-coordinator | docs/PHASE_8C_RECOVERY_COORDINATOR.md | Pending |
| A8-09-03 | Concurrent recovery operations cause state corruption | Advisory lock on recovery operation; optimistic concurrency control | `internal/platform/statemachine.go`, `pkg/postgres/rollback.go` | C | `internal/platform/statemachine.go`, `pkg/postgres/rollback.go` | TestConcurrentRecoveryOverwritesState | TestLockPreventsConcurrentRecovery | phase8c-recovery-coordinator | docs/PHASE_8C_RECOVERY_COORDINATOR.md | Pending |
| A8-09-04 | Recovery coordinator lacks rollback capability on failure | Recovery state includes rollback markers; rollback hooks registered per phase | `pkg/postgres/rollback.go`, `internal/platform/statemachine.go` | D | `pkg/postgres/rollback.go`, `internal/platform/statemachine.go` | TestRollbackOnFailureSkipped | TestRollbackStateTransitionTriggers | phase8c-recovery-coordinator | docs/PHASE_8C_RECOVERY_COORDINATOR.md | Pending |
| A8-09-05 | Recovery coordinator timeout does not trigger failover | Context cancellation; deadline propagation; failover state transition | `internal/platform/statemachine.go`, `pkg/postgres/rollback.go` | E | `internal/platform/statemachine.go`, `pkg/postgres/rollback.go` | TestTimeoutDoesNotChangeState | TestFailoverOnTimeoutTriggered | phase8c-recovery-coordinator | docs/PHASE_8C_RECOVERY_COORDINATOR.md | Pending |

## Summary Statistics

- **Total Requirements**: 30
- **Requirements by Gate**:
  - A8-06 (PostgreSQL recovery): 6
  - A8-07 (Messaging recovery): 5
  - A8-08 (Object recovery): 5
  - A8-09 (Recovery coordinator): 5

## Owner Allocation

| Owner | Requirements |
|-------|--------------|
| A | 6 |
| B | 5 |
| C | 5 |
| D | 5 |
| E | 5 |
| F | 0 |

## CI Job Coverage

| CI Job | Requirements Covered |
|--------|---------------------|
| phase8c-postgres-restore-integrity | A8-06-01, A8-06-02, A8-06-03, A8-06-04 |
| phase8c-postgres-restore-performance | A8-06-05 |
| phase8c-postgres-restore-audit | A8-06-06 |
| phase8c-messaging-idempotency | A8-07-01 |
| phase8c-messaging-recovery | A8-07-02, A8-07-03, A8-07-04 |
| phase8c-messaging-security | A8-07-05 |
| phase8c-object-restore-integrity | A8-08-01, A8-08-03 |
| phase8c-object-restore-security | A8-08-02, A8-08-05 |
| phase8c-object-restore-performance | A8-08-04 |
| phase8c-recovery-coordinator | A8-09-01, A8-09-02, A8-09-03, A8-09-04, A8-09-05 |

## Documentation Files

- `docs/PHASE_8C_POSTGRES_RECOVERY.md`
- `docs/PHASE_8C_MESSAGING_RECOVERY.md`
- `docs/PHASE_8C_OBJECT_RECOVERY.md`
- `docs/PHASE_8C_RECOVERY_COORDINATOR.md`

## Notes

- All requirements must pass negative tests (edge cases, adversarial inputs) and integration tests (end-to-end scenarios)
- CI jobs must run on every PR and merge to master
- Documentation must include runbooks, operational procedures, and failure mode analysis
- Each requirement must have an assigned owner (A-F) for accountability
