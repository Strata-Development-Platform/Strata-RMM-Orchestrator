#!/bin/sh
set -eu

COSIGN_VERSION="v3.1.3"
COSIGN_COMMIT="11926fa5bbbbde47e88fc006b625a17769b743b2"
X_TEXT_VERSION="v0.39.0"
X_MOD_VERSION="v0.40.0"
GRPC_VERSION="v1.82.1"
OUT_DIR="${1:-build/cosign-verifier}"

# Resolve the output path before changing into the disposable source checkout.
# $OLDPWD changes when `cd "$repo"` runs and is therefore not a safe anchor for
# an absolute /tmp output: it can redirect the artifact into the temporary tree
# that the cleanup trap deletes.
case "$OUT_DIR" in
  /*) OUTPUT_DIR="$OUT_DIR" ;;
  *) OUTPUT_DIR="$(pwd)/$OUT_DIR" ;;
esac
OUT_PATH="${OUTPUT_DIR}/cosign"

case "$(go env GOVERSION)" in
  go1.26.6|go1.26.[7-9]*|go1.2[7-9].*) ;;
  *)
    echo "ERROR: patched Cosign verifier requires Go 1.26.6 or newer; found $(go env GOVERSION)" >&2
    exit 1
    ;;
esac

workdir="$(mktemp -d)"
cleanup() {
  rm -rf "$workdir"
}
trap cleanup EXIT INT TERM

repo="$workdir/cosign"
git init -q "$repo"
cd "$repo"
git remote add origin https://github.com/sigstore/cosign.git
git fetch -q --depth 1 origin "$COSIGN_COMMIT"
git checkout -q --detach FETCH_HEAD

actual_commit="$(git rev-parse HEAD)"
if [ "$actual_commit" != "$COSIGN_COMMIT" ]; then
  echo "ERROR: Cosign source commit mismatch: expected $COSIGN_COMMIT, got $actual_commit" >&2
  exit 1
fi

# Preserve upstream behavior while overriding only dependencies with fixes
# required by the current HIGH-severity vulnerability gate. The source tree is
# disposable, so tidy the patched module before compiling to ensure go.sum
# covers the complete transitive graph selected by these overrides.
go mod edit \
  -require="golang.org/x/text@${X_TEXT_VERSION}" \
  -require="golang.org/x/mod@${X_MOD_VERSION}" \
  -require="google.golang.org/grpc@${GRPC_VERSION}"
go mod tidy

mkdir -p "$OUTPUT_DIR"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$OUT_PATH" ./cmd/cosign

"$OUT_PATH" version >&2
printf '%s\n' "$COSIGN_VERSION source=$COSIGN_COMMIT x/text=$X_TEXT_VERSION x/mod=$X_MOD_VERSION grpc=$GRPC_VERSION" > "$OUTPUT_DIR/BUILD-PROVENANCE.txt"
