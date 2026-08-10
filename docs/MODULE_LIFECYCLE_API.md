# Module Lifecycle API

## Purpose

The module lifecycle API is the platform-operator control plane for changing durable add-on state. It is intentionally separate from module service credentials: module JWTs can call only registered module-service routes, while lifecycle mutations require an authenticated user with a database-proven platform-global `platform_owner` or `platform_admin` grant.

## Endpoints

- `POST /api/v2/platform/modules` installs a validated module manifest in the `installed` state.
- `POST /api/v2/platform/modules/{moduleID}/enable` enables an installed or disabled module unless it is quarantined.
- `POST /api/v2/platform/modules/{moduleID}/disable` disables a module and requires an operator reason.
- `POST /api/v2/platform/modules/{moduleID}/quarantine` quarantines a module and requires an operator reason.
- `DELETE /api/v2/platform/modules/{moduleID}/uninstall` removes a non-enabled module and requires an operator reason.

The install request body is the module manifest itself. Disable, quarantine, and uninstall accept exactly one JSON object of the form:

```json
{"reason":"operator maintenance"}
```

Unknown JSON fields, empty reasons, multiple JSON values, and bodies larger than the configured lifecycle limit are rejected.

## Authorization boundary

Lifecycle paths are recognized by exact HTTP method and path shape inside the existing access-control middleware. Only those paths are promoted to `AccessAdmin`; unknown actions and wrong methods remain `AccessDenied`.

Before dispatch, the normal user-token validation, active-user lookup, database-backed authorization resolution, provider setup gate, and `AuthorizationResult.IsPlatformGlobal()` check all run. The lifecycle handler repeats the platform-global check and reads the audit actor from the authenticated `ctxKeyUserID`. The request body, query string, and headers cannot choose or replace the audit actor.

## Transaction and audit model

Each mutation runs in a serializable PostgreSQL transaction. The transaction establishes transaction-local platform RLS context (`app.role=platform_admin`, authenticated `app.user_id`, and platform scope), restores the current durable module registry, performs the lifecycle transition, persists it through `modules.SQLStore`, and appends the existing immutable module audit record before commit.

Serialization failures and deadlocks are retried a bounded number of times. The live authorization registry is never changed before the database commit succeeds. After a successful commit, the runtime refresh deadline is invalidated so the next module authorization request reloads committed lifecycle state immediately through the durable refresh mechanism introduced in PR #121.

This means a disable, quarantine, manifest permission reduction, or uninstall cannot be made visible to authorization before its audit/database transaction commits, and a failed transaction cannot partially alter the live authorization view.

## State rules

The API inherits the fail-closed registry rules:

- a newly installed module is not enabled automatically;
- a quarantined module cannot be enabled through the normal enable operation;
- an enabled module must be disabled before uninstall;
- undeclared or invalid manifest permissions are rejected at install;
- every persisted transition is validated against the previous durable state.

## Remaining add-on runtime work

This API controls lifecycle state only. Alpha-complete executable add-ons still require signed package ingestion, trusted publisher/package verification, process or container sandboxing, runtime resource limits, event-bus/service credential provisioning, install/update/rollback artifact handling, and the operator UI that consumes this API.
