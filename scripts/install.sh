#!/bin/bash
set -euo pipefail

BINARY_NAME="strata-rmm"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/strata-rmm"
DATA_DIR="/var/lib/strata-rmm"
SERVICE_NAME="strata-rmm-agent"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

require_root() {
	if [ "$(id -u)" -ne 0 ]; then
		log_error "This script must be run as root. Use sudo."
		exit 1
	fi
}

detect_platform() {
	OS=$(uname -s | tr '[:upper:]' '[:lower:]')
	ARCH=$(uname -m)
	case "$ARCH" in
		x86_64) ARCH="amd64" ;;
		aarch64) ARCH="arm64" ;;
		*) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
	esac
	log_info "Detected: $OS/$ARCH"
}

install_binary() {
	if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
		log_warn "$BINARY_NAME already installed at $INSTALL_DIR/$BINARY_NAME"
		return 0
	fi

	if [ ! -f "./bin/$BINARY_NAME" ]; then
		log_warn "Binary not found at ./bin/$BINARY_NAME"
		log_info "Building from source..."
		go build -o "./bin/$BINARY_NAME" .
	fi

	cp "./bin/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
	chmod 755 "$INSTALL_DIR/$BINARY_NAME"
	log_info "Installed binary to $INSTALL_DIR/$BINARY_NAME"
}

setup_directories() {
	mkdir -p "$CONFIG_DIR" "$DATA_DIR"
	chmod 755 "$CONFIG_DIR"
	chmod 700 "$DATA_DIR"
	log_info "Created directories: $CONFIG_DIR, $DATA_DIR"
}

install_systemd_service() {
	if [ -f "/etc/systemd/system/$SERVICE_NAME.service" ]; then
		log_warn "Systemd service already exists"
		return 0
	fi

	cp "deploy/$SERVICE_NAME.service" "/etc/systemd/system/$SERVICE_NAME.service"
	chmod 644 "/etc/systemd/system/$SERVICE_NAME.service"

	systemctl daemon-reload
	systemctl enable "$SERVICE_NAME"

	log_info "Installed systemd service: $SERVICE_NAME"
	log_info "Start with: systemctl start $SERVICE_NAME"
}

uninstall() {
	log_info "Stopping service..."
	systemctl stop "$SERVICE_NAME" 2>/dev/null || true
	systemctl disable "$SERVICE_NAME" 2>/dev/null || true
	rm -f "/etc/systemd/system/$SERVICE_NAME.service"
	rm -f "$INSTALL_DIR/$BINARY_NAME"
	systemctl daemon-reload
	log_info "Uninstalled $BINARY_NAME"

	read -p "Remove data directory $DATA_DIR? [y/N] " -n 1 -r
	echo
	if [[ $REPLY =~ ^[Yy]$ ]]; then
		rm -rf "$DATA_DIR"
		log_info "Removed data directory"
	fi
}

print_usage() {
	echo "Usage: $0 [command]"
	echo ""
	echo "Commands:"
	echo "  install     Install agent binary and systemd service"
	echo "  uninstall   Remove agent and systemd service"
	echo "  status      Show service status"
	echo "  help        Show this help"
}

case "${1:-install}" in
	install)
		require_root
		detect_platform
		install_binary
		setup_directories
		install_systemd_service
		log_info ""
		log_info "Installation complete!"
		log_info "1. Edit config: $CONFIG_DIR/agent.yaml"
		log_info "2. Start agent: systemctl start $SERVICE_NAME"
		log_info "3. Check logs: journalctl -u $SERVICE_NAME -f"
		;;
	uninstall)
		require_root
		uninstall
		log_info "Uninstall complete"
		;;
	status)
		if command -v systemctl &>/dev/null; then
			systemctl status "$SERVICE_NAME" 2>/dev/null || echo "Service not installed"
		fi
		;;
	help|--help|-h)
		print_usage
		;;
	*)
		log_error "Unknown command: $1"
		print_usage
		exit 1
		;;
esac
