# Strata RMM

Strata RMM Community Edition is open source under [AGPL-3.0-or-later](LICENSE). Commercial white-label deployments require a separately executed Enterprise License. See [LICENSING.md](LICENSING.md) and [TRADEMARKS.md](TRADEMARKS.md).

Strata RMM is a multi-tenant Remote Monitoring & Management platform written primarily in Go, with a React/TypeScript web console, PostgreSQL/TimescaleDB, NATS JetStream, and object storage.

## Pre-beta support boundary

This repository is still in pre-beta hardening. Code presence is not the same as a supported or hosted-accepted feature.

Current promoted lifecycle work is limited to:

- **Orchestrator host:** supported Linux native/systemd installation and single-host Docker Compose installation through the documented secure installer/release path.
- **Endpoint evidence baseline:** Linux AMD64 and Windows AMD64 are the minimum hosted validation matrix for Issue #15. Other endpoint architectures/platforms must not be treated as accepted until representative evidence exists.
- **Kubernetes/Helm/KOTS, air-gapped, and multi-region:** source assets may exist, but these are **not accepted pre-beta deployment paths** and must not be used as a substitute for the supported native/Docker lifecycle.

Use [docs/FEATURE_COMPLETENESS_MATRIX.md](docs/FEATURE_COMPLETENESS_MATRIX.md) for feature status and [docs/index.md](docs/index.md) for the documentation map.

## Installation

Do not deploy production from a source checkout or mutable image tag. Use the secure installer and immutable promoted-release artifacts described in:

- [docs/INSTALL.md](docs/INSTALL.md)
- [docs/UPGRADE.md](docs/UPGRADE.md)
- [docs/ROLLBACK.md](docs/ROLLBACK.md)

The installer is responsible for production preflight, secrets, TLS prerequisites, bootstrap, immutable artifact selection, and repeatable native/Docker setup.

## Local development

The repository contains a development Compose topology. It uses development-oriented dependency settings and must not be copied into production configuration.

```bash
git clone https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator.git
cd Strata-RMM-Orchestrator
make dev
```

See the Makefile and development documentation for local prerequisites. Never treat local `docker compose up`, mutable tags, generated dev users, or plaintext development transports as production deployment guidance.

## Architecture

```text
Endpoint Agent ── NATS/TLS ──> Orchestrator ──> PostgreSQL/TimescaleDB
       │                            │
       │                            ├──> Object storage
       │                            └──> Alerting / jobs / policy / audit
       └── local durable queue
```

Core controls include scoped enrollment, tenant-aware authorization, PostgreSQL row-level isolation, NATS subject isolation, durable command/result handling, audit evidence, monitoring/alerting, patch/software workflows, backup/recovery, and verified release/update machinery.

## Host-level CLI

The shipped binary exposes host/operator commands in addition to service modes. Relevant lifecycle commands include:

| Command | Purpose |
| --- | --- |
| `strata-rmm version` | Print build/release identity |
| `strata-rmm orchestrator` | Run platform services |
| `strata-rmm orchestrator update` | Verified native update flow; unsupported modes fail closed |
| `strata-rmm backup` | Create an encrypted integrated recovery set |
| `strata-rmm recovery ...` | Preflight, status, verify, key initialization, and isolated restore |
| `strata-rmm probe` | Network probe service |

Docker upgrades use the documented privileged **host-side** updater. The long-running web-facing orchestrator is intentionally not granted Docker-socket/root-equivalent access.

## Recovery

Use the integrated recovery engine rather than manual database replacement:

- [docs/BACKUP.md](docs/BACKUP.md)
- [docs/RESTORE.md](docs/RESTORE.md)

A green source test or successful dry run does not satisfy hosted RPO/RTO evidence. Hosted recovery acceptance remains tracked in Issue #15.

## Operations and configuration

- [docs/CONFIGURATION.md](docs/CONFIGURATION.md)
- [docs/RUNBOOK.md](docs/RUNBOOK.md)
- [docs/SECURITY.md](docs/SECURITY.md)
- [docs/INTERNAL_ALPHA_AGENT.md](docs/INTERNAL_ALPHA_AGENT.md)

Production secrets must be supplied through protected files/secret mechanisms where supported. Do not put secrets in command arguments, repository files, documentation examples, browser-readable profile records, or CI artifacts.

## Development and testing

Repository CI includes Go tests/race checks, frontend checks, security scanning, release packaging, recovery/resilience/observability gates, and internal-alpha source contracts. Exact-head green CI is required for merge but does not replace representative-host evidence.

Common development targets include:

```bash
make build
make test
make lint
make coverage
```

Refer to current CI workflows and Makefile targets rather than historical command lists when validating a release candidate.

## Project structure

```text
cmd/                 CLI entrypoints
internal/            application and endpoint implementation
pkg/                 shared auth/config/storage/recovery packages
deploy/              deployment assets (not all are accepted support paths)
docs/                operator, security, acceptance, and architecture docs
scripts/             installers, packaging, and validation helpers
ui/                  React/TypeScript web console
.github/workflows/   CI, security, release, and Phase 8 gates
```

## License

See [LICENSE](LICENSE).
