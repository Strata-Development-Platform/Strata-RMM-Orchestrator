# Policy Engine

## Overview

The policy engine provides a centralized, hierarchical configuration system for managing how devices are monitored, patched, and maintained. Policies are defined at higher levels of the deployment hierarchy and inherited downward, with more specific levels overriding less specific ones.

---

## Policy Categories

| Category | Description | Key Fields |
|----------|-------------|------------|
| **Patch Management** | Define patch approval mode, severity filters, platforms, maintenance windows | `approval_mode`, `severity`, `platforms`, `device_filter`, `max_retries` |
| **Alerting Rules** | Threshold and heartbeat alert definitions with severity, cooldown, notification channels | `metric_name`, `condition`, `threshold`, `timeout`, `severity`, `cooldown` |
| **Monitoring Thresholds** | Default metric collection intervals, retention policies per device/site | `collection_interval`, `retention_days`, `enabled_metrics` |
| **Software Deployment** | Approval policies for software installs, allowed package types, source restrictions | `approval_mode`, `allowed_types`, `allowed_sources` |
| **Script Execution** | Execution policies, allowed languages, timeout limits, parameter constraints | `timeout_max`, `allowed_languages`, `requires_approval` |
| **Maintenance Windows** | Scheduled time blocks for patch deployment, script execution, reboot | `start_time`, `end_time`, `days_of_week`, `timezone` |

---

## Inheritance Model

### Hierarchy

```
Platform Level  (operator-defined defaults)
  |
  └─ MSP Level  (MSP-specific overrides)
       |
       └─ Client Level  (client-specific overrides)
            |
            └─ Site Level  (site/location-specific overrides)
                 |
                 └─ Device Level  (device-specific overrides)
```

### Resolution Rules

1. A policy defined at any level applies to all descendants automatically.
2. If the same policy field is defined at multiple levels, the most specific (lowest) level wins.
3. Fields not overridden at a lower level inherit from the nearest ancestor that defines them.
4. A policy can be explicitly "blocked" at any level to prevent inheritance of unwanted rules.

### Example: Patch Policy Resolution

```
Platform:  approval_mode=auto, severity=critical, retries=3
MSP:       severity=important (overrides), retries=5 (overrides)
Client:    approval_mode=manual (overrides)
Site:      (no overrides -> inherits from Client)
Device:    (no overrides -> inherits from Site)

Effective for Device:
  approval_mode=manual   (from Client)
  severity=important     (from MSP)
  retries=5              (from MSP)
```

---

## Policy Lifecycle

### States

```
Draft ──> Validation ──> Preview ──> Published
  ^                        │
  └────────────────────────┘  (back to draft for edits)
```

### Stage Details

#### Draft
- Policy is created and editable.
- Not applied to any devices.
- Only visible to administrators.
- Changes saved as new revisions.

#### Validation
- System validates policy configuration:
  - Required fields present
  - Value ranges are valid
  - Referenced resources exist (e.g., notification channels)
  - No circular dependencies
- Validation errors returned to user.
- On success, policy advances to preview.

#### Preview
- Computed effective configuration is calculated without applying it.
- Admins can see which devices would be affected.
- Impact summary: number of devices, configuration diffs.
- Provides confidence check before production rollout.

#### Published
- Policy is active and enforced.
- Agents receive updated configuration on next poll.
- Evaluation engine applies new rules immediately.
- Previous published state is archived in revision history.

### Lifecycle Diagram

```
┌───────┐   validate   ┌──────────┐   preview   ┌────────┐   publish   ┌───────────┐
│ Draft │ ───────────> │Validation│ ───────────> │Preview │ ──────────> │ Published │
└───────┘              └──────────┘              └────────┘              └───────────┘
    ^                                                                         │
    │                              edit                                       │
    └─────────────────────────────────────────────────────────────────────────┘
```

---

## Effective Configuration Display

### Purpose
Show the computed, merged policy configuration at any level of the hierarchy so administrators can understand exactly what policies apply to a given device, site, or client.

### Display Format

| Level | Policy | Value | Source |
|-------|--------|-------|--------|
| Platform | approval_mode | auto | Platform default |
| MSP | approval_mode | auto | (inherited) |
| Client | approval_mode | manual | Client override |
| Site | approval_mode | manual | (inherited) |
| Device | approval_mode | manual | (inherited) |

### Visual Indicators
- **Inherited** values are shown in muted text with a reference to the source level.
- **Overridden** values are shown in bold with the overriding level.
- **Conflicts** (e.g., two policies at the same level defining the same field) are highlighted for resolution.

---

## Revision History

### Purpose
Maintain a complete, auditable trail of all policy changes.

### Captured Data

| Field | Description |
|-------|-------------|
| `revision_id` | Unique revision identifier |
| `policy_id` | The policy that was modified |
| `previous_payload` | Full JSON snapshot of the policy before change |
| `new_payload` | Full JSON snapshot of the policy after change |
| `changed_by` | User who made the change |
| `changed_at` | Timestamp of the change |
| `change_type` | created, updated, published, draft_saved |

### Features

- Full diff between any two revisions
- Rollback to any previous revision (creates a new revision)
- Audit integration: all changes logged to `audit_log` table
- Retention: last 100 revisions per policy (configurable)

### API

```
GET    /api/v1/policies/:id/revisions          — List revisions
GET    /api/v1/policies/:id/revisions/:rev     — Get revision detail
POST   /api/v1/policies/:id/rollback/:rev      — Rollback to revision
GET    /api/v1/policies/:id/diff?from=a&to=b   — Diff two revisions
```
