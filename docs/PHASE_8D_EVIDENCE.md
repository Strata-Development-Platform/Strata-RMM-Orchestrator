# Phase 8D Evidence

This document is the implementation ledger for observability, synthetics, log
safety, and alert/runbook wiring. Acceptance remains pending until exact-head
CI and an environment-level alert delivery exercise are recorded.

| Gate | Runtime behavior | Negative proof | Deployment evidence | Status |
|---|---|---|---|---|
| A8-10 | bounded-cardinality HTTP traffic, error, latency, and in-flight metrics | raw resource IDs excluded from labels | Prometheus scrape and Grafana platform dashboard | CI verified; environment alert delivery pending |
| A8-11 | live durable-job status, retry, age, and collector-health metrics | database collection failure emits failure gauge rather than empty queue | job alerts and runbooks | CI and PostgreSQL integration verified |
| A8-12 | independent liveness, readiness, login, authenticated API, device, and recording-path runner | login failure stops dependent checks; remote plaintext URL rejected | `strata-rmm synthetic` | implemented; environment exercise pending |
| A8-13 | metrics token redaction, constant-time comparison, route-pattern-only request telemetry | missing/wrong token rejected; raw IDs and credentials absent from results/logs | focused secret-safety CI job | CI verified; broader Phase 8G audit remains |
| A8-25 | every Phase 8D alert has severity, owner, and tested runbook anchor | CI rejects missing metadata/anchors | Prometheus rules and `docs/RUNBOOK.md` | implemented; paging delivery exercise pending |

## Exact-head evidence

Populate only after terminal GitHub Actions results exist.

| Head SHA | Phase 8D workflow | Full CI | Result |
|---|---|---|---|
| `2d9d91f60d2909fcf46b360221b0ca97a7549fcb` | [Phase 8D run 30604540531](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30604540531) — 6/6 | [CI run 30604540529](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30604540529) — 30/30; [Phase 8C regression 30604540595](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/actions/runs/30604540595) — 20/20 | Implementation head passed; environment paging/synthetic exercise remains |
