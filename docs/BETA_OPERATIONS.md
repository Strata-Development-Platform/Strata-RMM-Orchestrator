# Internal Alpha and Beta Operations

## Service catalog and support boundary

| Service | Internal-alpha support | Owner | Customer-impact boundary |
|---|---|---|---|
| Control-plane API and unified portal | supported for approved synthetic/internal tenants | Platform Operations | authentication, tenancy, dashboards, API workflows |
| Agent enrollment, inventory, and durable jobs | supported on the documented compatibility matrix | Endpoint Operations | enrollment, check-in, queued and approved work |
| PostgreSQL, JetStream, object storage | operated dependencies | Platform Operations | state, durable work, reports/recordings |
| Remote access and destructive automation | restricted; explicit approval and staffed observation required | Security + Endpoint Operations | interactive access and endpoint change |
| Custom domains and external providers | experimental unless separately approved | Platform Operations | DNS/certificate/provider availability |
| White-label/commercial licensing | contractual enablement only | Product/Legal | branding entitlement; never a client-side security toggle |

Internal alpha provides best-effort support during staffed test windows. It has
no production availability commitment and must not contain uncontrolled customer
data. Beta SLOs become enforceable only after A8-10 and A8-14–A8-17 environment
evidence establishes a measured baseline.

## Cohort admission

An alpha tenant must have an accountable sponsor, synthetic or explicitly
approved data, compatible agent platforms, a documented test objective, support
contacts, maintenance consent, and a removal date. Exclude regulated/production
workloads, unsupported integrations, safety-critical endpoints, and any tenant
whose incident/export/deletion ownership is unclear.

## Change and maintenance policy

- Use immutable release identities and the documented deployment preflight.
- Record owner, scope, risk, migration compatibility, rollback target, smoke
  tests, and communication plan before mutation.
- Schedule disruptive work in an announced staffed window. Emergency changes
  require incident-command ownership and retrospective review.
- Do not weaken production validation to recover or roll back.
- Publish status updates with impact, affected scope, mitigation, and next-update
  time; never include credentials or cross-tenant identifiers.

## Escalation and communications

| Trigger | Immediate owner | Escalation |
|---|---|---|
| Cross-tenant access, key loss, malicious command | Security lead | SEV-1 incident commander immediately |
| API/control-plane unavailable or durable work stalled | Platform Operations | incident commander; Endpoint Operations if jobs affected |
| Agent fleet degradation | Endpoint Operations | Platform Operations and Security if identity/replay suspected |
| Storage/report/recording integrity failure | Platform Operations | Security and tenant liaison |
| Entitlement/billing inconsistency | Control Plane owner | Product/Finance; suspend enforcement if unsafe, preserving audit |

Maintenance notices state start/end, affected capabilities, expected impact,
operator contact, and rollback condition. Incident notices follow
[Security Incident Response](INCIDENT_RESPONSE.md). Status-page publication is a
manual operator responsibility until a tested provider integration is approved.

## Pause, rollback, and exit criteria

Pause admissions or deployment on any critical/high security finding, unexplained
cross-scope result, data-integrity loss, duplicate destructive execution,
unrecoverable queue growth, missed mandatory alert, or absence of a verified
rollback/recovery owner. Exit an alpha tenant by revoking credentials, exporting
only when authorized, applying retention, and following the auditable offboarding
workflow. Promotion to beta requires every mandatory A8 gate and the signed
[go/no-go record](BETA_GO_NO_GO.md).
