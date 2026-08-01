# Strata RMM Operations Runbook

## Service Status

```bash
# Check all services
docker compose -f deploy/docker/docker-compose.yml ps

# Check orchestrator health
curl http://localhost:8080/health

# View logs
docker compose logs -f orchestrator
docker compose logs -f nats
docker compose logs -f postgres
```

## Starting/Stopping

```bash
# Start all services
make dev

# Start without agent
docker compose -f deploy/docker/docker-compose.yml up -d nats postgres orchestrator

# Stop everything
make docker-down

# Rebuild
make docker-build
```

## Agent Management

### Install on Linux
```bash
sudo ./scripts/install.sh
# Edit /etc/strata-rmm/agent.yaml with tenant-id and enrollment-token
sudo systemctl start strata-rmm-agent
journalctl -u strata-rmm-agent -f
```

### Run ad-hoc
```bash
TENANT_ID="00000000-0000-0000-0000-000000000001"
ENROLLMENT_TOKEN="dev-token"
make run-agent
```

### Windows
```powershell
# Coming soon: MSI installer
strata-rmm.exe agent --tenant-id TENANT_ID --enrollment-token TOKEN --nats-url nats://HOST:4222
```

## Network Probe

```bash
# Start a probe for a tenant
strata-rmm probe --tenant-id "00000000-0000-0000-0000-000000000001" --nats-url nats://localhost:4222

# With SNMP targets and discovery
strata-rmm probe \
  --tenant-id "TENANT_ID" \
  --nats-url nats://localhost:4222 \
  --config probe-config.yaml
```

## Database Operations

### Connect to TimescaleDB
```bash
docker compose exec postgres psql -U strata -d strata_rmm
```

### Check migrations
```sql
SELECT * FROM schema_migrations ORDER BY id;
```

## Provider business-profile setup and recovery

Provider setup is complete only when the server reports a non-null
`setup_completed_at` for the singleton platform. The browser route alone is not
authoritative. From an operator-controlled database session, check migration 67
and the profile state:

```sql
SELECT id, display_name,
       setup_completed_at IS NOT NULL AS setup_complete,
       setup_completed_at, setup_completed_by, updated_at
FROM platforms
WHERE id = '00000000-0000-0000-0000-000000000001';

SELECT id, name
FROM schema_migrations
WHERE id = 67;
```

If a first sign-in remains on setup:

1. Confirm migration 67 is present and the API/database readiness checks pass.
2. Confirm the user has an active, unexpired `platform_owner` or
   `platform_admin` membership with `scope_type = 'platform'` and `scope_id =
   '00000000-0000-0000-0000-000000000001'`.
3. Confirm the session has no MSP, client, or site scope. A switched or
   tenant-scoped token is deliberately forbidden from provider-profile routes.
4. If `setup_completed_at` is null, sign in at the top-level platform context,
   re-enter any values lost by a reload, proceed through Review, and explicitly
   select **Complete setup**.
5. If `setup_completed_at` is populated, sign out and back in to refresh the
   login and `/api/v2/context` state. Do not clear completion columns or edit
   profile columns directly.

The initial-admin bootstrap and migration 67 create or repair the required
platform-owner membership for installer-created administrators. If that
membership is still absent, stop and investigate bootstrap/migration evidence;
do not bypass the API with an ad hoc profile update.

An identical retry of a completed setup request is an idempotent success and
does not add another audit event. A different setup payload after completion is
rejected; use **Platform Settings → Provider business profile** for later
changes. A settings save sends only changed fields, but the server merges and
validates the entire profile. A failed request leaves the current wizard values
available only while that page remains loaded.

### Provider profile audit checks

Completion and updates are written to the platform-wide control-plane audit
trail in the same transaction as the profile change. Inspect them with an
authorized operator connection:

```sql
SELECT action, resource_type, resource_id, actor_user_id, details, created_at
FROM control_plane_audit
WHERE resource_type = 'platform'
  AND resource_id = '00000000-0000-0000-0000-000000000001'
  AND action IN ('provider.setup_completed', 'provider.profile_updated')
ORDER BY created_at;
```

Expect one `provider.setup_completed` event after first completion, with only a
profile schema version in `details`. Each effective later save records a
`provider.profile_updated` event whose details list sorted changed field names;
contact, address, and tax-identifier values are not copied into audit details.
No-op updates add no event. The migration-67 database trigger rejects UPDATE or
DELETE attempts against any control-plane audit row. Provider mutation handlers
also fail the request if the audit append fails, causing the request transaction
to roll back.

### View active alerts
```sql
SELECT * FROM alerts WHERE status = 'firing' ORDER BY fired_at DESC;
```

### Check device status
```sql
SELECT hostname, status, last_heartbeat FROM devices ORDER BY last_heartbeat DESC;
```

### View vulnerability state
```sql
SELECT d.hostname, dv.cve_id, dv.severity, dv.status
FROM device_vulnerabilities dv
JOIN devices d ON dv.device_id = d.id
WHERE dv.status = 'open'
ORDER BY dv.severity DESC;
```

## Alerting

### Create a threshold rule
```bash
curl -X POST localhost:8080/api/v1/rules/TENANT_ID \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "cpu-critical",
    "name": "CPU > 90%",
    "type": "threshold",
    "enabled": true,
    "severity": "critical",
    "metric_name": "cpu_percent",
    "condition": "gt",
    "threshold": 90,
    "cooldown": 300000000000
  }'
```

### Create a heartbeat rule
```bash
curl -X POST localhost:8080/api/v1/rules/TENANT_ID \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "heartbeat-5m",
    "name": "Heartbeat 5min",
    "type": "heartbeat",
    "severity": "critical",
    "timeout": 300000000000
  }'
```

### View active alerts
```bash
curl localhost:8080/api/v1/alerts/TENANT_ID
```

### Acknowledge alert
```bash
curl -X POST localhost:8080/api/v1/alerts/ALERT_ID/acknowledge
```

## Patch Management

### Create patch policy
```bash
curl -X POST localhost:8080/api/v1/patch/policies \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Critical Patches",
    "platforms": ["windows", "linux"],
    "approval_mode": "auto",
    "severity": "critical"
  }'
```

### Create deployment
```bash
curl -X POST localhost:8080/api/v1/patch/deployments \
  -H 'Content-Type: application/json' \
  -d '{
    "policy_id": "POLICY_ID",
    "scheduled_for": "2026-08-01T02:00:00Z",
    "device_ids": ["DEVICE_ID_1", "DEVICE_ID_2"]
  }'
```

## Kubernetes (Helm)

### Install
```bash
helm repo add strata-rmm https://strata-development-platform.github.io/helm-charts
helm upgrade --install strata-rmm strata-rmm/strata-rmm \
  --namespace strata-rmm --create-namespace
```

### With custom values
```bash
helm upgrade --install strata-rmm ./deploy/helm/strata-rmm \
  --values ./deploy/helm/strata-rmm/ci/multi-region-values.yaml \
  --set global.region=us-east-1
```

### Air-gapped
```bash
helm upgrade --install strata-rmm ./deploy/helm/strata-rmm \
  --values ./deploy/helm/strata-rmm/ci/air-gapped-values.yaml
```

### Uninstall
```bash
helm uninstall strata-rmm --namespace strata-rmm
```

## CVE Management

### Trigger a sync
```bash
curl -X POST localhost:8080/api/v1/cve/sync
```

### Check CVE database stats
```bash
curl localhost:8080/api/v1/cve/stats
```

### View sync status
```bash
curl localhost:8080/api/v1/cve/sync/status
```

### List tracked packages
```bash
curl localhost:8080/api/v1/cve/packages
```

### Add a package to track
```bash
curl -X POST localhost:8080/api/v1/cve/packages \
  -H 'Content-Type: application/json' \
  -d '{"name": "libssl", "ecosystem": "Debian"}'
```

### Remove a tracked package
```bash
curl -X DELETE localhost:8080/api/v1/cve/packages/libssl/Debian
```

### View CVEs for a package
```bash
curl localhost:8080/api/v1/cve/package/openssh
```

## Vulnerability Remediation

### View device vulnerabilities
```bash
curl localhost:8080/api/v1/vulnerabilities/device/DEVICE_ID
```

### View tenant vulnerabilities
```bash
curl localhost:8080/api/v1/vulnerabilities/tenant/TENANT_ID
```

### View remediation summary
```bash
curl localhost:8080/api/v1/vulnerabilities/tenant/TENANT_ID/summary
```

### Resolve a vulnerability
```bash
curl -X POST localhost:8080/api/v1/vulnerabilities/VULN_ID/resolve
```

### Ignore a vulnerability
```bash
curl -X POST localhost:8080/api/v1/vulnerabilities/VULN_ID/ignore
```

### SQL queries
```sql
-- Open vulnerabilities by severity
SELECT dv.cve_id, dv.severity, dv.package_name, d.hostname, dv.detected_at
FROM device_vulnerabilities dv
JOIN devices d ON dv.device_id = d.id
WHERE dv.status = 'open'
ORDER BY dv.severity DESC;

-- Remediation history
SELECT cve_id, package_name, status, detected_at, resolved_at
FROM device_vulnerabilities
WHERE resolved_at IS NOT NULL
ORDER BY resolved_at DESC LIMIT 20;
```

## Encryption Keys

### Create a tenant encryption key
```bash
curl -X POST localhost:8080/api/v1/keys/TENANT_ID \
  -H 'Content-Type: application/json' \
  -d '{"kms_type": "local", "encryption": "aes-256-gcm"}'
```

### List tenant keys
```bash
curl localhost:8080/api/v1/keys/TENANT_ID
```

### Get active key
```bash
curl localhost:8080/api/v1/keys/TENANT_ID/active
```

### Rotate key
```bash
curl -X POST localhost:8080/api/v1/keys/TENANT_ID/rotate
```

### Revoke a key
```bash
curl -X DELETE localhost:8080/api/v1/keys/TENANT_ID/KEY_ID
```

## Access Review

### View audit log
```bash
curl localhost:8080/api/v1/access/audit/TENANT_ID
```

### View tenant users
```bash
curl localhost:8080/api/v1/access/users/TENANT_ID
```

### View tenant permissions
```bash
curl localhost:8080/api/v1/access/permissions/TENANT_ID
```

## Performance Testing

### Quick API check
```bash
./scripts/loadtest.sh api
```

### Full load test (100 rps, 500 agents, 60s)
```bash
API_URL=http://localhost:8080 NATS_URL=nats://localhost:4222 ./scripts/loadtest.sh full
```

### Alert storm test
```bash
./scripts/loadtest.sh alerts
```

## Troubleshooting

### Agent won't connect
1. Check NATS is running: `curl localhost:8222/varz`
2. Verify enrollment token isn't expired
3. Check agent logs: `journalctl -u strata-rmm-agent -f`
4. Verify network connectivity to NATS host:4222

### High memory usage
1. Check TimescaleDB compression: `SELECT * FROM timescaledb_information.compression_settings;`
2. Verify retention policies are active: `SELECT * FROM timescaledb_information.jobs WHERE proc_name LIKE '%retention%';`
3. Check number of chunks: `SELECT * FROM timescaledb_information.chunks;`

### Alerts not firing
1. Verify rule is enabled: `SELECT * FROM alert_rules WHERE enabled = true;`
2. Check the alert engine is running in orchestrator logs
3. Verify metrics are flowing: `SELECT count(*) FROM metrics WHERE time > NOW() - INTERVAL '5 minutes';`

## Session Recordings

### List recordings for a tenant
```bash
curl localhost:8080/api/v1/recordings/TENANT_ID
```

### View/playback a recording
```bash
# Requires MFA code in header
curl localhost:8080/api/v1/recordings/RECORDING_ID/playback \
  -H 'X-MFA-Code: 123456'
```

### Delete a recording
```bash
curl -X DELETE localhost:8080/api/v1/recordings/RECORDING_ID
```

### Configure retention
Recordings expire automatically based on the `expires_at` column.
Default: 90 days. Override per recording on creation.

## MFA Management

### Enroll a user in TOTP
```bash
curl -X POST localhost:8080/api/v1/mfa/enroll/USER_ID
# Returns: secret + provisioning URI + QR code URL
```

### Verify and activate MFA
```bash
curl -X POST localhost:8080/api/v1/mfa/verify/USER_ID \
  -H 'Content-Type: application/json' \
  -d '{"code": "123456"}'
```

### Check MFA status
```bash
curl localhost:8080/api/v1/mfa/status/USER_ID
```

### Disable MFA
```bash
curl -X DELETE localhost:8080/api/v1/mfa/USER_ID
```

## Agent Auto-Update

### Check agent version
```bash
# From agent host
strata-rmm version --output json

# From orchestrator - check update state
# (recorded in agent's BBolt store, reported via NATS status subject)
```

### Manual update trigger (via NATS)
```bash
# Pause rollouts
nats pub tenant.TENANT_ID.rollout.AGENT_ID '{"action":"pause"}'

# Resume rollouts
nats pub tenant.TENANT_ID.rollout.AGENT_ID '{"action":"resume"}'

# Set channel config
nats pub tenant.TENANT_ID.rollout.AGENT_ID '{"action":"set_config","config":{"channel":"stable","rollout_percent":50}}'

# Force rollback
nats pub tenant.TENANT_ID.rollout.AGENT_ID '{"action":"rollback"}'
```

### View update status
```bash
nats sub "tenant.TENANT_ID.rollout.status.>"
```

## Storage Backends

### Configure MinIO (self-hosted, default)
```yaml
STORAGE_BACKEND: minio
STORAGE_BUCKET: strata-recordings
STORAGE_MINIO_ENDPOINT: minio:9000
STORAGE_MINIO_ACCESS_KEY: minioadmin
STORAGE_MINIO_SECRET_KEY: minioadmin
STORAGE_MINIO_USE_SSL: "false"
```

### Configure AWS S3 (SaaS)
```yaml
STORAGE_BACKEND: s3
STORAGE_BUCKET: strata-recordings-prod
STORAGE_S3_REGION: us-east-1
STORAGE_S3_KMS_KEY_ID: "arn:aws:kms:..."  # Optional SSE-KMS
# Credentials via IAM role or env vars
```

### Configure Local FS (dev/testing)
```yaml
STORAGE_BACKEND: local
STORAGE_LOCAL_PATH: /tmp/strata-recordings
```

## Kubernetes (Helm)

### Install
```bash
helm repo add strata-rmm https://strata-development-platform.github.io/helm-charts
helm upgrade --install strata-rmm strata-rmm/strata-rmm \
  --namespace strata-rmm --create-namespace
```

### With custom values
```bash
helm upgrade --install strata-rmm ./deploy/helm/strata-rmm \
  --values ./deploy/helm/strata-rmm/ci/multi-region-values.yaml \
  --set global.region=us-east-1
```

### Air-gapped
```bash
helm upgrade --install strata-rmm ./deploy/helm/strata-rmm \
  --values ./deploy/helm/strata-rmm/ci/air-gapped-values.yaml
```

### Configure agent update channel
```bash
# View current channel CRD
kubectl get agentupdatechannels

# Update rollout percentage
kubectl patch agentupdatechannel RELEASE-stable --type merge \
  -p '{"spec":{"rolloutPercent":25}}'
```

### Uninstall
```bash
helm uninstall strata-rmm --namespace strata-rmm
```

## Performance Testing
```sql
-- Check active queries
SELECT pid, query, state, wait_event FROM pg_stat_activity WHERE state != 'idle';

-- Check slow queries
SELECT query, calls, mean_exec_time FROM pg_stat_statements
ORDER BY mean_exec_time DESC LIMIT 10;
```

## Phase 8D Platform Alerts

Every alert below includes an owner and this runbook anchor. Silence an alert
only with an incident/change reference and an expiry.

### Strata API down

1. Confirm the independent `/health/live` and `/health/ready` probes.
2. Check orchestrator process/container state and the last termination reason.
3. Verify Prometheus can reach the orchestrator network endpoint and that its
   token file matches `STRATA_METRICS_TOKEN`; never print either value.
4. If readiness is failing, use the named component in its response to select
   the PostgreSQL, NATS/JetStream, storage, migration, or dispatcher procedure.
5. Escalate to platform operations after five minutes or immediately when all
   public paths are unavailable.

### Strata API high error rate

1. Identify affected matched route patterns and status codes in the Phase 8D
   dashboard. Metric labels intentionally never contain tenant/resource IDs.
2. Correlate the onset with deployments and dependency alerts.
3. Inspect sanitized structured logs using the deployment/correlation ID.
4. Roll back only under `docs/ROLLBACK.md`; never weaken production validation.

### Strata API high p95 latency

1. Compare request rate, in-flight requests, and p95 latency.
2. Check PostgreSQL pool saturation/slow queries, NATS state, storage latency,
   CPU, memory, and pod/container throttling.
3. Identify the matched route family; do not introduce raw-path labels.
4. Scale or roll back using the documented deployment workflow, then verify the
   synthetic login and authenticated API checks.

### Strata job metrics collection failed

1. Check `/health/ready` database status.
2. Verify the metrics collector has database connectivity and migration state.
3. Treat missing job telemetry as loss of operational visibility, not as an
   empty queue.
4. Restore collection before closing the incident.

### Strata oldest job stalled

1. Check dispatcher readiness, NATS/JetStream account access, queue depth, and
   active recovery/quiesce state.
2. Inspect jobs and targets by correlation ID through authorized APIs.
3. Do not manually mark jobs successful or delete durable outbox records.
4. Recover the dependency or dispatcher and verify the age gauge returns to
   steady state without duplicate destructive execution.

### Strata job failures

1. Separate failed from expired targets and inspect sanitized error categories.
2. Verify retry counts, next retry time, agent reconnect state, approval state,
   and job expiry.
3. Retry only through the authorized idempotent job API.
4. Escalate repeated destructive-operation failures to the job-platform owner.
