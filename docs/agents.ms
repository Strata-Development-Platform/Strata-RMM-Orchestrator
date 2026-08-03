# Agent remediation and learning ledger

This file is append-only. It records mistakes, their causes, corrective rules, evidence, and remaining limitations. An implementation label, commit message, route, schema, UI control, mock, or green unrelated test is never acceptance evidence.

## 2026-08-03 — Post-PR #44 authorization and migration audit remediation

- Base audited: `7ecac6824c6cde8e7c9eb92a7f8bfc5ceff8c927` (origin/master HEAD)
- PR/commit being remediated: `#44 / 7ecac6824c6cde8e7c9eb92a7f8bfc5ceff8c927`
- Observation: PR #44 claimed "explicit authorization for billing, reports, retention, and CMDB" but source inspection of merge commit 7ecac68 showed 8 handlers still missing Authorize calls: handleCreateBillingAccount, handleDeleteBillingAccount, handleCreateSubscription, handleSubmitUsage, handleGetUsage, handleAddPaymentMethod (billing), handleCreateSchedule, handleDeleteSchedule (reports), handleGetRetention, handleUpdateRetention (retention). Also, standalone migrations 86-88 contained invalid PostgreSQL syntax (CREATE POLICY IF NOT EXISTS, CREATE TRIGGER IF NOT EXISTS, DROP POLICY IF NOT EXISTS). Migration 87 attempted to ADD COLUMN client_id TEXT which was silently skipped by IF NOT EXISTS because migration 86 already created client_id UUID. CMDB package-sync bug: loop modifies req.Packages[0].PackageType, continues with unchanged loop variable, omits package_type SQL argument, ignores ExecContext errors, reports input length as synced count. Retention handler calls Timescale global retention on shared hypertables from tenant endpoint, logs errors but returns success.
- Root cause: Agent conflated route classification (AccessAdmin default) with handler-level authorization. Agent counted route-level authentication as equivalent to resource-level authorization. Agent assumed embedded migration fixes propagated to standalone files. Agent did not enumerate every registered handler against the authorization claim. Agent fixed SQL from memory without execution evidence against real PostgreSQL. Agent did not distinguish application scope from database ownership for CMDB context values.
- Corrective rule: Enumerate every registered route, identify its handler, operation type, required role, scope source, resource-ownership query, and RLS table. Test every row. Route classification is not handler authorization. Context values establish claimed scope; they do not prove resource ownership. SQL syntax and rollback safety require execution evidence against real PostgreSQL, not memory. Standalone and embedded migrations must be reconciled mechanically. Tests must assert outcomes at the boundary being claimed.
- Code correction:
  - `internal/platform/middleware.go`: Added `AccessDenied` constant; `classifyRoute` returns `AccessDenied` for unknown routes; `withAccessControl` returns 403 for `AccessDenied`
  - `internal/platform/billing_handlers.go`: Added `AuthorizeMSPAccess`/`AuthorizeMSPManage` to 6 handlers
  - `internal/platform/report_handlers.go`: Added `AuthorizeClientAccess` to 2 handlers
  - `internal/platform/api.go`: Added `AuthorizeClientAccess` to 2 retention handlers; added `FeatureFlags` struct; gated physical retention mutation
  - `internal/platform/device_handlers.go`: Fixed package-sync bugs (transaction, local variable, SQL args, error handling, count)
  - `migrations/00086_client_auth_providers.{up,down}.sql`: DO blocks for CREATE POLICY/TRIGGER; DROP POLICY IF EXISTS
  - `migrations/00087_client_portal_enhancements.{up,down}.sql`: Renamed `client_id TEXT` to `oauth_client_id`; down no longer drops FK
  - `migrations/00088_client_session_activity.{up,down}.sql`: Same DO block and DROP POLICY fixes
  - `docs/agents.ms`: Created remediation ledger
- Tests added:
  - `TestAccessControlPrivilegedNamespaceFailClosed` (403 for unclassified privileged namespace routes)
  - `TestProtectedRoutesRejectMissingCredentials` updated (POST method for remote session)
  - `TestPrivilegedNamespacesFailClosed` updated (AccessDenied instead of AccessAdmin)
  - `TestRouteInventoryComplete` updated (AccessDenied default check)
- Local evidence: Go build succeeds. All `go test ./...` pass locally. PostgreSQL/TimescaleDB integration tests require local database (unavailable).
- Exact-head CI evidence: PR #45 created at https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/pull/45. CI runs pending.
- Failures/reruns: None.
- Infrastructure evidence: Environment pending for hosted acceptance, real PostgreSQL integration tests, RLS testing.
- Limitations/remaining work: Real OIDC provider integration not implemented. Billing provider integration not implemented. Full retention tiering (hot/warm/cold) deferred. PR #46 password recovery + refresh tokens deferred. Migration 71-85 standalone files do not exist (embedded-only). Migration 87 client_id TEXT column renamed to oauth_client_id in this PR — embedded schema.go also needs updating for source parity. CMDB ownership verification at database boundary requires future work. RLS policy additions require future work.
- Merge state: Draft and unmerged; explicit owner authorization required.
