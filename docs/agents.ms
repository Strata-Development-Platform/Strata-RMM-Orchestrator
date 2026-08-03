# Agent remediation and learning ledger

This file is append-only. It records mistakes, their causes, corrective rules, evidence, and remaining limitations. An implementation label, commit message, route, schema, UI control, mock, or green unrelated test is never acceptance evidence.

## 2026-08-03 — Post-PR #44 authorization and migration audit remediation

- Base audited: `7ecac6824c6cde8e7c9eb92a7f8bfc5ceff8c927` (origin/master HEAD)
- PR/commit being remediated: `#44 / 7ecac6824c6cde8e7c9eb92a7f8bfc5ceff8c927`
- Observation: PR #44 claimed "explicit authorization for billing, reports, retention, and CMDB" but source inspection of merge commit 7ecac68 showed 8 handlers still missing Authorize calls: handleCreateBillingAccount, handleDeleteBillingAccount, handleCreateSubscription, handleSubmitUsage, handleGetUsage, handleAddPaymentMethod (billing), handleCreateSchedule, handleDeleteSchedule (reports), handleGetRetention, handleUpdateRetention (retention). Also, standalone migrations 86-88 contained invalid PostgreSQL syntax (CREATE POLICY IF NOT EXISTS, CREATE TRIGGER IF NOT EXISTS, DROP POLICY IF NOT EXISTS). Migration 87 attempted to ADD COLUMN client_id TEXT which was silently skipped by IF NOT EXISTS because migration 86 already created client_id UUID. CMDB package-sync bug: loop modifies req.Packages[0].PackageType, continues with unchanged loop variable, omits package_type SQL argument, ignores ExecContext errors, reports input length as synced count. Retention handler calls Timescale global retention on shared hypertables from tenant endpoint, logs errors but returns success.
- Root cause: Agent conflated route classification (AccessAdmin default) with handler-level authorization. Agent counted route-level authentication as equivalent to resource-level authorization. Agent assumed embedded migration fixes propagated to standalone files. Agent did not enumerate every registered handler against the authorization claim. Agent fixed SQL from memory without execution evidence against real PostgreSQL. Agent did not distinguish application scope from database ownership for CMDB context values.
- Corrective rule: Enumerate every registered route, identify its handler, operation type, required role, scope source, resource-ownership query, and RLS table. Test every row. Route classification is not handler authorization. Context values establish claimed scope; they do not prove resource ownership. SQL syntax and rollback safety require execution evidence against real PostgreSQL, not memory. Standalone and embedded migrations must be reconciled mechanically. Tests must assert outcomes at the boundary being claimed.
- Code correction: See remediation PR entries below.
- Tests added: See remediation PR entries below.
- Local evidence: Go 1.24 environment available. PostgreSQL/TimescaleDB integration tests require local database. CI evidence is terminal and exact-head.
- Exact-head CI evidence: Pending implementation.
- Failures/reruns: None yet.
- Infrastructure evidence: Environment pending for hosted acceptance.
- Limitations/remaining work: Real OIDC provider integration not implemented. Billing provider integration not implemented. Full retention tiering (hot/warm/cold) deferred. PR #46 password recovery + refresh tokens deferred. Migration 71-85 standalone files do not exist (embedded-only). Migration 87 client_id TEXT column renamed to oauth_client_id in this PR — embedded schema.go already uses this naming from PR #44's embedded fix. CMDB ownership verification at database boundary requires future work.
- Merge state: Draft and unmerged; explicit owner authorization required.
