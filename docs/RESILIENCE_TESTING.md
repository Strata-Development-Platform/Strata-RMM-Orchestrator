# Resilience Testing

Phase 8E separates short, deterministic CI contracts from exercises that must
run against an isolated deployment. A short CI test is not evidence of a
24-hour soak, production-scale capacity, or recovery from a real dependency
outage.

## Bounded load runner

The repository ships a dependency-free HTTP load runner:

```bash
STRATA_RESILIENCE_BEARER_TOKEN='redacted' \
  strata-rmm resilience \
  --base-url https://alpha-rmm.example.com \
  --path /api/v1/auth/me \
  --label authenticated_api \
  --duration 10m \
  --rate 100 \
  --concurrency 20 \
  --request-timeout 10s \
  --max-error-rate 0.01 \
  --max-p95 500ms
```

It exits nonzero when the error-rate or p95 threshold fails and writes one JSON
report. The report contains only the path, counts, timing, percentiles, and
threshold result. It never contains the bearer token, hostname credentials,
response body, or raw request headers.

Remote targets require HTTPS. Redirects are not followed, and base URLs
containing credentials, queries, or paths are rejected. Use an isolated alpha
or beta environment; never point an unscheduled load exercise at production.

## Required environment profiles

| Profile | Duration | Purpose | Acceptance use |
|---|---:|---|---|
| CI contract | under 5 minutes | runner, threshold, jitter, and degradation behavior | implementation evidence only |
| Alpha smoke load | 10–30 minutes | validate expected internal cohort and basic headroom | internal-alpha gate |
| Beta baseline | at least 60 minutes | agreed MSP, technician, and agent concurrency | A8-14 |
| Beta soak | at least 24 hours | detect unbounded resource, lock, connection, and queue growth | A8-15 |
| Reconnect storm | until steady state | disconnect and reconnect the agreed simulated fleet | A8-16 |
| Dependency matrix | outage plus recovery per dependency | PostgreSQL, NATS, storage, DNS, and outbound providers | A8-17 |

## Mandatory evidence

Every non-CI exercise records:

- exact release SHA and immutable deployment identity;
- environment identifier and topology;
- sanitized configuration and workload profile;
- start and end timestamps;
- request rate, concurrency, and simulated-agent count;
- p50, p95, p99, maximum latency, throughput, and error rate;
- process memory, goroutines, open connections, database pool, queue depth, and
  oldest-job age at regular intervals;
- injected failure timestamps and recovery timestamps;
- tenant-isolation and duplicate-execution checks;
- links to dashboards and sanitized logs;
- operator, result, deviations, and residual risks.

## Failure rules

An exercise fails if any agreed threshold fails, any required sample is
missing, a secret appears in evidence, tenant data crosses scope, destructive
work executes twice, readiness remains healthy while a required dependency is
unusable, recovery requires undocumented mutation, or resource/queue growth
does not return to a stable bound.

Do not average away an outage. Record every error interval and the worst
observed percentile. Do not rerun a failed exercise until it happens to pass;
identify and remediate the cause, then run a new exercise against a new exact
head.

## Reconnect policy

Agent and orchestrator NATS clients use capped exponential backoff with full
jitter. This disperses reconnect attempts across the retry window rather than
allowing a fleet-wide fixed-delay wave. The beta reconnect-storm exercise must
still prove the behavior with the agreed fleet size, JetStream workload, and
database capacity.
