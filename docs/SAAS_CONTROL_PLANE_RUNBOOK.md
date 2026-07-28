# SaaS Control Plane Runbook

## Deployment

1. Back up PostgreSQL and record the deployed application commit.
2. Deploy the application binary. Schema migrations 53 and 54 run through the
   existing schema manager and are transactional per migration.
3. Confirm `/health` is healthy.
4. Confirm the `SaaS Control Plane` and `Database Isolation` GitHub checks passed
   on the deployed commit.
5. Sign in as a platform administrator, create a test MSP, open its workspace,
   and confirm the Free entitlement and empty usage counters.

## Custom domains

Cloudflare is optional. Any reverse proxy or certificate controller may be used.

1. The MSP adds `rmm.example.com`.
2. The portal displays `_strata-verification.rmm.example.com` and its random TXT
   value.
3. The MSP publishes that TXT record and selects **Verify DNS**.
4. The application performs an authoritative resolver lookup. A match changes
   the domain to `verified` and the certificate state to `requested`.
5. The ingress controller provisions TLS and calls:

   `PATCH /api/v2/platform/domains/{domainID}/certificate`

   with `{"status":"issued"}` using a platform-admin service identity. Failed or
   expired issuance is reported with `failed` or `expired`.
6. Route only domains whose verification status is `active`.

Never accept a hostname based solely on an HTTP header. Domain resolution is
performed by the branding middleware against an active database record.

## Subscription operations

- Platform administrators update plan and state in the MSP workspace.
- `active` allows new memberships and device enrollment within plan limits.
- `past_due`, `suspended`, and `cancelled` reject new billable resources.
- A plan limit of zero means unlimited; positive limits are enforced.
- Existing managed devices remain visible when a subscription becomes
  non-active so administrators can recover or export data.

## Enrollment

- Tokens expire after 24 hours and default to one use.
- Only token hashes are persisted.
- Revoke tokens immediately when an installer or ticket is no longer active.
- Agent registration consumes the token and device quota in one transaction;
  a failed or over-quota registration rolls the token use back.

## Security checks

Review the MSP audit tab for:

- tenant lifecycle changes;
- plan and subscription changes;
- membership grants and revocations;
- branding and domain changes;
- enrollment token creation and revocation;
- workspace switches made by privileged users.

Support access continues to require an active, unexpired support grant.

## Rollback

Application rollback is preferred before schema rollback because migrations 53
and 54 are additive.

1. Stop writes and back up PostgreSQL.
2. Roll back the application to the recorded pre-Phase-6 commit.
3. Leave additive tables in place unless a clean schema rollback is mandatory.
4. For a mandatory schema rollback, first export `control_plane_audit`,
   `usage_snapshots`, and `plan_entitlements`, then run migration downs in
   reverse order: 54 followed by 53.
5. Restore the pre-Phase-6 database backup if any rollback validation fails.

Schema rollback deletes subscription and audit history and is therefore a
destructive, last-resort operation.
