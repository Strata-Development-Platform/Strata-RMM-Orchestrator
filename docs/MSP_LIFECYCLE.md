# MSP lifecycle

Phase 8F provides platform-operated controls for onboarding, entitlement changes,
suspension, reactivation, and retention-safe offboarding. Platform routes require
`platform_owner` or `platform_admin`; MSP-scoped roles cannot invoke them.

## Entitlement grace periods

`PATCH /api/v2/platform/msps/{mspID}/entitlement` accepts:

```json
{
  "plan_slug": "professional",
  "status": "past_due",
  "grace_period_days": 14
}
```

`grace_period_days` is required for `past_due`, must be between 1 and 90, and is
rejected for other states. Returning an entitlement to `active`, `suspended`, or
`cancelled` clears the grace deadline.

## Offboarding

`POST /api/v2/platform/msps/{mspID}/offboarding` requires a reason and accepts a
retention period from 30 to 3650 days (default 90). The request runs inside the
API request transaction and:

- deactivates the MSP;
- cancels its entitlement and clears grace;
- revokes MSP, client, and site memberships;
- revokes active support grants and enrollment tokens;
- disables registered agents and managed devices;
- suspends custom domains;
- records the retention deadline and an immutable control-plane audit event.

Repeating the request is safe before deletion approval. It can extend, but cannot
shorten, the existing retention deadline.

`GET /api/v2/platform/msps/{mspID}/offboarding` returns the current state and
retention deadline.

Deletion approval is a separate guarded action:

`POST /api/v2/platform/msps/{mspID}/offboarding/approve-deletion`

```json
{
  "confirm_slug": "example-msp"
}
```

Approval requires the exact MSP slug, revoked access, and an expired retention
period. It records the approving operator and timestamp. It does not physically
delete tenant data.

## Current limits

- A complete, bounded customer export is not yet implemented.
- Physical deletion requires a separately reviewed executor and recovery
  procedure; approval alone never deletes data.
- Environment acceptance must demonstrate credential rejection, retained-data
  integrity, audit evidence, and cross-scope authorization before A8-21 can pass.
