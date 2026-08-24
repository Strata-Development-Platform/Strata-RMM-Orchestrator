# Strata RMM — Configuration Reference

**Last Updated:** 2026-08-24

Configuration is loaded by `LoadOrchestratorConfig()` and validated before the orchestrator starts. In production, `ProductionValidate()` adds stricter TLS, authentication, origin, and secret requirements. This document is an operator reference; the current configuration package remains the executable authority if a new variable is introduced before the docs are updated.

## Core runtime

| Variable | Purpose |
| --- | --- |
| `STRATA_RUNTIME_MODE` | `development`, `test`, or `production` |
| `TIMESCALE_DSN` | Primary PostgreSQL/TimescaleDB DSN; `STRATA_DB_DSN`/`DATABASE_URL` are compatibility fallbacks |
| `DB_REPLICA_DSN` | Optional read replica; `TIMESCALE_REPLICA_DSN` is a fallback |
| `STRATA_API_ADDR` | HTTP listen address |
| `STRATA_PUBLIC_URL` | Public base URL; required and HTTPS in production where public links are generated |
| `CORS_ORIGINS` | Allowed browser origins; wildcard is rejected in production |
| `JWT_SECRET` / `JWT_SECRET_FILE` | JWT signing secret; use the file form in production |
| `STRATA_METRICS_TOKEN` / `STRATA_METRICS_TOKEN_FILE` | Metrics authentication token |
| `STRATA_TUNNEL_ADDR` | Raw tunnel listener; not accepted in production validation |

Database pool controls:

- `DB_MAX_OPEN_CONNS`
- `DB_MAX_IDLE_CONNS`
- `DB_CONN_MAX_LIFETIME`

Production database DSNs must not disable TLS and must not use known default passwords.

## NATS / JetStream

| Variable | Purpose |
| --- | --- |
| `NATS_URL` | NATS connection URL |
| `NATS_ADVERTISE_URLS` | Agent-reachable NATS URLs |
| `NATS_TOKEN` / `NATS_TOKEN_FILE` | Token authentication |
| `NATS_TLS_ENABLED` | Enable TLS |
| `NATS_TLS_CA` | CA certificate path |
| `NATS_TLS_CERT` | Client certificate path |
| `NATS_TLS_KEY` | Client private-key path |
| `NATS_RECONNECT_WAIT` | Reconnect delay |
| `NATS_MAX_RECONNECTS` | Reconnect attempt policy |
| `JS_MAX_MEMORY_STORE` | JetStream memory limit |
| `JS_MAX_FILE_STORE` | JetStream file-store limit |
| `JS_STORAGE_PATH` | JetStream storage path |
| `JS_NUM_REPLICAS` | Stream replica count |

Production requires TLS, a CA, authentication by token or valid mutual-TLS configuration, and agent-advertised URLs that use `tls://` or `nats+tls://` and are not localhost/container-local addresses.

## Object storage

| Variable | Purpose |
| --- | --- |
| `STORAGE_BACKEND` | `local`, `minio`, `s3`, or `none` where accepted by the runtime |
| `STORAGE_BUCKET` | Primary object-storage bucket |
| `STORAGE_REGION` | Region |
| `STORAGE_ENDPOINT` | Optional custom endpoint |
| `STORAGE_ACCESS_KEY` / `STORAGE_ACCESS_KEY_FILE` | Access key |
| `STORAGE_SECRET_KEY` / `STORAGE_SECRET_KEY_FILE` | Secret key |
| `STORAGE_USE_SSL` | TLS/SSL use for storage transport |
| `STORAGE_KMS_KEY_ID` | Optional SSE-KMS key identifier |

Prefer file-backed secrets in production. Do not place object-storage credentials in Compose YAML, shell history, documentation, or CI variables that are printed.

## Redis

Redis is optional and used where configured for token blacklisting/session security support.

- `REDIS_URL`
- `REDIS_POOL_SIZE`
- `REDIS_MIN_IDLE_CONNS`
- `REDIS_MAX_RETRIES`

Use TLS (`rediss://`) where Redis crosses a trust boundary.

## SMTP / outbound email

- `STRATA_SMTP_HOST`
- `STRATA_SMTP_PORT`
- `STRATA_SMTP_USERNAME` / `STRATA_SMTP_USERNAME_FILE`
- `STRATA_SMTP_PASSWORD` / `STRATA_SMTP_PASSWORD_FILE`
- `STRATA_SMTP_FROM`
- `STRATA_SMTP_IMPLICIT_TLS`

Username/password must be configured together. SMTP-dependent links require a valid public URL. Provider-profile APIs expose readiness/status, not the SMTP password itself.

## Alert delivery

- `STRATA_ALERT_SLACK_URL` / `STRATA_ALERT_SLACK_URL_FILE`
- `STRATA_ALERT_TEAMS_URL` / `STRATA_ALERT_TEAMS_URL_FILE`
- `STRATA_ALERT_WEBHOOK_URL` / `STRATA_ALERT_WEBHOOK_URL_FILE`
- `STRATA_ALERT_PAGERDUTY_KEY` / `STRATA_ALERT_PAGERDUTY_KEY_FILE`
- `STRATA_ALERT_EMAIL_RECIPIENTS`

Webhook URLs must satisfy production HTTPS validation. Email delivery requires SMTP configuration.

## Development seeding

- `STRATA_SEED_DEV`
- `STRATA_DEV_ADMIN_EMAIL`
- `STRATA_DEV_ADMIN_PASSWORD_HASH`

Development seeding is rejected in production. Do not use development seeding to bootstrap a production administrator; use the secure installer/local bootstrap flow in `INSTALL.md`.

## Backup repository and recovery key

- `STRATA_BACKUP_ENABLED`
- `STRATA_BACKUP_ENVIRONMENT_ID`
- `STRATA_BACKUP_DATABASE_TYPE`
- `STRATA_BACKUP_DIRECTORY`
- `STRATA_BACKUP_ENCRYPTION_SCHEME`
- `STRATA_BACKUP_REPOSITORY_TYPE`
- `STRATA_BACKUP_EXTERNAL_BUCKET`
- `STRATA_BACKUP_EXTERNAL_REGION`
- `STRATA_BACKUP_EXTERNAL_ENDPOINT`
- `STRATA_BACKUP_EXTERNAL_ACCESS_KEY` / `STRATA_BACKUP_EXTERNAL_ACCESS_KEY_FILE`
- `STRATA_BACKUP_EXTERNAL_SECRET_KEY` / `STRATA_BACKUP_EXTERNAL_SECRET_KEY_FILE`
- `STRATA_BACKUP_KEY_PROVIDER_PATH`

The supported encryption scheme is AES-256-GCM. Filesystem repositories require `STRATA_BACKUP_DIRECTORY`; S3 repositories require bucket, region, and credentials. The key-provider file is root-sensitive recovery material.

## Isolated restore targets

Restore uses separate recovery target configuration and refuses in-place recovery:

- `STRATA_RECOVERY_NATS_URL`
- `STRATA_RECOVERY_NATS_TOKEN`
- `STRATA_RECOVERY_NATS_TLS_CA`
- `STRATA_RECOVERY_NATS_TLS_CERT`
- `STRATA_RECOVERY_NATS_TLS_KEY`
- `STRATA_RECOVERY_STORAGE_BACKEND`
- `STRATA_RECOVERY_STORAGE_BUCKET`
- `STRATA_RECOVERY_STORAGE_REGION`
- `STRATA_RECOVERY_STORAGE_ENDPOINT`
- `STRATA_RECOVERY_STORAGE_ACCESS_KEY`
- `STRATA_RECOVERY_STORAGE_SECRET_KEY`
- `STRATA_RECOVERY_STORAGE_USE_SSL`

The current restore runtime requires a distinct target database; a distinct recovery NATS target; and, when source object storage is enabled, a distinct recovery storage target. Production recovery NATS requires TLS plus token authentication or mTLS, and production recovery database transport must not disable TLS.

## Secret-file rules

Where a `_FILE` variant is supported, use it for production secrets. Secret files are expected to be absolute, canonical regular files, non-empty, bounded in size, and protected by host/container permissions. Direct and file-backed forms for the same value are mutually exclusive.

A safe pattern is:

```bash
export JWT_SECRET_FILE=/run/secrets/strata-jwt
export NATS_TOKEN_FILE=/run/secrets/strata-nats-token
export STORAGE_ACCESS_KEY_FILE=/run/secrets/strata-storage-access-key
export STORAGE_SECRET_KEY_FILE=/run/secrets/strata-storage-secret-key
```

The examples intentionally contain paths, not secret values.

## Production validation summary

When `STRATA_RUNTIME_MODE=production`, startup/recovery validation is fail-closed for unsafe configuration. Important enforced rules include:

- public URL is HTTPS where required;
- wildcard CORS is rejected;
- NATS uses TLS, a CA, valid advertised URLs, and token or mTLS authentication;
- database TLS cannot be explicitly disabled;
- known development/default secret patterns are rejected;
- raw tunnel mode and development seeding are rejected;
- SMTP configuration is internally consistent;
- webhook endpoints use acceptable HTTPS URLs;
- backup/recovery transport and target-separation rules are enforced by the recovery runtime.

Never weaken production validation to make a deployment start. Correct the configuration or narrow the deployment claim.

## Development-only configuration

Development mode may use local transports and repository Compose assets for developer testing. Those examples are intentionally excluded here because copying a mutable development image, plaintext dependency password, localhost NATS URL, or seeded dev identity into production is unsafe.

For local work, use the repository Makefile/development Compose topology. For production, use `INSTALL.md` and the installer-generated protected configuration.

## Redaction and logging

The configuration layer provides redacted summaries for logging. Operators should preserve that boundary: DSN passwords, tokens, SMTP credentials, storage credentials, alert keys, recovery credentials, bootstrap material, invitation tokens, and enrollment credentials must not appear in logs or evidence artifacts.

## Related references

- `INSTALL.md` — secure production configuration creation
- `UPGRADE.md` — update-time preflight and lifecycle
- `BACKUP.md` — backup repository/key use
- `RESTORE.md` — isolated recovery target requirements
- `SECURITY.md` — production security model
