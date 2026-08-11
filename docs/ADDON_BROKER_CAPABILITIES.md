# Add-on Broker Capabilities

This document tracks reviewed concrete host capabilities exposed to executable WASI add-ons through `strata_broker.call`.

## Security model

Broker capabilities are host-registered operations with fixed permissions. Guest code may select only the registered operation name and provide bounded opaque input. It never selects its own permission or organizational scope. The trusted invocation scope is supplied by Strata host code, and handlers must independently resolve every resource identifier from authoritative platform storage before returning data or performing side effects.

A handler must fail closed when the resolved resource identity or scope does not exactly match the trusted invocation target. Cross-MSP, sibling-client, and sibling-site substitution are authorization failures even when the module otherwise possesses the operation permission.

## `devices.get`

Permission: `devices.read`

Input:

```json
{"device_id":"<device-id>"}
```

Output:

```json
{"id":"<device-id>","hostname":"<hostname>","status":"<status>"}
```

The resolver is a trusted host dependency and must derive device ownership from authoritative platform storage. Scope is not returned to guest code. Guest-supplied MSP, client, site, tenant, permission, or credential fields are rejected by strict JSON decoding.

The initial capability boundary intentionally exposes only a minimal device projection. Additional fields or operations require separate review because expanding output can expose inventory, addressing, security, or customer metadata.
