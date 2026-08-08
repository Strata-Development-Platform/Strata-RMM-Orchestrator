# Strata RMM — Policy Engine Reference

**Version:** 2026-08-08
**Last Updated: 2026-08-08

---

## 1. Policy Engine Overview

Strata's hierarchical policy engine supports recursive policy merging with most-specific-wins precedence.

---

## 2. Policy Hierarchy

```
Global Policy (level 0)
└── MSP Policy (level 1)
    └── Client Policy (level 2)
        └── Device Group Policy (level 3)
            └── Device Policy (level 4)
```

### 2.1 Scope Ranking

| Level | Scope | Precedence |
|-------|-------|------------|
| 0 | Global | Lowest |
| 1 | MSP | ↑ |
| 2 | Client | ↑ |
| 3 | Device Group | ↑ |
| 4 | Device | Highest |

**Most-specific-wins:** Lower levels (higher scope) override higher levels.

---

## 3. Policy Structure

### 3.1 Policy Fields

```go
type PatchPolicy struct {
    ID             string            `json:"id"`
    TenantID       string            `json:"tenant_id"`
    Name           string            `json:"name"`
    Enabled        bool              `json:"enabled"`
    Platforms      []Platform        `json:"platforms"`
    ApprovalMode   string            `json:"approval_mode"`
    Severity       PatchSeverity     `json:"severity"`
    MaintenanceWin string            `json:"maintenance_window"`
    DeviceFilter   map[string]string `json:"device_filter"`
    MaxRetries     int               `json:"max_retries"`
    CreatedAt      time.Time         `json:"created_at"`
    UpdatedAt      time.Time         `json:"updated_at"`
}
```

### 3.2 Policy States

| State | Description |
|-------|-------------|
| Draft | Created but not published |
| Published | Active and enforced |
| Archived | Deprecated, preserved for history |

---

## 4. Policy Lifecycle

### 4.1 Create

```bash
curl -X POST https://strata.example.com/api/v1/policies \
  -H "Authorization: Bearer {token}" \
  -d '{"name": "Critical Security Policy", "severity": "critical", "enabled": true}'
```

### 4.2 Validate

```bash
curl -X POST https://strata.example.com/api/v1/policies/{policyID}/validate \
  -H "Authorization: Bearer {token}"
```

### 4.3 Preview

```bash
curl -X POST https://strata.example.com/api/v1/policies/{policyID}/preview \
  -H "Authorization: Bearer {token}"
```

### 4.4 Publish

```bash
curl -X POST https://strata.example.com/api/v1/policies/{policyID}/publish \
  -H "Authorization: Bearer {token}"
```

### 4.5 Rollback

```bash
curl -X POST https://strata.example.com/api/v1/policies/{policyID}/rollback \
  -H "Authorization: Bearer {token}"
```

---

## 5. Effective Policy Computation

### 5.1 Recursive Merge

```go
// Compute effective policy for device
func ComputeEffectivePolicy(deviceID string) (*Policy, error) {
    // Fetch policies at all levels
    policies := fetchPolicies(deviceID)

    // Merge with most-specific-wins
    merged := mergePolicies(policies)
    return merged, nil
}
```

### 5.2 Merge Rules

| Field | Merge Strategy |
|-------|---------------|
| `enabled` | Most-specific wins |
| `severity` | Most-specific wins |
| `platforms` | Union (all applicable) |
| `approval_mode` | Most-specific wins |
| `maintenance_window` | Most-specific wins |

---

## 6. Policy Diff

### 6.1 Compare Policies

```bash
curl -X POST https://strata.example.com/api/v1/policies/{policyID}/diff \
  -H "Authorization: Bearer {token}" \
  -d '{"basePolicyID": "policy-123"}'
```

### 6.2 Diff Output

```json
{
  "fields_changed": ["severity", "enabled"],
  "old_values": {"severity": "moderate", "enabled": true},
  "new_values": {"severity": "critical", "enabled": true}
}
```

---

## 7. Policy Revisions

### 7.1 Revision History

Each publish creates a revision:

```sql
CREATE TABLE policy_revisions (
    id          UUID PRIMARY KEY,
    policy_id   UUID NOT NULL,
    version     INTEGER NOT NULL,
    data        JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL
);
```

### 7.2 List Revisions

```bash
curl -X GET https://strata.example.com/api/v1/policies/{policyID}/revisions \
  -H "Authorization: Bearer {token}"
```

---

## 8. Policy Enforcement

### 8.1 Scheduler

Policies are enforced on intervals:

```go
// policy_scheduler.go
func (s *PolicyScheduler) Run() {
    ticker := time.NewTicker(1 * time.Hour)
    for {
        select {
        case <-ticker.C:
            s.enforceAll()
        case <-ctx.Done():
            return
        }
    }
}
```

### 8.2 Enforcement Actions

| Action | Description |
|--------|-------------|
| Auto-remediate | Apply patches automatically |
| Alert | Create alert for non-compliance |
| Schedule | Queue remediation for maintenance window |

---

## 9. Maintenance Windows

### 9.1 Create Window

```bash
curl -X POST https://strata.example.com/api/v1/maintenance-windows \
  -H "Authorization: Bearer {token}" \
  -d '{"name": "Sunday Maintenance", "start": "02:00", "end": "06:00", "days": ["Sunday"]}'
```

### 9.2 Query Windows

```bash
curl -X GET https://strata.example.com/api/v1/maintenance-windows \
  -H "Authorization: Bearer {token}"
```

---

## 10. Script Vault Binding

### 10.1 Bind Script to Policy

```bash
curl -X POST https://strata.example.com/api/v1/device-groups/{groupID}/script-bindings \
  -H "Authorization: Bearer {token}" \
  -d '{"scriptID": "script-123", "action": "execute"}'
```

### 10.2 Script Schedule

```bash
curl -X POST https://strata.example.com/api/v1/tenants/{tenantID}/scripts/schedule \
  -H "Authorization: Bearer {token}" \
  -d '{"scriptID": "script-123", "schedule": "0 2 * * *"}'
```

---

## 11. Policy Validation

### 11.1 Validation Rules

| Rule | Description |
|------|-------------|
| Name required | Policy name must be non-empty |
| Description required | Policy description must be non-empty |
| Category required | Policy category must be set |
| Scope required | Policy scope must be set |
| Config depth limit | Nested config max 10 levels |
| Config size limit | Max 1MB config payload |
| Time format | Time must be valid HH:MM |
| Day validation | Days must be valid weekday names |

### 11.2 Validation Response

```json
{
  "valid": true,
  "errors": [],
  "warnings": []
}
```

---

## 12. Smart Group Integration

### 12.1 Smart Group Evaluation

```bash
curl -X POST https://strata.example.com/api/v1/device-groups/{groupID}/evaluate \
  -H "Authorization: Bearer {token}"
```

### 12.2 Evaluation Result

```json
{
  "groupID": "group-123",
  "members": ["device-1", "device-2"],
  "evaluatedAt": "2024-01-01T00:00:00Z"
}
```

---

## 13. Testing

### 13.1 Unit Tests

| Test | Coverage |
|------|----------|
| `TestPatchStructFields` | Patch struct fields |
| `TestPatchPolicy_StructFields` | PatchPolicy struct fields |
| `TestPatchStatusConstants` | PatchStatus constants |
| `TestPatchSeverityConstants` | PatchSeverity constants |
| `TestPlatformConstants` | Platform constants |
| `TestPolicyDiff` | Policy diff computation |
| `TestEffectivePolicy` | Recursive merge |

### 13.2 Behavioral Tests

| Test | Coverage |
|------|----------|
| 68 behavioral tests | Policy categories, scope, validation |
| JSON round-trips | Policy serialization |
| Zero-value handling | Default values |

---

*Last Updated: 2026-08-08*
