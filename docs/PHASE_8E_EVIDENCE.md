# Phase 8E Evidence

Phase 8E covers A8-14 through A8-17. Implementation and CI evidence do not
replace the required isolated-environment exercises.

| Gate | Implementation | CI proof | Environment proof | Status |
|---|---|---|---|---|
| A8-14 baseline load | bounded HTTP load runner with enforced latency and error thresholds | threshold contract verified in CI | agreed MSP, technician, API, and agent workload report | environment baseline pending |
| A8-15 soak | evidence profile and explicit resource/queue stability requirements | evidence contract validation only | uninterrupted 24-hour report | pending |
| A8-16 reconnect storm | capped exponential full-jitter NATS reconnect policy | reconnect dispersion and runtime wiring verified in CI | simulated-fleet disconnect/recovery report | environment storm pending |
| A8-17 dependency failure | readiness/degradation regression contract and required matrix | degraded-readiness contract verified in CI | PostgreSQL, NATS, storage, DNS, and provider outage/recovery report | environment matrix pending |

## Exact-head CI

Populate only after every required workflow is terminal.

| Head SHA | Phase 8E workflow | Full CI | Result |
|---|---|---|---|
| `f027ec11f4d9f8c5fedb71a897cc6d46c39eb6b3` | [Phase 8E run 30605843176](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30605843176) — 5/5; [Phase 8D regression 30605843142](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30605843142) — 6/6 | [CI run 30605843147](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30605843147) — 30/30 after one infrastructure-only job rerun; [Phase 8C regression 30605843189](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30605843189) — 20/20 | Implementation head passed 61/61; environment gates remain pending |

## Internal-alpha boundary

Internal alpha requires a 10–30 minute isolated smoke-load exercise, successful
alert and synthetic delivery, dependency readiness spot checks, and a named
rollback operator. It does not require pretending that A8-14 through A8-17 are
fully accepted. Those gates remain pending until their beta-scale evidence is
recorded.
