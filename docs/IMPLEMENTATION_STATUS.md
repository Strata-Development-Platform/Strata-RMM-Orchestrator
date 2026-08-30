# Strata RMM — Implementation status

**Reconciled:** 2026-08-30  
**Current posture:** pre-beta source audit / hosted acceptance pending

The authoritative feature-by-feature status is [`FEATURE_COMPLETENESS_MATRIX.md`](FEATURE_COMPLETENESS_MATRIX.md). This document is a concise operational summary and must not broaden claims made by that matrix.

## Current state

The repository contains a substantial multi-tenant RMM implementation and the automated source/integration gates needed to prepare for internal-alpha testing. It is **not correct to describe the broader product as feature-complete**, and repository CI alone does not establish hosted internal-alpha acceptance.

Gate D (#167) has been auditing mandatory source paths for tenant isolation, authorization, durable external effects, endpoint identity binding, restart/idempotency, result vocabulary, migration safety, secret handling and unsupported feature claims. Focused remediations completed during this audit include #168, #170, #172, #174, #175, #178, #181 and #183. #185 reconciles the feature/status documentation itself.

Hosted representative-environment validation remains tracked by #15 and is intentionally independent from repository source closure.

## Repository-established core

- Multi-tenant provider/MSP/client/site control-plane paths with authorization/RLS coverage.
- Scoped agent enrollment and persistent endpoint identity.
- Heartbeat/telemetry ingestion and historical metrics paths.
- Alert evaluation, grouping/maintenance state and notification adapters.
- Policy lifecycle/effective configuration/enforcement scheduling.
- Script execution/scheduling and durable endpoint jobs with outbox/retry/result identity protections.
- Crash-durable job cancellation and stale-result protection.
- Provider/MSP setup, invitation, branding, entitlement and offboarding paths.
- Backup/recovery, migration, deployment-template, supply-chain, secret and vulnerability scanning automation.
- Cross-platform agent builds and install/service assets.

These statements describe repository behavior. They do not replace the representative-host exercise required by #15.

## Partial or deferred product areas

The completeness matrix currently classifies patch/software deployment, third-party catalog behavior, remote support, network discovery, vulnerability management, reports/object storage, client portal, full account lifecycle, commercial billing and third-party integration acceptance as Partial and/or Environment pending where appropriate.

Explicit deferred/not-implemented expansion scope includes SSO/OIDC, password recovery, refresh-token rotation, asymmetric access-token signing, external commercial billing backend, immutable billable events, provider-backed invoice/reconciliation workflow, policy-to-script binding, and full LLDP/CDP/STP topology discovery.

The settings Integration Dashboard is mock-backed. It must not be described as real provider-health/event API integration until such a backend and persisted acknowledgement behavior exist and are tested.

## CI evidence policy

Do not maintain a timeless table claiming every workflow is green. Exact-head workflow evidence belongs to the PR/commit being evaluated. Every merge that closes a source gate must re-run the required workflows on its exact head; any head change invalidates prior evidence.

The immediately preceding implementation PR #184 passed the mandated seven workflow families on its exact head before merge, including CI, Internal Alpha Agent, Phase 8C, 8D, 8E, 8F and 8G. That evidence supported #184 only; this reconciliation change requires its own exact-head evidence before merge.

## Remaining acceptance gate

Even after Gate D source closure, #15 remains the authoritative hosted acceptance gate. It requires representative Linux and Windows endpoints, clean install, tenant lifecycle, enrollment, live telemetry/dashboard observation, broker/database/storage fault injection, reconnect storms, harmless/destructive remote operations, baseline load, 24-hour soak, backup restore with measured RPO/RTO, and signed owner go/no-go evidence.

Until that exercise passes, the correct product posture is **repository-ready for hosted internal-alpha validation, not hosted beta accepted**.
