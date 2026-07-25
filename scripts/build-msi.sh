#!/bin/bash
# Build Windows MSI installer for Strata RMM Agent
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-$PROJECT_DIR/dist}"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")}"
BINARY="${BINARY:-$PROJECT_DIR/bin/strata-rmm.exe}"
CONFIG="${CONFIG:-$SCRIPT_DIR/wix/agent-config.example.yaml}"
UPGRADE_CODE="${UPGRADE_CODE:-A1B2C3D4-E5F6-7890-ABCD-EF1234567890}"

WIX_DIR="${WIX_DIR:-/c/Program Files (x86)/WiX Toolset v3.11/bin}"
CANDLE="${CANDLE:-$WIX_DIR/candle.exe}"
LIGHT="${LIGHT:-$WIX_DIR/light.exe}"
HEAT="${HEAT:-$WIX_DIR/heat.exe}"

AGENT_BINARY_GUID="${AGENT_BINARY_GUID:-$(uuidgen 2>/dev/null || python3 -c 'import uuid; print(str(uuid.uuid4()).upper())')}"
AGENT_CONFIG_GUID="${AGENT_CONFIG_GUID:-$(uuidgen 2>/dev/null || python3 -c 'import uuid; print(str(uuid.uuid4()).upper())')}"
UNINSTALL_SHORTCUT_GUID="${UNINSTALL_SHORTCUT_GUID:-$(uuidgen 2>/dev/null || python3 -c 'import uuid; print(str(uuid.uuid4()).upper())')}"
START_MENU_GUID="${START_MENU_GUID:-$(uuidgen 2>/dev/null || python3 -c 'import uuid; print(str(uuid.uuid4()).upper())')}"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

mkdir -p "$OUTPUT_DIR"

# Build the Windows binary if not present
if [ ! -f "$BINARY" ]; then
  info "Building Windows agent binary..."
  GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.version=$VERSION" -o "$BINARY" "$PROJECT_DIR"
fi

# Check for WiX toolset
if ! command -v "$CANDLE" &>/dev/null && ! command -v candle.exe &>/dev/null; then
  error "WiX Toolset not found. Install from https://wixtoolset.org or set WIX_DIR"
fi

CANDLE="${CANDLE:-candle.exe}"
LIGHT="${LIGHT:-light.exe}"

# Compile WiX source to .wixobj
info "Compiling WiX source..."
"$CANDLE" -dVersion="$VERSION" \
  -dUpgradeCode="$UPGRADE_CODE" \
  -dBinaryPath="$BINARY" \
  -dConfigPath="$CONFIG" \
  -dAgentBinaryGuid="$AGENT_BINARY_GUID" \
  -dAgentConfigGuid="$AGENT_CONFIG_GUID" \
  -dUninstallShortcutGuid="$UNINSTALL_SHORTCUT_GUID" \
  -dStartMenuGuid="$START_MENU_GUID" \
  -arch x64 \
  -out "$OUTPUT_DIR/strata-agent.wixobj" \
  "$SCRIPT_DIR/wix/strata-agent.wxs"

# Link to MSI
info "Linking MSI package..."
"$LIGHT" -out "$OUTPUT_DIR/strata-agent-$VERSION.msi" \
  -ext WixUIExtension \
  -cultures:en-US \
  "$OUTPUT_DIR/strata-agent.wixobj"

info "MSI installer created: $OUTPUT_DIR/strata-agent-$VERSION.msi"
ls -lh "$OUTPUT_DIR/strata-agent-$VERSION.msi"
