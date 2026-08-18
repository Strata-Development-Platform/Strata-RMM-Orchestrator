#!/bin/sh
set -eu

SERVICE_NAME="${STRATA_UPGRADE_SERVICE:-strata-rmm.service}"
BINARY_PATH="${1:-/usr/local/bin/strata-orchestrator}"
SOURCE_SCHEMA="${2:-}"
TARGET_SCHEMA="${3:-}"
BACKUP_PATH="${BINARY_PATH}.bak"
FAILED_PATH="${BINARY_PATH}.failed"
HEALTH_URL="${STRATA_UPGRADE_HEALTH_URL:-http://127.0.0.1:8080/health}"
ATTEMPTS="${STRATA_UPGRADE_HEALTH_ATTEMPTS:-30}"
SLEEP_SECONDS="${STRATA_UPGRADE_HEALTH_INTERVAL:-2}"
UPDATE_DIR="/var/lib/strata-rmm/updates"
ENV_FILE="${STRATA_UPGRADE_ENV_FILE:-/etc/strata-rmm/orchestrator.env}"

log() {
  printf '%s %s\n' "strata-upgrade-finalizer:" "$*" >&2
}

health_ok() {
  curl --fail --silent --show-error --max-time 5 "$HEALTH_URL" >/dev/null 2>&1
}

wait_for_health() {
  attempt=1
  while [ "$attempt" -le "$ATTEMPTS" ]; do
    if health_ok; then
      return 0
    fi
    sleep "$SLEEP_SECONDS"
    attempt=$((attempt + 1))
  done
  return 1
}

valid_schema_number() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

rollback_schema_if_needed() {
  if ! valid_schema_number "$SOURCE_SCHEMA" || ! valid_schema_number "$TARGET_SCHEMA"; then
    log "schema rollback boundary is missing or invalid; refusing to start previous binary against an unknown schema"
    return 1
  fi
  if [ "$SOURCE_SCHEMA" = "$TARGET_SCHEMA" ]; then
    log "candidate did not change schema; database rollback is not required"
    return 0
  fi
  if [ "$SOURCE_SCHEMA" -gt "$TARGET_SCHEMA" ]; then
    log "invalid schema rollback boundary: source=$SOURCE_SCHEMA target=$TARGET_SCHEMA"
    return 1
  fi
  if [ ! -r "$ENV_FILE" ]; then
    log "protected orchestrator environment is unavailable for schema rollback: $ENV_FILE"
    return 1
  fi

  # The transient finalizer runs as root. Load the same protected environment
  # used by the systemd service without exposing credentials in argv or logs.
  set -a
  # shellcheck source=/dev/null
  . "$ENV_FILE"
  set +a

  log "rolling database schema back from candidate target $TARGET_SCHEMA to source $SOURCE_SCHEMA"
  if ! "$BINARY_PATH" orchestrator rollback "$SOURCE_SCHEMA"; then
    log "database schema rollback failed; previous binary will remain stopped"
    return 1
  fi
  log "database schema rollback completed"
  return 0
}

if [ ! -x "$BINARY_PATH" ]; then
  log "candidate binary is missing or not executable: $BINARY_PATH"
  exit 1
fi

if [ ! -f "$BACKUP_PATH" ]; then
  log "rollback binary is missing: $BACKUP_PATH"
  exit 1
fi

log "restarting $SERVICE_NAME into staged candidate"
if ! systemctl restart "$SERVICE_NAME"; then
  log "service restart command failed; beginning rollback"
else
  if wait_for_health; then
    log "candidate became healthy; finalizing upgrade"
    rm -f "$BACKUP_PATH" "$FAILED_PATH"
    rm -rf "$UPDATE_DIR"
    exit 0
  fi
  log "candidate did not become healthy; beginning rollback"
fi

systemctl stop "$SERVICE_NAME" || true

# Never restore the previous binary until the database is back at the exact
# schema version captured before staging. If this fails, retain both candidate
# and backup for operator recovery and leave the service stopped.
if ! rollback_schema_if_needed; then
  log "rollback halted fail-closed; candidate and backup binaries were retained"
  exit 3
fi

rm -f "$FAILED_PATH"
if ! mv "$BINARY_PATH" "$FAILED_PATH"; then
  log "unable to preserve failed candidate"
  exit 1
fi
if ! mv "$BACKUP_PATH" "$BINARY_PATH"; then
  log "unable to restore rollback binary"
  exit 1
fi

log "rollback binary restored; starting $SERVICE_NAME"
if ! systemctl start "$SERVICE_NAME"; then
  log "rollback service start failed"
  exit 1
fi
if ! wait_for_health; then
  log "rollback binary failed health verification"
  exit 1
fi

log "rollback completed and previous version is healthy"
rm -rf "$UPDATE_DIR"
exit 2
