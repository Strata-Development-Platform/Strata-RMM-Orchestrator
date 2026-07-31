# Disaster-Recovery Runbook

This runbook covers recovery when the source control plane may be unavailable. The backup repository and recovery key provider are intentionally independent of the source database.

## Declare and contain

1. Open an incident record and assign incident commander, recovery operator, and security owner.
2. Stop deployment automation and revoke compromised credentials when applicable.
3. Preserve source logs and storage snapshots; do not attempt an in-place restore.
4. Record the last known healthy release and the incident start time.

## Select and verify a recovery point

From a trusted recovery host with repository and key-provider access:

```bash
strata-rmm orchestrator recovery status
strata-rmm orchestrator recovery verify --backup-id <backup-id>
```

Select only a finalized, verified backup whose source release and schema are compatible with the recovery release. If verification fails, do not mutate targets; investigate repository/key integrity or select an earlier verified backup.

## Build an isolated target

Provision new PostgreSQL, JetStream-enabled NATS, and object-storage targets. Use new credentials. The PostgreSQL target must exist but be empty. NATS and object-storage targets must be distinct from source endpoints/buckets.

Configure the recovery variables in `docs/RESTORE.md`, run restore preflight, then execute the confirmed restore.

## Validate before cutover

The restore command proves component-level integrity and target availability. The incident team must additionally prove application behavior:

- orchestrator starts on the restored schema;
- liveness and readiness are healthy;
- login and an authenticated platform request succeed;
- MSP/client/device scoping remains intact;
- pending and queued durable jobs are present;
- JetStream consumers resume from preserved acknowledgement progress;
- representative stored objects match recorded hashes and metadata;
- audit evidence is present and append-only behavior remains enforced.

Record actual recovery-point age and elapsed recovery duration. The documented beta objectives are PostgreSQL RPO of 15 minutes or better, object-storage RPO of 24 hours or better, and control-plane RTO of four hours or better. Unit tests do not prove those objectives; only a timestamped drill can.

## Cutover

1. Pin the verified recovery release and immutable image digest.
2. Rotate source-era application, NATS, database, storage, signing, and enrollment credentials as required.
3. Enable routing only after readiness and smoke checks pass.
4. Monitor authentication failures, queue depth, dispatch age, NATS redelivery, database errors, and storage errors.
5. Preserve the failed source environment until incident and data-integrity review permits disposal.

## Failed restore

Cross-service restore is not transactional and does not automatically reverse partial target mutations. Never reuse a partially restored target:

1. keep the immutable backup set;
2. capture sanitized failure output;
3. discard the isolated target resources;
4. correct configuration, capacity, or compatibility;
5. create fresh empty targets; and
6. repeat verification and restore.

## Evidence and exit

The incident cannot close until the record includes backup ID, manifest identity, key ID, release/schema versions, target identities, operators, timestamps, verification results, tenant-isolation checks, actual RPO/RTO, residual data loss, and follow-up actions.

## Known limitations

- Automatic scheduling, retention enforcement, regional replication, and periodic drill orchestration remain operational responsibilities.
- The current artifact envelope buffers each component during encryption/decryption and enforces a 512 MiB plaintext-component limit.
- A successful exact-head CI run validates controlled fixtures, not the production RPO/RTO objective.
