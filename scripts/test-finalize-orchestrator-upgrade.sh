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
  updatedir="$temp/updates"
  staged="$updatedir/strata-rmm-next"
  dbdir="$temp/db-backups"
  dbbackup="$dbdir/pre-upgrade.dump"
  handoff="$updatedir/database-backup.env"
  fakebin="$temp/fakebin"
  events="$temp/events"
  healthy="$temp/healthy"
  start_count="$temp/start-count"
  mkdir -p "$fakebin" "$dbdir" "$updatedir"
  : > "$events"
  printf '%s\n' 0 > "$start_count"

  cat > "$bin" <<EOF
#!/bin/sh
printf '%s\n' "previous:\$*" >> "$events"
exit 0
EOF
  chmod +x "$bin"

  cat > "$staged" <<EOF
#!/bin/sh
printf '%s\n' "candidate:\$*" >> "$events"
exit 0
EOF
  chmod +x "$staged"

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
PGPASSWORD='fixture-value'
PGDATABASE='strata_upgrade_test'
PGSSLMODE='disable'
EOF
  chmod 600 "$handoff"

  cat > "$fakebin/systemctl" <<EOF
#!/bin/sh
printf '%s\n' "systemctl:\$*" >> "$events"
case "\${1:-}" in
  stop) exit 0 ;;
  start)
    count="\$(cat "$start_count")"
    count=\$((count + 1))
    printf '%s\n' "\$count" > "$start_count"
    if [ "\$count" -ge 2 ]; then
      : > "$healthy"
    fi
    exit 0
    ;;
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
  STRATA_UPGRADE_UPDATE_DIR="$updatedir" \
  STRATA_UPGRADE_DB_BACKUP_DIR="$dbdir" \
  STRATA_UPGRADE_DB_HANDOFF="$handoff" \
  STRATA_UPGRADE_HEALTH_ATTEMPTS=1 \
  STRATA_UPGRADE_HEALTH_INTERVAL=0 \
    sh "$FINALIZER" "$bin" "$staged" 92 93 >/dev/null 2>&1
  rc=$?
  set -e

  if [ "$rc" -ne "$expect_exit" ]; then
    echo "unexpected finalizer exit: got $rc want $expect_exit" >&2
    cat "$events" >&2
    exit 1
  fi

  if [ "$tamper" -ne 0 ]; then
    if grep -Eq '^systemctl:(stop|start) ' "$events"; then
      echo "service was mutated with an untrusted database backup" >&2
      cat "$events" >&2
      exit 1
    fi
    [ -x "$bin" ] || {
      echo "known-good active binary changed before recovery verification" >&2
      exit 1
    }
    [ -x "$staged" ] || {
      echo "staged candidate changed before recovery verification" >&2
      exit 1
    }
    [ -f "$dbbackup" ] || {
      echo "tampered database backup evidence was unexpectedly removed" >&2
      exit 1
    }
  elif [ "$restore_exit" -ne 0 ]; then
    grep -q '^systemctl:stop ' "$events" || {
      echo "current service was not stopped before binary mutation" >&2
      cat "$events" >&2
      exit 1
    }
    grep -q '^systemctl:start ' "$events" || {
      echo "candidate service was not started" >&2
      cat "$events" >&2
      exit 1
    }
    grep -q '^pg_restore:' "$events" || {
      echo "PostgreSQL restore was not attempted" >&2
      cat "$events" >&2
      exit 1
    }
    [ "$(grep -c '^systemctl:start ' "$events")" -eq 1 ] || {
      echo "previous service was started after PostgreSQL restore failure" >&2
      cat "$events" >&2
      exit 1
    }
    [ -f "$bin.bak" ] || {
      echo "rollback binary was not retained after PostgreSQL restore failure" >&2
      exit 1
    }
    [ -f "$dbbackup" ] || {
      echo "database backup was not retained after PostgreSQL restore failure" >&2
      exit 1
    }
  else
    stop_line="$(grep -n '^systemctl:stop ' "$events" | head -n1 | cut -d: -f1)"
    first_start_line="$(grep -n '^systemctl:start ' "$events" | head -n1 | cut -d: -f1)"
    restore_line="$(grep -n '^pg_restore:' "$events" | head -n1 | cut -d: -f1)"
    verify_line="$(grep -n '^psql:' "$events" | head -n1 | cut -d: -f1)"
    second_start_line="$(grep -n '^systemctl:start ' "$events" | tail -n1 | cut -d: -f1)"
    [ -n "$stop_line" ] && [ -n "$first_start_line" ] && [ -n "$restore_line" ] && \
      [ -n "$verify_line" ] && [ -n "$second_start_line" ] && \
      [ "$stop_line" -lt "$first_start_line" ] && \
      [ "$first_start_line" -lt "$restore_line" ] && \
      [ "$restore_line" -lt "$verify_line" ] && \
      [ "$verify_line" -lt "$second_start_line" ] || {
      echo "finalizer stop/swap/start/restore ordering was violated" >&2
      cat "$events" >&2
      exit 1
    }
    [ "$(grep -c '^systemctl:start ' "$events")" -eq 2 ] || {
      echo "expected candidate and rollback service starts" >&2
      cat "$events" >&2
      exit 1
    }
    [ -f "$bin.failed" ] || {
      echo "failed candidate was not preserved" >&2
      exit 1
    }
    [ -x "$bin" ] || {
      echo "rollback binary was not promoted to active binary" >&2
      exit 1
    }
    [ ! -f "$bin.bak" ] || {
      echo "rollback backup remained after successful promotion" >&2
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
# A checksum mismatch must prevent any service or binary mutation.
run_case 0 3 1

echo "upgrade finalizer PostgreSQL rollback tests passed"
