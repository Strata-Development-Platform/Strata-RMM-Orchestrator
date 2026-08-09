# Strata RMM — Alpha Deployment Guide

This is the shortest supported path for real internal Alpha deployment testing. The detailed operational reference remains in `docs/DEPLOYMENT.md`.

## Recommended Alpha path: hardened Docker installer

Use a clean Linux VM or dedicated test host. Do not reuse a development database for acceptance evidence.

### Prerequisites

- Docker Engine with Compose v2
- Git
- OpenSSL and curl
- a DNS name that resolves to the Alpha host
- TCP 80/443 available for automatic HTTPS
- outbound internet access unless intentionally testing an air-gapped path

### 1. Clone the exact Alpha candidate

```bash
git clone https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator.git
cd Strata-RMM-Orchestrator
git checkout <alpha-candidate-tag-or-sha>
git rev-parse HEAD
```

Record that exact SHA with every acceptance artifact.

### 2. Run the supported platform installer

The repository installer generates strong service secrets, configures encrypted PostgreSQL/NATS transport for the Docker installation, securely bootstraps the first administrator, renders/validates Compose, starts dependencies, and verifies public HTTPS readiness.

```bash
sudo ./scripts/install-platform.sh \
  --mode docker \
  --domain rmm.example.com \
  --admin-email owner@example.com \
  --acme-email owner@example.com
```

The administrator password is prompted without echo. For an unattended test, provide a protected password file through `STRATA_BOOTSTRAP_PASSWORD_FILE` as documented by the installer. Do not use development seed credentials for Alpha acceptance.

To validate generated deployment configuration without starting services:

```bash
sudo ./scripts/install-platform.sh \
  --mode docker \
  --domain rmm.example.com \
  --admin-email owner@example.com \
  --acme-email owner@example.com \
  --prepare-only
```

Use `--acme-ca staging` for certificate-issuance testing when appropriate; production Alpha evidence should use a normally trusted certificate.

### 3. Verify readiness and start the evidence bundle

After installation, verify the public endpoint using the configured domain:

```bash
export STRATA_ALPHA_URL=https://rmm.example.com
./scripts/alpha-acceptance.sh preflight
```

The harness checks `/health`, `/health/live`, and `/health/ready`, records the exact Git SHA and host metadata, and writes a local evidence bundle. It does **not** declare any Phase 8 gate accepted.

A container merely being `running` is not sufficient. Readiness must verify the dependencies required by the selected configuration.

### 4. Complete provider setup

Sign in with the bootstrapped administrator and complete the provider business-profile workflow. Record evidence that setup succeeds without direct database edits.

### 5. Execute the Alpha lifecycle

For every Alpha candidate, prove this sequence end to end:

```text
provider bootstrap
  -> provider business profile
  -> create MSP
  -> activate MSP owner
  -> create client
  -> create site
  -> create scoped enrollment token
  -> install Linux and/or Windows agent
  -> heartbeat online
  -> inventory visible
  -> telemetry visible
  -> monitoring rule triggers
  -> alert visible
  -> ticket/notification path
  -> remote command/script result
  -> patch/software workflow where supported
  -> backup
  -> isolated restore
  -> upgrade
  -> rollback exercise
```

Capture exact commit/release, timestamps, environment details, pass/fail result, and links/artifacts for each acceptance row. Use `./scripts/alpha-acceptance.sh snapshot` at important lifecycle boundaries to capture health/metrics observations.

## Linux agent installation

Use a scoped one-time enrollment token from the Strata console. From a source checkout, the convenience wrapper is:

```bash
sudo ./scripts/install.sh \
  --server https://rmm.example.com \
  --token '<scoped-enrollment-token>'
```

The wrapper fetches the current installer from the selected Strata server. It does not request direct NATS credentials and does not reboot the endpoint automatically.

## Native/bare-metal Alpha path

`scripts/install-platform.sh --mode native` is the supported native installer path. It requires the packaged release plus protected PostgreSQL/NATS secret files and TLS configuration. Use its built-in help and `docs/DEPLOYMENT.md` for the full native requirements. The acceptance rules are identical: clean host, non-default secrets, exact build identity, readiness verification, and the complete lifecycle without database edits.

## Baseline HTTP load

Use `scripts/loadtest.sh` for authorized HTTP baseline measurements. The load harness deliberately does not publish directly to tenant NATS subjects or manufacture enrollment traffic with hard-coded tenant IDs.

```bash
API_URL=https://rmm.example.com DURATION=5m RATE=100 \
  ./scripts/loadtest.sh health
```

For authenticated API load, use a short-lived real token:

```bash
API_URL=https://rmm.example.com AUTH_TOKEN='<short-lived-token>' \
  DURATION=5m RATE=50 ./scripts/loadtest.sh authenticated
```

Agent-scale, telemetry, and reconnect-storm evidence must use enrolled agents or the repository's dedicated resilience tooling. Bypassing enrollment or tenant authorization is not valid Alpha evidence.

## 24-hour soak evidence

The default soak duration is 24 hours and samples readiness every 60 seconds:

```bash
STRATA_ALPHA_URL=https://rmm.example.com \
  ./scripts/alpha-acceptance.sh soak
```

A shorter run may be used while validating the harness, but it must not be represented as A8-15 acceptance. For example:

```bash
STRATA_ALPHA_SOAK_SECONDS=600 ./scripts/alpha-acceptance.sh soak
```

The environment operator must additionally retain the corresponding server metrics needed to evaluate memory, goroutine/thread, connection, queue, latency, and telemetry-loss behavior. Readiness samples alone are not sufficient to accept A8-15.

## Controlled dependency-failure exercises

The acceptance harness can verify readiness degradation and recovery, but it intentionally does not assume Docker, Kubernetes, systemd, or cloud topology. The operator supplies explicit fault/recovery hooks.

Example for a Docker test host:

```bash
export STRATA_ALPHA_URL=https://rmm.example.com
export STRATA_ALPHA_ALLOW_DESTRUCTIVE=1
export STRATA_ALPHA_FAULT_POSTGRES_CMD='docker compose stop postgres'
export STRATA_ALPHA_RECOVER_POSTGRES_CMD='docker compose start postgres'
./scripts/alpha-acceptance.sh fault postgres
```

Supported hook names are `postgres`, `nats`, `storage`, and `orchestrator`. Run these only in an isolated Alpha environment. A fault exercise is not complete until telemetry/jobs/data integrity are checked after recovery; the harness proves health transition timing, not all subsystem semantics.

## HTTPS and DNS

Production-mode Alpha testing must use the configured HTTPS public URL. DNS must resolve before hosted enrollment is accepted as evidence. Do not disable TLS validation simply to make an acceptance run pass.

## Upgrade and rollback

Before an upgrade exercise:

1. create and verify a backup;
2. record the current version/SHA;
3. follow `docs/UPGRADE.md`;
4. verify health, migrations, agent compatibility, telemetry, commands, and UI after upgrade;
5. inject or simulate a failed rollout using the documented procedure;
6. follow `docs/ROLLBACK.md` and verify data/tenant integrity.

## Disaster recovery

A backup file existing is not recovery evidence. Restore into an isolated target and verify schema, tenant data, durable work, audit evidence, and required object data. Measure and record RPO/RTO.

## Alpha failure tests

At least once for the candidate release, exercise:

- PostgreSQL/TimescaleDB outage and recovery;
- NATS/JetStream outage and recovery;
- orchestrator restart;
- endpoint disconnect/reconnect;
- duplicate/retried durable command handling;
- object-storage failure when configured;
- failed upgrade/rollback;
- reconnect storm using the repository resilience harness;
- sustained soak according to the Phase 8 acceptance matrix.

No silent telemetry loss, cross-tenant data exposure, unauthorized command execution, or destructive duplicate execution is acceptable.

## Finalize the evidence bundle

After the environment exercise:

```bash
STRATA_ALPHA_URL=https://rmm.example.com \
  ./scripts/alpha-acceptance.sh finalize
```

The command records candidate identity and, when available, checksums the evidence files. The bundle remains observational evidence only. Acceptance requires review against `docs/PHASE_8_ACCEPTANCE_MATRIX.md`, CI records, lifecycle evidence, recovery results, security review, and the signed go/no-go record.

## Troubleshooting order

When the deployment is not ready, check in this order:

1. platform installer error output and service/container status;
2. `/health/live` and `/health/ready`;
3. orchestrator logs;
4. PostgreSQL/TimescaleDB connectivity and migrations;
5. NATS/JetStream connectivity;
6. storage readiness if enabled;
7. public URL/TLS/DNS correctness;
8. agent enrollment identity and NATS subject scope.

Do not bypass a failing dependency just to make the dashboard load. Fix the failing readiness dependency or explicitly disable only genuinely optional functionality.

## Evidence checklist

An Alpha deployment is not accepted until the relevant `docs/PHASE_8_ACCEPTANCE_MATRIX.md` rows have durable evidence tied to the exact candidate SHA. Documentation statements, screenshots without commit identity, generated evidence files without review, or stale CI runs are not sufficient.
