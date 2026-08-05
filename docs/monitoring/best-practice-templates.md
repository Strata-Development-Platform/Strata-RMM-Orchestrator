# Monitoring Best Practice Templates

Master tables for server and hardware monitoring configuration. Each template defines:
- **OIDs / endpoints** polled by the infrastructure probe engine
- **Thresholds** that trigger alerting rules
- **Self-healing targets** — automatic remediation actions
- **Collection intervals** and **retention policies**

These templates are referenced by `internal/probe/` collectors, `internal/monitoring/` ingestion, and `internal/alerting/` rule engine.

---

## 1. Server Hardware Monitoring (Physical + Hypervisor)

### SNMP OID Table (RFC 1213 MIB-2 + vendor extensions)

| OID | Name | Type | Poll Interval | Severity | Threshold | Self-Healing |
|-----|------|------|---------------|----------|-----------|--------------|
| `.1.3.6.1.2.1.1.3.0` | sysUpTime | gauge | 5 min | — | — | — |
| `.1.3.6.1.2.1.2.1.0` | ifInErrors | counter | 1 min | warning | > 100 errors/min | Reset interface |
| `.1.3.6.1.2.1.2.2.1.19` | ifInDiscards | counter | 1 min | warning | > 50 discards/min | — |
| `.1.3.6.1.2.1.2.2.1.20` | ifOutErrors | counter | 1 min | warning | > 100 errors/min | Reset interface |
| `.1.3.6.1.2.1.2.2.1.21` | ifOutDiscards | counter | 1 min | warning | > 50 discards/min | — |
| `.1.3.6.1.2.1.11.1.0` | tcpInSegs | counter | 1 min | — | — | — |
| `.1.3.6.1.2.1.11.2.0` | tcpOutSegs | counter | 1 min | — | — | — |
| `.1.3.6.1.2.1.25.1.1.0` | hrSystemUptime | gauge | 5 min | — | — | — |

### CPU / Memory / Disk (Agent-collected, 60s interval)

| Metric | Type | Unit | Warning | Critical | Self-Healing |
|--------|------|------|---------|----------|--------------|
| `cpu.percent` | gauge | % | > 80 | > 95 | Kill highest-CPU process |
| `load.1` | gauge | avg | > cores×2 | > cores×4 | — |
| `load.5` | gauge | avg | > cores×1.5 | > cores×3 | — |
| `memory.percent` | gauge | % | > 85 | > 95 | Trigger OOM killer hint |
| `swap.percent` | gauge | % | > 50 | > 80 | — |
| `disk.percent` | gauge | % | > 85 | > 95 | Trigger cleanup script |
| `diskio.iops_in_progress` | gauge | count | > 1000 | > 5000 | Throttle I/O |

### Redfish Endpoint Table (Dell iDRAC / HPE iLO / Lenovo XCC)

| Endpoint | Data Extracted | Interval | Threshold |
|----------|---------------|----------|-----------|
| `/redfish/v1/Systems/` | PowerState, Health, CPUModel, CPUCount, MemoryTotalMB | 5 min | Health ≠ "OK" |
| `/redfish/v1/Chassis/` | ThermalStatus, PowerStatus | 2 min | ThermalStatus ≠ "OK" |
| `/redfish/v1/Managers/` | ManagerHealth, FirmwareVersion | 30 min | Firmware outdated > 1 release |
| `/redfish/v1/Systems/LogServices/` | EventCount, LastEvent | 5 min | New error event within 5 min |

### IPMI Sensor Table (NetFn 0x0a)

| Sensor Type | NetFn | Request | Unit | Warning | Critical | Self-Healing |
|-------------|-------|---------|------|---------|----------|--------------|
| Temperature | 0x0a | 0x2d | °C | > 75 | > 90 | Reduce CPU frequency |
| Fan | 0x0a | 0x2d | RPM | < 1000 | < 500 | — |
| Voltage | 0x0a | 0x2d | V | < 3.3 or > 3.7 | < 3.1 or > 3.9 | — |
| Current | 0x0a | 0x2d | A | > 15 | > 20 | — |
| Physical security | 0x0a | 0x2d | status | breach | — | Alert security team |

---

## 2. Network Device Monitoring

### Switch / Firewall SNMP OIDs

| OID | Name | Type | Poll Interval | Severity | Threshold | Self-Healing |
|-----|------|------|---------------|----------|-----------|--------------|
| `.1.3.6.1.2.1.2.2.1.19` (all ifIndex) | ifInErrors | counter | 1 min | warning | > 50 errors/min | Log; escalate if > 5 min |
| `.1.3.6.1.2.1.2.2.1.20` (all ifIndex) | ifOutErrors | counter | 1 min | warning | > 50 errors/min | Log; escalate if > 5 min |
| `.1.3.6.1.2.1.4.22.1.2*` | arpTable | table | 5 min | — | > 50% ARP miss | ARP flush |
| `.1.3.6.1.2.1.17.4.3.1.1*` | macTable | table | 5 min | — | MAC count > 500 | Alert |
| `.1.3.6.1.2.1.4.3.0` | ipForwarding | gauge | 5 min | critical | = 0 (was 1) | Re-enable IP forwarding |
| `.1.3.6.1.2.1.6.13.1.3` | tcpCurrEstab | gauge | 1 min | warning | > 5000 | — |
| `.1.3.6.1.2.1.6.13.1.4` | tcpInErrs | counter | 1 min | warning | > 100/min | — |

### BGP Monitoring (RFC 4273 MIB)

| OID | Name | Type | Severity | Threshold | Self-Healing |
|-----|------|------|----------|-----------|--------------|
| `.1.3.6.1.4.1.171.10.8.2.1.1.2` | bgpPeerState | gauge | critical | < 5 (established) | Restart BGP session |
| `.1.3.6.1.2.1.15.7.1.6` | tcpCurrEstab | gauge | warning | > 100 | — |

### Router Interface Table

| OID | Name | Type | Severity | Threshold | Self-Healing |
|-----|------|------|----------|-----------|--------------|
| `.1.3.6.1.2.1.2.2.1.7` | ifAdminStatus | gauge | critical | = 2 (down) | — |
| `.1.3.6.1.2.1.2.2.1.8` | ifOperStatus | gauge | critical | = 2 (down) | Alert NOC |
| `.1.3.6.1.2.1.31.1.1.1.6` | ifHCInOctets | counter | warning | throughput < 1 Mbps | — |
| `.1.3.6.1.2.1.31.1.1.1.10` | ifHCOutOctets | counter | warning | throughput < 1 Mbps | — |

---

## 3. UPS / Power Monitoring (SNMP RFC 1628)

### UPS MIB-2 OIDs (RFC 1628, MIB-2 UPS)

| OID | Name | Type | Unit | Warning | Critical | Shutdown Trigger |
|-----|------|------|------|---------|----------|-----------------|
| `.1.3.6.1.2.1.33.1.4.1.1` | batteryCapacity | gauge | % | < 70 | < 30 | — |
| `.1.3.6.1.2.1.33.1.4.1.2` | batteryPercent | gauge | % | < 50 | < 30 | — |
| `.1.3.6.1.2.1.33.1.4.2.1` | batteryRuntime | gauge | seconds | < 1800 (30 min) | < 600 (10 min) | Yes (see table below) |
| `.1.3.6.1.2.1.33.1.3.1.1` | inputStatus | gauge | — | ≠ 1 (normal) | ≠ 1 | — |
| `.1.3.6.1.2.1.33.1.3.2.1` | inputVoltage | gauge | VAC | < 100 or > 260 | < 90 or > 280 | — |
| `.1.3.6.1.2.1.33.1.5.1.1` | outputStatus | gauge | — | ≠ 1 (normal) | ≠ 1 | — |
| `.1.3.6.1.2.1.33.1.5.2.1` | outputVoltage | gauge | VAC | < 100 or > 260 | < 90 or > 280 | — |
| `.1.3.6.1.2.1.33.1.5.3.1` | outputLoad | gauge | % | > 80 | > 95 | — |

### UPS Shutdown Orchestration (from `internal/orchestrator/power_events.go`)

When `batteryRuntime` < **600 seconds**, the orchestrator executes this ordered shutdown sequence:

| Order | Target Type | Action | Timeout |
|-------|------------|--------|---------|
| 1 | Hypervisor | Graceful shutdown (ESXi, Proxmox, Hyper-V) | 30 min |
| 2 | SAN | Graceful shutdown (Pure, PowerStore, HPE Alletra) | 20 min |
| 3 | NAS | Graceful shutdown (Synology, QNAP, TrueNAS) | 10 min |
| 4 | VM Pool | Graceful shutdown all VMs | 15 min |

---

## 4. Storage / SAN / NAS Monitoring

### SNMP Storage OIDs

| OID | Name | Type | Severity | Threshold | Self-Healing |
|-----|------|------|----------|-----------|--------------|
| `.1.3.6.1.2.1.25.2.3.1.5` | hrStorageUsed | gauge | warning | > 85% of hrStorageSize | — |
| `.1.3.6.1.2.1.25.2.3.1.6` | hrStorageSize | gauge | — | — | — |
| `.1.3.6.1.2.1.25.2.3.1.4` | hrStorageAllocationUnits | gauge | — | — | — |
| `.1.3.6.1.4.1.2021.9.1.6` | dskTotal (UCD-SNMP) | gauge | warning | > 85% | — |
| `.1.3.6.1.4.1.2021.9.1.7` | dskAvail (UCD-SNMP) | gauge | critical | < 5% | — |

### Redfish Storage

| Endpoint | Data Extracted | Severity | Threshold |
|----------|---------------|----------|-----------|
| `/redfish/v1/Systems/*/Storage/` | VolumeStatus, CapacityUsedPercent | warning | > 85% |
| `/redfish/v1/Chassis/*/DriveList/` | DriveStatus, Health | critical | ≠ "OK" | — |

---

## 5. Agent Collector Metrics (60s collection interval)

### System Metrics

| Metric Name | Type | Tags | Warning Threshold | Critical Threshold |
|-------------|------|------|-------------------|-------------------|
| `system.uptime` | float | os, arch, hostname, platform, platform_version | — | — |
| `cpu.percent` | float | — | > 80 | > 95 |
| `cpu.cores` | float | — | — | — |
| `load.1` | float | — | > cores×2 | > cores×4 |
| `load.5` | float | — | > cores×1.5 | > cores×3 |
| `load.15` | float | — | > cores | > cores×2 |
| `memory.total` | float | unit=bytes | — | — |
| `memory.used` | float | unit=bytes | — | — |
| `memory.available` | float | unit=bytes | — | — |
| `memory.percent` | float | — | > 85 | > 95 |
| `swap.total` | float | unit=bytes | — | — |
| `swap.used` | float | unit=bytes | — | — |
| `swap.percent` | float | — | > 50 | > 80 |

### Disk Metrics

| Metric Name | Type | Tags | Warning Threshold | Critical Threshold |
|-------------|------|------|-------------------|-------------------|
| `disk.total` | float | mount, fstype, device | — | — |
| `disk.used` | float | mount, fstype, device | — | — |
| `disk.free` | float | mount, fstype, device | — | — |
| `disk.percent` | float | mount, fstype, device | > 85 | > 95 |
| `diskio.read_count` | float | disk | — | — |
| `diskio.write_count` | float | disk | — | — |
| `diskio.read_bytes` | float | disk | — | — |
| `diskio.write_bytes` | float | disk | — | — |
| `diskio.iops_in_progress` | float | disk | > 1000 | > 5000 |

### Network Metrics

| Metric Name | Type | Tags | Warning Threshold | Critical Threshold |
|-------------|------|------|-------------------|-------------------|
| `net.bytes_sent` | float | interface | — | — |
| `net.bytes_recv` | float | interface | — | — |
| `net.packets_sent` | float | interface | — | — |
| `net.packets_recv` | float | interface | — | — |
| `net.errors_in` | float | interface | > 100/min | > 1000/min |
| `net.errors_out` | float | interface | > 100/min | > 1000/min |

### System Info

| Metric Name | Type | Tags | Description |
|-------------|------|------|-------------|
| `system.info` | float (1) | os, arch, hostname, platform, platform_version | Always 1, tagged with system identity |

---

## 6. Self-Healing / Remediation Policy Table

### Default Remediation Policies

| Condition | Severity | Auto-Remediate | Action | Max Retries | Retry Delay |
|-----------|----------|---------------|--------|-------------|-------------|
| CPU > 95% sustained 5 min | critical | Yes | Kill highest-CPU process | 3 | 1 hour |
| Memory > 95% sustained 5 min | critical | Yes | Trigger OOM killer | 3 | 1 hour |
| Disk > 95% | critical | Yes | Run cleanup script | 3 | 1 hour |
| Disk > 85% | warning | No | Alert only | — | — |
| Disk I/O > 5000 IOPS | critical | Yes | Throttle I/O | 3 | 30 min |
| Interface errors > 1000/min | critical | Yes | Reset interface | 3 | 30 min |
| BGP session down | critical | Yes | Restart BGP session | 3 | 15 min |
| UPS battery < 30% | critical | Yes | Start shutdown sequence | 1 | — |
| UPS battery < 50% | warning | No | Alert only | — | — |
| UPS input power lost | critical | Yes | Start shutdown sequence | 1 | — |
| Agent heartbeat > 5 min | critical | Yes | Restart agent service | 3 | 5 min |
| Agent heartbeat > 15 min | critical | No | Alert NOC | — | — |
| Redfish Health ≠ OK | critical | Yes | Reset management controller | 3 | 1 hour |
| IPMI temperature > 90°C | critical | Yes | Reduce CPU frequency | 3 | 30 min |
| IPMI fan < 500 RPM | critical | Yes | Alert hardware team | 3 | 15 min |

### Remediation Engine Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | `false` | Global self-healing toggle |
| `severity_threshold` | `"critical"` | Only remediate at or above this severity |
| `auto_remediate` | `false` | Per-rule auto-remediation override |
| `max_retries` | `3` | Maximum remediation attempts before escalating |
| `retry_delay_hours` | `1` | Delay between retry attempts |
| `auto_approve` | `true` | Auto-approve remediation actions |
| `reboot_behavior` | `"automatic"` | When reboot is required |
| `remediation_loop_interval` | `1 hour` | How often the remediation engine scans for conditions |

---

## 7. TimescaleDB Retention & Compression Policy

### Hypertable Retention Table

| Hypertable | Retention Period | Compression Trigger | Chunk Interval | API Method |
|------------|-----------------|---------------------|----------------|------------|
| `metrics` | 365 days | 7 days | 1 day | `SetRetentionPolicy(days)` |
| `heartbeats` | Configurable | N/A | 7 days | `SetHeartbeatsRetention(days)` |
| `alerts_ts` | 365 days | 30 days | 7 days | `SetAlertsRetention(days)` |
| `snmp_polls` | 90 days | 7 days | 1 day | `SetSNMPPollsRetention(days)` |
| `flow_records` | 30 days | 7 days | 1 day | `SetFlowRecordsRetention(days)` |
| `topology_edges` | 90 days | N/A | 7 days | `SetTopologyEdgesRetention(days)` |

### Continuous Aggregate Views

| View Name | Bucket | Schedule | Start Offset | End Offset |
|-----------|--------|----------|-------------|------------|
| `metrics_1m` | 1 minute | Every 1 minute | 3 days | 1 hour |
| `metrics_1h` | 1 hour | Every 1 hour | 7 days | 1 hour |

---

## 8. Prometheus Alert Rules (deploy/prometheus/alerts.yml)

| Rule Name | Condition | For Duration | Severity | Description |
|-----------|-----------|-------------|----------|-------------|
| `StrataAPIDown` | `up{job="strata-orchestrator"} == 0` | 2 min | critical | Orchestrator HTTP endpoint unreachable |
| `StrataAPIHighErrorRate` | 5xx ratio > 5% (5m) | 10 min | warning | API error rate exceeds threshold |
| `StrataAPIHighP95Latency` | p95 > 1s | 10 min | warning | API latency degradation |
| `StrataJobMetricsCollectionFailed` | `strata_job_metrics_collection_success == 0` | 2 min | critical | Metrics collection pipeline failure |
| `StrataOldestJobStalled` | `strata_job_oldest_active_seconds > 900` | 5 min | critical | Job stuck > 15 minutes |
| `StrataJobFailures` | `strata_job_targets{status=~"failed|expired"} > 0` | 5 min | warning | Active job failures detected |

---

## 9. NATS JetStream Subject Map

| Subject Pattern | Stream | Consumer Group | Data Source |
|-----------------|--------|---------------|-------------|
| `tenant.*.agent.*.metrics` | STRATA_METRICS | ingestion | Agent collectors (system.go, 60s) |
| `tenant.*.agent.*.events` | STRATA_EVENTS | ingestion | Agent events |
| `tenant.*.agent.*.heartbeat` | STRATA_HEARTBEATS | ingestion | Agent heartbeats |
| `tenant.*.probe.*.snmp` | STRATA_PROBES | probe | SNMP probe (5 min) |
| `tenant.*.probe.*.flow` | STRATA_PROBES | probe | NetFlow/IPFIX |
| `tenant.*.probe.*.discovery` | STRATA_DISCOVERY | probe | Network discovery |

### Batch Processing Configuration

| Setting | Value |
|---------|-------|
| Batch flush interval | 5 seconds |
| JetStream ack wait | 30 seconds |
| Max deliveries | 10 |

---

## 10. Hardware Type Detection & Role Mapping

### Syslog Device Classification (from internal/probe/syslog.go)

| Device Type | App Name Patterns |
|------------|-------------------|
| firewall | `firewall`, `forti`, `palo`, `fw` |
| switch | `switch`, `sw` |
| router | `router`, `rt`, `rtr` |
| server | `server`, `bmc`, `ipmi` |
| ups | `ups`, `apc` |
| storage | `san`, `storage` |

### Server Role Detection (from internal/agent/core/role_scanner.go)

| Role | Detection Method | Platform |
|------|-----------------|----------|
| `ad_dc` | Windows WMIC DomainRole = 5 | Windows |
| `sql_server` | `sc query MSSQLSERVER` | Windows |
| `hyper_v` | DISM/PowerShell Hyper-V feature check | Windows |
| `linux_dns` | systemctl: named, knot, unbound, bind9 | Linux |
| `linux_web_server` | systemctl: nginx, apache2, httpd, caddy | Linux |
| `linux_database` | systemctl: postgresql, mysql, mariadb, mongod | Linux |

### Network Discovery Types (from internal/probe/discovery.go)

| Type | Detection |
|------|-----------|
| router | Port 22 open |
| database | Port 3306 or 5432 open |
| cache | Port 6379 open |
| server | Port 8080 open |
| device | MAC address present |
| host | Default |

---

## 11. Alerting Engine Configuration

### Condition Types

| Condition | Operator | Description |
|-----------|----------|-------------|
| `gt` | `>` | Greater than |
| `gte` | `>=` | Greater than or equal |
| `lt` | `<` | Less than |
| `lte` | `<=` | Less than or equal |
| `eq` | `==` | Equal |
| `neq` | `!=` | Not equal |

### Severity Levels

| Severity | Priority | Auto-Remediate (default) |
|----------|----------|-------------------------|
| `critical` | 1 (highest) | Yes |
| `warning` | 2 | No |
| `info` | 3 (lowest) | No |

### Evaluation & Cleanup Intervals

| Setting | Interval | Description |
|---------|----------|-------------|
| Evaluation interval | 30 seconds | How often rules are evaluated |
| Stale state cleanup | 1 hour | Remove states with no data for this duration |
| OK state cleanup | 24 hours | Remove resolved OK states |

---

## 12. Resilience Load Thresholds (from internal/resilience/)

| Metric | Default Threshold | Description |
|--------|------------------|-------------|
| Max error rate | 0.01 (1%) | Request error rate threshold |
| Max P95 latency | 500 ms | 95th percentile latency threshold |
| Rate | 100 req/s | Maximum sustained request rate |
| Concurrency | 20 | Maximum concurrent requests |
| Duration | 1 minute | Load test duration |
| Request timeout | 10 s | Per-request timeout |
