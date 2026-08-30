# Strata RMM — Beta go/no-go

**Reconciled:** 2026-08-30  
**Current decision:** **NO-GO for hosted beta acceptance**

This decision deliberately separates repository source readiness from representative hosted acceptance.

## Gate 1 — repository beta-quality source

Repository source readiness is governed by #167 (Gate D). The gate requires the mandatory source audit and focused remediations to be complete, the feature/status documentation to be truthful, and the required exact-head workflow families to be terminal green on the final source candidate.

The feature-by-feature source disposition is authoritative in [`FEATURE_COMPLETENESS_MATRIX.md`](FEATURE_COMPLETENESS_MATRIX.md). Deferred/full-spec features are not silently converted into beta must-haves; they remain explicitly Partial/deferred/not implemented unless the beta boundary is expanded.

A successful earlier PR does not prove a later candidate. Exact-head CI must be evaluated for the actual merge candidate.

## Gate 2 — hosted internal-alpha / beta evidence

Hosted acceptance is governed by #15 and **cannot be inferred from source CI**. The following representative exercise remains required:

- clean isolated platform installation and provider bootstrap;
- MSP owner activation, client/site creation and scoped endpoint enrollment;
- Linux AMD64 and Windows AMD64 endpoint installation/service execution;
- live heartbeat/telemetry and dashboard observation;
- broker/database/object-storage outage and recovery behavior;
- reconnect storms, duplicate/replay handling and bounded queues;
- harmless and destructive endpoint operations with restart/ambiguous-outcome checks;
- baseline load and 24-hour soak;
- backup restore drill with measured RPO/RTO;
- secret/log review and preserved audit evidence;
- exact release/build provenance and signed owner go/no-go decision.

Until #15 contains successful evidence for that exercise, the hosted decision remains **NO-GO**, even if #167 closes successfully.

## Feature-claim guardrails

The following must not be used as beta acceptance evidence by themselves:

- mock UI data or simulated refresh/acknowledgement;
- structural type/JSON tests without runtime behavior;
- route/schema/component presence;
- successful compilation alone;
- stale test counts or old PR numbers;
- a timeless `all CI green` statement;
- LLDP/CDP/STP stub functions;
- mocked third-party providers as proof of live interoperability.

The settings Integration Dashboard is currently mock-backed and therefore Partial/deferred, not a real provider-health/event integration. Full LLDP/CDP/STP topology is not implemented. Other deferred/full-spec items remain enumerated in the completeness matrix.

## Go criteria

A hosted beta **GO** requires both conditions:

1. #167 is closed with a final source-audit ledger and exact-head repository gate evidence; and
2. #15 is closed with representative-host, fault/load/soak, restore/RPO-RTO and owner-signoff evidence for the promoted release.

If either condition is absent, the decision remains **NO-GO**. This prevents source readiness, future product scope and hosted operational acceptance from being conflated.
