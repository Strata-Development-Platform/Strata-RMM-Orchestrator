# Phase 8C Remediation Ledger

## Current Status

### Known Issues from Previous Attempt

1. **Unattached work on pr-9 branch**
   - Commit 11da8d6 contains evidence documents but not integrated into PR branch
   - Must review and selectively incorporate

2. **CI baseline destroyed**
   - .github/workflows/ci.yml was reduced from ~1803 to ~1000 lines
   - Must restore complete Phase 8B CI coverage

3. **Evidence documents instead of fixes**
   - 8 redundant root-level Phase 8C documents added
   - Need to consolidate into docs/ structure

4. **Unrelated file changes**
   - ~45 formatting/cleanup changes not tied to Phase 8C requirements
   - Must revert unrelated changes

5. **Missing behavioral tests**
   - Verification script uses grep instead of runtime tests
   - Need actual integration tests for backup/restore

### Remediation Plan

1. **Create authoritative ledger** (this document)
2. **Restore CI baseline** from origin/master
3. **Review unattached commit** 11da8d6 and incorporate selectively
4. **Implement missing behavioral tests**
5. **Remove redundant evidence documents**
6. **Revert unrelated file changes**
7. **Run exact-head CI verification**

## Requirements Mapping

| ID | Requirement | Current State | Action Required |
|----|-------------|---------------|-----------------|
| BDR-01 | Externally durable backup storage | Partial - metadata tables exist but not external | Implement external storage backend |
| BDR-02 | Encryption and key independence | Partial - encryption exists but keys may be database-dependent | Ensure keys survive database loss |
| BDR-03 | PostgreSQL backup/restore | Partial - pg_dump exists but no explicit target handling | Add clean-target restore with verification |
| BDR-04 | JetStream backup/restore | Partial - backupMessages exists but no real JetStream | Add NATS/JetStream integration tests |
| BDR-05 | Object storage backup/restore | Partial - base64 encoding exists but no streaming | Implement bounded streaming |
| BDR-06 | Recovery coordination and locking | Partial - 20 states exist but no advisory lock | Implement pg_try_advisory_lock |
| BDR-07 | Quiescing and consistency | Missing - no actual service pausing | Implement real quiescing |
| BDR-08 | Recovery verification | Missing - no restored system queries | Add verification tests |
| BDR-09 | Failure handling and rollback | Missing - no failure injection | Add failure injection tests |
| BDR-10 | RPO/RTO measurement | Missing - no timestamps | Add measured recovery evidence |
| BDR-11 | Tenant isolation | Missing - no tenant checks in restore | Add tenant isolation tests |
| BDR-12 | Credential redaction | Missing - no DSN redaction | Implement redaction |
| BDR-13 | Runtime wiring | Partial - recovery.go added but not fully wired | Complete CLI wiring |
| BDR-14 | Schema migrations | Complete - migrations 63-66 exist | Verify idempotent |
| BDR-15 | Documentation | Partial - docs exist but incomplete | Consolidate into docs/ |
| BDR-16 | CI and evidence | Failed - baseline destroyed | Restore and add Phase 8C jobs |
| BDR-17 | Regression preservation | Failed - CI jobs removed | Restore all Phase 8B jobs |
| BDR-18 | PR management | Failed - stale description | Rewrite PR description |

## Files to Review from Unattached Commit

### Adopt Unchanged
- verify_phase8c.sh (useful for local verification)

### Adopt with Correction
- cmd/orchestrator/recovery.go (needs CLI wiring and tests)
- pkg/postgres/schema.go additions (need verification)

### Discard as Redundant
- All PHASE8C_*.md files (consolidate into docs/)

### Revert as Unrelated
- All formatting-only changes
- Any changes outside Phase 8C scope

## Next Steps

1. Restore .github/workflows/ci.yml from origin/master
2. Add Phase 8C CI jobs without removing existing ones
3. Implement actual integration tests for backup/restore
4. Consolidate evidence into docs/phase8c-evidence.md
5. Remove redundant root-level Phase 8C documents
6. Verify all Phase 8B CI jobs still pass
7. Run exact-head verification
