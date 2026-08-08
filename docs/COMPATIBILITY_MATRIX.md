# Strata RMM — Compatibility Matrix

**Version:** 2026-08-08
**Last Updated: 2026-08-08

---

## 1. Platform Compatibility

### 1.1 Agent Platforms

| Platform | Version | Arch | Binary Type | Status |
|----------|---------|------|-------------|--------|
| Windows 10/11 | 10/11 | amd64, arm64 | EXE, MSI | ✅ Supported |
| Windows Server | 2016, 2019, 2022 | amd64, arm64 | Service | ✅ Supported |
| Ubuntu | 20.04, 22.04, 24.04 | amd64, arm64 | DEB, systemd | ✅ Supported |
| Debian | 11, 12 | amd64, arm64 | DEB | ✅ Supported |
| RHEL/Rocky/Alma | 8, 9 | amd64, arm64 | RPM | ✅ Supported |
| Fedora | 38, 39, 40 | amd64, arm64 | RPM | ✅ Supported |
| Alpine | 3.18+ | amd64, arm64 | APK | ✅ Supported |
| macOS | 12 (Monterey)+ | amd64, arm64 | PKG | ✅ Supported |

### 1.2 Orchestrator Platforms

| Platform | Arch | Status |
|----------|------|--------|
| Linux | amd64, arm64 | ✅ Supported |
| Windows | amd64 | ✅ Supported |
| macOS | amd64, arm64 | ✅ Supported |

---

## 2. Database Compatibility

### 2.1 PostgreSQL/TimescaleDB

| Version | Status | Notes |
|---------|--------|-------|
| PostgreSQL 15+ | ✅ Supported | Required for RLS features |
| TimescaleDB 2.12+ | ✅ Supported | Required for hypertables |
| PostgreSQL 14 | ⏸️ Deprecated | Not tested |

### 2.2 Connection Requirements

| Parameter | Requirement |
|-----------|-------------|
| SSL | `sslmode=require` (production) |
| Max connections | 25 (default) |
| Schema | Self-managed (migrations run on startup) |

---

## 3. Message Broker Compatibility

### 3.1 NATS

| Version | Status | Notes |
|---------|--------|-------|
| NATS 2.10+ | ✅ Supported | JetStream required |
| NATS Core | ✅ Supported | Without JetStream (limited) |

### 3.2 Subjects

| Subject | Description |
|---------|-------------|
| `tenant.{id}.agent.{id}.heartbeat` | Agent heartbeat |
| `tenant.{id}.agent.{id}.metrics` | Telemetry metrics |
| `tenant.{id}.agent.{id}.events` | Agent events |
| `tenant.{id}.cmd.{id}` | Commands to agent |
| `tenant.{id}.agent.{id}.software.result` | Software results |
| `tenant.{id}.agent.{id}.patch.result` | Patch results |

---

## 4. Object Storage Compatibility

### 4.1 Backends

| Backend | Status | Notes |
|---------|--------|-------|
| Local (filesystem) | ✅ Supported | Default for self-hosted |
| MinIO | ✅ Supported | Self-hosted S3-compatible |
| AWS S3 | ✅ Supported | Cloud object storage |

### 4.2 Features

| Feature | Local | MinIO | S3 |
|---------|-------|-------|-----|
| Upload | ✅ | ✅ | ✅ |
| Download | ✅ | ✅ | ✅ |
| Delete | ✅ | ✅ | ✅ |
| Exists | ✅ | ✅ | ✅ |
| Presigned URLs | ✅ | ✅ | ✅ |
| SSE-KMS | ❌ | ✅ | ✅ |

---

## 5. API Compatibility

### 5.1 API Versions

| Version | Status | Notes |
|---------|--------|-------|
| v1 | ✅ Supported | Current stable |
| v2 | ✅ Supported | Current stable |

### 5.2 Content Types

| Type | Supported |
|------|-----------|
| application/json | ✅ |
| application/pdf | ✅ (reports) |

### 5.3 Authentication

| Method | Supported |
|--------|-----------|
| JWT (HS256) | ✅ |
| TOTP/MFA | ✅ |
| API Key | ✅ |
| OAuth/OIDC | ⏸️ Deferred |

---

## 6. Agent Compatibility

### 6.1 Communication

| Protocol | Port | Direction |
|----------|------|-----------|
| NATS | 4222 (default) | Outbound only |
| NATS TLS | 4222 (tls://) | Outbound only |
| HTTPS | 443 | Outbound (for updates) |

### 6.2 Capabilities

| Capability | Windows | Linux | macOS |
|------------|---------|-------|-------|
| System metrics | ✅ | ✅ | ✅ |
| Software inventory | ✅ | ✅ | ✅ |
| Script execution | ✅ | ✅ | ✅ |
| Software install | ✅ | ✅ | ✅ |
| Patch management | ✅ | ✅ | ⏸️ Partial |
| Remote capture | ✅ | ✅ | ✅ |
| Input injection | ✅ | ✅ | ✅ |
| WebRTC | ✅ | ✅ | ✅ |

---

## 7. Browser Compatibility

### 7.1 Frontend

| Browser | Version | Status |
|---------|---------|--------|
| Chrome | 120+ | ✅ Supported |
| Firefox | 120+ | ✅ Supported |
| Safari | 17+ | ✅ Supported |
| Edge | 120+ | ✅ Supported |

### 7.2 WebRTC

| Browser | WebRTC | Status |
|---------|--------|--------|
| Chrome | ✅ | Supported |
| Firefox | ✅ | Supported |
| Safari | ✅ | Supported (limited) |
| Edge | ✅ | Supported |

---

## 8. Dependency Compatibility

### 8.1 Go

| Go Version | Status |
|------------|--------|
| Go 1.22+ | ✅ Required |
| Go 1.21 | ⏸️ Not tested |

### 8.2 Node.js (Frontend)

| Node Version | Status |
|-------------|--------|
| Node 18+ | ✅ Required |
| Node 16 | ⏸️ Deprecated |

---

## 9. Kubernetes Compatibility

| K8s Version | Status |
|-------------|--------|
| 1.27+ | ✅ Supported |
| 1.26 | ⏸️ Not tested |

---

## 10. Helm Compatibility

| Helm Version | Status |
|-------------|--------|
| Helm 3.12+ | ✅ Supported |

---

## 11. Docker Compatibility

| Docker Version | Status |
|---------------|--------|
| Docker 24+ | ✅ Supported |
| Docker Compose v2 | ✅ Supported |

---

## 12. Security Requirements

### 12.1 TLS

| Component | Requirement |
|-----------|-------------|
| API | TLS 1.3 (production) |
| NATS | TLS 1.2+ (production) |
| Agent | TLS 1.3 (recommended) |

### 12.2 Certificates

| Component | Type |
|-----------|------|
| API | TLS cert (Let's Encrypt, self-signed) |
| NATS | TLS cert + CA |
| Agent | TLS cert (optional) |

---

*Last Updated: 2026-08-08*
