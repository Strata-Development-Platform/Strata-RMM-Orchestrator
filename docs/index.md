# Strata RMM documentation

Strata RMM is a multi-tenant remote monitoring and management platform for MSPs. It supports hosted SaaS operation and self-hosted deployments with tenant-isolated MSP, client, and site workspaces.

## Start here

- [Installation](INSTALL.md)
- [Architecture](ARCHITECTURE.md)
- [Operations runbook](RUNBOOK.md)
- [Production observability and synthetics](OBSERVABILITY.md)
- [Resilience testing](RESILIENCE_TESTING.md)
- [Unified dashboard architecture](DASHBOARD_ARCHITECTURE.md)
- [MSP lifecycle](MSP_LIFECYCLE.md)
- [Security architecture](SECURITY.md)
- [Security model and compensating controls](SECURITY_MODEL.md)
- [Threat model and data flows](THREAT_MODEL.md)
- [Security incident response](INCIDENT_RESPONSE.md)
- [External penetration-test scope](PENETRATION_TEST_SCOPE.md)
- [SaaS tenancy](SAAS_TENANCY.md)
- [Agent protocol](AGENT_PROTOCOL.md)
- [API compatibility](API_COMPATIBILITY.md)
- [StrataLabs website integration](stratalabs-integration.md)

## Delivery and production readiness

- [Master plan](MASTER_PLAN.md)
- [Historical Phase 0 implementation inventory](IMPLEMENTATION_STATUS.md)
- [Phase 8 production beta plan](PHASE_8_PRODUCTION_BETA.md)
- [Phase 8 risk register](PHASE_8_RISK_REGISTER.md)
- [Phase 8 acceptance matrix](PHASE_8_ACCEPTANCE_MATRIX.md)
- [Phase 8G security evidence](PHASE_8G_EVIDENCE.md)
- [Internal alpha and beta operations](BETA_OPERATIONS.md)
- [Hosted internal-alpha end-to-end evidence](HOSTED_INTERNAL_ALPHA_EVIDENCE.md)
- [Beta go/no-go record](BETA_GO_NO_GO.md)
- [Agent engineering and PR standard](AGENT_ENGINEERING_STANDARD.md)
- [Licensing](../LICENSING.md)

Phase completion is not equivalent to unrestricted production readiness. A hosted beta requires durable evidence for every mandatory Phase 8 acceptance gate.

## Operators

Deployment resources are under `deploy/`, including Docker Compose, Helm, Grafana, and KOTS configurations. Review the installation guide, operations runbook, and Phase 8 gates before exposing an orchestrator to production traffic.

## Developers

The repository requires Go 1.25 or newer and Node.js for the React UI. Before opening a pull request, follow [CONTRIBUTING.md](../CONTRIBUTING.md) and run the relevant Go, frontend, database, integration, security, and browser checks. Phase 8 PRs must record CI evidence for their exact head SHA.

## Security

Do not disclose vulnerabilities through public issues. Follow [SECURITY.md](../SECURITY.md) for private reporting instructions.
