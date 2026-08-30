# Strata RMM — Feature specification

**Reconciled:** 2026-08-30  
**Purpose:** intended product scope, not an acceptance ledger.

For current implementation/acceptance status, use [`FEATURE_COMPLETENESS_MATRIX.md`](FEATURE_COMPLETENESS_MATRIX.md). That matrix is authoritative when this specification and status documents differ.

## Product scope

Strata RMM is a multi-tenant Remote Monitoring & Management platform for a SaaS provider and MSPs. The intended product covers provider/MSP/client/site tenancy, endpoint enrollment and identity, monitoring, alerting, policy, automation, patch/software operations, remote support, discovery, vulnerability management, reporting, integrations, backup/recovery, and secure deployment lifecycle.

The product specification is broader than the current beta boundary. A capability can therefore be part of intended scope while still being **Partial**, **deferred**, **Not implemented**, or **Environment pending** in the completeness matrix.

## Capability specification

### Tenant and endpoint lifecycle
- Provider → MSP → client → site hierarchy with strict tenant isolation.
- Scoped enrollment tokens and persistent endpoint identity.
- Agent heartbeat, telemetry, inventory, remote commands, and durable result handling.
- Role/permission boundaries for provider, MSP owner/admin, technician, client-facing flows, and endpoint credentials.

### Monitoring and alerting
- System telemetry ingestion and historical metrics.
- Threshold and heartbeat monitoring rules, grouping, cooldown, maintenance behavior, acknowledgment/history, and notification routing.
- Prometheus/Grafana/platform dashboards.
- Live provider delivery and live dashboard acceptance must be proven separately on representative infrastructure.

### Policy and automation
- Hierarchical policy configuration, validation, effective-policy computation, publication/revision/rollback, and scheduled enforcement.
- Script vault/execution and scheduled automation through durable endpoint jobs.
- Policy-to-script binding is an intended expansion, not a currently accepted beta capability.

### Patch and software operations
- OS patch scan/install workflows across supported Windows/Linux package managers.
- Software package deployment with integrity verification and lifecycle/result tracking.
- Third-party application catalog/version discovery and deployment orchestration.
- Representative OS/package-manager acceptance is environment-dependent.

### Remote support
- Tenant/device/agent-bound interactive sessions.
- WebRTC/TURN-STUN transport, input/capture, recordings, and transcription/provider adapters where configured.
- Native host behavior and provider interoperability require representative-host acceptance.

### Discovery and network visibility
- ARP/ping/port/service discovery, SNMP and network-flow parsing.
- Full topology discovery is intended product scope.
- LLDP/CDP/STP topology discovery is currently stubbed and therefore not accepted as implemented; see the completeness matrix.

### Vulnerability management
- Vulnerability records, version matching, lifecycle, external feed adapters, and remediation orchestration.
- Live feed behavior and end-to-end remediation require representative external/OS environments.

### Reports and storage
- Report generation/export/scheduling and object-storage backends.
- Hosted generation/download/delivery requires representative infrastructure acceptance.

### Provider/MSP commercial lifecycle
- Provider profile/setup, MSP creation/owner invitation, branding, entitlement, tenant lifecycle and offboarding.
- External billing provider integration, immutable billable events, provider-backed invoice generation/reconciliation and related commercial automation are intended expansion scope unless promoted into the beta must-have boundary.

### Client portal and account lifecycle
- Client-facing support/service experience and branding.
- JWT/TOTP/MFA/API-key/invitation account security for implemented platform flows.
- SSO/OIDC, password recovery, refresh-token rotation, asymmetric token signing and complete end-client session flow are deferred expansion scope unless separately promoted.

### Integrations
- EDR, backup, PSA and generic webhook/provider adapter surfaces with authentication and tenant boundaries.
- Mocks validate contracts, not live-provider acceptance.

### Deployment, update, backup and recovery
- Secure clean install and replay-safe reconciliation.
- Signed release provenance, immutable image/artifact selection, compatibility/preflight, rollback and crash reconciliation.
- Docker Compose as the supported automatic container lifecycle where implemented; unsupported deployment mutation paths must fail closed rather than suggest bypasses.
- Backup/restore, recovery exclusion/integrity controls, tenant preservation, migration safety, SBOM and vulnerability/secret scanning.
- Clean-host Linux/Windows execution, soak, fault injection and measured RPO/RTO remain representative-environment acceptance work.

## Acceptance model

A capability is only **Verified** when the completeness matrix cites executable production paths plus automated behavioral/integration/security evidence appropriate to the claim. Structural tests, mock UI data, schemas, routes, comments, build success, or a prior PR number do not establish product completeness by themselves.

Hosted internal-alpha acceptance is governed by #15. Repository-wide beta-quality source closure is governed by #167. These gates must remain separate.
