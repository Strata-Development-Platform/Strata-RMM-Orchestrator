# Feature Specification

## Overview

Strata RMM is a horizontally-scalable, multi-tenant Remote Monitoring & Management platform with cross-platform Go agents. It supports both SaaS and self-hosted deployments and is designed to match the capabilities of Kaseya VSA, Datto RMM, and NinjaRMM.

The platform is built on a polyglot microservices architecture with NATS JetStream for messaging, TimescaleDB for metrics, PostgreSQL for relational data, and a React/Tailwind CSS web UI.

---

## Device Management

### Purpose
Maintain a real-time inventory of all managed endpoints across the deployment hierarchy (Platform -> MSP -> Client -> Site -> Device).

### Key Capabilities
- Automatic device registration on agent first check-in
- Device CRUD with hardware/software inventory
- Status tracking (online, offline, pending, archived)
- Grouping by client, site, and custom tags
- Software inventory snapshots from dpkg/Win32_Product
- CMDB relationship mapping for dependency discovery
- Device-level configuration (alert overrides, maintenance windows)

### Agent Identity
- ECDSA P256 keypair generated on first run
- Deployment ID ties agent to customer on registration
- JWT authentication for NATS communications
- Identity persisted to disk, survives restarts

---

## Monitoring

### Purpose
Collect, store, and visualize system metrics from managed devices to detect issues and track performance trends.

### Key Capabilities
- **System Metrics**: CPU, memory, disk, disk I/O, network, system info via gopsutil
- **Collection Modes**: Agent-based (high-fidelity), agentless (SNMP/ICMP/API)
- **Check Types**:
  - System: CPU, RAM, Disk, Net, Processes, Services
  - Network: ICMP, SNMP (v2c/v3), NetFlow/sFlow, IPMI/Redfish
  - Application: HTTP, TCP, DNS, custom scripts
  - IP Phones: SIP registration, RTP quality
- **TimescaleDB Storage**: Hypertables with continuous aggregates (1m, 1h downsampling), compression after 7 days
- **Retention**: Hot (7d, 1s resolution) -> Warm (90d, 1m) -> Cold (7y, 1h)
- **Grafana Dashboards**: Pre-provisioned for system metrics and alerts

---

## Alerting

### Purpose
Detect and notify on anomalous conditions, device health changes, and security findings.

### Key Capabilities
- **Rule Types**:
  - Threshold: GT/GTE/LT/LTE/EQ/NEQ on metric values
  - Heartbeat: Fires when device has not reported within N minutes
  - CVE: Auto-fires when critical/high CVEs detected
- **Alert State Machine**: OK -> Firing -> Resolved (with cooldown to prevent re-fire)
- **Severity Levels**: critical, warning, info
- **Notification Channels**: Slack, Webhook, Email (SMTP), Microsoft Teams, PagerDuty
- **Deduplication**: Correlation, grouping, auto-resolution
- **Silencing**: Maintenance windows, cooldown periods
- **CVE Alerts**: Auto-resolution when device patches the affected package

---

## Policy Engine

### Purpose
Centrally define and enforce configuration across the deployment hierarchy using an inheritance-based policy model.

### Key Capabilities
- **Policy Categories**: Patch management, alerting rules, monitoring thresholds, software deployment, script execution, maintenance windows
- **Inheritance Model**: Platform level -> MSP level -> Client level -> Site level -> Device level (most specific wins)
- **Policy Lifecycle**: Draft -> Validation -> Preview -> Publish
- **Effective Configuration**: Computed display showing merged result at each level
- **Revision History**: Full version tracking with audit trail
- **Enforcement**: Automatic application on publish, scheduled re-evaluation

---

## Scripts/Automation

### Purpose
Execute remediation, maintenance, and automation scripts on managed devices on-demand or on a schedule.

### Key Capabilities
- **Supported Languages**: PowerShell, Bash, Python, Batch
- **Execution Methods**: NATS-dispatched commands to agents
- **Parameter Interpolation**: `{{param_name}}` syntax in script content
- **Timeout Enforcement**: Configurable per-script (default 300s)
- **Output Capture**: stdout + stderr, truncated at 100KB
- **Execution History**: Full output viewer in Web UI
- **Multi-Device Targeting**: Run on selected devices with per-device status
- **Script Library**: Saved scripts with name, description, language, parameters
- **Schedule Integration**: Recurring scripts via the job system

---

## Patch Management

### Purpose
Automate the discovery, approval, and deployment of OS and third-party patches across managed devices.

### Key Capabilities
- **Windows**: Windows Update API, Chocolatey, Winget, WSUS integration
- **Linux**: apt/dnf/zypper/pacman, Flatpak, Snap
- **Third-Party Patching**: Auto-discovered versions for Chrome, Firefox, Acrobat, 7-Zip, VLC, Teams, Zoom, Notepad++, LibreOffice
- **Patch Policies**: Approval mode (auto/manual), severity filters, platform targeting, device filters, maintenance windows
- **Staged Rollouts**: Canary deployments, scheduled maintenance windows
- **Deployment Tracking**: Per-device status (pending -> downloading -> installing -> success/failed/reboot_required)
- **Rollback**: Support for MSI `/x`, DEB `dpkg -r`, RPM `rpm -e`

---

## Software Deployment

### Purpose
Install, update, and remove software packages across managed devices centrally.

### Key Capabilities
- **Supported Package Types**: MSI (msiexec /quiet), EXE, DEB (dpkg), RPM (rpm), AppImage, Script-based
- **SHA256 Verification**: Checksum match before install
- **Silent Install**: Pre-configured arguments per package type
- **Multi-Device Deploy**: Any number of target devices
- **Per-Device Status**: pending -> downloading -> installing -> success/failed
- **Deployment History**: Full audit trail with results
- **Uninstall Support**: MSI `/x`, DEB `dpkg -r`, RPM `rpm -e`
- **Source**: Upload or URL-based package distribution

---

## Remote Support

### Purpose
Provide secure remote access to managed devices for troubleshooting and administration.

### Key Capabilities
- **Built-in Remote Control**:
  - Screen capture: Linux X11, Windows DirectX/BitBlt, macOS CGDisplay
  - Input injection: xdotool, SendInput, CGEvent
  - JPEG encoding at adaptive 5-10 FPS with quality slider (10-100)
  - HTML5 canvas web viewer with toolbar
- **RDP/SSH/VNC Tunnel**: NATS-relayed bidirectional streaming
  - Protocol detection with default ports
  - JSON handshake protocol
  - Byte counting for session tracking
- **Session Recording**:
  - Raw byte-level capture
  - Storage: MinIO, S3, or local filesystem
  - SHA256 checksum verification
  - Presigned URL playback (1-hour TTL, MFA-gated)
  - Configurable retention (default 90 days)
  - Optional SSE-KMS encryption
- **MFA**: Required for all interactive sessions
- **Session Lifecycle**: Start -> active -> disconnect (auto-cleanup)

---

## Network Discovery

### Purpose
Discover, map, and monitor network infrastructure without requiring agent installation.

### Key Capabilities
- **SNMP Probe**: Standalone Go binary deployed per customer
- **Versions**: SNMP v1, v2c, v3 with auth/priv
- **Polling**: Configurable interval per target
- **MIB Coverage**: CPU, memory, disk, interfaces, uptime
- **Discovery Methods**: ARP scan, SNMP walk, LLDP, CDP, STP
- **Flow Collection**: NetFlow v9, IPFIX, sFlow parsing
- **Topology Mapping**: Automated edge discovery from LLDP/CDP
- **Synthetic Monitoring**: HTTP, DNS, TCP, ICMP from multiple vantage points
- **Agentless**: Ideal for network gear, printers, IP phones

---

## Vulnerability Management

### Purpose
Identify, track, and remediate security vulnerabilities across managed devices.

### Key Capabilities
- **CVE Feeds**:
  - OSV.dev (primary): Free, no auth, broad OSS coverage
  - NVD (optional): Comprehensive CVE database with free API key
- **Scan Process**:
  1. Load CVE records from DB (auto-synced every 6 hours)
  2. Query device software inventory from patch_inventory snapshots
  3. Match installed packages against CVE database
  4. Record new vulnerabilities with severity, current/fixed version
  5. Auto-remediate when device reports updated package version
  6. Fire alerts for critical/high severity findings
- **Remediation Workflows**: Auto-remediate, manual resolve, manual ignore
- **Tracked Packages**: 17 packages monitored by default across major ecosystems
- **CVE Sync**: Automatic 6-hour sync interval with status tracking

---

## Reports

### Purpose
Generate and schedule PDF reports summarizing platform health and compliance.

### Key Capabilities
- **Report Sections**:
  - Executive Summary: Device count, online %, alert count, CVE count
  - Active Alerts: Severity, message, device, timestamp
  - Open CVEs: CVE ID, severity, package, device
  - Patch Status: Deployment compliance summary
- **Scheduling**: Daily, weekly, or monthly generation
- **On-Demand**: Generate via API at any time
- **Storage**: Auto-stored to MinIO/S3 with historical listing
- **Format**: PDF via gofpdf v2
- **Configurable Sections**: Per-schedule section selection

---

## Integrations

### Purpose
Extend platform capabilities through external service integrations.

### Key Capabilities
- **Notification Channels**: Slack webhook, Microsoft Teams webhook, PagerDuty, generic webhook, SMTP email
- **Storage Backends**: MinIO (self-hosted), AWS S3, local filesystem
- **KMS Providers**: Local key material, AWS KMS, GCP KMS, Azure Key Vault
- **SSO/OIDC**: Future integration with external identity providers
- **Monitoring**: Grafana dashboards (pre-provisioned)
- **CI/CD**: GitHub Actions with GoReleaser, Cosign signing, SBOM generation
- **Helm**: Kubernetes deployment with HPA, PDB, network policies
- **API**: 60+ REST endpoints for all platform capabilities

---

## Billing

### Purpose
Track usage and manage subscription billing for MSP tenants.

### Key Capabilities
- **Usage Tracking**: Per-tenant device counts, storage usage, feature utilization
- **Plan Tiers**: Configurable plans (e.g., per-device, per-feature)
- **Billing Integration**: Pluggable backend for external billing systems (Stripe, etc.)
- **Invoicing**: Automated invoice generation from usage data
- **Subscription Management**: Plan upgrades/downgrades, provisioning changes
- **Usage Reports**: Historical usage data for audit and reconciliation
- **Tenant Plans**: Enterprise plan designation with configurable limits

---

## Client Portal

### Purpose
Provide end-client organizations with self-service visibility into their managed infrastructure.

### Key Capabilities
- **Dashboard**: Client-specific view of device status, alerts, and patch compliance
- **User Access**: Role-based access for client users (viewer role)
- **Report Access**: View and download generated reports
- **Request Management**: Submit support requests and view status
- **Authentication**: SSO integration, separate from MSP admin login
- **White-Labeling**: Custom branding options for MSP reseller scenarios
- **Audit**: Client-scoped audit logging and access review
