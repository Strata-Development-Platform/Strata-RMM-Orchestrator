#!/bin/sh
set -eu

FINALIZER="${1:-scripts/finalize-orchestrator-upgrade.sh}"

run_case() {
  restore_exit="$1"
  expect_exit="$2"
  tamper="${3:-0}"
  temp="$(mktemp -d)"
  trap 'rm -rf "$temp"' EXIT INT TERM

  bin="$temp/strata-orchestrator"
  backup="$bin.bak"
  dbdir="$temp/db-backups"
  dbbackup="$dbdir/pre-upgrade.dump"
  handoff="$temp/database-backup.env"
  fakebin="$temp/fakebin"
  events="$temp/events"
  healthy="$temp/healthy"
  mkdir -p "$fakebin" "$dbdir"
  : > "$events"

  cat > "$bin" <<EOF
#!/bin/sh
printf '%s\n' "candidate:\$*" >> "$events"
exit 0
EOF
  chmod +x "$bin"

  cat > "$backup" <<EOF
#!/bin/sh
printf '%s\n' "previous:\$*" >> "$events"
exit 0
EOF
  chmod +x "$backup"

  printf '%s\n' 'row-level-postgresql-backup-fixture' > "$dbbackup"
  digest="$(sha256sum "$dbbackup" | awk '{print $1}')"
  if [ "$tamper" -ne 0 ]; then
    digest="0000000000000000000000000000000000000000000000000000000000000000"
  fi

  cat > "$handoff" <<EOF
DB_BACKUP_PATH='$dbbackup'
DB_BACKUP_SHA256='$digest'
PGHOST='database.invalid'
PGPORT='5432'
PGUSER='strata_upgrade_test'
PGPASSWORD='not-a-real-secret'
PGDATABASE='strata_upgrade_test'
PGSSLMODE='disable'
EOF
  chmod 600 "$handoff"

  cat > "$fakebin/systemctl" <<EOF
#!/bin/sh
printf '%s\n' "systemctl:\$*" >> "$events"
case "\${1:-}" in
  restart) exit 0 ;;
  stop) exit 0 ;;
  start) : > "$healthy"; exit 0 ;;
  *) exit 0 ;;
esac
EOF
  chmod +x "$fakebin/systemctl"

  cat > "$fakebin/curl" <<EOF
#!/bin/sh
[ -f "$healthy" ] && exit 0
exit 1
EOF
  chmod +x "$fakebin/curl"

  cat > "$fakebin/pg_restore" <<EOF
#!/bin/sh
printf '%s\n' "pg_restore:\$*" >> "$events"
exit $restore_exit
EOF
  chmod +x "$fakebin/pg_restore"

  cat > "$fakebin/psql" <<EOF
#!/bin/sh
printf '%s\n' "psql:\$*" >> "$events"
printf '%s\n' 92
exit 0
EOF
  chmod +x "$fakebin/psql"

  set +e
  PATH="$fakebin:$PATH" \
  STRATA_UPGRADE_DB_BACKUP_DIR="$dbdir" \
  STRATA_UPGRADE_DB_HANDOFF="$handoff" \
  STRATA_UPGRADE_HEALTH_ATTEMPTS=1 \
  STRATA_UPGRADE_HEALTH_INTERVAL=0 \
    sh "$FINALIZER" "$bin" 92 93 >/dev/null 2>&1
  rc=$?
  set -e

  if [ "$rc" -ne "$expect_exit" ]; then
    echo "unexpected finalizer exit: got $rc want $expect_exit" >&2
    cat "$events" >&2
    exit 1
  fi

  if [ "$tamper" -ne 0 ]; then
    if grep -q '^systemctl:restart ' "$events"; then
      echo "candidate was started with an untrusted database backup" >&2
      cat "$events" >&2
      exit 1
    fi
    [ -f "$dbbackup" ] || {
      echo "tampered database backup evidence was unexpectedly removed" >&2
      exit 1
    }
  elif [ "$restore_exit" -ne 0 ]; then
    grep -q '^pg_restore:' "$events" || {
      echo "PostgreSQL restore was not attempted" >&2
      cat "$events" >&2
      exit 1
    }
    if grep -q '^systemctl:start ' "$events"; then
      echo "previous service was started after PostgreSQL restore failure" >&2
      cat "$events" >&2
      exit 1
    fi
    [ -f "$backup" ] || {
      echo "rollback binary was not retained after PostgreSQL restore failure" >&2
      exit 1
    }
    [ -f "$dbbackup" ] || {
      echo "database backup was not retained after PostgreSQL restore failure" >&2
      exit 1
    }
  else
    restore_line="$(grep -n '^pg_restore:' "$events" | head -n1 | cut -d: -f1)"
    verify_line="$(grep -n '^psql:' "$events" | head -n1 | cut -d: -f1)"
    start_line="$(grep -n '^systemctl:start ' "$events" | head -n1 | cut -d: -f1)"
    [ -n "$restore_line" ] && [ -n "$verify_line" ] && [ -n "$start_line" ] && \
      [ "$restore_line" -lt "$verify_line" ] && [ "$verify_line" -lt "$start_line" ] || {
      echo "database restore and schema verification did not complete before previous service start" >&2
      cat "$events" >&2
      exit 1
    }
    [ -f "$bin.failed" ] || {
      echo "failed candidate was not preserved" >&2
      exit 1
    }
    [ ! -f "$backup" ] || {
      echo "rollback binary was not promoted to active binary" >&2
      exit 1
    }
    [ ! -f "$dbbackup" ] || {
      echo "consumed database backup was not cleaned after successful rollback" >&2
      exit 1
    }
    [ ! -f "$handoff" ] || {
      echo "database handoff was not cleaned after successful rollback" >&2
      exit 1
    }
  fi

  rm -rf "$temp"
  trap - EXIT INT TERM
}

# Restore failure must leave the service stopped and retain all recovery assets.
run_case 1 3
# Successful restore and schema verification must precede previous binary start.
run_case 0 2
# A checksum mismatch must prevent the candidate from ever being started.
run_case 0 3 1

echo "upgrade finalizer PostgreSQL rollback tests passed"
