# Phase 8 Risk Register

| ID | Risk | Impact | Likelihood | Required mitigation | Evidence owner | Launch blocking |
|---|---|---:|---:|---|---|---|
| R8-01 | Cross-MSP or cross-client data exposure | Critical | Medium | RLS, scoped transactions, negative API/database tests, background-worker scope propagation | Security | Yes |
| R8-02 | Duplicate destructive endpoint execution | Critical | Medium | Idempotency keys, agent receipt ledger, replay tests, reconnect-storm tests | Agent/Jobs | Yes |
| R8-03 | Deployment applies partial schema or incompatible binaries | Critical | Medium | Migration lock, preflight compatibility, health gate, automated rollback exercise | Platform Ops | Yes |
| R8-04 | Backups exist but cannot be restored | Critical | Medium | Scheduled isolated restore verification and destructive recovery drill | Platform Ops | Yes |
| R8-05 | Lost signing/encryption keys make agents or tenant data unusable | Critical | Low | Escrow, rotation, restore procedure, access logging, dual-control recovery | Security/Ops | Yes |
| R8-06 | NATS outage loses or indefinitely stalls acknowledged work | High | Medium | JetStream durability validation, queue-age alerts, retry/backpressure and recovery tests | Messaging | Yes |
| R8-07 | Reconnect storm overloads control plane | High | High | Jitter, admission control, bounded retries, capacity tests and degraded-mode behavior | Reliability | Yes |
| R8-08 | Offline or expired work executes outside policy | High | Medium | Expiry enforcement, maintenance revalidation, reconnect acceptance tests | Endpoint Ops | Yes |
| R8-09 | Secrets appear in repository, logs, artifacts, or UI | Critical | Medium | Secret manager, redaction tests, secret scanning, artifact review | Security | Yes |
| R8-10 | Tenant suspension fails to block paid capabilities | High | Medium | Central entitlement checks, cached-state expiry, negative tests and reconciliation | Control Plane | Yes |
| R8-11 | Custom domain misroutes one MSP to another | Critical | Low | Verified ownership, canonical host mapping, certificate state, fail-closed routing tests | Portal | Yes |
| R8-12 | Monitoring misses a customer-visible outage | High | Medium | Independent synthetic checks, paging tests, runbook-linked alerts | SRE | Yes |
| R8-13 | Unbounded telemetry or audit growth exhausts storage | High | High | Retention, partitioning, quotas, saturation alerts, capacity model | Data | Yes |
| R8-14 | Agent upgrade creates incompatible mixed-version fleet | High | Medium | Capability negotiation, staged rings, pause/rollback, version matrix tests | Agent | Yes |
| R8-15 | Remote access exposes sessions or recordings | Critical | Medium | MFA, scoped authorization, short-lived access, encryption, audit and negative tests | Remote/Security | Yes |
| R8-16 | Billing/usage drift creates incorrect limits or charges | High | Medium | Immutable usage events, reconciliation, correction workflow, customer-visible totals | Billing | Yes |
| R8-17 | Operator error causes broad tenant impact | High | Medium | Least privilege, confirmations, dry runs, change windows, audit, tested rollback | Platform Ops | Yes |
| R8-18 | Beta scope expands faster than operational capacity | Medium | High | Cohort limits, explicit quotas, admission criteria and pause conditions | Product/Ops | No |
| R8-19 | Symmetric access tokens lack refresh-session revocation and asymmetric verifier separation | High | Medium | Short bounded lifetime, active membership validation, two-key rotation overlap, secret-store access control; approve asymmetric/session design or signed compensating risk before beta | Security | Yes |
| R8-20 | React Router 6 has moderate advisories with no non-high advisory-free upgrade line | Medium | Low | Pin latest 6.x; SPA does not use SSR/RSC hydration and navigation targets are fixed same-origin paths; reject high/critical production dependency findings; reassess when an advisory-free release exists | UI/Security | No |

## Maintenance

- Review this register in every Phase 8 PR.
- A mitigation is not complete without a linked test, drill, dashboard, runbook, or review record.
- Any newly discovered critical or high risk must be added before the discovering PR is merged.
- Launch-blocking risks cannot be accepted implicitly.
