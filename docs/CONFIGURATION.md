# Strata RMM — Configuration Reference

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Configuration Overview

Configuration is loaded from environment variables at startup. The orchestrator validates all configuration and refuses to start if required values are missing or invalid.

**Loading:** `pkg/config/config.go` → `LoadOrchestratorConfig()`
**Validation:** `OrchestratorConfig.Validate()` + `ProductionValidate()`

---

## 2. Environment Variables

### 2.1 Core Configuration

| Variable | Default | Required | Type | Description |
|----------|---------|----------|------|-------------|
| `STRATA_RUNTIME_MODE` | `development` | No | enum | `development`, `test`, `production` |
| `TIMESCALE_DSN` | `postgres://localhost:5432/strata_rmm` | Yes | string | PostgreSQL/TimescaleDB connection string. Also accepts `STRATA_DB_DSN` or `DATABASE_URL` as fallback |
| `DB_REPLICA_DSN` | — | No | string | Read replica connection string. Also accepts `TIMESCALE_REPLICA_DSN` as fallback |
| `NATS_URL` | `nats://localhost:4222` | Yes | URL | NATS connection URL. Schemes: `nats`, `nats+tls`, `tls` |
| `NATS_ADVERTISE_URLS` | — | No | comma-separated | Agent-reachable NATS URLs. Comma-separated list of absolute URLs |
| `NATS_TOKEN` | — | No | secret | NATS authentication token. Also accepts `NATS_TOKEN_FILE` for file-based secret |
| `NATS_TLS_ENABLED` | `false` | No | bool | Enable NATS TLS. Values: `true`, `false`, `yes`, `no`, `1`, `0` |
| `NATS_TLS_CERT` | — | No | path | NATS client certificate path |
| `NATS_TLS_KEY` | — | No | path | NATS client key path |
| `NATS_TLS_CA` | — | No | path | NATS CA certificate path |
| `NATS_RECONNECT_WAIT` | `5s` | No | duration | Reconnect wait interval |
| `NATS_MAX_RECONNECTS` | `-1` | No | int | Max reconnect attempts (-1 = unlimited) |
| `REDIS_URL` | — | No | URL | Redis connection URL. Schemes: `redis`, `rediss`, `tcp`, `tls`. Optional — only needed for token blacklisting |
| `REDIS_POOL_SIZE` | `10` | No | int | Redis connection pool size |
| `REDIS_MIN_IDLE_CONNS` | `2` | No | int | Redis minimum idle connections |
| `REDIS_MAX_RETRIES` | `3` | No | int | Redis max retries |
| `JWT_SECRET` | — | Yes | secret | JWT signing secret (HS256, min 32 characters). Also accepts `JWT_SECRET_FILE` |
| `STRATA_METRICS_TOKEN` | — | No | secret | Prometheus metrics authentication token (min 32 characters) |
| `STRATA_API_ADDR` | `:8080` | No | addr | HTTP API listen address |
| `STRATA_TUNNEL_ADDR` | — | No | addr | Raw tunnel gateway address (not production-safe) |
| `STRATA_PUBLIC_URL` | — | No | URL | Public-facing base URL (HTTPS in production). Used for SMTP links |
| `CORS_ORIGINS` | — | No | comma-separated | CORS allowed origins. Wildcard (`*`) rejected in production |

### 2.2 Database Configuration

| Variable | Default | Required | Type | Description |
|----------|---------|----------|------|-------------|
| `DB_MAX_OPEN_CONNS` | `25` | No | int | Max open PostgreSQL connections |
| `DB_MAX_IDLE_CONNS` | `5` | No | int | Min idle PostgreSQL connections (must be ≤ MaxOpenConns) |
| `DB_CONN_MAX_LIFETIME` | `5m` | No | duration | Connection max lifetime |

### 2.3 Object Storage

| Variable | Default | Required | Type | Description |
|----------|---------|----------|------|-------------|
| `STORAGE_BACKEND` | `local` | No | string | Storage backend: `local`, `minio`, `s3`. `none` disables storage |
| `STORAGE_BUCKET` | `strata-recordings` | No | string | Object storage bucket name |
| `STORAGE_REGION` | — | No | string | Storage region |
| `STORAGE_ENDPOINT` | — | No | string | Custom endpoint URL |
| `STORAGE_ACCESS_KEY` | — | No | secret | Storage access key. Also accepts `STORAGE_ACCESS_KEY_FILE` |
| `STORAGE_SECRET_KEY` | — | No | secret | Storage secret key. Also accepts `STORAGE_SECRET_KEY_FILE` |
| `STORAGE_USE_SSL` | — | No | bool | Use SSL for storage connections |
| `STORAGE_KMS_KEY_ID` | — | No | string | KMS key ID for SSE-KMS encryption |

### 2.4 JetStream Configuration

| Variable | Default | Required | Type | Description |
|----------|---------|----------|------|-------------|
| `JS_MAX_MEMORY_STORE` | `2GB` | No | size | JetStream max memory store |
| `JS_MAX_FILE_STORE` | `50GB` | No | size | JetStream max file store |
| `JS_STORAGE_PATH` | `/var/lib/strata/jetstream` | No | path | JetStream file storage path |
| `JS_NUM_REPLICAS` | `1` | No | int | JetStream stream replicas |

### 2.5 SMTP / Email

| Variable | Default | Required | Type | Description |
|----------|---------|----------|------|-------------|
| `STRATA_SMTP_HOST` | — | No | string | SMTP host |
| `STRATA_SMTP_PORT` | `0` | No | int | SMTP port (1-65535) |
| `STRATA_SMTP_USERNAME` | — | No | secret | SMTP username. Must be paired with password |
| `STRATA_SMTP_PASSWORD` | — | No | secret | SMTP password. Must be paired with username |
| `STRATA_SMTP_FROM` | — | No | string | SMTP from address (RFC 5322) |
| `STRATA_SMTP_IMPLICIT_TLS` | `false` | No | bool | Use implicit TLS (SMTPS) |

### 2.6 Alert Delivery

| Variable | Default | Required | Type | Description |
|----------|---------|----------|------|-------------|
| `STRATA_ALERT_SLACK_URL` | — | No | secret | Slack webhook URL (HTTPS, no credentials) |
| `STRATA_ALERT_TEAMS_URL` | — | No | secret | Teams webhook URL (HTTPS, no credentials) |
| `STRATA_ALERT_WEBHOOK_URL` | — | No | secret | Generic webhook URL (HTTPS, no credentials) |
| `STRATA_ALERT_PAGERDUTY_KEY` | — | No | secret | PagerDuty integration key |
| `STRATA_ALERT_EMAIL_RECIPIENTS` | — | No | comma-separated | Comma-separated email recipients. Requires SMTP configured |

### 2.7 Seeding

| Variable | Default | Required | Type | Description |
|----------|---------|----------|------|-------------|
| `STRATA_SEED_DEV` | `false` | No | bool | Seed dev tenant on startup (rejected in production) |
| `STRATA_DEV_ADMIN_EMAIL` | — | No | string | Dev admin email address |
| `STRATA_DEV_ADMIN_PASSWORD_HASH` | — | No | string | Dev admin password hash |

### 2.8 Backup Configuration

| Variable | Default | Required | Type | Description |
|----------|---------|----------|------|-------------|
| `STRATA_BACKUP_ENABLED` | `false` | No | bool | Enable backup engine |
| `STRATA_BACKUP_ENVIRONMENT_ID` | — | No | string | Environment identifier (required if enabled) |
| `STRATA_BACKUP_DATABASE_TYPE` | `timescaledb` | No | string | Database type: `postgresql`, `timescaledb` |
| `STRATA_BACKUP_DIRECTORY` | — | No | path | Backup directory (for filesystem repository) |
| `STRATA_BACKUP_ENCRYPTION_SCHEME` | `aes-256-gcm` | No | string | Encryption scheme: only `aes-256-gcm` allowed |
| `STRATA_BACKUP_REPOSITORY_TYPE` | `filesystem` | No | string | Backup repository: `filesystem`, `s3` |
| `STRATA_BACKUP_EXTERNAL_BUCKET` | — | No | string | S3 backup bucket |
| `STRATA_BACKUP_EXTERNAL_REGION` | — | No | string | S3 backup region |
| `STRATA_BACKUP_EXTERNAL_ENDPOINT` | — | No | string | S3 backup endpoint |
| `STRATA_BACKUP_EXTERNAL_ACCESS_KEY` | — | No | secret | S3 backup access key |
| `STRATA_BACKUP_EXTERNAL_SECRET_KEY` | — | No | secret | S3 backup secret key |
| `STRATA_BACKUP_KEY_PROVIDER_PATH` | — | No | path | Key provider path (required if enabled) |
| `STRATA_RECOVERY_STORAGE_BACKEND` | — | No | string | Recovery storage backend: `local`, `minio`, `s3` |
| `STRATA_RECOVERY_STORAGE_BUCKET` | — | No | string | Recovery storage bucket |
| `STRATA_RECOVERY_STORAGE_REGION` | — | No | string | Recovery storage region |
| `STRATA_RECOVERY_STORAGE_ENDPOINT` | — | No | string | Recovery storage endpoint |
| `STRATA_RECOVERY_STORAGE_ACCESS_KEY` | — | No | secret | Recovery storage access key |
| `STRATA_RECOVERY_STORAGE_SECRET_KEY` | — | No | secret | Recovery storage secret key |
| `STRATA_RECOVERY_STORAGE_USE_SSL` | `false` | No | bool | Use SSL for recovery storage |
| `STRATA_RECOVERY_NATS_URL` | — | No | URL | Recovery NATS URL |
| `STRATA_RECOVERY_NATS_TOKEN` | — | No | secret | Recovery NATS token |
| `STRATA_RECOVERY_NATS_TLS_CA` | — | No | path | Recovery NATS CA certificate |
| `STRATA_RECOVERY_NATS_TLS_CERT` | — | No | path | Recovery NATS client certificate |
| `STRATA_RECOVERY_NATS_TLS_KEY` | — | No | path | Recovery NATS client key |

---

## 3. Secret File Support

Many secret variables support file-based secrets via `{VARIABLE}_FILE`. This enables K8s Secret mounts, Docker secrets, etc.

**Example:**
```bash
# Direct secret
JWT_SECRET=your-32-char-secret-here

# File-based secret (K8s Secret mount)
JWT_SECRET_FILE=/run/secrets/jwt-secret
```

**Validation:**
- File path must be absolute and canonical (no relative paths, no traversal)
- File must be a regular file
- File size must not exceed 16 KiB
- File must not be empty
- Direct and file modes are mutually exclusive

**Supported variables with `_FILE` suffix:**
- `NATS_TOKEN_FILE`
- `JWT_SECRET_FILE`
- `STORAGE_ACCESS_KEY_FILE`
- `STORAGE_SECRET_KEY_FILE`
- `STRATA_SMTP_USERNAME_FILE`
- `STRATA_SMTP_PASSWORD_FILE`
- `STRATA_ALERT_SLACK_URL_FILE`
- `STRATA_ALERT_TEAMS_URL_FILE`
- `STRATA_ALERT_WEBHOOK_URL_FILE`
- `STRATA_ALERT_PAGERDUTY_KEY_FILE`
- `STRATA_BACKUP_EXTERNAL_ACCESS_KEY_FILE`
- `STRATA_BACKUP_EXTERNAL_SECRET_KEY_FILE`

---

## 4. Validation Rules

### 4.1 General Validation

| Rule | Details |
|------|---------|
| `DB.MaxIdleConns ≤ DB.MaxOpenConns` | Must not exceed open connections |
| `DB.MaxIdleConns ≥ 0` | Must be non-negative |
| `JWT.Secret ≥ 32 chars` | Min 32 characters for HS256 |
| `STRATA_METRICS_TOKEN ≥ 32 chars` | Min 32 characters when set |
| `SMTP.Port 1-65535` | Valid port range |
| `SMTP.FromAddress` | RFC 5322 valid email |
| `SMTP.Username + Password` | Must be configured together |
| `SMTP configured → Email recipients` | Requires SMTP |
| `SMTP configured → PublicURL` | Required for SMTP links |
| `Alert URL` | Must be absolute HTTPS without credentials |

### 4.2 Production Validation (`ProductionValidate()`)

Additional rules when `STRATA_RUNTIME_MODE=production`:

| Rule | Details |
|------|---------|
| `HTTP.PublicURL` | Required, must be HTTPS |
| `CORS origins` | No wildcard (`*`) |
| `NATS.TLS` | TLS required |
| `NATS.TLSCA` | CA file required |
| `NATS auth` | Token or mTLS certificate required |
| `NATS.AdvertiseURLs` | At least one URL required |
| `NATS.AdvertiseURLs` | Must be absolute `tls://` or `nats+tls://` URLs |
| `NATS.AdvertiseURLs` | No localhost, no loopback, no container-local hosts |
| `DB.DSN` | No `sslmode=disable` |
| `DB.DSN` | No default passwords (password, postgres, strata) |
| `JWT.Secret` | No `dev-` or `test-` prefix |
| `TunnelAddr` | Not allowed in production |
| `STRATA_SEED_DEV` | Rejected in production |

### 4.3 Storage Validation

| Rule | Details |
|------|---------|
| `STORAGE_BACKEND` | Required if not `none` or empty |
| `STORAGE_BUCKET` | Required when backend is set |
| `STORAGE_REPOSITORY_TYPE` | Must be `filesystem` or `s3` |
| `STRATA_BACKUP_ENCRYPTION_SCHEME` | Only `aes-256-gcm` allowed |

---

## 5. Configuration Summary (Redacted)

`OrchestratorConfig.RedactedSummary()` returns a redacted view for logging:

```go
map[string]interface{}{
    "runtime_mode":             "production",
    "nats_url":                 "tls://nats.example.com:4222",
    "nats_tls":                 true,
    "db_dsn":                   "postgres://user:***@db.example.com:5432/strata_rmm",
    "db_pool_max":              25,
    "db_pool_idle":             5,
    "redis_url":                "rediss://redis.example.com:6380",
    "redis_pool_size":          10,
    "js_memory_store":          "2GB",
    "js_file_store":            "50GB",
    "js_storage_path":          "/var/lib/strata/jetstream",
    "js_replicas":              1,
    "storage_type":             "s3",
    "storage_bucket":           "strata-recordings",
    "api_addr":                 ":8080",
    "tunnel_addr":              "",
    "public_url":               "https://strata.example.com",
    "cors_origins":             ["https://app.strata.example.com"],
    "jwt_configured":           true,
    "metrics_token_configured": true,
    "smtp_configured":          true,
    "alert_delivery_channels":  4,
    "seed_dev":                 false,
}
```

---

## 6. Quick Start Configuration

### 6.1 Development

```bash
export STRATA_RUNTIME_MODE=development
export JWT_SECRET=dev-secret-that-is-at-least-32-chars-long
# Default DSN: postgres://localhost:5432/strata_rmm
# Default NATS: nats://localhost:4222
```

### 6.2 Docker Compose (Development)

```yaml
services:
  orchestrator:
    image: strata-rmm/orchestrator:latest
    environment:
      STRATA_RUNTIME_MODE: development
      JWT_SECRET: dev-secret-that-is-at-least-32-chars-long
      STRATA_SEED_DEV: "true"
      STRATA_DEV_ADMIN_EMAIL: admin@localhost
      STRATA_DEV_ADMIN_PASSWORD_HASH: hash
    ports:
      - "8080:8080"
    depends_on:
      - nats
      - postgres
```

### 6.3 Production

```bash
export STRATA_RUNTIME_MODE=production
export STRATA_PUBLIC_URL=https://strata.example.com
export JWT_SECRET=production-secret-at-least-32-characters
export NATS_URL=nats+tls://nats.example.com:443
export NATS_TLS_ENABLED=true
export NATS_TLS_CA=/run/secrets/nats-ca.crt
export NATS_TOKEN_FILE=/run/secrets/nats-token
export NATS_ADVERTISE_URLS=tls://nats1.example.com:4222,tls://nats2.example.com:4222
export TIMESCALE_DSN=postgres://user:***@db.example.com:5432/strata_rmm?sslmode=require
export STORAGE_BACKEND=s3
export STORAGE_BUCKET=strata-recordings
export STORAGE_ENDPOINT=https://s3.amazonaws.com
export STORAGE_ACCESS_KEY_FILE=/run/secrets/s3-access-key
export STORAGE_SECRET_KEY_FILE=/run/secrets/s3-secret-key
export STRATA_METRICS_TOKEN=metrics-token-at-least-32-chars
```

---

*Last Updated: 2026-08-08*
