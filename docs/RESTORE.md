# Restore Operations

Restore is deliberately target-oriented. It does not require the failed source services to be reachable, and it refuses in-place database, NATS, or object-storage targets.

## Required recovery targets

Set the normal backup repository and key-provider variables described in `docs/BACKUP.md`, plus:

| Variable / flag | Purpose |
|---|---|
| `--backup-id` | Finalized backup set to restore |
| `--target-dsn` | Existing, empty PostgreSQL database distinct from the source |
| `--confirm` | Explicit destructive-operation acknowledgement |
| `STRATA_RECOVERY_NATS_URL` | Empty JetStream-enabled NATS target distinct from the source URL |
| `STRATA_RECOVERY_NATS_TOKEN` | Target NATS token when used |
| `STRATA_RECOVERY_NATS_TLS_CA` | Target NATS trusted CA |
| `STRATA_RECOVERY_NATS_TLS_CERT` | Optional target mTLS certificate |
| `STRATA_RECOVERY_NATS_TLS_KEY` | Optional target mTLS key |
| `STRATA_RECOVERY_STORAGE_BACKEND` | Recovery storage backend type |
| `STRATA_RECOVERY_STORAGE_BUCKET` | Empty target bucket/path distinct from the source |
| `STRATA_RECOVERY_STORAGE_REGION` | Target region |
| `STRATA_RECOVERY_STORAGE_ENDPOINT` | Target endpoint |
| `STRATA_RECOVERY_STORAGE_ACCESS_KEY` | Target access key |
| `STRATA_RECOVERY_STORAGE_SECRET_KEY` | Target secret key |
| `STRATA_RECOVERY_STORAGE_USE_SSL` | Target transport setting |

If source object storage is disabled, recovery object-storage variables are not required. Production NATS TLS and authentication validation still applies to the recovery target.

## Procedure

1. List and verify the backup without contacting source services:

   ```bash
   strata-rmm orchestrator recovery status
   strata-rmm orchestrator recovery verify --backup-id <backup-id>
   ```

2. Provision isolated PostgreSQL, JetStream, and object-storage targets.

3. Run a non-mutating restore preflight:

   ```bash
   strata-rmm orchestrator recovery restore \
     --backup-id <backup-id> \
     --target-dsn "$RECOVERY_DATABASE_DSN" \
     --dry-run
   ```

4. Execute the restore:

   ```bash
   strata-rmm orchestrator recovery restore \
     --backup-id <backup-id> \
     --target-dsn "$RECOVERY_DATABASE_DSN" \
     --confirm \
     --timeout 4h
   ```

The command verifies and decrypts every required artifact before acquiring the target lock or mutating a target. It restores PostgreSQL first, then JetStream, then object storage, verifying each component before continuing. Replaying the same JetStream or object artifact reconciles identical state without appending duplicate messages or objects; divergent non-empty JetStream state is rejected.

The PostgreSQL restore resets the copied mutation gate before post-restore verification. Do not route traffic to the target until application readiness and the manual smoke checks below pass.

## Required post-restore checks

- start the candidate orchestrator against the recovery targets;
- verify `/health/live` and `/health/ready`;
- authenticate as a platform operator;
- query representative MSP, client, device, durable-job, approval, and audit records;
- verify tenant-scoped requests cannot cross MSP/client boundaries;
- inspect JetStream stream, consumer, and message counts;
- download and hash representative reports and recordings;
- record the backup ID, release, schema version, start/end times, and operators.

## Failure handling

The command returns nonzero on any integrity, decryption, restore, or verification failure and always attempts bounded lock/gate cleanup. It does not claim automatic transactional rollback across PostgreSQL, JetStream, and object storage. On failure, discard the isolated targets, create new empty targets, correct the cause, and rerun from the immutable backup set.

There is no `--force` bypass.
