# Phase 6 — SaaS Control Plane

## Current implementation

- Workspace context endpoint with roles, derived permissions, available scopes,
  tenant branding, subscription entitlement, and validated support-grant context.
- Platform-only MSP list and create APIs.
- MSP creation atomically validates and assigns a plan entitlement.
- Tenant-scoped row-level security for plan entitlements, including forced RLS.
- Platform-only MSP navigation and route guard in the web UI.
- Dedicated CI coverage for control-plane authorization and entitlement isolation.

## Remaining scope

This phase is intentionally incomplete. Before the pull request can leave draft:

- Complete MSP lifecycle operations and their audit records.
- Implement subscription state transitions, usage metering, and quota enforcement.
- Implement support-grant creation/revocation workflows with approval controls.
- Add tenant switching and branding application to the authenticated shell.
- Add end-to-end browser tests for platform and MSP personas.
- Document migration, rollback, and operational runbooks.

Do not describe Phase 6 as complete until every remaining item has executable
acceptance tests and all required GitHub checks pass on the current PR head.
