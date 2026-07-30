# Phase 8C Requirements Ledger

## Current Status

### ✅ Completed Items

1. **Backup Tables Migration** (Coordinator)
   - Added migrations 63-66 for backup tables
   - Tables: backups, jetstream_backups, object_storage_backups, recovery_state
   - Schema includes proper keys, indexes, and constraints

2. **PostgreSQL Backup Implementation** (Agent A)
   - Real `pg_dump` execution with proper flags
   - Mandatory encryption (no plaintext fallback)
   - Integrity verification with SHA-256
   - Error handling and cleanup

3. **Recovery Coordinator** (Agent B)
   - 20-state state machine implemented
   - Locking mechanism with pg_try_advisory_lock
   - Dry-run mode
   - State persistence

4. **Unit Tests** (Both Agents)
   - All backup unit tests passing
   - Coordinator state transitions tested
   - Error cases covered

### ⚠️ Incomplete Items

1. **JetStream Integration Tests**
   - Need real NATS server with JetStream enabled in CI
   - Current tests use nil connections

2. **Object Storage Integration Tests**
   - Need real object storage backend in CI
   - Current tests are unit-only

3. **End-to-End Recovery Tests**
   - Full recovery drill not implemented
   - Need integration test with real PostgreSQL

4. **CI Configuration**
   - Phase 8C workflow needs PostgreSQL service
   - JetStream service needed in CI
   - Object storage service needed

### 🚫 Blockers

1. **Missing Tables in Base**
   - Backup tables not in original schema
   - Added in migration 63-66

2. **No Runtime Wiring**
   - Backup/restore not connected to CLI or API
   - Agent B needs to implement operator interface

3. **No Authorization**
   - No platform-operator checks
   - No tenant isolation enforcement

4. **CI Services Missing**
   - No PostgreSQL service in workflows
   - No NATS with JetStream
   - No object storage

## Required Evidence

### A8-06 PostgreSQL Recovery
- [ ] Isolated restore report
- [ ] Tenant data preserved
- [ ] Jobs, approvals, audit evidence restored
- [ ] TimescaleDB data verified

### A8-07 Messaging Recovery
- [ ] NATS restart preserves durable work
 - [ ] No duplicate destructive execution
 - [ ] JetStream streams/consumers verified

### A8-08 Object Recovery
- [ ] Required reports/recordings restore
 - [ ] Integrity verification passed
 - [ ] Authorization intact

### A8-09 RPO/RTO
- [ ] Recovery drill meets targets
 - [ ] Timestamped evidence

## Next Steps

1. Launch Agent A for backup data plane
2. Launch Agent B for recovery control plane  
3. Integrate both workstreams
4. Perform adversarial review
5. Fix CI configuration
6. Run exact-head verification
