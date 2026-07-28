# Configuration Reference (Phase 8A)

## Runtime Mode

`STRATA_RUNTIME_MODE` — Sets the runtime mode.

| Value | Behavior |
|-------|----------|
| `development` or `dev` (default) | Relaxed validation, development defaults |
| `test` | Isolated test mode |
| `production` or `prod` | Strict validation, fail-closed |

Production mode rejects:
- Placeholder or short JWT secrets
- `sslmode=disable` (unless explicitly overridden by policy)
- Default database passwords
- Wildcard CORS origins when credentials are supported
- Public URLs without HTTPS
- SeedDev enabled

## Exhaustive Configuration Inventory

| Setting | Aliases | Type | Default | Dev Required | Prod Required | Sensitive | Validation | Consumer |
|---------|---------|------|---------|-------------|--------------|-----------|------------|----------|
| `STRATA_RUNTIME_MODE` | — | string | `development` | no | yes | no | must be dev/test/prod | orchestrator startup |
| `NATS_URL` | — | string | `nats://localhost:4222` | no | yes | no | URL scheme nats/nats+tls/tls | NATS connection |
| `NATS_TOKEN` | — | string | — | no | conditional | yes | none | NATS auth |
| `NATS_TLS_ENABLED` | — | bool | `false` | no | no | no | strict boolean parse | NATS TLS |
| `NATS_TLS_CERT` | — | string | — | no | conditional | no | required if TLS enabled | NATS TLS |
| `NATS_TLS_KEY` | — | string | — | no | conditional | yes | required if TLS enabled | NATS TLS |
| `NATS_TLS_CA` | — | string | — | no | no | no | none | NATS TLS |
| `NATS_RECONNECT_WAIT` | — | duration | `5s` | no | no | no | must be positive | NATS reconnect |
| `NATS_MAX_RECONNECTS` | — | int | `-1` | no | no | no | valid integer | NATS reconnect |
| `TIMESCALE_DSN` | `STRATA_DB_DSN`, `DATABASE_URL` | string | `postgres://localhost:5432/strata_rmm?sslmode=disable` | no | yes | yes | valid URL with host + db name; prod rejects sslmode=disable and default passwords | database pool |
| `DB_MAX_OPEN_CONNS` | — | int | `25` | no | no | no | must be positive | database pool |
| `DB_MAX_IDLE_CONNS` | — | int | `5` | no | no | no | must be non-negative and ≤ MaxOpenConns | database pool |
| `DB_CONN_MAX_LIFETIME` | — | duration | `5m` | no | no | no | must be positive | database pool |
| `STORAGE_BACKEND` | — | string | `local` | no | no | no | bucket required if backend ≠ local/none | storage backend |
| `STORAGE_BUCKET` | — | string | `strata-recordings` | no | conditional | no | required if backend set | storage backend |
| `STORAGE_REGION` | — | string | — | no | conditional | no | none | storage backend |
| `STORAGE_ENDPOINT` | — | string | — | no | conditional | no | none | storage backend |
| `STORAGE_ACCESS_KEY` | — | string | — | no | conditional | yes | none | storage backend |
| `STORAGE_SECRET_KEY` | — | string | — | no | conditional | yes | none | storage backend |
| `STORAGE_USE_SSL` | — | bool | `false` | no | no | no | strict boolean parse | storage backend |
| `STORAGE_KMS_KEY_ID` | — | string | — | no | no | yes | none | encryption |
| `JWT_SECRET` | — | string | — | yes | yes | yes | min 32 chars; prod rejects dev-/test- prefix | JWT auth |
| `JWT_ISSUER` | — | string | `strata-rmm` | no | no | no | none | JWT auth |
| `JWT_AUDIENCE` | — | string | `strata-rmm-api` | no | no | no | none | JWT auth |
| `JWT_TOKEN_DURATION` | — | duration | `24h` | no | no | no | must be positive | JWT auth |
| `STRATA_API_ADDR` | `API_ADDR` | string | `:8080` | no | no | no | none | API server |
| `STRATA_TUNNEL_ADDR` | `TUNNEL_ADDR` | string | — | no | no | no | none | tunnel server |
| `STRATA_PUBLIC_URL` | — | string | — | no | yes | no | must be https, no credentials, host required | CORS / public access |
| `CORS_ORIGINS` | — | string | — | no | yes | no | prod rejects wildcard `*` | CORS middleware |
| `TRUSTED_PROXIES` | — | string | — | no | no | no | comma-separated list | proxy middleware |
| `HTTP_READ_TIMEOUT` | — | duration | `10s` | no | no | no | must be positive | HTTP server |
| `HTTP_WRITE_TIMEOUT` | — | duration | `10s` | no | no | no | must be positive | HTTP server |
| `HTTP_IDLE_TIMEOUT` | — | duration | `60s` | no | no | no | must be positive | HTTP server |
| `HTTP_MAX_BODY_SIZE` | — | int64 | `10485760` (10 MB) | no | no | no | must be positive | HTTP server |
| `STRATA_SEED_DEV` | — | bool | `false` | no | no | no | must be false in production | dev seeding |
| `STRATA_DEV_ADMIN_EMAIL` | — | string | — | no | no | no | none | dev seeding |
| `STRATA_DEV_ADMIN_PASSWORD_HASH` | — | string | — | no | no | yes | none | dev seeding |

## Startup Sequence

1. Load raw configuration from environment
2. Validate mode-independent requirements
3. Validate production policy when applicable
4. Log redacted configuration summary
5. Initialize JWT
6. Connect NATS (with authentication and TLS if configured)
7. Connect database and apply migrations
8. Start ingestion pipeline
9. Start alerting engine
10. Start vulnerability engines
11. Initialize storage backend
12. Start job dispatcher
13. Start API server and mark ready

## Health Endpoints

| Endpoint | Type | Returns |
|----------|------|---------|
| `GET /health` | Readiness | `200 OK` with `{"status":"ok","ready":"true"}` when initialized; `"status":"starting"` during startup |
| `GET /health?liveness=1` | Liveness | `200 OK` with `{"status":"alive"}` — never checks dependencies |
| `GET /health?mode=full` | Diagnostic | Includes runtime mode information |

## Migration

Existing installations should set `STRATA_RUNTIME_MODE` to the appropriate value.
The default is `development`, preserving backward compatibility. Production
deployments must explicitly set it to `production`.

## Rollback

To revert to the previous startup behavior, remove or unset `STRATA_RUNTIME_MODE`.
The orchestrator will default to `development` mode with relaxed validation.
