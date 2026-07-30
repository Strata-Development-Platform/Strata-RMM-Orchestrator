# PHASE 8C REMEDIATION LEDGER
# Date: 2026-07-30
# Status: IN_PROGRESS
# Base SHA: 0368cbe5ef0a203ef2907d371e2fa4a8aadc1c6c
# Current Head SHA: 41dede05714144522a4f3737c88f853f816371f0

## CRITICAL BLOCKERS IDENTIFIED

1. **No runtime wiring** - pkg/backup is not imported or instantiated
2. **Backup tables have no migrations** - backups, jetstream_backups, object_storage_backups, recovery_state tables missing
3. **Database backups stored in database being protected** - encrypted dump and keys disappear with PostgreSQL
4. **Recovery restores onto live source DSN** - no isolated target requirement
5. **Recovery lock doesn't prevent concurrency** - uses different lock per recovery
6. **Missing components silently pass** - nil database, JetStream, object storage pass silently
7. **Component backup identifiers lost** - only database backup ID stored
8. **Recovery verification superficial** - only pings PostgreSQL, doesn't verify application state
9. **JetStream unsafe** - CI starts NATS without -js, encryption falls back to plaintext
10. **Object backup not scalable** - loads all objects into memory as base64 JSON
11. **Database command safety incomplete** - DSNs passed on command line, raw stderr with credentials
12. **Tests largely structural** - no destructive restore drill with real tenant data
13. **CI damaged** - removed 36 jobs from Phase 8B baseline
14. **Exact-head CI failing** - Lint, Security Scan, Phase 8B Deployment Template Rendering

## REQUIRED CHANGES

### Immediate Scope Restoration
- [ ] Restore unrelated source files to base branch contents
- [ ] Restore full Phase 8B CI suite (36 jobs)
- [ ] Add Phase 8C jobs without deleting existing jobs
- [ ] Remove formatting-only changes from unrelated files

### Database Backup & Restore
- [ ] Add migrations for backup catalog tables
- [ ] Store encrypted artifacts externally (not in protected database)
- [ ] Require explicit target DSN distinct from source
- [ ] Sanitize stderr and errors (redact credentials)
- [ ] Use secure DSN passing (env vars or temp files, not command line)
- [ ] Verify source and target compatibility
- [ ] Test full restore with seeded tenant data

### JetStream Recovery
- [ ] Start NATS with -js flag in CI
- [ ] Verify JetStream capability through real server operation
- [ ] Require encryption (never fallback to plaintext)
- [ ] Remove 1,000 message cap
- [ ] Preserve consumer delivery and acknowledgement state
- [ ] Add real integration tests

### Object Storage Recovery
- [ ] Implement streaming backup (not in-memory base64 marshal)
 - [ ] Stream to external backend
 - [ ] Compute SHA-256 digests while streaming
 - [ ] Encrypt while streaming in bounded chunks
 - [ ] Preserve tenant ownership and metadata
 - [ ] Test with large objects and tenant isolation

### Recovery Locking
- [ ] Use stable environment-scoped advisory lock key
- [ ] Use pinned *sql.Conn (not connection pool)
 - [ ] Propagate unlock and connection close errors
 - [ ] Cleanup on success, failure, cancellation, panic
 - [ ] Test concurrency (second recovery blocked)

### Recovery Coordinator
- [ ] Fail closed when required stores are nil
- [ ] Implement real authorization
- [ ] Implement real quiescing (not just logging)
- [ ] Implement real application verification
- [ ] Use backup-set manifest with distinct component IDs
- [ ] Measure real RPO/RTO from drill
- [ ] Implement real rollback (not just logging)

### Runtime Wiring
- [ ] Create authenticated administrative CLI
- [ ] Connect backup operations to operator interface
- [ ] Implement create backup, list, validate, plan, dry-run, execute
- [ ] Add authorization and cross-tenant tests

### CI Repair
- [ ] Restore all 36 Phase 8B jobs
- [ ] Fix lint, security scan, Helm rendering failures
- [ ] Add Phase 8C integration tests with real restore drills
- [ ] Test corruption, truncation, wrong key scenarios

### Documentation
- [ ] Update PR description with accurate status
- [ ] Remove "Implementation Complete" claims
- [ ] Document backup artifact storage policy
- [ ] Document encryption and key recovery policy
- [ ] Document recovery state machine

## FILES TO INSPECT

1. pkg/backup/database.go - critical safety issues
2. pkg/backup/jetstream.go - unsafe operations
3. pkg/backup/objectstorage.go - memory issues
4. pkg/backup/coordinator.go - missing runtime wiring
5. pkg/postgres/schema.go - missing backup table migrations
6. .github/workflows/ci.yml - damaged CI
7. go.mod/go.sum - check for unnecessary dependencies
8. cmd/ - add operator CLI commands

## NEXT STEPS

1. Create subagents for each component area
2. Restore PR scope and Phase 8B CI baseline
3. Add backup table migrations
4. Implement external artifact storage
5. Add runtime wiring to operator CLI
6. Fix CI failures
7. Create comprehensive integration tests
