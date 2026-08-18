#!/bin/sh
set -eu

VERIFIER=/usr/local/bin/cosign
FINALIZER=/usr/lib/strata-rmm/finalize-orchestrator-upgrade.sh
PROVENANCE=/usr/share/strata-rmm/cosign-verifier-provenance.txt

if [ ! -x "$VERIFIER" ]; then
  echo "ERROR: packaged Cosign verifier is missing or not executable" >&2
  exit 1
fi
if [ ! -x "$FINALIZER" ]; then
  echo "ERROR: packaged upgrade finalizer is missing or not executable" >&2
  exit 1
fi
if [ ! -r "$PROVENANCE" ]; then
  echo "ERROR: packaged Cosign verifier provenance is missing" >&2
  exit 1
fi

"$VERIFIER" version >/dev/null

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
fi
