# Beta Go/No-Go Record

## Current decision

**NO-GO for production beta.**

The software may enter a bounded internal alpha only after Phase 8G exact-head CI
passes and a staffed hosted smoke exercise verifies login, tenant-scoped API,
agent enrollment/check-in, a non-destructive durable job, readiness, storage when
enabled, alert visibility, and rollback ownership.

## Blocking evidence

| Gate group | Current state | Required before beta |
|---|---|---|
| Recovery A8-09 | not accepted | timestamped isolated RPO/RTO drill |
| Observability A8-10/A8-12/A8-25 | partial | hosted synthetic, page delivery, and runbook/escalation audit |
| Resilience A8-14–A8-17 | harness only | baseline, 24-hour soak, reconnect storm, dependency-failure matrix |
| MSP lifecycle A8-18–A8-21 | CI contracts | hosted onboarding/domain/offboarding exercise |
| Security A8-22/A8-23 | exact-head CI pending | passing exact-head workflows and evidence links |
| Incident response A8-24 | procedure only | signed four-scenario tabletop record |
| External review | scoped only | penetration test, blocker remediation, and independent re-test |
| Final decision A8-26 | unsigned | risk, operations, security, product, and release-owner signatures |

## Sign-off template

Record candidate SHA/release/digest, environment, acceptance-evidence index,
unresolved risks and expiry dates, beta cohort and quotas, support window,
incident/rollback/recovery owners, and decision timestamp. Required signatories:
Security, Platform Operations, Endpoint Operations, Product, and Release Owner.

This checked-in template is not a signature and must never be converted to GO by
CI alone. The signed record belongs in the approved operational evidence system
and must link back to immutable workflow and exercise evidence.
