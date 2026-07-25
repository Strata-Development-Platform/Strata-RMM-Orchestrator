# Strata RMM SOC 2 Compliance Evidence

## CC6.1 — Logical Access
| Evidence | Location |
|----------|----------|
| JWT auth (HS256) | `pkg/auth/jwt.go` |
| MFA/TOTP (RFC 6238) | `pkg/auth/totp.go` |
| API key auth | `pkg/auth/ratelimit.go` |
| Rate limiting (IP token bucket) | `pkg/auth/ratelimit.go` |

## CC6.2 — User Access Provisioning
| Evidence | Location |
|----------|----------|
| User CRUD API | `GET /api/v1/access/users/{tenantID}` |
| RBAC (admin/technician/viewer) | `pkg/postgres/schema.go` |
| Permission review API | `GET /api/v1/access/permissions/{tenantID}` |

## CC6.4 — Data Access Restriction
| Evidence | Location |
|----------|----------|
| Row-Level Security (all tables) | `pkg/postgres/schema.go` |
| NATS subject isolation | `tenant.{id}.*` patterns |
| Per-tenant encryption keys | `pkg/encrypt/keys.go` |

## CC6.6 — Encryption at Rest
| Evidence | Location |
|----------|----------|
| SSE-KMS (S3) | `pkg/storage/s3_backend.go` |
| SSE-S3 (MinIO) | `pkg/storage/minio_backend.go` |
| AES-256-GCM tenant keys | `pkg/encrypt/keys.go` |

## CC6.7 — Data Disposal
| Evidence | Location |
|----------|----------|
| Recording retention (90d) + cleanup | `internal/remote/cleanup.go` |
| Key rotation lifecycle | `pkg/encrypt/keys.go` |

## CC7.2 — Monitoring
| Evidence | Location |
|----------|----------|
| Audit log (append-only) | `api/v1/access/audit/{tenantID}` |
| Alerting engine | `internal/alerting/engine.go` |
| Heartbeat monitoring | `internal/monitoring/ingestion.go` |

## CC7.4 — Change Detection
| Evidence | Location |
|----------|----------|
| Versioned DB migrations | `pkg/postgres/schema.go` |
| Cosign-signed releases | `.goreleaser.yml` |
| CI/CD (build/test/lint/scan) | `.github/workflows/ci.yml` |
