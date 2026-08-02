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

Automatic HTTPS uses the Let's Encrypt production directory by default. Use
`--acme-ca staging` only for issuance rehearsals; staging certificates are not
trusted by browsers. The installer verifies public DNS before mutation, and the
host must continue to accept inbound TCP 80 and 443 so Caddy can issue and renew
certificates. Certificate account and key material persist in the Compose
`caddy_data` volume across same-version redeployments.

### Native installation

- a verified versioned `.deb` or `.rpm` orchestrator package;
- an existing TLS-enabled PostgreSQL/TimescaleDB service;
- an existing authenticated, TLS-enabled NATS JetStream service;
- a protected DSN file, NATS token file, and NATS CA certificate;
- either a trusted, distribution-installed Caddy service for automatic HTTPS,
  or an operator-managed HTTPS reverse proxy.

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
12. starts the HTTPS web console, waits for readiness, and verifies the public
    certificate hostname and that it remains valid for at least seven days.

After success, visit `https://rmm.example.com` and sign in with the administrator email and the password you supplied.

## First sign-in and provider business setup

The bootstrap command grants the initial administrator an active
`platform_owner` membership on the singleton platform. On that account's first
sign-in, the web console reads the server-owned setup status and redirects the
administrator to `/provider/setup` until the provider business profile is
complete.

Complete all four wizard stages:

1. enter the legal and display names;
2. enter the primary, support, billing, and telephone contacts;
3. enter the address and regional defaults;
4. review the complete profile, then explicitly select **Complete setup**.

The Continue control on Regional Defaults only opens Review; it does not submit
the profile. Completion is accepted only after server-side authorization and
validation. The profile, immutable completion actor/time, and
`provider.setup_completed` audit event are committed together. A successful
completion returns the administrator to the provider dashboard, and later
sign-ins do not repeat the wizard.

If setup is interrupted before completion, sign in again and complete the
wizard. Unsaved wizard values are browser state and must be re-entered after a
reload or sign-out. After completion, a top-level platform owner or platform
administrator can edit the business fields under **Settings → Platform
Settings → Provider business profile**. Completion metadata cannot be edited.
See [CONFIGURATION.md](CONFIGURATION.md#provider-business-profile) for the field
contract and [RUNBOOK.md](RUNBOOK.md#provider-business-profile-setup-and-recovery)
for recovery and audit checks.

## Create an MSP and activate its owner

Configure the account mailer and `STRATA_PUBLIC_URL` before inviting an MSP
owner; see [SMTP account mailer and owner lifecycle](CONFIGURATION.md#smtp-account-mailer-and-owner-lifecycle).
Then a top-level platform owner or platform administrator completes the
provider-approved onboarding flow:

1. In **MSP Tenants**, create the MSP with its name, slug, plan, and intended
   owner email address. The tenant is created as **Pending owner activation**;
   it cannot resolve by MSP host name or be opened as a workspace yet.
2. Strata sends that address a 72-hour email link of the form
   `https://rmm.example.com/activate-account#<token>`. The token is in the URL
   fragment and is not sent to the web server by browser navigation.
3. The invited owner opens the link, confirms the masked destination and MSP
   name, and sets a 14–72-byte password. Successful acceptance verifies the
   email address, creates the first `msp_owner` membership, and activates the
   MSP and its entitlement atomically.
4. Acceptance does not create a session. The owner follows the sign-in link and
   signs in with the invited email address and new password.

There is no open public sign-up path. If delivery is failed or unconfigured, or
the link expires, a top-level platform operator can rotate and resend it from
**MSP Tenants**. A still-valid invitation already recorded as delivered is not
rotated. See [RUNBOOK.md](RUNBOOK.md#msp-owner-activation-recovery) for recovery
and audit checks.

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

The installer copies protected inputs into `/etc/strata-rmm/secrets`, generates JWT and metrics secrets, writes a non-secret environment mapping, runs the one-time bootstrap, and enables `strata-rmm.service`. The package contains the version-matched web console under `/usr/share/strata-rmm/ui`.

The compatibility default is `--https-mode external`. To make the native
installation browser-ready with automatic Let's Encrypt certificates, install
Caddy from a trusted distribution or Caddy repository first, ensure DNS points
to the host and ports 80/443 are free, then add:

```bash
  --https-mode automatic \
  --acme-email certificates@example.com
```

The installer does not download or execute a mutable proxy installer. In
automatic mode it validates its generated configuration, preserves an existing
configuration under `/etc/caddy/Caddyfile.strata-backup.*`, restarts Caddy, and
verifies public readiness and the production certificate. Because this mode
owns the complete Caddyfile, use `--https-mode external` on a shared proxy. If
proxy startup or verification fails, it restores the immediately preceding
configuration, or removes the new configuration and stops Caddy on a first
install. Package, database migration, bootstrap, and orchestrator-service
changes are not rolled back.

With `--https-mode external`, native mode verifies only the local API readiness
endpoint. Configure an external HTTPS proxy to serve the packaged console and
forward `/api/*`, `/health*`, and `/ready*` to `127.0.0.1:8080` before exposing
it to users.

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
- the Docker `caddy_data` volume, or `/var/lib/caddy` and `/etc/caddy` for a
  native automatic-HTTPS installation;
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

Automatic HTTPS status:

```bash
systemctl status caddy.service
journalctl -u caddy.service --since "15 minutes ago"
curl --fail --show-error https://rmm.example.com/health/ready
openssl s_client -connect rmm.example.com:443 -servername rmm.example.com \
  -verify_hostname rmm.example.com -verify_return_error </dev/null
```

Redact DSNs, tokens, certificate private keys, and environment contents before sharing diagnostics.
