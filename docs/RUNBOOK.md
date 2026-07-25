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
