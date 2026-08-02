#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

MODE="docker"
DOMAIN=""
ADMIN_EMAIL=""
ACME_EMAIL=""
ACME_CA="production"
HTTPS_MODE=""
PLATFORM_NAME="Platform Administration"
PACKAGE_FILE=""
DATABASE_DSN_FILE=""
NATS_URL=""
NATS_ADVERTISE_URL=""
NATS_TOKEN_FILE=""
NATS_CA_FILE=""
PREPARE_ONLY="false"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_DIR="$REPO_DIR/deploy/docker"

usage() {
  cat <<'USAGE'
Usage:
  sudo ./scripts/install-platform.sh --mode docker --domain rmm.example.com \
    --admin-email owner@example.com [--acme-email owner@example.com]

  Automatic HTTPS uses Let's Encrypt production by default. Add
    --acme-ca staging
  for non-production issuance tests.

  sudo ./scripts/install-platform.sh --mode native --domain rmm.example.com \
    --admin-email owner@example.com --package-file ./strata-rmm-orchestrator_VERSION_ARCH.deb \
    --database-dsn-file /secure/postgres-dsn --nats-url tls://broker.example.com:4222 \
    --nats-advertise-url tls://broker.example.com:4222 \
    --nats-token-file /secure/nats-token --nats-ca-file /secure/nats-ca.crt

  Native HTTPS modes:
    --https-mode automatic  Use an installed Caddy package and packaged web UI.
    --https-mode external   Leave TLS and the web console to an external proxy.

The administrator password is prompted without echo. For unattended installation,
set STRATA_BOOTSTRAP_PASSWORD_FILE to a protected regular file instead.
Use --prepare-only to generate and validate Docker configuration without starting services.
USAGE
}

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
require_command() { command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"; }

validate_secret_file() {
  local path="$1" mode
  [[ -f "$path" ]] || die "secret file is not a regular file: $path"
  mode="$(stat -c '%a' "$path")"
  (( (8#$mode & 8#077) == 0 )) || die "secret file must not be accessible by group or others: $path"
}

prompt_admin_password() {
  local target="$1"
  if [[ -n "${STRATA_BOOTSTRAP_PASSWORD_FILE:-}" ]]; then
    validate_secret_file "$STRATA_BOOTSTRAP_PASSWORD_FILE"
    install -m 0600 "$STRATA_BOOTSTRAP_PASSWORD_FILE" "$target"
    return
  fi
  [[ -t 0 ]] || die "interactive terminal required or set STRATA_BOOTSTRAP_PASSWORD_FILE"
  local first second
  read -r -s -p "Initial administrator password (14-72 characters): " first; printf '\n'
  read -r -s -p "Confirm administrator password: " second; printf '\n'
  [[ "$first" == "$second" ]] || die "administrator passwords do not match"
  (( ${#first} >= 14 && ${#first} <= 72 )) || die "administrator password must be 14-72 characters"
  printf '%s' "$first" > "$target"
  unset first second
}

generate_certificate() {
  local name="$1" san="$2" secrets_dir="$3"
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "$secrets_dir/${name}.key" >/dev/null 2>&1
  openssl req -new -key "$secrets_dir/${name}.key" -subj "/CN=$name" -out "$secrets_dir/${name}.csr"
  openssl x509 -req -days 397 -sha256 -in "$secrets_dir/${name}.csr"     -CA "$secrets_dir/platform_ca.crt" -CAkey "$secrets_dir/platform_ca.key" -CAcreateserial     -extfile <(printf 'subjectAltName=%s\nextendedKeyUsage=serverAuth\n' "$san")     -out "$secrets_dir/${name}.crt"
  rm -f "$secrets_dir/${name}.csr"
}

wait_for_url() {
  local url="$1" attempts="${2:-60}"
  for ((i=1; i<=attempts; i++)); do
    curl --fail --silent --show-error "$url" >/dev/null 2>&1 && return 0
    sleep 2
  done
  return 1
}

acme_directory_url() {
  case "$ACME_CA" in
    production) printf '%s' 'https://acme-v02.api.letsencrypt.org/directory' ;;
    staging) printf '%s' 'https://acme-staging-v02.api.letsencrypt.org/directory' ;;
    *) die "--acme-ca must be production or staging" ;;
  esac
}

require_domain_resolution() {
  getent ahosts "$DOMAIN" 2>/dev/null | awk '{print $1}' | grep -q . || die "domain does not resolve: $DOMAIN"
}

verify_public_https() {
  local cert_file curl_args=(--fail --silent --show-error)
  [[ "$ACME_CA" == "production" ]] || curl_args+=(--insecure)
  wait_for_url_with_args "https://$DOMAIN/health/ready" 90 "${curl_args[@]}" || return 1
  if [[ "$ACME_CA" == "production" ]]; then
    cert_file="$(mktemp)"
    if ! openssl s_client -connect "$DOMAIN:443" -servername "$DOMAIN" \
      -verify_hostname "$DOMAIN" -verify_return_error -showcerts \
      </dev/null >"$cert_file" 2>/dev/null ||
      ! openssl x509 -in "$cert_file" -checkend 604800 -noout >/dev/null 2>&1; then
      rm -f "$cert_file"
      return 1
    fi
    rm -f "$cert_file"
  fi
}

require_public_ports_available() {
  if command -v ss >/dev/null 2>&1 &&
    ! systemctl is-active --quiet caddy.service 2>/dev/null &&
    ss -H -ltn | awk '{print $4}' | grep -Eq '(^|:)(80|443)$'; then
    die "TCP port 80 or 443 is already in use; stop the conflicting service or use --https-mode external"
  fi
}

wait_for_url_with_args() {
  local url="$1" attempts="$2"; shift 2
  for ((i=1; i<=attempts; i++)); do
    curl "$@" "$url" >/dev/null 2>&1 && return 0
    sleep 2
  done
  return 1
}

install_docker() {
  require_command docker; require_command openssl; require_command curl
  docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required"

  local secrets_dir="$COMPOSE_DIR/secrets"
  local admin_password="$secrets_dir/bootstrap_admin_password"
  install -d -m 0700 "$secrets_dir"
  [[ ! -e "$secrets_dir/postgres_password" ]] || die "installation secrets already exist; refusing to overwrite $secrets_dir"

  openssl rand -hex 32 > "$secrets_dir/postgres_password"
  openssl rand -hex 32 > "$secrets_dir/nats_token"
  openssl rand -hex 48 > "$secrets_dir/jwt_secret"
  openssl rand -hex 32 > "$secrets_dir/metrics_token"
  printf 'strata%s\n' "$(openssl rand -hex 8)" > "$secrets_dir/storage_access_key"
  openssl rand -base64 36 | tr -d '\n' > "$secrets_dir/storage_secret_key"
  printf '\n' >> "$secrets_dir/storage_secret_key"
  prompt_admin_password "$admin_password"

  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:4096 -out "$secrets_dir/platform_ca.key" >/dev/null 2>&1
  openssl req -x509 -new -sha256 -days 1825 -key "$secrets_dir/platform_ca.key"     -subj "/CN=Strata RMM Local Platform CA" -out "$secrets_dir/platform_ca.crt"
  generate_certificate "postgres_server" "DNS:postgres" "$secrets_dir"
  generate_certificate "nats_server" "DNS:nats,DNS:$DOMAIN" "$secrets_dir"

  local postgres_password nats_token
  postgres_password="$(<"$secrets_dir/postgres_password")"
  nats_token="$(<"$secrets_dir/nats_token")"
  printf 'postgres://strata:%s@postgres:5432/strata_rmm?sslmode=verify-full&sslrootcert=/run/secrets/platform_ca\n'     "$postgres_password" > "$secrets_dir/timescale_dsn"
  cat > "$secrets_dir/nats.conf" <<NATS
port: 4222
jetstream: enabled
authorization { token: "$nats_token" }
tls {
  cert_file: "/run/secrets/nats_tls_cert"
  key_file: "/run/secrets/nats_tls_key"
  ca_file: "/run/secrets/platform_ca"
  timeout: 5
}
NATS
  unset postgres_password nats_token
  chmod 0600 "$secrets_dir"/*

  [[ "$PREPARE_ONLY" == "true" ]] || require_domain_resolution
  cat > "$COMPOSE_DIR/.install.env" <<ENV
STRATA_DOMAIN=$DOMAIN
ACME_EMAIL=$ACME_EMAIL
ACME_CA_DIRECTORY=$(acme_directory_url)
ENV
  chmod 0600 "$COMPOSE_DIR/.install.env"

  local compose=(docker compose --env-file "$COMPOSE_DIR/.install.env" -f "$COMPOSE_DIR/docker-compose.install.yml")
  "${compose[@]}" config --quiet
  if [[ "$PREPARE_ONLY" == "true" ]]; then
    printf 'Docker installation configuration prepared and validated.\n'
    return
  fi
  "${compose[@]}" up -d postgres nats minio
  "${compose[@]}" run --rm --no-deps     -v "$admin_password:/run/bootstrap/admin-password:ro"     orchestrator orchestrator bootstrap-admin --email "$ADMIN_EMAIL"     --tenant-name "$PLATFORM_NAME" --password-file /run/bootstrap/admin-password
  rm -f "$admin_password"

  "${compose[@]}" up -d
  verify_public_https || die "public HTTPS readiness or certificate verification failed for $DOMAIN"
  printf 'Installation complete. Sign in at https://%s with %s\n' "$DOMAIN" "$ADMIN_EMAIL"
}

install_native() {
  require_command openssl; require_command curl
  [[ $EUID -eq 0 ]] || die "native installation must run as root"
  [[ -f "$PACKAGE_FILE" ]] || die "--package-file is required for native installation"
  validate_secret_file "$DATABASE_DSN_FILE"
  validate_secret_file "$NATS_TOKEN_FILE"
  [[ -f "$NATS_CA_FILE" ]] || die "--nats-ca-file is required"
  [[ "$NATS_URL" == tls://* || "$NATS_URL" == nats+tls://* ]] || die "native NATS URL must use TLS"
  [[ "$NATS_ADVERTISE_URL" == tls://* || "$NATS_ADVERTISE_URL" == nats+tls://* ]] || die "native advertised NATS URL must use TLS"

  if [[ "$HTTPS_MODE" == "automatic" ]]; then
    require_command caddy
    require_domain_resolution
    require_public_ports_available
  fi

  case "$PACKAGE_FILE" in
    *.deb) require_command apt-get; apt-get install -y "$PACKAGE_FILE" ;;
    *.rpm) require_command dnf; dnf install -y "$PACKAGE_FILE" ;;
    *) die "native package must be a .deb or .rpm file" ;;
  esac

  getent group strata-rmm >/dev/null || groupadd --system strata-rmm
  id strata-rmm >/dev/null 2>&1 || useradd --system --gid strata-rmm --home-dir /var/lib/strata-rmm --shell /usr/sbin/nologin strata-rmm
  install -d -o strata-rmm -g strata-rmm -m 0750 /var/lib/strata-rmm
  install -d -o root -g strata-rmm -m 0750 /etc/strata-rmm/secrets
  install -o root -g strata-rmm -m 0640 "$DATABASE_DSN_FILE" /etc/strata-rmm/secrets/timescale_dsn
  install -o root -g strata-rmm -m 0640 "$NATS_TOKEN_FILE" /etc/strata-rmm/secrets/nats_token
  install -o root -g strata-rmm -m 0644 "$NATS_CA_FILE" /etc/strata-rmm/secrets/nats_ca.crt
  openssl rand -hex 48 > /etc/strata-rmm/secrets/jwt_secret
  openssl rand -hex 32 > /etc/strata-rmm/secrets/metrics_token
  chown root:strata-rmm /etc/strata-rmm/secrets/jwt_secret /etc/strata-rmm/secrets/metrics_token
  chmod 0640 /etc/strata-rmm/secrets/jwt_secret /etc/strata-rmm/secrets/metrics_token

  local admin_password
  admin_password="$(mktemp /etc/strata-rmm/secrets/bootstrap-admin.XXXXXX)"
  prompt_admin_password "$admin_password"
  chown strata-rmm:strata-rmm "$admin_password"; chmod 0600 "$admin_password"

  cat > /etc/strata-rmm/orchestrator.env <<ENV
STRATA_RUNTIME_MODE=production
STRATA_PUBLIC_URL=https://$DOMAIN
CORS_ORIGINS=https://$DOMAIN
STRATA_API_ADDR=:8080
TIMESCALE_DSN_FILE=/etc/strata-rmm/secrets/timescale_dsn
NATS_URL=$NATS_URL
NATS_ADVERTISE_URLS=$NATS_ADVERTISE_URL
NATS_TOKEN_FILE=/etc/strata-rmm/secrets/nats_token
NATS_TLS_ENABLED=true
NATS_TLS_CA=/etc/strata-rmm/secrets/nats_ca.crt
JWT_SECRET_FILE=/etc/strata-rmm/secrets/jwt_secret
STRATA_METRICS_TOKEN_FILE=/etc/strata-rmm/secrets/metrics_token
STORAGE_BACKEND=none
STRATA_SEED_DEV=false
ENV
  chown root:strata-rmm /etc/strata-rmm/orchestrator.env
  chmod 0640 /etc/strata-rmm/orchestrator.env

  runuser -u strata-rmm -- env STRATA_RUNTIME_MODE=production     TIMESCALE_DSN_FILE=/etc/strata-rmm/secrets/timescale_dsn     /usr/local/bin/strata-orchestrator orchestrator bootstrap-admin     --email "$ADMIN_EMAIL" --tenant-name "$PLATFORM_NAME" --password-file "$admin_password"
  rm -f "$admin_password"

  systemctl daemon-reload
  systemctl enable --now strata-rmm.service
  wait_for_url "http://127.0.0.1:8080/health/ready" 60 || die "native orchestrator did not become ready"
  if [[ "$HTTPS_MODE" == "automatic" ]]; then
    [[ -f /usr/share/strata-rmm/ui/index.html ]] || die "native package does not contain the version-matched web console"
    install -d -m 0755 /etc/caddy
    local caddy_candidate caddy_backup=""
    caddy_candidate="$(mktemp /etc/caddy/Caddyfile.strata.XXXXXX)"
    cat > "$caddy_candidate" <<CADDY
# Managed by Strata RMM installer
{
  admin off
  email $ACME_EMAIL
  acme_ca $(acme_directory_url)
}

$DOMAIN {
  encode zstd gzip
  @api path /api/* /health /health/* /ready /ready/*
  handle @api {
    reverse_proxy 127.0.0.1:8080
  }
  handle {
    root * /usr/share/strata-rmm/ui
    try_files {path} /index.html
    file_server
  }
  header {
    Strict-Transport-Security "max-age=31536000; includeSubDomains"
    X-Content-Type-Options "nosniff"
    X-Frame-Options "DENY"
    Referrer-Policy "strict-origin-when-cross-origin"
    -Server
  }
}
CADDY
    chmod 0644 "$caddy_candidate"
    caddy validate --config "$caddy_candidate" --adapter caddyfile >/dev/null || { rm -f "$caddy_candidate"; die "generated Caddy configuration is invalid"; }
    if [[ -f /etc/caddy/Caddyfile ]]; then
      caddy_backup="$(mktemp /etc/caddy/Caddyfile.strata-backup.XXXXXX)"
      install -m 0600 /etc/caddy/Caddyfile "$caddy_backup"
    fi
    install -m 0644 "$caddy_candidate" /etc/caddy/Caddyfile
    rm -f "$caddy_candidate"
    if ! systemctl enable --now caddy.service || ! systemctl restart caddy.service || ! verify_public_https; then
      if [[ -n "$caddy_backup" ]]; then
        install -m 0644 "$caddy_backup" /etc/caddy/Caddyfile
        systemctl restart caddy.service || true
      else
        rm -f /etc/caddy/Caddyfile
        systemctl stop caddy.service || true
      fi
      die "automatic HTTPS failed; the previous Caddy configuration was restored when available"
    fi
    printf 'Native installation complete. Sign in at https://%s with %s\n' "$DOMAIN" "$ADMIN_EMAIL"
  else
    printf 'Native orchestrator installed. Configure the version-matched web console and HTTPS proxy for https://%s.\n' "$DOMAIN"
  fi
}

while (( $# > 0 )); do
  case "$1" in
    --mode) MODE="${2:-}"; shift 2 ;;
    --domain) DOMAIN="${2:-}"; shift 2 ;;
    --admin-email) ADMIN_EMAIL="${2:-}"; shift 2 ;;
    --acme-email) ACME_EMAIL="${2:-}"; shift 2 ;;
    --acme-ca) ACME_CA="${2:-}"; shift 2 ;;
    --https-mode) HTTPS_MODE="${2:-}"; shift 2 ;;
    --platform-name) PLATFORM_NAME="${2:-}"; shift 2 ;;
    --package-file) PACKAGE_FILE="${2:-}"; shift 2 ;;
    --database-dsn-file) DATABASE_DSN_FILE="${2:-}"; shift 2 ;;
    --nats-url) NATS_URL="${2:-}"; shift 2 ;;
    --nats-advertise-url) NATS_ADVERTISE_URL="${2:-}"; shift 2 ;;
    --nats-token-file) NATS_TOKEN_FILE="${2:-}"; shift 2 ;;
    --nats-ca-file) NATS_CA_FILE="${2:-}"; shift 2 ;;
    --prepare-only) PREPARE_ONLY="true"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ "$MODE" == "docker" || "$MODE" == "native" ]] || die "--mode must be docker or native"
[[ "$DOMAIN" =~ ^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$ ]] || die "a valid public --domain is required"
[[ "$ADMIN_EMAIL" =~ ^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$ ]] || die "a valid --admin-email is required"
ACME_EMAIL="${ACME_EMAIL:-$ADMIN_EMAIL}"
HTTPS_MODE="${HTTPS_MODE:-$(if [[ "$MODE" == "docker" ]]; then printf automatic; else printf external; fi)}"
[[ "$HTTPS_MODE" == "automatic" || "$HTTPS_MODE" == "external" ]] || die "--https-mode must be automatic or external"
[[ "$ACME_CA" == "production" || "$ACME_CA" == "staging" ]] || die "--acme-ca must be production or staging"
[[ "$MODE" != "docker" || "$HTTPS_MODE" == "automatic" ]] || die "Docker mode currently requires --https-mode automatic"

[[ $EUID -eq 0 ]] || die "installation must run as root"
if [[ "$MODE" == "docker" ]]; then install_docker; else install_native; fi
