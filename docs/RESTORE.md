# Strata RMM — Restore Reference

**Last Updated:** 2026-08-24

This document describes the supported host-level disaster-recovery interface. Restore is intentionally fail-closed. Do not replace this workflow with manual database drops, raw SQL replay, ad-hoc decryption, or in-place restore commands.

## Supported recovery operations

Run recovery commands on the orchestrator host or through an authenticated administrative shell:

```bash
strata-rmm recovery preflight
strata-rmm recovery status
strata-rmm recovery verify --backup-id <backup-id>
strata-rmm recovery key-init
strata-rmm recovery restore --backup-id <backup-id> --target-dsn '<target-dsn>' --dry-run
strata-rmm recovery restore --backup-id <backup-id> --target-dsn '<target-dsn>' --confirm
```

There is no supported `recovery decrypt` command and no supported manual in-place database replacement path.

## Prerequisites

Recovery uses the same configured backup repository and key provider as backup. The runtime requires:

- `STRATA_BACKUP_ENVIRONMENT_ID`
- `STRATA_BACKUP_KEY_PROVIDER_PATH`
- an active recovery key
- access to the configured backup repository
- a clean, distinct target PostgreSQL/TimescaleDB database for restore
- a distinct recovery NATS target
- when source object storage is enabled, a distinct recovery object-storage target

Check the repository and recovery prerequisites before planning mutation:

```bash
strata-rmm recovery preflight
strata-rmm recovery status
strata-rmm recovery verify --backup-id <backup-id>
```

## Restore safety contract

The recovery engine enforces these rules before restoring:

1. `--backup-id` is required.
2. `--target-dsn` is required.
3. The target DSN must differ from the configured source DSN.
4. The resolved target database identity must differ from the source database identity.
5. The target database must be empty.
6. Backup integrity is verified before target mutation.
7. `--confirm` is required for a real restore.
8. `--dry-run` performs preflight/integrity validation without target mutation.
9. The recovery NATS target must differ from the source NATS target.
10. If source object storage is enabled, the recovery storage target must differ from the source target.

Do not weaken or bypass these checks to make a restore proceed.

## Recovery target configuration

Use protected secret files where the configuration layer supports them. Do not place real credentials in documentation or shell history.

Example non-secret recovery target metadata:

```bash
STRATA_RECOVERY_NATS_URL=nats+tls://recovery-nats.example.invalid:4222
STRATA_RECOVERY_NATS_TLS_CA=/run/secrets/recovery-nats-ca.crt
STRATA_RECOVERY_NATS_TLS_CERT=/run/secrets/recovery-nats-client.crt
STRATA_RECOVERY_NATS_TLS_KEY=/run/secrets/recovery-nats-client.key

STRATA_RECOVERY_STORAGE_BACKEND=s3
STRATA_RECOVERY_STORAGE_BUCKET=strata-rmm-recovery-target
STRATA_RECOVERY_STORAGE_REGION=example-region
STRATA_RECOVERY_STORAGE_ENDPOINT=https://recovery-object-storage.example.invalid
```

Configure recovery NATS authentication and storage credentials through the production secret mechanism. Never publish actual tokens/access keys in runbooks or command examples.

## Production transport requirements

In `STRATA_RUNTIME_MODE=production`, recovery refuses unsafe transports. Production recovery requires:

- NATS TLS enabled with a CA file;
- `tls://` or `nats+tls://` recovery NATS URL;
- NATS token authentication or mutual TLS;
- client certificate and key configured together when mTLS is used;
- a target database DSN that does not set `sslmode=disable`.

These are runtime checks, not recommendations.

## Dry run

Always exercise the exact candidate backup and exact target first:

```bash
strata-rmm recovery restore \
  --backup-id <backup-id> \
  --target-dsn '<clean-distinct-target-dsn>' \
  --dry-run
```

A passing dry run means the current preflight and integrity checks succeeded; it does not prove the full restore or RPO/RTO objective.

## Execute restore

After dry-run validation and an approved change/recovery window:

```bash
strata-rmm recovery restore \
  --backup-id <backup-id> \
  --target-dsn '<clean-distinct-target-dsn>' \
  --confirm \
  --timeout 4h
```

The command restores through the recovery engine and reports success only after its target component verification completes.

## Post-restore validation

Do not mutate the restored target with ad-hoc SQL as part of the recovery mechanism. Validate it through the intended isolated environment and application health/acceptance workflows. Record the exact backup ID, source release/SHA, target environment identity, UTC start/end timestamps, observed result, and cleanup result for hosted evidence.

## Failure handling

If restore fails:

- preserve the sanitized recovery error and logs;
- do not retry against a partially mutated target as though it were clean;
- create a new empty target before the next real attempt unless the recovery procedure explicitly proves the existing target is safe;
- verify the same backup set again before retrying if artifact integrity or repository access was implicated;
- do not fall back to manual `DROP DATABASE`, raw `psql` replay, or custom decryption commands as a substitute for the supported recovery contract.

## RPO/RTO evidence

Source code, CI, `recovery verify`, and a passing dry run are not hosted recovery acceptance. The timestamped isolated recovery drill, measured RPO/RTO, retained evidence, and cleanup proof remain mandatory under Issue #15 before internal-alpha approval.
