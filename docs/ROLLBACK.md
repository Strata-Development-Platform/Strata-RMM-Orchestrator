# Rollback Procedures

## Code Rollback

### Option A: Git Revert

Revert a specific commit or range of commits and deploy the result.

```bash
# 1. Identify the commit(s) to revert
git log --oneline -10

# 2. Revert the bad commit (creates a new commit)
git revert <bad-commit-hash>

# 3. Push the revert
git push origin main

# 4. Deploy the reverted code
```

### Option B: Branch Checkout

Switch to a known-good branch or tag.

```bash
# 1. List tags
git tag -l

# 2. Checkout the previous release tag
git checkout tags/v1.2.3 -b rollback-v1.2.3

# 3. Rebuild and deploy
make build

# 4. Push rollback branch
git push origin rollback-v1.2.3
```

### Option C: Binary Rollback (Bare Metal)

If the orchestrator has auto-updated and the new version is broken:

```bash
# 1. Stop the service
systemctl stop strata-rmm

# 2. Restore the previous binary
cp /usr/local/bin/strata-rmm.bak /usr/local/bin/strata-rmm

# 3. Start the service
systemctl start strata-rmm

# 4. Verify health
curl http://localhost:8080/health

# 5. Pin version to prevent re-update
strata-rmm orchestrator update --pin v1.2.3
```

### Agent Rollback

```bash
# 1. Pause auto-updates for affected agents
nats pub tenant.<msp_id>.rollout.<agent_id> '{"action":"pause"}'

# 2. Force rollback on a specific agent
nats pub tenant.<msp_id>.rollout.<agent_id> '{"action":"rollback"}'

# 3. For mass rollback, set rollout to 0% on canary channel
#    then manually trigger rollback per agent or group
```

---

## Database Rollback

### Migration Reversal

Database migrations are ordered (1..N). To rollback:

```bash
# 1. Connect to the database
psql -U strata_rmm_app -d strata_rmm

# 2. Check current migration state
SELECT id, name, applied_at FROM schema_migrations ORDER BY id;
```

### Migration 24: Reporting Engine

```sql
-- Reverse: Drop report engine tables
DROP TABLE IF EXISTS generated_reports CASCADE;
DROP TABLE IF EXISTS report_schedules CASCADE;

-- Remove migration record
DELETE FROM schema_migrations WHERE id = 24;
```

### Migration 23: Software Deployment

```sql
-- Reverse: Drop software deployment tables
DROP TABLE IF EXISTS software_deployments CASCADE;
DROP TABLE IF EXISTS software_packages CASCADE;

-- Remove migration record
DELETE FROM schema_migrations WHERE id = 23;
```

### Migration 22: Scripting Engine

```sql
-- Reverse: Drop scripting tables
DROP TABLE IF EXISTS script_executions CASCADE;
DROP TABLE IF EXISTS scripts CASCADE;

-- Remove migration record
DELETE FROM schema_migrations WHERE id = 22;
```

### Migration 21: Agent Registrations

```sql
-- Reverse: Drop agent registration table
DROP TABLE IF EXISTS agent_registrations CASCADE;

-- Remove migration record
DELETE FROM schema_migrations WHERE id = 21;
```

### Earlier Migrations (Partial Rollback)

```sql
-- Migration 20: User tenant access
DROP TABLE IF EXISTS audit_auth CASCADE;
DROP TABLE IF EXISTS user_tenant_access CASCADE;
DELETE FROM schema_migrations WHERE id = 20;

-- Migration 19: Encryption keys
DROP TABLE IF EXISTS tenant_encryption_keys CASCADE;
DELETE FROM schema_migrations WHERE id = 19;

-- Migration 18: CVE sync
DROP TABLE IF EXISTS cve_package_ecosystem CASCADE;
DROP TABLE IF EXISTS cve_sync_state CASCADE;
DELETE FROM schema_migrations WHERE id = 18;

-- Migration 17: Session recordings
DROP TABLE IF EXISTS session_recordings CASCADE;
DELETE FROM schema_migrations WHERE id = 17;

-- Migration 16: MFA secrets
DROP TABLE IF EXISTS mfa_secrets CASCADE;
DELETE FROM schema_migrations WHERE id = 16;
```

### Full Database Reset (Development Only)

```bash
# ⚠️ DESTRUCTIVE — for dev/staging only
psql -U strata_rmm_app -d strata_rmm -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
strata-rmm orchestrator --apply-migrations
```

---

## Service Restart Procedures

### Full Platform Restart (Docker Compose)

```bash
# 1. Pull the previous version
docker compose pull orchestrator

# 2. Rollback database if needed
#    (execute migration reversal SQLs first)

# 3. Restart all services
docker compose down
docker compose up -d

# 4. Verify all services
docker compose ps
curl http://localhost:8080/health
```

### Full Platform Restart (Bare Metal)

```bash
# 1. Stop services in dependency order
systemctl stop strata-rmm
systemctl stop nats
systemctl stop postgresql

# 2. Start in reverse order
systemctl start postgresql
systemctl start nats
systemctl start strata-rmm

# 3. Verify
systemctl status strata-rmm nats postgresql
curl http://localhost:8080/health
```

### Rolling Restart (Kubernetes)

```bash
# Rollback to previous revision
kubectl rollout undo deployment/strata-rmm -n strata-rmm

# Monitor rollout status
kubectl rollout status deployment/strata-rmm -n strata-rmm

# Verify
kubectl get pods -n strata-rmm
kubectl exec -it deploy/strata-rmm -n strata-rmm -- curl http://localhost:8080/health
```

### Single Service Restart

```bash
# Orchestrator only
systemctl restart strata-rmm

# NATS only
systemctl restart nats

# PostgreSQL only
systemctl restart postgresql
```

---

## Verification Steps After Rollback

### 1. Health Check

```bash
curl http://localhost:8080/health

# Expected response:
# {"status":"ok","time":"2026-07-27T12:00:00Z","version":"<rolled-back-version>"}
```

### 2. API Smoke Test

```bash
# Run the smoke test suite
./scripts/smoke_test.sh

# Expected output:
# [PASS] Health endpoint returns ok
# [PASS] Enrollment token generated
# [PASS] CVE database has 10+ records
# [PASS] MFA enrollment generated secret
# [PASS] Recording list endpoint works
# [PASS] All smoke tests passed!
```

### 3. Database Verification

```bash
# Check migration state matches expected rollback
psql -U strata_rmm_app -d strata_rmm -c "SELECT id, name FROM schema_migrations ORDER BY id;"

# Check data integrity
psql -U strata_rmm_app -d strata_rmm -c "SELECT COUNT(*) FROM devices;"
psql -U strata_rmm_app -d strata_rmm -c "SELECT COUNT(*) FROM tenants;"
```

### 4. Agent Connectivity

```bash
# Check device heartbeats (should be recent)
curl http://localhost:8080/api/v1/heartbeat/<tenant-id>/<device-id>

# Check online device count
curl http://localhost:8080/api/v1/overview | jq '.online_devices'
```

### 5. Alerting Verification

```bash
# Verify alert engine is running
curl http://localhost:8080/api/v1/alerts/<tenant-id>

# Create a test threshold rule and verify evaluation
curl -X POST http://localhost:8080/api/v1/rules/<tenant-id> \
  -H 'Content-Type: application/json' \
  -d '{"id":"test-rollback","name":"rollback-test","type":"threshold","enabled":true,"severity":"info","metric_name":"cpu.percent","condition":"gt","threshold":0}'
```

### 6. NATS Verification

```bash
# Check NATS connections
curl http://localhost:8222/connz | jq '.num_connections'

# Check NATS streams
nats stream list
```

### 7. Client UI Check

```bash
# Verify Web UI loads
curl -s http://localhost:8080 | head -20
# Should return the index.html of the React app

# Verify protected endpoints work with valid JWT
curl -H "Authorization: Bearer <valid-jwt>" http://localhost:8080/api/v1/overview
```

### Rollback Success Criteria

| Check | Expected | Action if Failed |
|-------|----------|-----------------|
| Health endpoint | `{"status":"ok"}` | Check logs: `journalctl -u strata-rmm -n 50` |
| Smoke test | All PASS | Re-run, check specific failing test |
| Device count | Matches pre-rollback | Check DB connection and migrations |
| Agent heartbeats | < 5 min old | Check NATS connectivity |
| Alert engine | Rules loaded | Check `alert_rules` table |
| NATS connections | > 0 | Check `nats-server` service status |
