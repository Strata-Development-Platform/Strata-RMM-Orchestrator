# Secure Installation

Strata RMM supports a complete Docker installation and a native orchestrator package installation. Docker is the authoritative self-hosted alpha path because it includes the web console, automatic HTTPS, PostgreSQL/TimescaleDB, NATS JetStream, and the orchestrator in one validated topology.

Never pipe the installer directly from the network into a privileged shell. Obtain an immutable release, verify its checksum and Sigstore provenance, inspect the installer, and run it from that release.

## Requirements

### Docker installation

- 64-bit Linux host with at least 4 CPU cores, 8 GB RAM, and 40 GB free storage;
- Docker Engine with Compose v2;
- `openssl` and `curl`;
- a public DNS name pointed to the server;
- inbound TCP 80 and 443 for the web console and certificate issuance;
- inbound TCP 4222 for TLS-secured endpoint-agent messaging;
- outbound HTTPS for release and certificate services.

### Native installation

- a verified versioned `.deb` or `.rpm` orchestrator package;
- an existing TLS-enabled PostgreSQL/TimescaleDB service;
- an existing authenticated, TLS-enabled NATS JetStream service;
- a protected DSN file, NATS token file, and NATS CA certificate;
- an operator-managed HTTPS reverse proxy and web-console deployment.

The native installer does not silently create insecure plaintext dependencies.

## Docker clean install

From a verified release checkout:

```bash
sudo bash ./scripts/install-platform.sh \
  --mode docker \
  --domain rmm.example.com \
  --admin-email owner@example.com
```

The installer prompts twice for the initial administrator password without echo. It must contain 14–72 characters. PostgreSQL, NATS, JWT, and metrics credentials are generated independently with cryptographic randomness.

For an approved unattended deployment, place only the initial administrator password in a root-readable file:

```bash
sudo install -m 600 /secure/input/admin-password /run/strata-bootstrap-password
sudo env STRATA_BOOTSTRAP_PASSWORD_FILE=/run/strata-bootstrap-password \
  bash ./scripts/install-platform.sh \
  --mode docker \
  --domain rmm.example.com \
  --admin-email owner@example.com
sudo rm -f /run/strata-bootstrap-password
```

Do not pass any password as a command-line argument or ordinary environment value.

The Docker installer:

1. refuses to overwrite an existing secret set;
2. creates a private platform certificate authority;
3. issues service certificates with exact DNS identities;
4. enables PostgreSQL TLS and SCRAM host authentication;
5. enables NATS TLS, token authentication, and JetStream;
6. stores runtime credentials as Compose secrets;
7. renders and validates Compose before starting dependencies;
8. applies schema migrations under the migration lock;
9. creates the first administrator under a separate bootstrap lock;
10. records the bootstrap audit event;
11. removes the temporary bootstrap password copy;
12. starts the HTTPS web console and waits for readiness.

After success, visit `https://rmm.example.com` and sign in with the administrator email and the password you supplied.

## Native package install

The native mode never downloads a mutable package. Provide the locally verified release artifact and protected dependency files:

```bash
sudo bash ./scripts/install-platform.sh \
  --mode native \
  --domain rmm.example.com \
  --admin-email owner@example.com \
  --package-file ./strata-rmm-orchestrator_VERSION_ARCH.deb \
  --database-dsn-file /secure/postgres-dsn \
  --nats-url tls://broker.example.com:4222 \
  --nats-advertise-url tls://broker.example.com:4222 \
  --nats-token-file /secure/nats-token \
  --nats-ca-file /secure/nats-ca.crt
```

The installer copies protected inputs into `/etc/strata-rmm/secrets`, generates JWT and metrics secrets, writes a non-secret environment mapping, runs the one-time bootstrap, and enables `strata-rmm.service`.

Native mode verifies the local API readiness endpoint. You must separately deploy the version-matched web console and HTTPS reverse proxy before exposing it to users. Until that is done, the native path is not a complete browser installation.

## One-time administrator bootstrap

The local command is:

```bash
strata-orchestrator orchestrator bootstrap-admin \
  --email owner@example.com \
  --tenant-name "Platform Administration" \
  --password-file /protected/admin-password
```

The command:

- accepts no password flag;
- rejects group/world-readable password files;
- serializes concurrent attempts;
- refuses to run after any user exists;
- creates the administrator and audit event in one serializable transaction;
- never returns the password or hash.

It is not an HTTP endpoint and cannot be invoked remotely.

## Preparation-only validation

Operators and CI can generate and render the Docker configuration without starting services:

```bash
sudo env STRATA_BOOTSTRAP_PASSWORD_FILE=/protected/admin-password \
  bash ./scripts/install-platform.sh \
  --mode docker \
  --domain rmm.example.com \
  --admin-email owner@example.com \
  --prepare-only
```

Preparation leaves generated secret material in `deploy/docker/secrets`. Protect or remove that directory before retrying; the installer intentionally refuses to overwrite it.

## Secret backup

Back up these items through a protected, encrypted operator process:

- `deploy/docker/secrets/platform_ca.key`;
- runtime secret files;
- `deploy/docker/.install.env`;
- PostgreSQL data and the configured backup repository.

Do not copy them into Git, tickets, chat, logs, or CI artifacts.

## Updates

Use only immutable semantic-version releases. Verify checksums, signatures, and provenance before an upgrade. See [UPGRADE.md](UPGRADE.md).

Do not use `latest`, unpinned container tags, unauthenticated update endpoints, or destructive down migrations.

## Troubleshooting

Docker status:

```bash
docker compose \
  --env-file deploy/docker/.install.env \
  -f deploy/docker/docker-compose.install.yml ps
```

Docker logs:

```bash
docker compose \
  --env-file deploy/docker/.install.env \
  -f deploy/docker/docker-compose.install.yml logs --tail=200
```

Native status:

```bash
systemctl status strata-rmm.service
journalctl -u strata-rmm.service --since "15 minutes ago"
```

Redact DSNs, tokens, certificate private keys, and environment contents before sharing diagnostics.
