#!/usr/bin/env bash
set -euo pipefail

# Evidence-first helper for hosted Alpha validation.
# This script never marks a Phase 8 gate accepted by itself. It records facts
# from a real environment and exits non-zero when required observations fail.

BASE_URL="${STRATA_ALPHA_URL:-}"
EVIDENCE_DIR="${STRATA_ALPHA_EVIDENCE_DIR:-alpha-evidence}"
SOAK_DURATION_SECONDS="${STRATA_ALPHA_SOAK_SECONDS:-86400}"
SOAK_INTERVAL_SECONDS="${STRATA_ALPHA_SOAK_INTERVAL_SECONDS:-60}"
ALLOW_DESTRUCTIVE="${STRATA_ALPHA_ALLOW_DESTRUCTIVE:-0}"

usage() {
  cat <<'EOF'
Usage:
  scripts/alpha-acceptance.sh preflight
  scripts/alpha-acceptance.sh snapshot
  scripts/alpha-acceptance.sh soak
  scripts/alpha-acceptance.sh fault <name>
  scripts/alpha-acceptance.sh finalize

Required:
  STRATA_ALPHA_URL=https://rmm.example.com

Optional:
  STRATA_ALPHA_EVIDENCE_DIR=alpha-evidence
  STRATA_ALPHA_SOAK_SECONDS=86400
  STRATA_ALPHA_SOAK_INTERVAL_SECONDS=60

Fault injection is disabled unless STRATA_ALPHA_ALLOW_DESTRUCTIVE=1 and a
matching hook is supplied, for example:
  STRATA_ALPHA_FAULT_POSTGRES_CMD='docker compose stop postgres'
  STRATA_ALPHA_RECOVER_POSTGRES_CMD='docker compose start postgres'

Supported fault names: postgres, nats, storage, orchestrator.
The hook commands are operator supplied because topology differs between Docker,
native, Kubernetes, and hosted environments.
EOF
}

log() { printf '[alpha] %s\n' "$*"; }
fail() { printf '[alpha] ERROR: %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

require_base_url() {
  [[ -n "$BASE_URL" ]] || fail "STRATA_ALPHA_URL is required"
  [[ "$BASE_URL" == https://* ]] || fail "STRATA_ALPHA_URL must use https:// for Alpha evidence"
  BASE_URL="${BASE_URL%/}"
}

utc_now() { date -u +'%Y-%m-%dT%H:%M:%SZ'; }

repo_sha() {
  git rev-parse HEAD 2>/dev/null || printf 'unknown\n'
}

safe_slug() {
  printf '%s' "$1" | tr -cs 'A-Za-z0-9._-' '_'
}

init_evidence() {
  mkdir -p "$EVIDENCE_DIR"
  umask 077
}

metadata_file() { printf '%s/metadata.env' "$EVIDENCE_DIR"; }

metadata_value() {
  local key="$1" file
  file="$(metadata_file)"
  [[ -f "$file" ]] || return 1
  awk -F= -v wanted="$key" '$1 == wanted {sub(/^[^=]*=/, ""); print; exit}' "$file"
}

ensure_metadata() {
  init_evidence
  local file sha existing_sha existing_url
  file="$(metadata_file)"
  sha="$(repo_sha)"
  [[ "$sha" != unknown ]] || fail "Alpha evidence must be collected from a git checkout"

  if [[ -f "$file" ]]; then
    existing_sha="$(metadata_value candidate_sha || true)"
    existing_url="$(metadata_value base_url || true)"
    [[ "$existing_sha" == "$sha" ]] || fail "evidence directory belongs to candidate $existing_sha, not $sha"
    [[ "$existing_url" == "$BASE_URL" ]] || fail "evidence directory belongs to $existing_url, not $BASE_URL"
    return
  fi

  {
    printf 'created_at=%s\n' "$(utc_now)"
    printf 'candidate_sha=%s\n' "$sha"
    printf 'base_url=%s\n' "$BASE_URL"
    printf 'hostname=%s\n' "$(hostname 2>/dev/null || printf unknown)"
    printf 'kernel=%s\n' "$(uname -sr 2>/dev/null || printf unknown)"
  } > "$file"
}

curl_probe() {
  local path="$1"
  local out="$2"
  local code
  code="$(curl --silent --show-error --location --connect-timeout 10 --max-time 30 \
    --output "$out" --write-out '%{http_code}' "$BASE_URL$path")" || return 1
  [[ "$code" =~ ^2[0-9][0-9]$ ]] || {
    printf '\nhttp_status=%s\n' "$code" >> "$out"
    return 1
  }
}

preflight() {
  require_base_url
  require_cmd curl
  require_cmd git
  ensure_metadata

  local failed=0
  for path in /health /health/live /health/ready; do
    local name
    name="$(safe_slug "$path")"
    if curl_probe "$path" "$EVIDENCE_DIR/preflight-${name}.txt"; then
      log "$path passed"
    else
      log "$path failed"
      failed=1
    fi
  done

  [[ "$failed" -eq 0 ]] || fail "preflight failed; inspect $EVIDENCE_DIR"
  printf 'preflight_at=%s\npreflight=pass\n' "$(utc_now)" > "$EVIDENCE_DIR/preflight-summary.env"
}

snapshot() {
  require_base_url
  require_cmd curl
  ensure_metadata
  local stamp
  stamp="$(date -u +'%Y%m%dT%H%M%SZ')"
  for path in /health /health/live /health/ready /metrics; do
    local name
    name="$(safe_slug "$path")"
    if ! curl_probe "$path" "$EVIDENCE_DIR/${stamp}-${name}.txt"; then
      log "snapshot warning: $path did not return 2xx"
    fi
  done
  log "snapshot stored in $EVIDENCE_DIR"
}

soak() {
  require_base_url
  require_cmd curl
  ensure_metadata
  [[ "$SOAK_DURATION_SECONDS" =~ ^[0-9]+$ ]] || fail "STRATA_ALPHA_SOAK_SECONDS must be an integer"
  [[ "$SOAK_INTERVAL_SECONDS" =~ ^[0-9]+$ ]] || fail "STRATA_ALPHA_SOAK_INTERVAL_SECONDS must be an integer"
  (( SOAK_DURATION_SECONDS > 0 )) || fail "soak duration must be > 0"
  (( SOAK_INTERVAL_SECONDS > 0 )) || fail "soak interval must be > 0"

  local start end samples failures=0
  start="$(date +%s)"
  end=$((start + SOAK_DURATION_SECONDS))
  samples=0
  printf 'timestamp,ready_http,total_time_seconds\n' > "$EVIDENCE_DIR/soak.csv"

  while (( $(date +%s) < end )); do
    local stamp metrics code total
    stamp="$(utc_now)"
    metrics="$(curl --silent --show-error --output /dev/null --connect-timeout 10 --max-time 30 \
      --write-out '%{http_code},%{time_total}' "$BASE_URL/health/ready" || printf '000,30')"
    code="${metrics%%,*}"
    total="${metrics#*,}"
    printf '%s,%s,%s\n' "$stamp" "$code" "$total" >> "$EVIDENCE_DIR/soak.csv"
    samples=$((samples + 1))
    if [[ ! "$code" =~ ^2[0-9][0-9]$ ]]; then failures=$((failures + 1)); fi
    sleep "$SOAK_INTERVAL_SECONDS"
  done

  {
    printf 'soak_started_epoch=%s\n' "$start"
    printf 'soak_finished_epoch=%s\n' "$(date +%s)"
    printf 'soak_requested_seconds=%s\n' "$SOAK_DURATION_SECONDS"
    printf 'soak_interval_seconds=%s\n' "$SOAK_INTERVAL_SECONDS"
    printf 'soak_samples=%s\n' "$samples"
    printf 'soak_failed_samples=%s\n' "$failures"
  } > "$EVIDENCE_DIR/soak-summary.env"

  [[ "$failures" -eq 0 ]] || fail "soak observed $failures failed readiness samples"
  log "soak completed with $samples successful samples"
}

hook_var() {
  local prefix="$1" name="$2"
  printf 'STRATA_ALPHA_%s_%s_CMD' "$prefix" "$(printf '%s' "$name" | tr '[:lower:]' '[:upper:]')"
}

run_hook() {
  local var="$1"
  local cmd="${!var:-}"
  [[ -n "$cmd" ]] || fail "required operator hook $var is not set"
  bash -c "$cmd"
}

wait_ready() {
  local expected="$1" deadline=$(( $(date +%s) + 120 ))
  while (( $(date +%s) < deadline )); do
    local code
    code="$(curl --silent --output /dev/null --connect-timeout 5 --max-time 10 --write-out '%{http_code}' "$BASE_URL/health/ready" || printf '000')"
    if [[ "$expected" == down && ! "$code" =~ ^2[0-9][0-9]$ ]]; then return 0; fi
    if [[ "$expected" == up && "$code" =~ ^2[0-9][0-9]$ ]]; then return 0; fi
    sleep 2
  done
  return 1
}

fault() {
  require_base_url
  require_cmd curl
  local name="${1:-}"
  case "$name" in postgres|nats|storage|orchestrator) ;; *) fail "unsupported fault: $name" ;; esac
  [[ "$ALLOW_DESTRUCTIVE" == 1 ]] || fail "fault injection requires STRATA_ALPHA_ALLOW_DESTRUCTIVE=1"
  ensure_metadata

  local fault_var recover_var started recovered
  fault_var="$(hook_var FAULT "$name")"
  recover_var="$(hook_var RECOVER "$name")"
  started="$(date +%s)"
  log "injecting $name fault using operator-supplied hook"
  run_hook "$fault_var"

  if ! wait_ready down; then
    run_hook "$recover_var" || true
    fail "readiness did not degrade after $name fault"
  fi

  run_hook "$recover_var"
  if ! wait_ready up; then
    fail "readiness did not recover within 120 seconds after $name recovery"
  fi
  recovered="$(date +%s)"

  {
    printf 'fault=%s\n' "$name"
    printf 'fault_started_epoch=%s\n' "$started"
    printf 'fault_recovered_epoch=%s\n' "$recovered"
    printf 'recovery_seconds=%s\n' "$((recovered - started))"
  } > "$EVIDENCE_DIR/fault-${name}.env"
  log "$name recovery verified in $((recovered - started)) seconds"
}

finalize() {
  require_base_url
  ensure_metadata
  local sha
  sha="$(repo_sha)"
  [[ -f "$EVIDENCE_DIR/soak.csv" ]] || log "warning: no soak.csv present"

  cat > "$EVIDENCE_DIR/README.txt" <<EOF
Strata RMM Alpha evidence bundle
Candidate SHA: $sha
Base URL: $BASE_URL
Generated: $(utc_now)

This bundle records observations only. It does not automatically mark any
Phase 8 acceptance gate as accepted. Review the files, correlate them with the
hosted lifecycle, CI runs, recovery drill, security review, and signed go/no-go.
EOF

  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$EVIDENCE_DIR" && find . -maxdepth 1 -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS)
  fi
  log "evidence bundle finalized at $EVIDENCE_DIR"
}

main() {
  case "${1:-}" in
    preflight) preflight ;;
    snapshot) snapshot ;;
    soak) soak ;;
    fault) shift; fault "${1:-}" ;;
    finalize) finalize ;;
    -h|--help|help|'') usage ;;
    *) usage; fail "unknown command: $1" ;;
  esac
}

main "$@"
