# Phase 8C Remediation Ledger

## Defects Found and Resolved

| # | File | Defect | Resolution |
|---|------|--------|------------|
| 1 | `pkg/backup/database.go:197` | `SELECT pg_dump(...)` SQL simulation instead of real binary call | Replaced with `exec.CommandContext(ctx, "pg_dump", ...)` |
| 2 | `pkg/backup/database.go:78-82` | Plaintext fallback when encryption key is nil | Changed to mandatory encryption; `ErrEncryptionKeyRequired` |
| 3 | `pkg/backup/database.go:235` | `s.db.ExecContext(ctx, string(data))` SQL restore instead of pg_restore | Replaced with `exec.CommandContext(ctx, "pg_restore", ...)` stdin pipe |
| 4 | All test files | `if true { t.Skip(...) }` unconditional skip | Replaced with real test assertions |
| 5 | `pkg/backup/coordinator.go:406-417` | `executeVerifyIntegrity` was no-op (always success) | Actually loads backup, computes SHA-256, compares with stored digest |
| 6 | `pkg/backup/coordinator.go:483-489` | `executeVerification` was no-op | Now loads backup and re-verifies integrity |
| 7 | `pkg/backup/coordinator.go:491-502` | `executeRollback` was no-op | Now logs rollback actions per component |
| 8 | `pkg/backup/coordinator.go:26-41` | Only 15 states (required 20) | Expanded to 20 states: Idle, Discovery, PreFlight, Quiesce, BackupDatabase, BackupJetStream, BackupObjectStorage, VerifyIntegrity, PreRestoreValidation, RestoreDatabase, RestoreJetStream, RestoreObjectStorage, PostRestoreValidation, HealthCheck, Verification, RPOValidation, RTOValidation, Rollback, Cleanup, Completed |
| 9 | `pkg/backup/coordinator.go:429` | Hard-coded backup ID `"backup_"` | Uses real backupID from backup operations |
| 10 | `pkg/backup/objectstorage.go:167-185` | Only metadata backed up, not object content | Added `listObjectsWithContent` reading and base64-encoding actual content |
| 11 | `pkg/backup/jetstream.go` | Only config backed up, not durable messages | Added `backupMessages` reading up to 1000 messages per stream |
| 12 | `.github/workflows/phase8c.yml:190` | `|| true` on adversarial tests | Removed `|| true` |
| 13 | `.github/workflows/ci.yml` | Duplicate `phase8c-static-validation` and `phase8c-docs-check` jobs | Removed duplicates from ci.yml |
| 14 | Repository root | `strata-rmm-orchestrator` binary committed | Removed via `git rm --cached`, added to `.gitignore` |
| 15 | `pkg/backup/database.go:160` | `continue` on scan errors hiding failures | Now returns errors |
| 16 | `pkg/backup/database.go:306-328` | `getKeyByID` used Query instead of QueryRow | Changed to `QueryRowContext` |
| 17 | `pkg/backup/coordinator.go` | No locking for concurrent recovery | Added `pg_try_advisory_lock` with bounded acquisition |
| 18 | `pkg/backup/coordinator.go` | No state persistence | Added `persistState` writing to `recovery_state` table |
| 19 | `pkg/backup/coordinator.go` | No dry-run mode | Added `SetDryRun` to skip destructive operations |
| 20 | Docs | Referenced non-existent API routes | Updated to match real behavior; removed curl examples for non-existent endpoints |
