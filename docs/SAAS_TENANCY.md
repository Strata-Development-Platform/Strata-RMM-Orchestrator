# Strata RMM — SaaS Tenancy Reference

**Version:** 2026-08-08
**Last Updated: 2026-08-08

---

## 1. Tenancy Model

### 1.1 Shared Schema (Default)

All tenants share a single PostgreSQL/TimescaleDB database with `tenant_id` columns and Row-Level Security (RLS) policies.

**Benefits:**
- Cost-efficient
- Easy operations
- Simpler backups

### 1.2 Tenant Isolation

| Layer | Mechanism |
|-------|-----------|
| Database | `tenant_id` column + RLS policies |
| NATS | Subject prefixes: `tenant.{id}.*` |
| API | Path-scoped routes with tenant lookup |

---

## 2. Tenant Hierarchy

```
Platform Operator
└── MSP (Managed Service Provider)
    └── Client
        └── Site
            └── Devices
```

### 2.1 Entities

| Entity | Table | Description |
|--------|-------|-------------|
| Tenant | `tenants` | Root tenant entity |
| MSP | `msp_tenants` | MSP tenant relationship |
| Client | `clients` | Client account |
| Site | `sites` | Physical/logical site |
| Device | `devices` | Managed endpoint |

---

## 3. Row-Level Security (RLS)

### 3.1 RLS Policies

All tenant-scoped tables have RLS:

```sql
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON devices
    USING (tenant_id = current_tenant_id());

CREATE POLICY tenant_isolation_check ON devices
    WITH CHECK (tenant_id = current_tenant_id());
```

### 3.2 Protected Tables

| Table | RLS Column |
|-------|------------|
| `tenants` | `id` |
| `devices` | `tenant_id` |
| `alerts` | `tenant_id` |
| `users` | `tenant_id` |
| `policies` | `msp_id` |
| `scripts` | `tenant_id` |
| `jobs` | `tenant_id` |
| `audit_log` | `tenant_id` |

### 3.3 RLS Verification

RLS policies are verified by tests in `internal/platform/`:
- `TestRLSPoliciesExist`
- `TestRLSForceStatement`
- Cross-tenant denial tests

---

## 4. NATS Subject Isolation

### 4.1 Subject Hierarchy

```
tenant.{tenantID}.agent.{agentID}.heartbeat
tenant.{tenantID}.agent.{agentID}.metrics
tenant.{tenantID}.cmd.{agentID}
```

### 4.2 Consumer Setup

```go
// Subscribe to tenant-specific subjects
subject := fmt.Sprintf("tenant.%s.>", tenantID)
nc.Subscribe(subject, handler)
```

### 4.3 Agent Subject Registration

```go
// Agent registers with platform
subject := fmt.Sprintf("tenant.%s.agent.%s.>", tenantID, agentID)
nc.Subscribe(subject, agentHandler)
```

---

## 5. API Scoping

### 5.1 Path-Based Scoping

All API routes are scoped to tenant or MSP:

```go
// Example: Device metrics
GET /api/v1/devices/{tenantID}/{deviceID}/metrics/{metricName}
```

### 5.2 Tenant Resolution

Tenant is resolved from:
- Path parameter (`{tenantID}`)
- JWT claims (`tenant_id`)
- NATS subject prefix

---

## 6. Tenant Onboarding Flow

```
1. Create tenant record → 2. Provision RLS policies → 3. Create NATS stream
   → 4. Generate enrollment token → 5. Agent enrolls
```

### 6.1 Enrollment Token

```bash
# Create enrollment token
curl -X POST https://strata.example.com/api/v1/enrollment/tokens \
  -H "Authorization: Bearer {token}" \
  -d '{"tenantID": "tenant-uuid", "expiresIn": "24h"}'

# Validate token
curl -X POST https://strata.example.com/api/v1/enrollment/validate \
  -d '{"token": "enrollment-token"}'
```

### 6.2 Agent Registration

```bash
# Register agent
curl -X POST https://strata.example.com/api/v1/agent/register \
  -H "Authorization: Bearer {enrollment-token}" \
  -d '{"hostname": "device-01", "os": "linux", "arch": "amd64"}'
```

---

## 7. Multi-Tenant Queries

### 7.1 Tenant-Scoped Queries

```sql
-- Tenant-scoped device query
SELECT * FROM devices WHERE tenant_id = $1;

-- MSP-wide device query
SELECT * FROM devices
WHERE tenant_id IN (
    SELECT tenant_id FROM msp_tenants WHERE msp_id = $1
);
```

### 7.2 Cross-Tenant Queries

Only platform operators can query across tenants:

```sql
-- Platform operator (no RLS)
SELECT * FROM devices;
```

---

## 8. Tenant Migration (Future)

### 8.1 Dedicated Schema (Premium)

Future support for dedicated schema per tenant:

```sql
-- Each tenant gets own schema
CREATE SCHEMA tenant_123;
SET search_path TO tenant_123;

-- Tables in tenant schema
CREATE TABLE devices (...);
CREATE TABLE alerts (...);
```

---

## 9. Data Residency

### 9.1 Region Scoping

```yaml
# Region-specific tenant data
tenants:
  - id: tenant-1
    region: us-east-1
    data_location: us-east-1

  - id: tenant-2
    region: eu-west-1
    data_location: eu-west-1
```

### 9.2 Storage Backend

```yaml
storage:
  # Per-tenant storage bucket
  buckets:
    - tenant_id: tenant-1
      bucket: strata-tenant-1-us-east-1
    - tenant_id: tenant-2
      bucket: strata-tenant-2-eu-west-1
```

---

## 10. Tenant Metrics

### 10.1 Usage Tracking

| Metric | Description |
|--------|-------------|
| Device count | Number of devices per tenant |
| Alert count | Active alerts per tenant |
| API calls | Requests per tenant |
| Storage used | Object storage per tenant |

### 10.2 Billing Integration

Usage metrics feed into billing:

```bash
# Report usage
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/billing/usage \
  -H "Authorization: Bearer {token}" \
  -d '{"meterName": "device_count", "value": 100}'
```

---

## 11. Tenant Lifecycle

### 11.1 Tenant States

| State | Description |
|-------|-------------|
| Active | Normal operation |
| Suspended | Temporarily disabled |
| Archived | Soft delete, data preserved |
| Deleted | Permanent deletion |

### 11.2 Tenant Deletion

```bash
# Archive tenant
curl -X POST https://strata.example.com/api/v2/msps/{mspID}/clients/{clientID}/archive \
  -H "Authorization: Bearer {token}"

# Delete tenant (with approval)
curl -X POST https://strata.example.com/api/v2/platform/msps/{mspID}/offboarding/approve-deletion \
  -H "Authorization: Bearer {token}"
```

---

*Last Updated: 2026-08-08*
