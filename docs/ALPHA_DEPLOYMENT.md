# Strata RMM — Alpha Deployment Guide

This guide is the shortest supported path for real internal Alpha deployment testing. The detailed operational reference remains in `docs/DEPLOYMENT.md`.

## Recommended Alpha path: Docker Compose

Use a clean Linux VM or dedicated test host. Do not reuse a development database for acceptance evidence.

### Prerequisites

- Docker Engine with Compose support
- Git
- DNS name for the test deployment when testing HTTPS/public enrollment
- outbound internet access for images/releases unless testing an intentionally air-gapped path

### 1. Clone the exact Alpha candidate

```bash
git clone https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator.git
cd Strata-RMM-Orchestrator
git checkout <alpha-candidate-tag-or-sha>
```

Record the exact SHA used for every acceptance run:

```bash
git rev-parse HEAD
```

### 2. Prepare secrets and configuration

Copy the example environment file and replace every placeholder/default secret before using production mode:

```bash
cp .env.example .env
```

At minimum verify the public URL, database credentials/DSN, JWT secret, NATS configuration, storage configuration, and initial bootstrap settings. Do not use development seed credentials for Alpha acceptance.

Generate long random secrets with an operating-system secret generator or password manager. Never paste committed defaults into an Alpha evidence environment.

### 3. Start the stack

Use the Compose definition shipped with the exact candidate being tested:

```bash
docker compose up -d --build
```

### 4. Verify infrastructure before opening the UI

```bash
docker compose ps
curl --fail http://127.0.0.1:8080/health
curl --fail http://127.0.0.1:8080/health/live
curl --fail http://127.0.0.1:8080/health/ready
```

A container merely being `running` is not sufficient. Readiness must verify the dependencies required by the selected configuration.

### 5. Complete provider bootstrap

Open the configured Strata URL and complete the initial provider administrator/business-profile workflow. Record evidence that bootstrap succeeded without direct database edits.

### 6. Execute the Alpha lifecycle

For every Alpha candidate, prove this sequence end to end:

```text
provider bootstrap
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

Capture exact commit/release, timestamps, environment details, pass/fail result, and links/artifacts for each acceptance row.

## Bare-metal/systemd Alpha path

Use `docs/DEPLOYMENT.md` for the full systemd installation. The acceptance rules are identical: start from a clean host, use non-default secrets, verify readiness, record the exact build SHA, and execute the complete lifecycle without database edits.

## HTTPS and DNS

Production-mode Alpha testing must use the configured HTTPS public URL. DNS must resolve to the test host before agent enrollment is accepted as hosted evidence. Do not disable TLS validation to make an acceptance run pass.

## Upgrade and rollback

Before an upgrade exercise:

1. create and verify a backup;
2. record the current version/SHA;
3. follow `docs/UPGRADE.md`;
4. verify health, migrations, agent compatibility, telemetry, commands, and UI after upgrade;
5. inject or simulate a failed rollout using the documented procedure;
6. follow `docs/ROLLBACK.md` and verify data/tenant integrity.

## Disaster recovery

A backup file existing is not recovery evidence. Restore into an isolated target and verify schema, tenant data, durable work, audit evidence, and required object data. Measure and record RPO/RTO for the drill.

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

## Troubleshooting order

When the deployment is not ready, check in this order:

1. `docker compose ps` or service status;
2. `/health/live` and `/health/ready`;
3. orchestrator logs;
4. PostgreSQL/TimescaleDB connectivity and migrations;
5. NATS/JetStream connectivity;
6. storage readiness if enabled;
7. public URL/TLS/DNS correctness;
8. agent enrollment identity and NATS subject scope.

Do not bypass a failing dependency simply to get the dashboard to load. Fix the failing readiness dependency or explicitly disable only genuinely optional functionality.

## Evidence checklist

An Alpha deployment is not accepted until the relevant `docs/PHASE_8_ACCEPTANCE_MATRIX.md` rows have durable evidence tied to the exact candidate SHA. Documentation statements, screenshots without commit identity, or stale CI runs are not sufficient.
