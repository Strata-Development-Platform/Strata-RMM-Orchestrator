# Strata RMM documentation

Strata RMM is a multi-tenant remote monitoring and management platform for MSPs. It supports hosted SaaS operation and self-hosted deployments with tenant-isolated MSP, client, and site workspaces.

## Start here

- [Installation](INSTALL.md)
- [Architecture](ARCHITECTURE.md)
- [Operations runbook](RUNBOOK.md)
- [Security architecture](SECURITY.md)
- [SaaS tenancy](SAAS_TENANCY.md)
- [Agent protocol](AGENT_PROTOCOL.md)
- [API compatibility](API_COMPATIBILITY.md)
- [StrataLabs website integration](stratalabs-integration.md)

## Operators

Deployment resources are under `deploy/`, including Docker Compose, Helm, Grafana, and KOTS configurations. Review the installation guide and operations runbook before exposing an orchestrator to production traffic.

## Developers

The repository requires Go 1.25 or newer and Node.js for the React UI. Before opening a pull request, follow [CONTRIBUTING.md](../CONTRIBUTING.md) and run the relevant Go, frontend, database, integration, and browser checks.

## Security

Do not disclose vulnerabilities through public issues. Follow [SECURITY.md](../SECURITY.md) for private reporting instructions.
