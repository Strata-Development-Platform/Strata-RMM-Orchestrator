# Phase 8E Evidence

Phase 8E covers A8-14 through A8-17. Implementation and CI evidence do not
replace the required isolated-environment exercises.

| Gate | Implementation | CI proof | Environment proof | Status |
|---|---|---|---|---|
| A8-14 baseline load | bounded HTTP load runner with enforced latency and error thresholds | pending exact-head Phase 8E CI | agreed MSP, technician, API, and agent workload report | pending |
| A8-15 soak | evidence profile and explicit resource/queue stability requirements | evidence contract validation only | uninterrupted 24-hour report | pending |
| A8-16 reconnect storm | capped exponential full-jitter NATS reconnect policy | pending exact-head reconnect tests | simulated-fleet disconnect/recovery report | pending |
| A8-17 dependency failure | readiness/degradation regression contract and required matrix | pending exact-head readiness tests | PostgreSQL, NATS, storage, DNS, and provider outage/recovery report | pending |

## Exact-head CI

Populate only after every required workflow is terminal.

| Head SHA | Phase 8E workflow | Full CI | Result |
|---|---|---|---|
| Pending | Pending | Pending | Not accepted |

## Internal-alpha boundary

Internal alpha requires a 10–30 minute isolated smoke-load exercise, successful
alert and synthetic delivery, dependency readiness spot checks, and a named
rollback operator. It does not require pretending that A8-14 through A8-17 are
fully accepted. Those gates remain pending until their beta-scale evidence is
recorded.
