# Backup Operations

Phase 8C backs up PostgreSQL, JetStream, and configured object storage into one externally discoverable backup set. Each component contains real data, is encrypted independently with AES-256-GCM, and is finalized only after its plaintext and ciphertext digests are recorded.

## Required configuration

| Variable | Purpose | Required |
|---|---|---|
| `STRATA_BACKUP_ENVIRONMENT_ID` | Stable identity of the protected environment | yes |
| `STRATA_BACKUP_KEY_PROVIDER_PATH` | File-key-provider directory on independently protected storage | yes |
| `STRATA_BACKUP_REPOSITORY_TYPE` | `filesystem` or `s3` | yes; default `filesystem` |
| `STRATA_BACKUP_DIRECTORY` | Mounted repository root for `filesystem` | for filesystem |
| `STRATA_BACKUP_EXTERNAL_BUCKET` | Backup bucket | for S3 |
| `STRATA_BACKUP_EXTERNAL_REGION` | Backup bucket region | for S3 |
| `STRATA_BACKUP_EXTERNAL_ENDPOINT` | Optional S3-compatible endpoint | optional |
| `STRATA_BACKUP_EXTERNAL_ACCESS_KEY` | S3 access key | for S3 |
| `STRATA_BACKUP_EXTERNAL_SECRET_KEY` | S3 secret key | for S3 |
| `STRATA_BACKUP_ENCRYPTION_SCHEME` | Must be `aes-256-gcm` | yes; default shown |

The normal database, NATS, and storage variables configure the protected source services. The repository and key-provider storage must survive loss of the source environment. A filesystem repository is only disaster-recoverable when its root is an independently protected mount.

Use an external scheduler to invoke the command, and apply repository lifecycle policy independently. No in-process scheduling or retention variable is claimed in this PR.

## Provision the first key

Create one active recovery key before the first backup:

```bash
strata-rmm orchestrator recovery key-init
```

The command refuses to overwrite or silently rotate an existing active key. Protect and replicate both the key-provider directory and backup repository; losing either makes recovery impossible.

## Preflight and backup

```bash
strata-rmm orchestrator backup --dry-run
strata-rmm orchestrator backup --timeout 2h
```

Preflight fails when the repository, active recovery key, PostgreSQL mutation gate, NATS JetStream account, or required object storage is unavailable. A real backup:

1. acquires a pinned, session-level PostgreSQL advisory lock;
2. closes the database-backed mutation gate;
3. snapshots PostgreSQL with `pg_dump --format=custom`;
4. captures JetStream stream configuration, consumer progress, headers, subjects, and message bytes;
5. streams object bytes and metadata into an archive;
6. encrypts each component with authenticated manifest identity;
7. writes and verifies component digests in the external repository;
8. publishes the final manifest; and
9. reopens the mutation gate and releases the lock through bounded cleanup.

PostgreSQL credentials are passed to client tools through the process environment, not command arguments. Command errors redact connection credentials.

## Verify and list

These commands require only the independent repository and key provider; source services may be unavailable:

```bash
strata-rmm orchestrator recovery status
strata-rmm orchestrator recovery verify --backup-id <backup-id>
```

Verification checks manifest structure, repository identifier safety, ciphertext digest, AES-GCM authentication, authenticated metadata, plaintext digest, and component payload digest. It does not mutate a recovery target.

## Current limitations

- Backup scheduling and retention deletion are operator-managed.
- Component encryption currently buffers a complete component payload in memory before AES-GCM sealing and enforces a 512 MiB plaintext-component limit; capacity-test and split larger data sets before beta use.
- A timestamped operational recovery drill is still required before A8-09 can be accepted.
