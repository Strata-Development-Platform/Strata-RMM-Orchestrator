# Strata RMM Operations Runbook

**Last Updated:** 2026-08-24

This runbook is for operators of the supported pre-beta deployment paths. It intentionally does not document Kubernetes/KOTS/air-gapped/multi-region operations as accepted paths. Development Makefile/Compose commands are not production procedures.

## Authoritative lifecycle documents

- Install: [INSTALL.md](INSTALL.md)
- Upgrade: [UPGRADE.md](UPGRADE.md)
- Rollback: [ROLLBACK.md](ROLLBACK.md)
- Backup: [BACKUP.md](BACKUP.md)
- Restore: [RESTORE.md](RESTORE.md)
- Configuration: [CONFIGURATION.md](CONFIGURATION.md)
- Security: [SECURITY.md](SECURITY.md)
- Hosted internal-alpha evidence: [INTERNAL_ALPHA_AGENT.md](INTERNAL_ALPHA_AGENT.md)

Do not replace those transactions with generic `docker compose pull`, mutable image tags, manual package copying, Helm commands, raw migration reversal, or direct database repair unless a separate documented break-glass procedure explicitly authorizes it.

## Deployment boundary

Supported pre-beta orchestrator lifecycle paths are:

1. Linux native/systemd through the secure installer/package lifecycle.
2. Single-host Docker Compose through the secure installer and privileged host-side update wrapper.

The long-running Docker orchestrator service must not receive the Docker socket. Docker lifecycle mutation occurs only in the transient root-controlled host updater described in `UPGRADE.md`.

## Service health

Use the deployment-mode-specific service manager first.

### Native/systemd

Inspect the exact service installed by the release package/installer and its journal. Do not assume historical service names from old runbooks; use the unit installed by the current release bundle.

### Docker

Run Compose commands only from the protected installed bundle directory and with its protected environment file. Do not run production from the repository development Compose file.

For either mode, verify application readiness using the route documented by the current installer/release and the authenticated/metrics checks appropriate to the environment. Public liveness alone is not sufficient release acceptance.

## Incident triage order

When the platform is degraded:

1. Record UTC start time, release/source SHA, deployment mode, and affected tenant/scope.
2. Check orchestrator process/container state and sanitized logs.
3. Check PostgreSQL/TimescaleDB reachability and TLS state.
4. Check NATS/JetStream reachability, TLS/authentication, stream/consumer health, and reconnect behavior.
5. Check object-storage reachability when recordings/reports/recovery are implicated.
6. Check application readiness and the relevant synthetic/Phase 8 signal.
7. Preserve evidence before restart or rollback if the event may affect hosted acceptance.

Never paste tokens, DSNs with passwords, invitation tokens, enrollment credentials, SMTP credentials, object-storage keys, recovery keys, or raw secret-file contents into incident notes.

### Strata API Down

1. Confirm the independent liveness and readiness probes from the deployment's documented health endpoint.
2. Check orchestrator process/container state and the last termination reason.
3. Verify Prometheus can reach the orchestrator metrics endpoint and that the protected metrics-token file matches the configured service; never print the token.
4. If readiness is failing, use the named failing component to follow the PostgreSQL, NATS/JetStream, object-storage, migration, or dispatcher branch of this runbook.
5. Escalate to platform operations after five minutes of sustained unavailability, or immediately if all public paths are unavailable.

Do not restart repeatedly without first preserving the failure evidence needed to distinguish process failure from dependency/readiness failure.

### Strata API High Error Rate

1. Identify affected matched route patterns and status-code families from the observability dashboard. Route labels must not contain raw tenant/resource IDs.
2. Correlate onset with deployments, dependency alerts, and authentication/authorization changes.
3. Inspect sanitized structured logs using deployment/correlation identifiers rather than copying request secrets or raw credentials.
4. If rollback is required, use `ROLLBACK.md`; never bypass signed artifacts, schema compatibility, or production validation to reduce the error rate.

### Strata API High P95 Latency

1. Compare request rate, in-flight requests, and p95 latency over the same window.
2. Check PostgreSQL pool saturation/slow operations, NATS state, object-storage latency, CPU, memory, and container/service throttling.
3. Identify the matched route family without introducing raw-path/resource-ID metric labels.
4. Scale or roll back only through the documented deployment lifecycle, then verify readiness plus the applicable synthetic/login/authenticated checks.

### Strata Job Metrics Collection Failed

1. Check database status in application readiness.
2. Verify the job-metrics collector has database connectivity and the expected migration state.
3. Treat missing job telemetry as loss of operational visibility, not proof that queues are empty.
4. Restore metrics collection and confirm fresh samples before closing the incident.

### Strata Oldest Job Stalled

1. Check dispatcher readiness, NATS/JetStream connectivity, queue depth, and active recovery/quiesce state.
2. Inspect the affected job and targets through authorized APIs/log correlation rather than direct state mutation.
3. Do not manually mark jobs successful or delete durable inbox/outbox/command records.
4. Restore the failed dependency or dispatcher and verify the oldest-job age returns toward steady state without duplicate destructive execution.

### Strata Job Failures

1. Separate failed, expired, retry-exhausted, and ambiguous/unknown outcomes and inspect only sanitized failure categories.
2. Check retry count, next retry time, agent reconnect state, approval state, maintenance-window state, and job expiry.
3. Retry only through the authorized idempotent job API/workflow; never blindly replay a destructive command with an ambiguous prior outcome.
4. Escalate repeated destructive-operation failures to the job/platform owner with the exact correlation ID and preserved evidence.

## Provider first-login setup

Provider setup is server-authoritative. An incomplete platform administrator session is expected to receive `provider_setup_required` for non-allowlisted administrative operations until required provider profile fields are completed.

Use the browser setup workflow and provider settings routes. Do not complete setup by editing provider/profile tables directly. Provider profile mutations and completion are audited transactionally; a failed audit append fails the mutation.

If setup appears stuck:

- verify the current migrations are applied and the service is ready;
- verify the administrator has an active platform-scoped owner/admin membership;
- return to platform scope if the session is switched into an MSP/client/site context;
- inspect the authenticated session/context endpoints through the supported UI/API tooling;
- retry the wizard/profile mutation rather than editing completion columns in SQL.

## MSP owner invitation/activation

Pending MSPs remain inactive until owner invitation acceptance completes. Do not manually mark the owner verified or activate a pending MSP in SQL.

For invitation delivery failures, correct SMTP/public-origin configuration and use the supported resend action. Resend rotates the invitation; older links must remain invalid. Raw invitation tokens are not recoverable from stored hashes and must not be reconstructed.

## Endpoint enrollment and identity

Create MSP/client/site state and scoped enrollment credentials through supported platform APIs/UI. Do not manufacture endpoint identities with direct database inserts.

After enrollment, verify that:

- endpoint identity persists across restart;
- AgentID/device identity mapping remains authoritative;
- telemetry is attributed to the correct tenant/device;
- reconnect/replay does not duplicate acknowledged telemetry or destructive work;
- cross-tenant and cross-endpoint operations fail closed.

Representative-host proof belongs in Issue #15 and must not be inferred from source CI.

## Patch and software operations

Patch/software dispatch is policy- and tenant-scoped. Before broad deployment:

- validate target inventory;
- use canary/maintenance-window controls where applicable;
- monitor durable command/result state;
- distinguish failed, unknown/ambiguous, expired, and retry-exhausted outcomes;
- treat `reboot_required` as state/evidence, not permission for an automatic reboot unless an explicit policy authorizes it.

Do not blindly replay destructive actions when the previous outcome is ambiguous after an agent or NATS restart.

## Backup

Run host-level backup preflight before a real backup:

```bash
strata-rmm backup --dry-run
```

Create an integrated encrypted recovery set with:

```bash
strata-rmm backup --timeout 2h
```

Verify and list sets through the recovery command, not by inspecting random files in the repository or storage bucket. See `BACKUP.md`.

## Restore

Never restore in place. Always use a clean distinct database, distinct recovery NATS target, and distinct recovery object-storage target when source storage is enabled.

Validate first:

```bash
strata-rmm recovery preflight
strata-rmm recovery verify --backup-id <backup-id>
strata-rmm recovery restore --backup-id <backup-id> --target-dsn '<clean-distinct-target-dsn>' --dry-run
```

Execute only with explicit confirmation after change approval:

```bash
strata-rmm recovery restore \
  --backup-id <backup-id> \
  --target-dsn '<clean-distinct-target-dsn>' \
  --confirm \
  --timeout 4h
```

Do not fall back to `DROP DATABASE`, raw `psql` replay, custom decrypt commands, or copied `.down.sql` files as a substitute for the integrated recovery engine.

## Upgrade and rollback

Native and Docker updates must use signed release metadata, semantic compatibility, shared preflight, health verification, and rollback rules.

- Native: use the supported CLI/in-app stage plus external finalizer described in `UPGRADE.md`.
- Docker: use the root-controlled host updater; it applies an immutable `repository@sha256` image and retains the previous digest.
- Kubernetes: automatic apply is not an accepted pre-beta lifecycle and must fail closed.

A failed upgrade must restore the exact previous artifact/digest when rollback can be proven or leave protected recovery state for operator intervention. Never replace a failed verified update with a mutable/manual redeploy.

## Dependency outages

### PostgreSQL/TimescaleDB

Treat database unavailability as a control-plane incident. Do not bypass transaction/outbox or recovery locks by editing state directly. Restore service, validate readiness, then allow durable reconciliation to proceed.

### NATS/JetStream

Expect agents to disconnect and use bounded local/durable replay behavior. Restore TLS/authenticated NATS, observe reconnect/redelivery, and verify duplicate-execution protection before declaring recovery.

### Object storage

Recording/report/recovery functions may degrade while core control-plane functions remain available. Restore the configured backend and verify object integrity/authorization before resuming affected workflows.

## Security incident rules

- Preserve immutable audit evidence.
- Rotate/revoke exposed credentials rather than editing logs to hide them.
- Do not weaken RLS, tenant authorization, NATS isolation, TLS, checksum/signature verification, audit immutability, or duplicate-execution prevention to restore service.
- Use sanitized logs and redacted configuration summaries.
- Escalate any suspected cross-tenant exposure as a security incident even if availability is otherwise normal.

## Hosted evidence

For Issue #15 exercises record acceptance ID, exact source/release/image/agent digests, isolated environment identity, reproducible commands/workflow, UTC start/end, expected and observed result, retained artifacts, cleanup, owner, status, and limitations.

A green repository workflow is necessary but not sufficient evidence for hosted internal-alpha approval.
