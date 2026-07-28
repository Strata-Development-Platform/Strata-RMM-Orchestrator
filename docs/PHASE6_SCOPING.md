# Phase 6 — SaaS Control Plane

## Delivered scope

Phase 6 provides the production-oriented SaaS control-plane foundation used by
Strata operators and MSP tenants:

- Authoritative workspace context with roles, permissions, selectable scopes,
  tenant branding, subscription entitlement, and validated support access.
- Scope switching through a newly issued, eight-hour JWT. A caller cannot select
  an MSP, client, or site outside its active memberships.
- Platform MSP creation, activation, suspension, plan assignment, usage
  metering, and quota enforcement.
- MSP client, site, membership, branding, custom-domain, and enrollment-token
  administration.
- Provider-neutral custom-domain lifecycle: DNS TXT verification followed by a
  certificate request state. An ingress controller reports certificate results
  through the platform-only certificate callback.
- One-time enrollment secrets. Only hashes are stored; revealed tokens are
  shown once and can be revoked.
- Device and user quotas enforced on enrollment and membership creation.
- Append-only control-plane audit records for administrative mutations.
- A responsive MSP operations workspace with overview, clients/sites,
  membership, branding, domains, enrollment, and audit sections.
- Forced PostgreSQL row-level security for entitlements, usage snapshots, and
  control-plane audit data.

## Acceptance boundary

This phase is complete when:

1. All GitHub checks pass on the exact PR head.
2. Cross-MSP reads and writes are denied by database integration tests.
3. Platform-only subscription and certificate routes are classified as admin.
4. The frontend typecheck, lint, and production build pass.
5. The migration and rollback procedures below are reviewed.

Phase 6 does not claim feature parity with a complete Kaseya or Datto product.
It completes the SaaS tenancy and control-plane phase on which later RMM
operations phases build.
