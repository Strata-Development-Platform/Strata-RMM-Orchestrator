#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_DIR="$REPO_DIR/deploy/docker"
COMPOSE_FILE="$COMPOSE_DIR/docker-compose.install.yml"
ENV_FILE="$COMPOSE_DIR/.install.env"
PROJECT="strata-rmm"
JOURNAL_FILE="/var/lib/strata-rmm/docker-upgrade/docker-upgrade.json"
REPOSITORY="ghcr.io/strata-development-platform/strata-rmm-orchestrator"

usage() {
  cat <<'USAGE'
Usage:
  sudo ./scripts/update-docker-platform.sh [--repository REGISTRY/REPOSITORY]

Runs the verified Docker upgrade transaction in a one-shot privileged utility
container. The long-running orchestrator service never receives Docker-socket
access.
USAGE
}

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

while (($#)); do
  case "$1" in
    --repository) shift; (($#)) || die "--repository requires a value"; REPOSITORY="$1" ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

[[ $EUID -eq 0 ]] || die "Docker platform update must run as root"
command -v docker >/dev/null 2>&1 || die "docker is required"
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required"
[[ -S /var/run/docker.sock ]] || die "Docker socket is unavailable"
[[ -f "$COMPOSE_FILE" ]] || die "protected production Compose file is missing: $COMPOSE_FILE"
[[ -f "$ENV_FILE" ]] || die "protected production Compose environment is missing: $ENV_FILE"
[[ ! -L "$COMPOSE_FILE" && ! -L "$ENV_FILE" ]] || die "protected Compose files must not be symlinks"
(( (8#$(stat -c '%a' "$ENV_FILE") & 8#077) == 0 )) || die "protected Compose environment permissions are too broad"
[[ "$REPOSITORY" != *"@"* && "$REPOSITORY" != *"://"* && -n "$REPOSITORY" ]] || die "--repository must be a canonical registry/repository reference"

grep -Eq '^STRATA_ORCHESTRATOR_IMAGE=[^[:space:]@]+@sha256:[0-9a-f]{64}$' "$ENV_FILE" || \
  die "protected Compose state does not contain an immutable STRATA_ORCHESTRATOR_IMAGE"

compose=(docker compose --project-name "$PROJECT" --env-file "$ENV_FILE" -f "$COMPOSE_FILE")
"${compose[@]}" config --quiet

# The current signed orchestrator image is reused only as a one-shot utility.
# Service secrets, the orchestrator data volume, and backend network come from
# the service definition; Docker-socket access exists only for this transient
# container. Mounting COMPOSE_DIR at the identical absolute path keeps relative
# Compose secret paths valid when the nested Docker CLI talks to the host daemon.
"${compose[@]}" run --rm --no-deps --user 0:0 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$COMPOSE_DIR:$COMPOSE_DIR" \
  orchestrator \
  docker-update-host \
  --compose-file "$COMPOSE_FILE" \
  --env-file "$ENV_FILE" \
  --journal-file "$JOURNAL_FILE" \
  --project "$PROJECT" \
  --repository "$REPOSITORY"
