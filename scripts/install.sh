#!/bin/bash
set -euo pipefail

BINARY_NAME="strata-rmm"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/strata-rmm"
DATA_DIR="/var/lib/strata-rmm"
SERVICE_NAME="strata-rmm-agent"
DEPLOYMENT_ID=""

RELEASE_URL="${RELEASE_URL:-https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/latest/download}"
VERSION="${VERSION:-latest}"

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --deployment-id) DEPLOYMENT_ID="$2"; shift 2 ;;
    --tenant-id) TENANT_ID="$2"; shift 2 ;;
    --nats-url) NATS_URL="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --help|-h) echo "Usage: install.sh [--deployment-id ID] [--tenant-id ID] [--nats-url URL] [--version X.Y.Z]"; exit 0 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[1;34m'
BOLD='\033[1m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}*${NC} $1"; }
log_warn()  { echo -e "${YELLOW}*${NC} $1"; }
log_error() { echo -e "${RED}*${NC} $1"; }
log_step()  { echo -e "\n${BLUE}==>${NC} $2"; }


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

	BINARY_URL="$RELEASE_URL/strata-agent-linux-$ARCH"
	if [ "$VERSION" != "latest" ]; then
		BINARY_URL="https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/releases/download/v$VERSION/strata-agent-linux-$ARCH"
	fi

	log_info "Downloading agent from $BINARY_URL..."
	if command -v curl &>/dev/null; then
		curl -sL -o "$INSTALL_DIR/$BINARY_NAME" "$BINARY_URL" || {
			log_warn "Download failed, falling back to local build"
			go build -o "/tmp/$BINARY_NAME" . && cp "/tmp/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
		}
	elif command -v wget &>/dev/null; then
		wget -q -O "$INSTALL_DIR/$BINARY_NAME" "$BINARY_URL" || {
			log_warn "Download failed, falling back to local build"
			go build -o "/tmp/$BINARY_NAME" . && cp "/tmp/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
		}
	else
		log_info "No download tool found, building from source..."
		go build -o "$INSTALL_DIR/$BINARY_NAME" .
	fi

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

	ARGS="agent"
	if [ -n "$DEPLOYMENT_ID" ]; then
		ARGS="$ARGS --deployment-id $DEPLOYMENT_ID"
	fi
	if [ -n "${NATS_URL:-}" ]; then
		ARGS="$ARGS --nats-url $NATS_URL"
	fi

	sed "s|ExecStart=.*|ExecStart=$INSTALL_DIR/$BINARY_NAME $ARGS|" \
		"deploy/$SERVICE_NAME.service" > "/etc/systemd/system/$SERVICE_NAME.service"
	chmod 644 "/etc/systemd/system/$SERVICE_NAME.service"

	systemctl daemon-reload
	systemctl enable "$SERVICE_NAME"

	log_info "Installed systemd service: $SERVICE_NAME"
	log_info "Deployment ID: ${DEPLOYMENT_ID:-not set}"
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
		clear
		echo -e "${BLUE}════════════════════════════════════════════${NC}"
		echo -e "${BLUE}  Strata RMM Agent Installer${NC}"
		echo -e "${BLUE}════════════════════════════════════════════${NC}"
		echo ""

		require_root
		detect_platform

		if [ -z "$DEPLOYMENT_ID" ] && [ -z "${TENANT_ID:-}" ]; then
			echo ""
			read -p "  Enter deployment ID (from your Strata RMM console): " DEPLOYMENT_ID
			if [ -z "$DEPLOYMENT_ID" ]; then
				log_error "Deployment ID is required. Get it from your Strata RMM admin console."
				exit 1
			fi
		fi

		if [ -z "${NATS_URL:-}" ]; then
			read -p "  NATS server address [localhost:4222]: " NATS_URL
			NATS_URL="${NATS_URL:-localhost:4222}"
		fi

		echo ""
		log_step "1/4" "Downloading agent binary"
		install_binary

		log_step "2/4" "Setting up directories"
		setup_directories

		log_step "3/4" "Installing systemd service"
		install_systemd_service

		log_step "4/4" "Starting agent"
		systemctl start "$SERVICE_NAME" 2>/dev/null || true
		sleep 2
		if systemctl is-active --quiet "$SERVICE_NAME"; then
			log_info "Agent is running"
		else
			log_warn "Agent may not have started. Check: journalctl -u $SERVICE_NAME -f"
		fi

		echo ""
		echo -e "${GREEN}════════════════════════════════════════════${NC}"
		echo -e "${GREEN}  Agent Installation Complete!${NC}"
		echo -e "${GREEN}════════════════════════════════════════════${NC}"
		echo ""
		echo -e "  Deployment ID: ${DEPLOYMENT_ID:-not set}"
		echo -e "  NATS server:   ${NATS_URL:-localhost:4222}"
		echo -e "  Service:       $SERVICE_NAME"
		echo -e "  Logs:          journalctl -u $SERVICE_NAME -f"
		echo ""

		read -p "  ${YELLOW}Reboot now to complete installation? [Y/n]: ${NC}" REBOOT
		REBOOT="${REBOOT:-Y}"
		if [[ "$REBOOT" =~ ^[Yy]$ ]]; then
			log_info "Rebooting in 3 seconds..."
			sleep 3
			reboot
		fi
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
