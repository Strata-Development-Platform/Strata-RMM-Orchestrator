# Phase 8C File Ownership Table

## Agent A - Backup Data Plane

**Owned files:**
- `pkg/backup/**` (all backup-related packages)
- PostgreSQL migration additions (schema.go lines added for backup tables)
- Focused backup unit tests within `pkg/backup/*_test.go`

**Responsibilities:**
- External backup architecture
- Migrations for backup tables
- PostgreSQL and TimescaleDB backup/restore
- Encryption and key management
- JetStream backup/restore
- Object storage backup/restore
- Integrity verification
- Corruption and cancellation tests

---

## Agent B - Recovery Control Plane

**Owned files:**
- `cmd/orchestrator/**` (runtime wiring)
- Recovery operator interface
- Authorization and audit integration
- State machine implementation
- Locking mechanism
- `.github/workflows/**` (CI configuration)
- `docs/**` Phase 8C documentation
- End-to-end recovery tests

**Responsibilities:**
- Recovery coordinator wiring
- Locking and concurrency control
- State machine transitions
- Service quiescing
- Post-restore verification
- CI preservation and Phase 8C jobs
- Documentation accuracy
- PR evidence preparation

---

## Coordinator - Integration and Final Verification

**Owned files:**
- Resolution of merge conflicts
- Restoration of unrelated files to base
- Final requirements ledger
- Final adversarial review
- PR description update
- Exact-head CI verification

**Responsibilities:**
- Integration of both workstreams
- Independent adversarial testing
- Final CI monitoring
- PR readiness determination
