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
DB_BACKUP_DIR="/var/lib/strata-rmm/backups/upgrades"
DB_HANDOFF="${STRATA_UPGRADE_DB_HANDOFF:-$UPDATE_DIR/database-backup.env}"

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

load_and_verify_database_backup() {
  if ! valid_schema_number "$SOURCE_SCHEMA" || ! valid_schema_number "$TARGET_SCHEMA"; then
    log "schema boundary is missing or invalid; refusing upgrade without a verifiable rollback boundary"
    return 1
  fi
  if [ "$SOURCE_SCHEMA" -gt "$TARGET_SCHEMA" ]; then
    log "invalid schema boundary: source=$SOURCE_SCHEMA target=$TARGET_SCHEMA"
    return 1
  fi
  if [ ! -r "$DB_HANDOFF" ]; then
    log "upgrade database backup handoff is unavailable: $DB_HANDOFF"
    return 1
  fi

  set -a
  # shellcheck source=/dev/null
  . "$DB_HANDOFF"
  set +a

  case "${DB_BACKUP_PATH:-}" in
    "$DB_BACKUP_DIR"/*) ;;
    *) log "database backup path is outside the protected upgrade backup directory"; return 1 ;;
  esac
  case "${DB_BACKUP_SHA256:-}" in
    [0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]*) ;;
    *) log "database backup digest is missing or malformed"; return 1 ;;
  esac
  if [ "${#DB_BACKUP_SHA256}" -ne 64 ]; then
    log "database backup digest must be SHA-256"
    return 1
  fi
  if [ ! -s "$DB_BACKUP_PATH" ]; then
    log "bound PostgreSQL backup is missing or empty"
    return 1
  fi
  actual="$(sha256sum "$DB_BACKUP_PATH" | awk '{print $1}')"
  if [ "$actual" != "$DB_BACKUP_SHA256" ]; then
    log "bound PostgreSQL backup integrity verification failed"
    return 1
  fi
  if [ -z "${PGHOST:-}" ] || [ -z "${PGUSER:-}" ] || [ -z "${PGDATABASE:-}" ]; then
    log "database restore connection handoff is incomplete"
    return 1
  fi
  return 0
}

restore_database_backup() {
  log "restoring exact pre-upgrade PostgreSQL data backup before previous binary restart"
  if ! pg_restore \
    --clean \
    --if-exists \
    --no-owner \
    --no-acl \
    --exit-on-error \
    --dbname="$PGDATABASE" \
    "$DB_BACKUP_PATH"; then
    log "PostgreSQL data restore failed; previous binary will remain stopped"
    return 1
  fi

  restored_schema="$(psql --no-psqlrc --tuples-only --no-align --command 'SELECT COALESCE(MAX(id), 0) FROM schema_migrations' | tr -d '[:space:]')"
  if [ "$restored_schema" != "$SOURCE_SCHEMA" ]; then
    log "restored PostgreSQL schema verification failed: got=$restored_schema expected=$SOURCE_SCHEMA"
    return 1
  fi
  log "PostgreSQL data restore completed and source schema was verified"
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

# A candidate is never started unless the exact pre-upgrade database backup is
# already present and checksum-verified. This makes rollback capability a hard
# prerequisite rather than a best-effort reaction after a failed restart.
if ! load_and_verify_database_backup; then
  log "upgrade refused because the row-level rollback backup is not trustworthy"
  exit 3
fi

log "restarting $SERVICE_NAME into staged candidate"
if ! systemctl restart "$SERVICE_NAME"; then
  log "service restart command failed; beginning rollback"
else
  if wait_for_health; then
    log "candidate became healthy; finalizing upgrade"
    rm -f "$BACKUP_PATH" "$FAILED_PATH" "$DB_BACKUP_PATH" "$DB_HANDOFF"
    rm -rf "$UPDATE_DIR"
    exit 0
  fi
  log "candidate did not become healthy; beginning rollback"
fi

systemctl stop "$SERVICE_NAME" || true

# Restore the exact pre-upgrade row-level database state before making the old
# binary active. If restore or verification fails, retain both binaries and the
# backup artifacts and leave the service stopped for operator recovery.
if ! restore_database_backup; then
  log "rollback halted fail-closed; candidate, rollback binary, and database backup were retained"
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
rm -f "$DB_BACKUP_PATH" "$DB_HANDOFF"
rm -rf "$UPDATE_DIR"
exit 2
