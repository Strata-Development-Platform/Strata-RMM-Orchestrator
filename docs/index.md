# Strata RMM documentation

Strata RMM is a multi-tenant remote monitoring and management platform for MSPs. The repository is in pre-beta hardening; documentation must distinguish implemented assets from supported/accepted deployment evidence.

## Start here

- [Installation](INSTALL.md)
- [Upgrade](UPGRADE.md)
- [Rollback](ROLLBACK.md)
- [Backup](BACKUP.md)
- [Restore](RESTORE.md)
- [Configuration](CONFIGURATION.md)
- [Operations runbook](RUNBOOK.md)
- [Architecture](ARCHITECTURE.md)
- [Security architecture](SECURITY.md)
- [Security model and compensating controls](SECURITY_MODEL.md)
- [Threat model and data flows](THREAT_MODEL.md)
- [Security incident response](INCIDENT_RESPONSE.md)

## Platform and operations

- [Production observability and synthetics](OBSERVABILITY.md)
- [Resilience testing](RESILIENCE_TESTING.md)
- [Unified dashboard architecture](DASHBOARD_ARCHITECTURE.md)
- [MSP lifecycle](MSP_LIFECYCLE.md)
- [SaaS tenancy](SAAS_TENANCY.md)
- [Agent protocol](AGENT_PROTOCOL.md)
- [API compatibility](API_COMPATIBILITY.md)
- [External penetration-test scope](PENETRATION_TEST_SCOPE.md)
- [StrataLabs website integration](stratalabs-integration.md)

## Delivery and readiness

- [Master plan](MASTER_PLAN.md)
- [Historical Phase 0 implementation inventory](IMPLEMENTATION_STATUS.md)
- [Phase 8 production beta plan](PHASE_8_PRODUCTION_BETA.md)
- [Phase 8 risk register](PHASE_8_RISK_REGISTER.md)
- [Phase 8 acceptance matrix](PHASE_8_ACCEPTANCE_MATRIX.md)
- [Feature completeness matrix](FEATURE_COMPLETENESS_MATRIX.md)
- [Phase 8G security evidence](PHASE_8G_EVIDENCE.md)
- [Internal alpha and beta operations](BETA_OPERATIONS.md)
- [Hosted internal-alpha end-to-end evidence](HOSTED_INTERNAL_ALPHA_EVIDENCE.md)
- [Beta go/no-go record](BETA_GO_NO_GO.md)
- [Internal-alpha agent/evidence contract](INTERNAL_ALPHA_AGENT.md)
- [Agent engineering and PR standard](AGENT_ENGINEERING_STANDARD.md)
- [Licensing](../LICENSING.md)

Phase completion and green source CI are not equivalent to hosted internal-alpha or unrestricted production readiness. Mandatory hosted evidence remains tracked separately.

## Supported pre-beta deployment boundary

The accepted orchestrator lifecycle currently covers Linux native/systemd and single-host Docker Compose through the secure installer/release path. Source assets under `deploy/` may also include Helm/Kubernetes and KOTS resources, but their presence is not acceptance evidence and they are not promoted as supported pre-beta lifecycle paths unless a current acceptance document explicitly proves them.

Do not deploy production from a source checkout, repository development Compose file, mutable image tag, or generic Helm command. Use `INSTALL.md`, `UPGRADE.md`, and `ROLLBACK.md`.

## Developers

Follow [CONTRIBUTING.md](../CONTRIBUTING.md) and the current repository workflows. Pull requests must use exact-head evidence; a head change invalidates earlier workflow results.

## Security

Do not disclose vulnerabilities through public issues. Follow [SECURITY.md](../SECURITY.md) for private reporting instructions. Never place passwords, tokens, DSNs with credentials, invitation/enrollment secrets, recovery keys, or object-storage credentials in issues, documentation examples, CI output, or evidence artifacts.
