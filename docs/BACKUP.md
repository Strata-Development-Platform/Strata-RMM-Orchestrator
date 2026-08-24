# Strata RMM — Backup Reference

**Last Updated:** 2026-08-24

This document describes the host-level backup interface implemented by the orchestrator. It does not replace hosted recovery evidence required by Issue #15.

## Backup command

Run backup operations only on the orchestrator host (or through an authenticated administrative shell). The supported command is:

```bash
strata-rmm backup [--database-type postgresql|timescaledb] [--dry-run] [--timeout 2h]
```

`backup` creates one encrypted recovery set containing the configured PostgreSQL/TimescaleDB database, NATS JetStream state, and configured object-storage data. The engine acquires the recovery advisory lock, quiesces mutation/dispatch activity, captures component data, encrypts artifacts with AES-256-GCM, records SHA-256 integrity data, publishes the set to the configured repository, and resumes service activity.

Use `--dry-run` before a scheduled or manual backup to validate configuration and runtime prerequisites without creating a backup:

```bash
strata-rmm backup --dry-run
```

A successful real backup writes the backup manifest as JSON to stdout. Do not redirect that output to an insecure shared location if the manifest is operationally sensitive.

## Required configuration

The recovery runtime requires:

- `STRATA_BACKUP_ENVIRONMENT_ID`
- `STRATA_BACKUP_KEY_PROVIDER_PATH`
- an active recovery key in that provider
- the normal source database/NATS/storage configuration
- a configured backup repository

Initialize the first recovery key once with:

```bash
strata-rmm recovery key-init
```

The key-provider file is security-sensitive and must remain root-controlled. Do not copy key material into documentation, shell history, issue comments, or CI output.

### Filesystem repository

```bash
STRATA_BACKUP_REPOSITORY_TYPE=filesystem
STRATA_BACKUP_DIRECTORY=/var/lib/strata-rmm/backups
```

The configured directory must be writable by the host-level backup process and protected from untrusted users.

### S3-compatible repository

```bash
STRATA_BACKUP_REPOSITORY_TYPE=s3
STRATA_BACKUP_EXTERNAL_BUCKET=strata-rmm-backups
STRATA_BACKUP_EXTERNAL_REGION=example-region
STRATA_BACKUP_EXTERNAL_ENDPOINT=https://object-storage.example.invalid
STRATA_BACKUP_EXTERNAL_ACCESS_KEY_FILE=/run/secrets/backup-access-key
STRATA_BACKUP_EXTERNAL_SECRET_KEY_FILE=/run/secrets/backup-secret-key
```

Use the supported `_FILE` secret inputs. Do not place real access keys directly in documentation, Compose files, command lines, or shell history.

## Encryption and integrity

Recovery sets use AES-256-GCM with per-artifact nonces and SHA-256 integrity metadata. The configured key provider supplies the active encryption key. Backup verification uses the same repository and key provider used by the recovery engine.

Verify an existing set before relying on it:

```bash
strata-rmm recovery verify --backup-id <backup-id>
```

List available sets with:

```bash
strata-rmm recovery status
```

## Scheduling

The application does not install a backup scheduler. Use a root-controlled systemd timer or equivalent host scheduler that invokes the real command directly, for example:

```text
/usr/local/bin/strata-rmm backup --timeout 2h
```

Do not invent a `backup run` subcommand; it is not part of the CLI.

Scheduling, retention, and off-site replication remain operator policy. Retention automation must never remove the only verified recovery set required by the organization's RPO policy.

## Production transport requirements

Production backup uses the configured source database, NATS, and object-storage transports. Existing production validation remains authoritative. In particular, production database connections must not disable TLS, and production NATS must satisfy the platform TLS/authentication policy.

## Failure behavior

A backup failure is not success evidence. Preserve the sanitized error and do not delete the last known-good verified recovery set. If quiescing or component capture fails, investigate the failed component and rerun `backup --dry-run` before attempting another backup.

Do not manually substitute `pg_dump`, ad-hoc JetStream copies, or object-store copies as evidence that the integrated Strata recovery set succeeded. Those may be useful emergency diagnostics, but they are outside the supported backup transaction and do not satisfy the integrated recovery contract.

## Recovery testing

Use `docs/RESTORE.md` for the supported restore workflow. A source-level successful backup or verification does not prove RPO/RTO. The timestamped isolated restore drill and RPO/RTO evidence remain part of hosted internal-alpha validation in Issue #15.
