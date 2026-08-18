#!/usr/bin/env bash
set -Eeuo pipefail
umask 022

COSIGN_VERSION="v3.1.3"
COSIGN_AMD64_SHA256="4629c757b7618056f8ddd7e2625ae9fdd94c0372a65049520bc7d9df9efc7f71"
COSIGN_ARM64_SHA256="c5d324e091826b0d7a78eb16fef316450b4eb9aaec045611c08ba06f5e73220a"
INSTALL_PATH="${COSIGN_INSTALL_PATH:-/usr/local/bin/cosign}"

fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || fail "curl is required to install the release verifier"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required to install the release verifier"

case "$(uname -s)" in
  Linux) ;;
  *) fail "automatic Cosign verifier installation currently supports Linux only" ;;
esac

case "$(uname -m)" in
  x86_64|amd64)
    asset="cosign-linux-amd64"
    expected="$COSIGN_AMD64_SHA256"
    ;;
  aarch64|arm64)
    asset="cosign-linux-arm64"
    expected="$COSIGN_ARM64_SHA256"
    ;;
  *) fail "unsupported architecture for the pinned Cosign verifier: $(uname -m)" ;;
esac

if command -v cosign >/dev/null 2>&1; then
  current="$(cosign version 2>/dev/null | awk '/GitVersion/ {print $2; exit}' | tr -d '"')"
  if [[ "$current" == "$COSIGN_VERSION" || "$current" == "${COSIGN_VERSION#v}" ]]; then
    exit 0
  fi
fi

[[ $EUID -eq 0 ]] || fail "Cosign verifier installation must run as root"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
url="https://github.com/sigstore/cosign/releases/download/${COSIGN_VERSION}/${asset}"
curl --fail --show-error --silent --location --retry 3 --proto '=https' --tlsv1.2 "$url" -o "$tmp"
printf '%s  %s\n' "$expected" "$tmp" | sha256sum --check --status || fail "Cosign verifier checksum verification failed"
install -o root -g root -m 0755 "$tmp" "$INSTALL_PATH"
"$INSTALL_PATH" version >/dev/null
printf 'Installed pinned Cosign verifier %s at %s\n' "$COSIGN_VERSION" "$INSTALL_PATH"
