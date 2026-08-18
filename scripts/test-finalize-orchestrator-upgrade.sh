#!/bin/sh
set -eu

FINALIZER="${1:-scripts/finalize-orchestrator-upgrade.sh}"

run_case() {
  rollback_exit="$1"
  expect_exit="$2"
  temp="$(mktemp -d)"
  trap 'rm -rf "$temp"' EXIT INT TERM

  bin="$temp/strata-orchestrator"
  backup="$bin.bak"
  envfile="$temp/orchestrator.env"
  fakebin="$temp/fakebin"
  events="$temp/events"
  healthy="$temp/healthy"
  mkdir -p "$fakebin"
  : > "$events"

  cat > "$bin" <<EOF
#!/bin/sh
printf '%s\n' "candidate:\$*" >> "$events"
if [ "\${1:-}" = orchestrator ] && [ "\${2:-}" = rollback ]; then
  exit $rollback_exit
fi
exit 0
EOF
  chmod +x "$bin"

  cat > "$backup" <<EOF
#!/bin/sh
printf '%s\n' "previous:\$*" >> "$events"
exit 0
EOF
  chmod +x "$backup"

  # No credential-shaped literals belong in source-controlled fixtures.
  # The finalizer only needs to prove protected environment loading here.
  cat > "$envfile" <<'EOF'
STRATA_TEST_DATABASE_ENV_LOADED=1
EOF
  chmod 600 "$envfile"

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

  set +e
  PATH="$fakebin:$PATH" \
  STRATA_UPGRADE_ENV_FILE="$envfile" \
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

  grep -q '^candidate:orchestrator rollback 92$' "$events" || {
    echo "schema rollback command was not executed" >&2
    cat "$events" >&2
    exit 1
  }

  if [ "$rollback_exit" -ne 0 ]; then
    if grep -q '^systemctl:start ' "$events"; then
      echo "previous service was started after schema rollback failure" >&2
      cat "$events" >&2
      exit 1
    fi
    [ -f "$backup" ] || {
      echo "rollback binary was not retained after schema rollback failure" >&2
      exit 1
    }
  else
    rollback_line="$(grep -n '^candidate:orchestrator rollback 92$' "$events" | head -n1 | cut -d: -f1)"
    start_line="$(grep -n '^systemctl:start ' "$events" | head -n1 | cut -d: -f1)"
    [ -n "$start_line" ] && [ "$rollback_line" -lt "$start_line" ] || {
      echo "previous service start did not occur after schema rollback" >&2
      cat "$events" >&2
      exit 1
    }
    [ -f "$bin.failed" ] || {
      echo "failed candidate was not preserved" >&2
      exit 1
    }
    [ ! -f "$backup" ] || {
      echo "backup was not promoted to active binary" >&2
      exit 1
    }
  fi

  rm -rf "$temp"
  trap - EXIT INT TERM
}

run_case 1 3
run_case 0 2

echo "upgrade finalizer schema rollback tests passed"
