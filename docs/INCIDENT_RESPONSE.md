# Security Incident Response

## Roles and severity

| Role | Responsibility |
|---|---|
| Incident commander | owns severity, decisions, timeline, and handoffs |
| Security lead | containment, evidence preservation, credential/key response |
| Operations lead | service recovery, rollback, monitoring, status updates |
| Tenant liaison | scoped customer notification and follow-up |
| Recorder | immutable event timeline, actions, approvals, and evidence links |

SEV-1 includes suspected cross-tenant disclosure, signing/encryption-key loss,
malicious privileged command execution, or control-plane outage without a safe
workaround. SEV-2 covers contained security degradation or material partial
outage. Never lower severity merely to meet an SLO.

## Response sequence

1. Open an incident record; assign the roles above and a correlation ID.
2. Preserve logs, audit rows, deployment identity, queue state, and relevant
   object hashes before destructive remediation.
3. Bound the affected MSP/client/site/device, time interval, identities, tokens,
   jobs, and data classes. Do not export unrelated tenant data.
4. Contain with the narrowest safe action: revoke identity/grant/token, suspend
   tenant, pause dispatcher, isolate candidate release, or block enrollment.
5. Rotate exposed material using the credential-specific procedure. For JWT
   rotation, deploy the new `JWT_SECRET` with the old value temporarily in
   `JWT_SECRET_PREVIOUS`; remove the previous value no later than the maximum
   accepted token lifetime. A key believed actively abused skips overlap and
   requires immediate invalidation.
6. Recover from a verified release/backup, verify readiness and tenant isolation,
   then restore traffic gradually.
7. Notify affected tenants using confirmed facts, scope, mitigations, and the
   next update time. Never include another tenant's identifiers or evidence.
8. Close only after monitoring, evidence preservation, owner sign-off, and a
   tracked corrective-action review.

## Mandatory tabletop scenarios

| Scenario | Exercise decisions | Required evidence |
|---|---|---|
| Tenant boundary breach | containment scope, support-grant/token revocation, evidence-safe export, notification | timeline, affected-scope proof, cross-tenant negative re-test |
| Control-plane outage | dependency diagnosis, dispatcher pause, recovery/rollback, degraded communication | readiness history, recovery identity, smoke results |
| Signing/encryption key loss | compromise assumption, overlap versus immediate revocation, escrow/recovery, re-enrollment impact | rotation timestamps, old-key rejection proof, affected-token inventory |
| Malicious endpoint command | cancel/pause, attempt/correlation containment, approval and actor review, device follow-up | immutable job/audit chain, duplicate-execution check |

## Tabletop record template

Record date, environment, scenario, participants and roles, initial signal,
timeline, decisions, evidence links, notification decision, recovery result,
missed controls, corrective-action owners/dates, and signatures. A completed and
signed record must live in the approved operational evidence system—not in this
source template. Repository CI validates the procedure exists; it does not claim
that A8-24 has been exercised or signed.
