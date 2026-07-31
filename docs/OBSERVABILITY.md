# Production Observability and Synthetics

Phase 8D uses Prometheus for platform telemetry and Grafana for operator
dashboards. Customer device metrics remain in TimescaleDB and are not exported
as platform labels.

## Metrics security

Set `STRATA_METRICS_TOKEN` to a dedicated random value of at least 32
characters. It must not be a user JWT, administrator password, or NATS token.
The `/metrics` endpoint returns 404 when the value is absent and requires
constant-time bearer authentication when configured.

For Docker Compose, place the same value in a root-readable file and set
`STRATA_METRICS_TOKEN_FILE` to its path. Prometheus reads the file through
`authorization.credentials_file`; the secret is not committed in its
configuration.

For Helm, create the referenced secret before rollout and set
`orchestrator.observability.metrics.existingSecret` and `secretKey` to its name
and key. The chart never generates or stores the token in values:

```bash
kubectl -n strata-rmm create secret generic strata-rmm-observability \
  --from-literal=metrics-token='value-from-secret-manager'
```

## Dashboard and alert policy

The `Strata RMM Platform Operations` dashboard covers API traffic, errors,
latency, saturation, durable-job state, retry accumulation, and oldest active
work. HTTP labels use matched route patterns, never raw paths, tenant IDs,
device IDs, email addresses, or query values.

Rules in `deploy/prometheus/alerts.yml` include severity, owner, and a tested
runbook link. Missing job metrics is an alert, not an empty-queue result.

## Independent synthetic run

Use a dedicated, least-privilege synthetic user and tenant. Credentials are
accepted only through the environment so they do not appear in process
arguments:

```bash
export STRATA_SYNTHETIC_EMAIL='synthetic@example.invalid'
export STRATA_SYNTHETIC_PASSWORD='from-secret-manager'
export STRATA_SYNTHETIC_TENANT_ID='synthetic-tenant-id'
strata-rmm synthetic --base-url https://rmm.example.com
```

The command checks:

1. public liveness;
2. readiness and its dependencies;
3. real login;
4. an authenticated identity request;
5. the authorized agent/device path;
6. the authorized recording-storage path.

It emits one sanitized JSON record per check and exits nonzero when any check
fails. Passwords and returned session tokens are never included in results.
Run it from infrastructure independent of the application cluster and alert on
any nonzero exit. Do not use a production customer account.

The production binary/systemd deployment includes
`deploy/strata-rmm-synthetic.service` and `.timer`. Install them on an
independent probe host, store the base URL and three credential variables in
root-owned mode-0600 `/etc/strata-rmm/synthetic.env`, and enable the timer.
Monitor the oneshot unit's failed state from the external paging system.

## Current acceptance boundary

Code, deployment configuration, dashboards, alert metadata, and synthetic
behavior can be CI-verified. A8-10 through A8-13 and A8-25 are not finally
accepted until exact-head CI passes and an operator records an injected alert
delivery/test from the supported beta environment.
