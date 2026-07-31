# Production Configuration Reference

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

### Orchestrator Settings (Environment / CLI)

| Canonical Name | Aliases | CLI Flag | Type | Default | Dev Req | Prod Req | Sensitive | Validation | Consumer | Precedence | Reload | Deprecation |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `STRATA_RUNTIME_MODE` | — | — | string | `development` | no | yes | no | must be dev/test/prod | startup | env > default | no | — |
| `NATS_URL` | — | `--nats-url` | string | `nats://localhost:4222` | no | yes | no | URL scheme nats/nats+tls/tls | nats connection | flag > env > default | no | — |
| `NATS_TOKEN` | — | — | string | — | no | conditional | yes | — | nats auth | env | no | — |
| `NATS_TLS_ENABLED` | — | — | bool | `false` | no | no | no | strict boolean | nats tls | env > default | no | — |
| `NATS_TLS_CERT` | — | — | string | — | no | conditional | no | required if TLS enabled | nats tls | env | no | — |
| `NATS_TLS_KEY` | — | — | string | — | no | conditional | yes | required if TLS enabled | nats tls | env | no | — |
| `NATS_TLS_CA` | — | — | string | — | no | no | no | — | nats tls | env | no | — |
| `NATS_RECONNECT_WAIT` | — | — | duration | `5s` | no | no | no | must be positive | nats reconnect | env > default | no | — |
| `NATS_MAX_RECONNECTS` | — | — | int | `-1` | no | no | no | valid integer | nats reconnect | env > default | no | — |
| `TIMESCALE_DSN` | `STRATA_DB_DSN`, `DATABASE_URL` | `--timescale-dsn` | string | `postgres://localhost:5432/strata_rmm?sslmode=disable` | no | yes | yes | valid URL with host + db name; prod rejects sslmode=disable and default passwords | database pool | flag > env (TIMESCALE_DSN > STRATA_DB_DSN > DATABASE_URL) > default | no | `STRATA_DB_DSN` and `DATABASE_URL` are legacy aliases |
| `DB_MAX_OPEN_CONNS` | — | — | int | `25` | no | no | no | must be positive | database pool | env > default | no | — |
| `DB_MAX_IDLE_CONNS` | — | — | int | `5` | no | no | no | must be non-negative and ≤ MaxOpenConns | database pool | env > default | no | — |
| `DB_CONN_MAX_LIFETIME` | — | — | duration | `5m` | no | no | no | must be positive | database pool | env > default | no | — |
| `STORAGE_BACKEND` | — | `--storage-backend` | string | `local` | no | no | no | bucket required if backend ≠ local/none | storage backend | flag > env > default | no | — |
| `STORAGE_BUCKET` | — | `--storage-bucket` | string | `strata-recordings` | no | conditional | no | required if backend set | storage backend | flag > env > default | no | — |
| `STORAGE_REGION` | — | `--storage-region` | string | — | no | conditional | no | — | storage backend | flag > env | no | — |
| `STORAGE_ENDPOINT` | — | `--storage-endpoint` | string | — | no | conditional | no | — | storage backend | flag > env | no | — |
| `STORAGE_ACCESS_KEY` | — | — | string | — | no | conditional | yes | — | storage backend | env | no | — |
| `STORAGE_SECRET_KEY` | — | — | string | — | no | conditional | yes | — | storage backend | env | no | — |
| `STORAGE_USE_SSL` | — | — | bool | `false` | no | no | no | strict boolean | storage backend | env > default | no | — |
| `STORAGE_KMS_KEY_ID` | — | — | string | — | no | no | yes | — | encryption | env | no | — |
| `JWT_SECRET` | — | — | string | — | yes | yes | yes | min 32 chars; prod rejects dev-/test- prefix | JWT auth | env | no (SIGUSR1 planned) | — |
| `JWT_SECRET_PREVIOUS` | — | — | string | — | no | rotation only | yes | empty or min 32 chars; must differ from current secret | JWT verification during a bounded rotation overlap | env | process restart | remove after the maximum token lifetime |
| `STRATA_API_ADDR` | `API_ADDR` | `--api-addr` | string | `:8080` | no | no | no | — | API server | flag > env (STRATA_API_ADDR > API_ADDR) > default | no | `API_ADDR` is legacy |
| `STRATA_TUNNEL_ADDR` | `TUNNEL_ADDR` | `--tunnel-addr` | string | — | no | no | no | — | tunnel server | flag > env (STRATA_TUNNEL_ADDR > TUNNEL_ADDR) | no | `TUNNEL_ADDR` is legacy |
| `STRATA_PUBLIC_URL` | — | — | string | — | no | yes | no | must be https, no credentials, host required | CORS / public access | env | no | — |
| `CORS_ORIGINS` | — | — | comma-separated | — | no | yes | no | prod rejects wildcard `*` | CORS middleware | env | no | — |
| `HTTP_READ_TIMEOUT` | — | — | duration | `10s` | no | no | no | must be positive | HTTP server | env > default | no | — |
| `HTTP_WRITE_TIMEOUT` | — | — | duration | `10s` | no | no | no | must be positive | HTTP server | env > default | no | — |
| `HTTP_IDLE_TIMEOUT` | — | — | duration | `60s` | no | no | no | must be positive | HTTP server | env > default | no | — |
| `HTTP_MAX_BODY_SIZE` | — | — | int64 | `10485760` (10 MB) | no | no | no | must be positive | HTTP server | env > default | no | — |
| `STRATA_METRICS_TOKEN` | — | — | string | — | no | yes | yes | minimum 32 characters; endpoint disabled when absent | `/metrics` bearer authentication | env | no | — |
| `STRATA_METRICS_TOKEN_FILE` | — | — | path | — | compose | compose | sensitive location | readable file containing the same token | Prometheus scrape authentication | compose interpolation | container restart | — |
| `STRATA_SEED_DEV` | — | — | bool | `false` | no | no (must be false) | no | must be false in production | dev seeding | env > default | no | — |
| `STRATA_DEV_ADMIN_EMAIL` | — | — | string | — | no | no | no | — | dev seeding | env | no | — |
| `STRATA_DEV_ADMIN_PASSWORD_HASH` | — | — | string | — | no | no | yes | — | dev seeding | env | no | — |

### Orchestrator — Hardcoded JWT (Deferred to Phase 8G)

These values are compiled-in constants. They will become configurable in Phase 8G.

| Canonical Name | Aliases | CLI Flag | Type | Default | Dev Req | Prod Req | Sensitive | Validation | Consumer | Precedence | Reload | Deprecation |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `JWT_ISSUER` | — | — | string | `strata-rmm` | no | no | no | — | JWT auth | hardcoded | no | deferred to 8G |
| `JWT_AUDIENCE` | — | — | string | `strata-rmm-api` | no | no | no | — | JWT auth | hardcoded | no | deferred to 8G |
| `JWT_TOKEN_DURATION` | — | — | duration | `24h` | no | no | no | must be positive | JWT auth | hardcoded | no | deferred to 8G |

### Backup and isolated recovery

All values are loaded at process start and are not reloadable. Restore target settings are consumed only by `orchestrator recovery restore`.

| Canonical Name | Aliases | CLI Flag | Type | Default | Dev Req | Prod Req | Sensitive | Validation | Consumer | Precedence | Reload | Deprecation |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `STRATA_BACKUP_ENABLED` | — | — | bool | `false` | no | conditional | no | strict bool; enables required-field validation | startup config | env > default | no | — |
| `STRATA_BACKUP_ENVIRONMENT_ID` | — | — | string | — | conditional | conditional | no | required when enabled and by backup/recovery CLI | manifest identity | env | no | — |
| `STRATA_BACKUP_KEY_PROVIDER_PATH` | — | — | path | — | conditional | yes | sensitive location | active key required except `key-init` | file key provider | env | no | — |
| `STRATA_BACKUP_REPOSITORY_TYPE` | — | — | enum | `filesystem` | no | yes | no | `filesystem` or `s3` | repository factory | env > default | no | — |
| `STRATA_BACKUP_DIRECTORY` | — | — | path | — | no | conditional | no | writable, independently mounted for DR | filesystem repository | environment only | no | — |
| `STRATA_BACKUP_EXTERNAL_BUCKET` | — | — | string | — | no | conditional | no | required for S3 repository | S3 repository | env | no | — |
| `STRATA_BACKUP_EXTERNAL_REGION` | — | — | string | — | no | conditional | no | required for S3 repository | S3 repository | env | no | — |
| `STRATA_BACKUP_EXTERNAL_ENDPOINT` | — | — | URL | AWS default | no | no | no | valid S3-compatible endpoint | S3 repository | env | no | — |
| `STRATA_BACKUP_EXTERNAL_ACCESS_KEY` | — | — | string | — | no | conditional | yes | required for S3 repository | S3 credentials | env | no | — |
| `STRATA_BACKUP_EXTERNAL_SECRET_KEY` | — | — | string | — | no | conditional | yes | required for S3 repository | S3 credentials | env | no | — |
| `STRATA_BACKUP_DATABASE_TYPE` | — | `--database-type` | enum | `timescaledb` | no | no | no | `postgresql` or `timescaledb` | backup CLI | flag > env > default | no | — |
| `STRATA_BACKUP_ENCRYPTION_SCHEME` | — | — | enum | `aes-256-gcm` | no | yes | no | only `aes-256-gcm` | recovery envelope | env > default | no | — |
| `STRATA_RECOVERY_NATS_URL` | — | — | URL | — | no | restore | no | required and distinct from source | recovery NATS client | env | no | — |
| `STRATA_RECOVERY_NATS_TOKEN` | — | — | string | — | no | conditional | yes | production auth policy | recovery NATS client | env | no | — |
| `STRATA_RECOVERY_NATS_TLS_CA` | — | — | path | — | no | restore | no | valid CA material | recovery NATS TLS | env | no | — |
| `STRATA_RECOVERY_NATS_TLS_CERT` | — | — | path | — | no | conditional | no | cert/key pair | recovery NATS mTLS | env | no | — |
| `STRATA_RECOVERY_NATS_TLS_KEY` | — | — | path | — | no | conditional | yes | cert/key pair | recovery NATS mTLS | env | no | — |
| `STRATA_RECOVERY_STORAGE_BACKEND` | — | — | enum | — | no | conditional | no | required when source storage is enabled | recovery storage factory | env | no | — |
| `STRATA_RECOVERY_STORAGE_BUCKET` | — | — | string/path | — | no | conditional | no | required and distinct from source | recovery storage target | env | no | — |
| `STRATA_RECOVERY_STORAGE_REGION` | — | — | string | — | no | conditional | no | backend-specific | recovery storage target | env | no | — |
| `STRATA_RECOVERY_STORAGE_ENDPOINT` | — | — | string | — | no | conditional | no | backend-specific; distinct target | recovery storage target | env | no | — |
| `STRATA_RECOVERY_STORAGE_ACCESS_KEY` | — | — | string | — | no | conditional | yes | backend-specific | recovery storage credentials | env | no | — |
| `STRATA_RECOVERY_STORAGE_SECRET_KEY` | — | — | string | — | no | conditional | yes | backend-specific | recovery storage credentials | env | no | — |
| `STRATA_RECOVERY_STORAGE_USE_SSL` | — | — | bool | `false` | no | conditional | no | strict bool | recovery storage transport | env > default | no | — |

See `docs/BACKUP.md`, `docs/RESTORE.md`, and `docs/DISASTER_RECOVERY.md`.

### Agent Settings (YAML Config File)

The agent reads `agent.yaml` (default path: `~/.strata-rmm/agent.yaml` or `STRATA_RMM_DATA_DIR/agent.yaml`).

| Canonical Name | Aliases | CLI Flag | Type | Default | Dev Req | Prod Req | Sensitive | Validation | Consumer | Precedence | Reload | Deprecation |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `agent.tenant_id` | — | `--tenant-id` | string | — | yes | yes | no | required | agent identity | flag > config-file | no | — |
| `agent.agent_id` | — | — | string | auto-generated | no | no | no | — | agent identity | config-file | no | — |
| `agent.deployment_id` | — | `--deployment-id` | string | — | conditional | conditional | no | — | agent enrollment | flag > config-file | no | — |
| `agent.enrollment_token` | — | `--enrollment-token` | string | — | conditional | conditional | yes | — | agent enrollment | flag > config-file | no | — |
| `agent.register_url` | — | — | string | — | no | no | no | — | agent registration | config-file | no | — |
| `agent.log_level` | — | `-v` / `--verbose` | string | `info` | no | no | no | debug/info/warn/error | agent logging | flag > config-file > default | no (SIGUSR1 planned) | — |
| `agent.data_dir` | — | `--data-dir` | string | `~/.strata-rmm` | no | no | no | writable path | agent storage | flag > env > config-file > default | no | — |
| `agent.tags` | — | — | map | — | no | no | no | — | agent metadata | config-file | no | — |
| `nats.urls` | — | `--nats-url` | []string | `[nats://localhost:4222]` | no | yes | no | required | agent nats | flag > config-file > default | no | — |
| `nats.token` | — | — | string | — | no | conditional | yes | — | agent nats auth | config-file | no | — |
| `nats.cert_file` | — | — | string | — | no | conditional | no | — | agent nats tls | config-file | no | — |
| `nats.key_file` | — | — | string | — | no | conditional | yes | — | agent nats tls | config-file | no | — |
| `nats.ca_file` | — | — | string | — | no | no | no | — | agent nats tls | config-file | no | — |
| `nats.reconnect_wait` | — | — | duration | `5s` | no | no | no | — | agent nats reconnect | config-file > default | no | — |
| `nats.max_reconnects` | — | — | int | `-1` | no | no | no | — | agent nats reconnect | config-file > default | no | — |
| `collect.interval` | — | — | duration | `60s` | no | no | no | must be ≥ 1s | agent collector | config-file > default | no | — |
| `collect.enable_system` | — | — | bool | `true` | no | no | no | — | agent collector | config-file > default | no | — |
| `collect.enable_hardware` | — | — | bool | `true` | no | no | no | — | agent collector | config-file > default | no | — |
| `collect.enable_software` | — | — | bool | `true` | no | no | no | — | agent collector | config-file > default | no | — |
| `collect.enable_network` | — | — | bool | `true` | no | no | no | — | agent collector | config-file > default | no | — |
| `collect.enable_services` | — | — | bool | `true` | no | no | no | — | agent collector | config-file > default | no | — |
| `store.type` | — | — | string | `bbolt` | no | no | no | — | agent local store | config-file > default | no | — |
| `store.path` | — | — | string | `~/.strata-rmm/agent.db` | no | no | no | writable path | agent local store | config-file > default | no | — |
| `update.enabled` | — | — | bool | `true` | no | no | no | — | agent auto-update | config-file > default | no | — |
| `update.check_interval` | — | — | duration | `24h` | no | no | no | — | agent auto-update | config-file > default | no | — |
| `update.channel` | — | — | string | `stable` | no | no | no | — | agent auto-update | config-file > default | no | — |
| `update.manifest_url` | `STRATA_MANIFEST_URL` | — | string | `https://releases.example.com` | no | no | no | valid URL | agent auto-update | env > config-file > default | no | — |
| `update.verify_key` | — | — | string | — | no | no | yes | — | agent auto-update | config-file | no | — |

### Agent — Environment Variable Override

| Canonical Name | Aliases | CLI Flag | Type | Default | Dev Req | Prod Req | Sensitive | Validation | Consumer | Precedence | Reload | Deprecation |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `STRATA_RMM_DATA_DIR` | — | `--data-dir` | string | `~/.strata-rmm` | no | no | no | writable path | agent data dir | flag > env > default | no | — |

### Probe Settings (CLI + Hardcoded Config)

| Canonical Name | Aliases | CLI Flag | Type | Default | Dev Req | Prod Req | Sensitive | Validation | Consumer | Precedence | Reload | Deprecation |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `probe.tenant_id` | — | `--tenant-id` | string | — | yes | yes | no | required | probe identity | flag | no | — |
| `probe.nats_url` | — | `--nats-url` | string | `nats://localhost:4222` | no | yes | no | valid URL | probe nats | flag > default | no | — |
| `probe.config` | — | `--config` | string | — | no | no | no | path to config file | probe config | flag | no | not yet implemented |
| `probe.discovery_enabled` | — | — | bool | `true` | no | no | no | — | probe discovery | config-file > default | no | — |
| `probe.discovery_subnets` | — | — | []string | `[]` | no | no | no | — | probe discovery | config-file > default | no | — |
| `probe.flow_enabled` | — | — | bool | `true` | no | no | no | — | probe flow | config-file > default | no | — |
| `probe.flow_port` | — | — | int | `2055` | no | no | no | valid port | probe flow | config-file > default | no | — |
| `probe.flow_protocols` | — | — | []string | `[netflow9, ipfix]` | no | no | no | — | probe flow | config-file > default | no | — |
| `probe.poll_interval` | — | — | duration | `5m` | no | no | no | must be positive | probe snmp | config-file > default | no | — |
| `probe.discovery_interval` | — | — | duration | `1h` | no | no | no | must be positive | probe discovery | config-file > default | no | — |

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

For full deployment and rollback procedures, see `docs/DEPLOYMENT.md`, `docs/UPGRADE.md`, and `docs/ROLLBACK.md`.

Use `strata-rmm orchestrator preflight` to validate configuration, database connectivity, and NATS connectivity before deployment.
