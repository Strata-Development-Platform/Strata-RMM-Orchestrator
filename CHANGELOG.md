# Changelog

All notable changes to this project should be documented here.

The format follows Keep a Changelog principles, and releases should use semantic versioning where applicable.

## [0.2.0] - 2026-07-25

### Added
- **StorageBackend abstraction** (`pkg/storage/`): `Backend` interface with MinIO, AWS S3, Local, and Mock implementations. Upload/download/delete/exists/presigned URL/stat/list with SHA256 checksum verification, SSE-KMS/SSE-S3 encryption, multipart upload
- **Session Recording** (`internal/remote/recorder.go`): Raw byte-level recording pipeline integrated into the tunnel gateway. `SessionRecorder` captures tunnel down-stream data, computes SHA256, streams upload to storage backend
- **Recording CRUD API**: `GET /api/v1/recordings/{tenantID}`, `GET /api/v1/recordings/{id}/playback` (presigned URL), `DELETE /api/v1/recordings/{id}`
- **Recording retention cleanup** (`internal/remote/cleanup.go`): Scheduled job (default 24h) that deletes expired recordings from both storage and DB
- **Database migration 16**: `session_recordings` table with RLS, indexes, expiry support
- **MFA/TOTP** (`pkg/auth/totp.go`, `pkg/auth/mfa.go`): RFC 6238 TOTP with HMAC-SHA1, 6-digit codes, 30s window, ±1 step skew tolerance. PostgreSQL-backed secret store with RLS. MFA gate on playback endpoint (`X-MFA-Code` header)
- **MFA API endpoints**: `POST /api/v1/mfa/enroll/{userID}`, `POST /api/v1/mfa/verify/{userID}`, `GET /api/v1/mfa/status/{userID}`, `DELETE /api/v1/mfa/{userID}`
- **Database migration 17**: `mfa_secrets` table with RLS
- **Agent Auto-Update** (`internal/agent/update/`): Update manifest schema, BBolt state persistence, download with SHA256 verification, atomic binary replacement, rollback with backup, staged rollout via NATS commands (pause/resume, canary %, version approval/blocking)
- **Version subcommand**: `strata-rmm version --output=json` for programmatic version checking (used by update client's VerifyAndSwitch)
- **GoReleaser config** (`.goreleaser.yml`): Multi-platform builds (agent/orchestrator/probe), cosign signing, SPDX SBOM generation
- **CI/CD workflows** (`.github/workflows/`): `ci.yml` (lint, vet, test with race, matrix build, Trivy + Gosec), `release.yml` (goreleaser, cosign keyless signing, multi-arch Docker to GHCR, SBOM attestation)
- **Helm CRD**: `AgentUpdateChannel` custom resource definition with spec (channel, rolloutPercent, paused, approvedVersion, blockedVersions) and status
- **GitHub issue templates**: `sprint_task.yml` (standardized sprint card), `security_report.yml` (confidential vulnerability report), `PULL_REQUEST_TEMPLATE.md`

### Changed
- `main.go`: Added `version` ldflags (`version`, `commit`, `date`), version subcommand with JSON output
- `Makefile`: Updated with VERSION/COMMIT/DATE ldflags, `goreleaser` target
- `go.mod`: Upgraded to Go 1.25, added minio-go/v7, AWS SDK v2 (s3, config, credentials)
- `internal/platform/api.go`: Added MFA, recording store, and storage backend support
- `internal/remote/tunnel.go`: Gateway now supports optional recording with `WithRecorder`/`WithRecordingStore`

### Fixed
- `internal/alerting/rules.go:81`: Fixed `fmt.Sprintf` format verb mismatch (%.2f for string deviceID)
- Renumbered session_recordings migration from 16 to 17 to accommodate mfa_secrets migration

## [0.1.0] - 2026-07-25

### Added
- Architecture plan (docs/ARCHITECTURE.md)
- StrataLabs project template integration
- Core platform CLI with Cobra
- Agent, Probe, and Orchestrator command structure
- GitHub issue templates (bug report, feature request)
- Project roadmap (.stratalabs/roadmap.json)
