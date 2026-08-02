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
authoritative. From an operator-controlled database session, check migrations
67 and 69 and the profile state:

```sql
SELECT id, display_name,
       setup_completed_at IS NOT NULL AS setup_complete,
       setup_completed_at, setup_completed_by, updated_at
FROM platforms
WHERE id = '00000000-0000-0000-0000-000000000001';

SELECT id, name
FROM schema_migrations
WHERE id IN (67, 69)
ORDER BY id;
```

If a first sign-in remains on setup:

1. Confirm migrations 67 and 69 are present and the API/database readiness
   checks pass.
2. Confirm the user has an active, unexpired `platform_owner` or
   `platform_admin` membership with `scope_type = 'platform'` and `scope_id =
   '00000000-0000-0000-0000-000000000001'`.
3. Use `GET /api/v1/auth/me` or `GET /api/v2/context` to inspect the
   server-owned setup state. While setup is incomplete, a direct call to any
   non-allowlisted authenticated route for this administrator should return
   HTTP `428` with `code: provider_setup_required`; that is expected, not a
   reason to disable middleware.
4. Confirm the session has no MSP, client, or site scope before calling a
   provider-profile route. `POST /api/v2/context/switch` remains available so a
   switched administrator can return to the platform scope.
5. If `setup_completed_at` is null, sign in at the top-level platform context,
   re-enter any values lost by a reload, proceed through Review, and explicitly
   select **Complete setup**.
6. If `setup_completed_at` is populated, refresh `/api/v2/context`; the setup
   gate reads the database on each affected request, so no new token or service
   restart is required. Sign out and back in only to clear stale browser state.
   Do not clear completion columns or edit profile columns directly.

The exact authenticated setup-gate allowlist is:

- `GET /api/v1/auth/me`;
- `POST /api/v1/auth/logout`;
- `GET /api/v2/context`;
- `POST /api/v2/context/switch`;
- `GET /api/v2/platform/provider/profile`;
- `POST /api/v2/platform/provider/setup`; and
- `PATCH /api/v2/platform/provider/profile`.

Public health routes leave access control before the gate and remain available.
There is no authenticated HTTP recovery endpoint in the current route registry.

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

## Scope authorization and user-provisioning recovery

Migration 69 makes `memberships` authoritative, revalidates identity,
membership, and hierarchy state for protected requests, and records ambiguous
legacy state without manufacturing access. After upgrade, inspect its presence
and issue summary from a privileged read-only operator session:

```sql
SELECT id, name
FROM schema_migrations
WHERE id = 69;

SELECT issue_type, COUNT(*) AS affected_rows
FROM authorization_migration_issues
GROUP BY issue_type
ORDER BY issue_type;

SELECT issue_type, user_id, scope_type, scope_id, role, details, detected_at
FROM authorization_migration_issues
ORDER BY detected_at, id;
```

Interpret the issue types as follows:

- `invalid_membership` means the preserved row has a missing identity/scope or
  an illegal role/scope pairing. Runtime authorization ignores illegal pairings;
  correct access through the scoped membership APIs after validating intent.
- `legacy_role_without_membership` means `users.role` has no corresponding
  authoritative membership. The account has no access until an authorized
  administrator explicitly provisions a legal membership.
- `legacy_tenant_access_without_membership` means the
  `user_tenant_access` row is a compatibility mirror only. Do not translate it
  automatically into a grant; confirm the intended scope and role first.

Do not delete or rewrite `authorization_migration_issues` to make the report
empty. Migration 69 preserves evidence and protects it with the recovery
mutation gate. Do not restore authorization from `users.role`,
`users.tenant_id`, or `user_tenant_access`.

Top-level platform administrators may use **User Management**. The current
console route is platform-only; MSP/client/site scope managers use the same API
from an approved operator client until a scoped console is implemented.
Creation requires an email, a 12–72-byte password, and one or more explicit
scope/type/role assignments. Membership changes use
`PUT /api/v1/admin/users/{userID}/memberships`; the legacy `/tenants` path is
only a route-name compatibility alias and accepts the same `memberships` body.
The server rejects inactive/mismatched scopes, illegal role/scope combinations,
duplicate scopes, assignments outside the actor's selected scope, and owner
escalation. Do not repair a failed operation with direct inserts: identity,
memberships, compatibility mirrors, and audit evidence are designed to commit
or roll back together. The handler locks the target identity, explicitly limits
revocation to memberships the actor may manage in the selected scope, and
serializes concurrent replacements. If an installation predating this hardening
accepted overlapping requests, inspect the authoritative rows below and use one
subsequent membership replacement to reconcile the managed scope.

Confirm successful changes and their sanitized audit evidence:

```sql
SELECT user_id, scope_type, scope_id, role, status, expires_at, created_at
FROM memberships
WHERE user_id = 'USER_UUID'
ORDER BY scope_type, scope_id, created_at;

SELECT action, actor_user_id, resource_id, details, created_at
FROM control_plane_audit
WHERE resource_type = 'user'
  AND action IN ('user.provisioned', 'user.memberships_updated')
ORDER BY created_at DESC;
```

Revoked or expired memberships, disabled/unverified users, suspended/pending
MSPs, and archived clients/sites invalidate outstanding user access on the next
protected request. Logout itself is stateless; it returns `204` and the client
discards local session state.

Migration 69's down migration is deliberately a no-op: the hardening,
compatibility disposition, and issue evidence remain in place. For rollback,
retain the database backup and schema 69, stop before deploying an older binary,
and verify that the candidate binary neither trusts legacy role/tenant mirrors
nor expects the pre-69 RLS helpers. If that cannot be proven, roll forward with
the current authorization-aware binary instead of weakening the database or
deleting evidence.

## MSP owner activation recovery

An MSP is incomplete while `onboarding_status = 'pending_owner'`. In that state
`is_active` is false, its initial entitlement is suspended, and host resolution
and workspace switching deliberately reject it. Do not use the ordinary MSP
activate endpoint to bypass owner verification; that endpoint updates only
MSPs whose onboarding status is already `active`.

Start with read-only state checks from a privileged operator database session
that can inspect forced-RLS tables:

```sql
SELECT id, name
FROM schema_migrations
WHERE id = 68;

SELECT m.id, m.name, m.slug, m.onboarding_status, m.is_active,
       e.status AS entitlement_status,
       i.id AS invitation_id, i.delivery_status,
       i.created_at, i.expires_at, i.delivered_at,
       i.accepted_at, i.revoked_at
FROM msp_tenants m
LEFT JOIN plan_entitlements e ON e.msp_id = m.id
LEFT JOIN LATERAL (
  SELECT id, delivery_status, created_at, expires_at, delivered_at,
         accepted_at, revoked_at
  FROM account_invitations
  WHERE msp_id = m.id
  ORDER BY created_at DESC
  LIMIT 1
) i ON true
WHERE m.onboarding_status = 'pending_owner'
ORDER BY m.created_at;
```

Interpret the latest delivery state as follows:

- `unconfigured`: no account mailer or usable public origin was wired when the
  invitation was delivered. Configure SMTP and `STRATA_PUBLIC_URL`, restart the
  orchestrator, then resend.
- `failed`: the SMTP attempt failed. Check redacted orchestrator errors, DNS,
  network reachability, TLS mode/certificate, credentials, sender policy, and
  recipient policy; then resend.
- `pending`: the service could not confirm persistence of the delivery result.
  Check database health before deciding whether to resend.
- `delivered`: the SMTP server accepted the message; this is not proof that the
  mailbox received it. A still-valid delivered invitation cannot be resent and
  returns `409`. Check spam/quarantine and recipient routing, or wait for expiry
  before rotation.

Use **MSP Tenants → Resend invitation** only after correcting the cause. Resend
is allowed for failed, unconfigured, pending, or expired invitations. It locks
and revokes the current invitation, creates a fresh 72-hour token, and attempts
delivery. Every older link then fails. Raw tokens cannot be recovered from the
database because only SHA-256 digests are stored, and the API never returns
them. Do not copy hashes into a URL, manually mark an email verified, edit
invitation timestamps, or activate the MSP in SQL.

After the owner accepts the latest link, expect the newest invitation to have
`accepted_at`, the MSP to be `onboarding_status = 'active'` and `is_active =
true`, its entitlement to be active, and exactly one active `msp_owner`
membership. Acceptance creates no browser session; the owner must sign in with
the invited email and chosen password. If the page reports an unavailable
invitation, check expiry/revocation/acceptance and resend if the MSP remains
pending. If the MSP is already active, treat the link as consumed and do not
issue another owner invitation.

### MSP owner activation audit checks

```sql
SELECT action, msp_id, actor_user_id, resource_type, resource_id,
       details, created_at
FROM control_plane_audit
WHERE action IN (
  'msp.owner_invitation_created',
  'msp.owner_invitation_resent',
  'msp.owner_activated'
)
ORDER BY created_at;
```

Creation and successful rotation are attributed to the top-level platform
operator; activation is attributed to the new owner. Invitation events refer
to the invitation row, and activation refers to the MSP. For all three events,
`details` must be an empty object: owner emails, raw or hashed tokens, passwords,
and password hashes must not appear. A rejected resend of a still-valid
delivered invitation adds no resend event. Creation/rotation state and their
audit event commit together before email delivery; activation state and its
audit event commit atomically with invitation acceptance.

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

## Automatic HTTPS and certificate renewal

Caddy manages the public Let's Encrypt certificate in Docker installations and
in native installations created with `--https-mode automatic`. It renews the
certificate before expiry; do not replace this with a cron-driven ACME client.

When issuance or renewal fails:

1. Confirm the public A/AAAA records resolve to the intended host and remove a
   stale AAAA record if IPv6 traffic cannot reach it.
2. Confirm inbound TCP 80 and 443 reach Caddy and are not intercepted by a
   different proxy. Keep both ports available for future renewal challenges.
3. Inspect sanitized Caddy logs. Do not share ACME account keys or storage
   contents.
4. Confirm the configured ACME directory. The staging directory is for
   rehearsals only and its certificates are deliberately not browser-trusted.
5. Verify the public chain and hostname with `openssl s_client
   -verify_hostname`; do not use `curl --insecure` as production evidence.

Docker keeps ACME state in the `caddy_data` volume. Native Caddy normally keeps
it under `/var/lib/caddy`; confirm the actual service environment before backup
or restore. Back up that state and the active Caddyfile together. A native
installer failure restores the immediately preceding Caddyfile when available,
but does not revert the installed Strata package, migrations, bootstrap, or
orchestrator service. After any restore, restart Caddy and verify
`https://<domain>/health/ready` plus the certificate hostname and expiry.

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
