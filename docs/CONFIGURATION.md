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

## Core Settings

| Variable | Flag | Default | Production Required | Sensitive |
|----------|------|---------|-------------------|-----------|
| `STRATA_RUNTIME_MODE` | — | `development` | yes | no |
| `STRATA_API_ADDR` / `API_ADDR` | — | `:8080` | yes | no |
| `STRATA_TUNNEL_ADDR` / `TUNNEL_ADDR` | — | `:8443` | no | no |
| `STRATA_PUBLIC_URL` | — | — | yes | no |
| `JWT_SECRET` | — | — | yes | **yes** |
| `NATS_URL` | `--nats-url` | `nats://localhost:4222` | yes | no |
| `NATS_TOKEN` | — | — | if auth required | **yes** |
| `TIMESCALE_DSN` / `STRATA_DB_DSN` / `DATABASE_URL` | `--timescale-dsn` | `postgres://localhost:5432/strata_rmm?sslmode=disable` | yes | **yes** |
| `DB_MAX_OPEN_CONNS` | — | `25` | no | no |
| `DB_MAX_IDLE_CONNS` | — | `5` | no | no |
| `CORS_ORIGINS` | — | — | yes | no |
| `STRATA_SEED_DEV` | — | `false` | no | no |

## Storage Settings

| Variable | Flag | Default | Sensitive |
|----------|------|---------|-----------|
| `STORAGE_BACKEND` | `--storage-backend` | `local` | no |
| `STORAGE_BUCKET` | `--storage-bucket` | `strata-recordings` | no |
| `STORAGE_REGION` | `--storage-region` | — | no |
| `STORAGE_ENDPOINT` | `--storage-endpoint` | — | no |
| `STORAGE_ACCESS_KEY` | — | — | **yes** |
| `STORAGE_SECRET_KEY` | — | — | **yes** |
| `STORAGE_USE_SSL` | — | `false` | no |
| `STORAGE_KMS_KEY_ID` | — | — | **yes** |

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
