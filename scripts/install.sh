#!/bin/sh
set -eu

usage() {
  cat <<'EOF'
Strata RMM Linux Agent Installer

Usage:
  sudo ./scripts/install.sh --server https://rmm.example.com --token <enrollment-token>

Options:
  --server URL   Strata RMM server URL. HTTPS is required outside localhost.
  --token TOKEN  One-time scoped enrollment token. If omitted on a TTY, you will be prompted securely.
  --help         Show this help.

The server remains the source of truth for the current agent installer. This wrapper
fetches /install.sh from the selected Strata server and executes that version with the
provided server URL and enrollment token.
EOF
}

SERVER_URL="${RMM_SERVER_URL:-}"
ENROLLMENT_TOKEN="${RMM_ENROLLMENT_TOKEN:-}"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --server)
      [ "$#" -ge 2 ] || { echo "--server requires a value" >&2; exit 2; }
      SERVER_URL="$2"
      shift 2
      ;;
    --token)
      [ "$#" -ge 2 ] || { echo "--token requires a value" >&2; exit 2; }
      ENROLLMENT_TOKEN="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[ "$(id -u)" -eq 0 ] || { echo "Run this installer as root (sudo)." >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "curl is required." >&2; exit 1; }

if [ -z "$SERVER_URL" ]; then
  if [ -t 0 ]; then
    printf 'Strata RMM server URL: ' >&2
    IFS= read -r SERVER_URL
  else
    echo "Set --server or RMM_SERVER_URL for non-interactive installation." >&2
    exit 1
  fi
fi

case "$SERVER_URL" in
  https://*) CURL_PROTO="=https" ;;
  http://localhost:*|http://127.0.0.1:*) CURL_PROTO="=http,https" ;;
  *) echo "The server URL must use HTTPS outside local development." >&2; exit 1 ;;
esac

SERVER_URL="${SERVER_URL%/}"

if [ -z "$ENROLLMENT_TOKEN" ]; then
  if [ -t 0 ]; then
    printf 'Enrollment token: ' >&2
    stty -echo
    trap 'stty echo 2>/dev/null || true' EXIT HUP INT TERM
    IFS= read -r ENROLLMENT_TOKEN
    stty echo
    trap - EXIT HUP INT TERM
    printf '\n' >&2
  else
    echo "Set --token or RMM_ENROLLMENT_TOKEN for non-interactive installation." >&2
    exit 1
  fi
fi

[ -n "$ENROLLMENT_TOKEN" ] || { echo "Enrollment token is required." >&2; exit 1; }

TMP="$(mktemp)"
trap 'rm -f "$TMP"; stty echo 2>/dev/null || true' EXIT HUP INT TERM

printf '==> Fetching the current installer from %s\n' "$SERVER_URL"
curl --fail --silent --show-error --location --proto "$CURL_PROTO" --tlsv1.2 \
  --connect-timeout 10 --max-time 60 --retry 3 --retry-delay 2 --retry-all-errors \
  --output "$TMP" "$SERVER_URL/install.sh"

[ -s "$TMP" ] || { echo "Server returned an empty installer." >&2; exit 1; }

printf '==> Installing and enrolling the Strata RMM agent\n'
RMM_SERVER_URL="$SERVER_URL" RMM_ENROLLMENT_TOKEN="$ENROLLMENT_TOKEN" sh "$TMP"
unset ENROLLMENT_TOKEN RMM_ENROLLMENT_TOKEN

printf '==> Installation completed without an automatic reboot.\n'
