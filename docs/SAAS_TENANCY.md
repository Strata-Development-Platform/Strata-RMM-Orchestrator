# SaaS Tenancy Model

## Hierarchy

```
platform_id (Strata platform operator)
  └─ msp_id (MSP tenant — pays for service)
       └─ client_id (Client organization — managed by MSP)
            └─ site_id (Site/location — physical or logical grouping)
                 └─ device_id (Managed asset)
```

## Migration from Legacy

Current model uses a single `tenant_id` for everything. Migration plan:

1. Add new tables: `msp_tenants`, `client_organizations`, `sites`
2. Create a default MSP tenant for all existing data
3. Add foreign keys to existing tables (nullable initially)
4. Backfill: assign existing tenants as client orgs under default MSP
5. Dual-read: support both `tenant_id` and new hierarchy during transition
6. Switch writes to new hierarchy
7. Verify legacy API compatibility
8. Deprecate `tenant_id` only after all consumers migrate

## Ownership Identifiers

| Field | Type | Scope |
|-------|------|-------|
| `platform_id` | UUID | Global |
| `msp_id` | UUID | Per-MSP |
| `client_id` | UUID | Per-client |
| `site_id` | UUID | Per-site |
| `device_id` | UUID | Per-device |

## Data Isolation

- Every query must be scoped by authenticated identity
- URL parameters are NEVER authority — always cross-reference with auth context
- RLS policies use `current_setting('app.msp_id')` set transactionally
- Background jobs receive explicit scope

## Default MSP

For the prototype:
- All existing data is assigned to a default MSP tenant
- Default MSP has UUID `00000000-0000-0000-0000-000000000001`
- All agents currently use `tenant_id` which maps to the default MSP's client org
