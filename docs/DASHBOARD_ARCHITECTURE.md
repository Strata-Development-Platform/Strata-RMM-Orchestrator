# Unified Dashboard Architecture

Strata RMM uses one dashboard shell with capability-driven modules. It must not
maintain unrelated provider, MSP, technician, client, and viewer applications
that drift independently.

## Experience levels

| Level | Primary outcome | Default dashboard emphasis |
|---|---|---|
| SaaS provider | operate and grow the hosted service | platform health, MSP lifecycle, capacity, risk, product capabilities |
| Platform support | resolve authorized tenant issues | active support grant, affected scope, incidents, health, audit |
| Platform billing | manage plans and usage | entitlements, utilization, reconciliation, lifecycle state |
| MSP owner/admin | run the managed-service business | customers, sites, fleet health, branding, users, entitlement capacity |
| Technician | complete endpoint work safely | assigned customers, alerts, devices, approvals, jobs, patching |
| Client/site admin | oversee delegated scope | local devices, health, approved jobs, reports |
| Viewer/auditor | understand state without mutation | scoped health, reports, evidence, audit |

## Wiring rules

1. The authenticated `/api/v2/context` response is the UI source of truth for
   roles, effective permissions, active MSP/client/site scope, branding, and
   entitlement.
2. Navigation and dashboard modules are selected by effective permissions, not
   by user-supplied labels or hidden CSS.
3. Backend authorization remains authoritative. Hiding a navigation item is
   usability, not a security boundary.
4. Every card links to a real workflow or is read-only operational evidence.
   Decorative cards and unsupported calls are prohibited.
5. Platform-only product positioning appears only to provider roles. It must
   not consume technician workspace or imply features that are not implemented.
6. MSP branding changes tenant presentation but not permission, edition, legal
   notices, health semantics, or audit behavior.
7. Community legal attribution is consistently available from login and the
   authenticated shell. Enterprise white-label behavior may change only after
   server-verified commercial entitlement is implemented.

## Visual language

- Lucide is the single application SVG icon family.
- Emoji are not interface icons.
- Typography uses a modern system sans stack with restrained tracking and
  high-contrast hierarchy.
- Slate/ink surfaces provide the neutral base; blue and cyan indicate action
  and platform identity; amber and red are reserved for risk.
- Motion is brief and functional. Status must never be communicated by color
  alone.
- Tables remain available for dense operational data, while summaries use
  compact cards and clear next actions.

## Acceptance

Browser acceptance must exercise at least provider, MSP administrator,
technician, and viewer sessions. For each session it must verify:

- correct dashboard title and scope;
- only permitted navigation is visible;
- direct navigation cannot bypass backend authorization;
- each visible workflow reaches a functioning route;
- tenant branding is scoped correctly;
- community legal/source access remains present;
- no raw token, customer identifier, or sensitive payload reaches UI telemetry.
