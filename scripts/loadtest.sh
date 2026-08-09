#!/usr/bin/env bash
set -euo pipefail

# Strata RMM HTTP load harness.
#
# This intentionally does NOT publish directly to tenant NATS subjects and does
# not synthesize enrollment requests with hard-coded tenant IDs. Agent-scale and
# reconnect testing must use enrolled agents or the dedicated resilience harness
# so Alpha evidence exercises the same authorization boundary as production.

API_URL="${API_URL:-https://localhost:8080}"
DURATION="${DURATION:-60s}"
RATE="${RATE:-100}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
TARGET_FILE="${TARGET_FILE:-}"
REPORT_DIR="${REPORT_DIR:-loadtest-evidence}"

usage() {
  cat <<'EOF'
Usage:
  scripts/loadtest.sh health
  scripts/loadtest.sh authenticated
  scripts/loadtest.sh targets

Environment:
  API_URL=https://rmm.example.com   Base platform URL
  DURATION=60s                      Vegeta duration
  RATE=100                          Requests per second
  REPORT_DIR=loadtest-evidence      Output directory

Authenticated mode additionally requires:
  AUTH_TOKEN=<short-lived bearer token>

Targets mode additionally requires:
  TARGET_FILE=<vegeta targets file>

The targets file must contain production-valid, authorized requests. Never put
long-lived credentials in the repository. Direct NATS publishing and hard-coded
tenant simulation were removed because they bypass the real Alpha trust model.
EOF
}

fail() { printf '[loadtest] ERROR: %s\n' "$*" >&2; exit 1; }
log() { printf '[loadtest] %s\n' "$*"; }

require_cmd() { command -v "$1" >/dev/null 2>&1 || fail "missing dependency: $1"; }

preflight() {
  require_cmd vegeta
  require_cmd date
  [[ "$API_URL" == https://* ]] || fail "API_URL must use https:// for Alpha load evidence"
  [[ "$RATE" =~ ^[0-9]+$ ]] || fail "RATE must be a positive integer"
  (( RATE > 0 )) || fail "RATE must be > 0"
  mkdir -p "$REPORT_DIR"
  umask 077
}

run_attack() {
  local name="$1"
  local targets="$2"
  local bin="$REPORT_DIR/${name}.bin"
  local txt="$REPORT_DIR/${name}.txt"
  local json="$REPORT_DIR/${name}.json"

  printf '%s' "$targets" | vegeta attack -duration="$DURATION" -rate="$RATE" > "$bin"
  vegeta report "$bin" | tee "$txt"
  vegeta report -type=json "$bin" > "$json"
  log "$name report written to $REPORT_DIR"
}

health() {
  preflight
  run_attack health "GET ${API_URL%/}/health/ready\n"
}

authenticated() {
  preflight
  [[ -n "$AUTH_TOKEN" ]] || fail "AUTH_TOKEN is required for authenticated mode"
  local target
  target=$(cat <<EOF
GET ${API_URL%/}/api/v1/devices
Authorization: Bearer ${AUTH_TOKEN}
EOF
)
  run_attack authenticated "$target"
}

targets() {
  preflight
  [[ -n "$TARGET_FILE" ]] || fail "TARGET_FILE is required for targets mode"
  [[ -f "$TARGET_FILE" ]] || fail "target file not found: $TARGET_FILE"
  local bin="$REPORT_DIR/targets.bin"
  vegeta attack -duration="$DURATION" -rate="$RATE" -targets="$TARGET_FILE" > "$bin"
  vegeta report "$bin" | tee "$REPORT_DIR/targets.txt"
  vegeta report -type=json "$bin" > "$REPORT_DIR/targets.json"
}

case "${1:-}" in
  health) health ;;
  authenticated) authenticated ;;
  targets) targets ;;
  -h|--help|help|'') usage ;;
  *) usage; fail "unknown mode: $1" ;;
esac
