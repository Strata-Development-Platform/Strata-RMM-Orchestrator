# Strata RMM — Security Reference

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Security Overview

Strata implements defense-in-depth with authentication, authorization, encryption, and audit at every layer.

**Key Principles:**
- Zero trust: Every request authenticated + authorized
- Tenant isolation: Database RLS + NATS subject isolation
- Fail-closed: Default deny for unclassified routes
- Audit trail: Immutable audit log table
- Supply chain: Keyless signing (cosign), SBOM generation

---

## 2. Authentication

### 2.1 Token Types

| Token | Format | Algorithm | Scope | Lifetime | Usage |
|-------|--------|-----------|-------|----------|-------|
| JWT (User) | `eyJ...` | HS256 | User + MSP/Client | Configurable | API access |
| JWT (Agent) | `eyJ...` | HS256 | Agent + Tenant | Enrollment token | Agent registration |
| Enrollment Token | UUID | — | Agent | Time-bound, single-use | One-time agent enrollment |
| API Key | `sk_...` | — | User | Persistent | Programmatic access |

### 2.2 JWT Claims

**User JWT:**
```json
{
  "sub": "user-uuid",
  "tenant_id": "tenant-uuid",
  "msp_id": "msp-uuid",
  "access_level": "msp_admin",
  "roles": ["admin", "technician"],
  "iat": 1234567890,
  "exp": 1234571490,
  "jti": "unique-token-id"
}
```

**Agent JWT:**
```json
{
  "sub": "agent-uuid",
  "tenant_id": "tenant-uuid",
  "agent_id": "agent-uuid",
  "iat": 1234567890,
  "exp": 1234571490,
  "jti": "unique-token-id"
}
```

### 2.3 MFA / TOTP

| Feature | Status |
|---------|--------|
| TOTP provisioning (QR code) | ✅ Implemented (`pkg/auth/totp.go`) |
| MFA enrollment (`POST /api/v1/mfa/enroll/{userID}`) | ✅ Implemented |
| MFA verification (`POST /api/v1/mfa/verify/{userID}`) | ✅ Implemented |
| MFA status check (`GET /api/v1/mfa/status/{userID}`) | ✅ Implemented |
| MFA deletion (`DELETE /api/v1/mfa/{userID}`) | ✅ Implemented |
| MFA on remote sessions | ✅ Required |

### 2.4 Session Management

| Feature | Implementation |
|---------|----------------|
| Token blacklisting | Redis set (`rmm:auth:blacklisted_tokens`), 24h TTL |
| Logout | `POST /api/v1/auth/logout` adds jti to blacklist |
| Me | `GET /api/v1/auth/me` returns current user |
| Refresh tokens | Deferred (future work) |

---

## 3. Authorization

### 3.1 Access Levels

| Level | Description | Example Routes |
|-------|-------------|----------------|
| `msp_owner` | MSP owner, full control | `/api/v2/msps/{mspID}/...` |
| `msp_admin` | MSP admin, manage devices/policies | `/api/v1/platform/*`, `/api/v2/msps/*` |
| `client_admin` | Client admin, view/manage clients | `/api/v2/clients/{clientID}/*` |
| `agent` | Agent identity, telemetry | `/api/v1/enroll`, `/api/v1/agent/*` |

### 3.2 Route Classification

Routes are classified by access level in `middleware.go`:

| Classification | Access Level | Default |
|---------------|--------------|---------|
| Public | None (no auth) | `/health`, `/releases/*` |
| Agent | Agent token required | `/api/v1/enroll`, `/api/v1/agent/*` |
| MSP Manage | `msp_admin` or `msp_owner` | `/api/v1/policies/*`, `/api/v1/devices/*` |
| MSP Admin | `msp_admin` or `msp_owner` | `/api/v2/msps/*` |
| Client Access | `client_admin` | `/api/v2/clients/*` |
| Platform | Platform admin | `/api/v1/platform/*` |
| Privileged | Special handling | `/api/v1/auth/*` |
| Denied | Explicit deny | (future) |

**Fail-closed:** Unknown routes return `403 AccessDenied` by default.

### 3.3 Handler Authorization

Every route has an explicit `Authorize*()` call in its handler:

| Handler | Authorization |
|---------|---------------|
| `handleCreateBillingAccount` | `AuthorizeMSPAccess` |
| `handleCreateSubscription` | `AuthorizeMSPManage` |
| `handleCreateRule` | `AuthorizeMSPManage` |
| `handleEnroll` | Agent token validation |
| `handleAgentRegister` | Agent token validation |
| `handleLogin` | Public (credentials) |
| `handleAdminCreateUser` | `AuthorizeMSPManage` |
| `handleCreatePatchPolicy` | `AuthorizeMSPManage` |
| `handleCreateSmartGroup` | `AuthorizeMSPManage` |
| `handleCreateRemoteSession` | `AuthorizeMSPManage` |
| `handleCreateWebRTCSesion` | `AuthorizeMSPManage` |

---

## 4. Data Protection

### 4.1 Encryption

| Data | Algorithm | Key Management |
|------|-----------|----------------|
| JWT signing | HS256 | `JWT_SECRET` env var (min 32 chars) |
| AES-256-GCM envelope | AES-256-GCM | Per-tenant, local file or cloud KMS |
| Backup encryption | AES-256-GCM (required) | Key provider file |
| Object storage (optional) | SSE-S3, SSE-KMS | Storage KMS key ID |
| Automation vault secrets | AES-256-GCM | Envelope encryption |

**Encryption Types Supported:**

| Type | Value | Description |
|------|-------|-------------|
| None | `none` | No encryption |
| SSE-S3 | `sse-s3` | Server-side encryption with S3-managed keys |
| SSE-KMS | `sse-kms` | Server-side encryption with KMS-managed keys |
| SSE-C | `sse-c` | Server-side encryption with customer-provided keys |

### 4.2 In Transit

| Layer | Protocol | Notes |
|-------|----------|-------|
| API | TLS 1.3 | Required in production (`STRATA_PUBLIC_URL=https://`) |
| NATS | TLS / mTLS | Required in production (`NATS_TLS_ENABLED=true`) |
| Agent | TLS 1.3 | Outbound-only, no inbound ports |
| Object storage | TLS | `STORAGE_USE_SSL=true` |
| SMTP | TLS / STARTTLS | Configurable |

### 4.3 At Rest

| Data | Protection |
|------|-----------|
| PostgreSQL | TDE optional, RLS policies |
| TimescaleDB | TDE optional, RLS policies |
| NATS JetStream | File store encryption optional |
| Backups | AES-256-GCM (mandatory) |
| Object storage | SSE-S3/SSE-KMS optional |
| Agent local store | BBolt (platform-dependent) |

---

## 5. Tenant Isolation

### 5.1 Database (RLS)

All tenant-scoped tables have Row-Level Security:

```sql
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON devices
    USING (tenant_id = current_tenant_id());
CREATE POLICY tenant_isolation_check ON devices
    WITH CHECK (tenant_id = current_tenant_id());
```

**Protected tables:**
- `tenants`, `devices`, `alerts`, `rules`, `users`, `memberships`
- `policies`, `patch_policies`, `patch_deployments`
- `scripts`, `script_schedules`, `script_executions`
- `billing_*`, `invoices`, `payment_methods`
- `audit_log`
- `device_relationships`, `device_packages`
- `remote_sessions`, `webrtc_sessions`
- `generated_reports`, `compliance_reports`

### 5.2 NATS Subject Isolation

```
tenant.{tenantID}.agent.{agentID}.>   — Agent telemetry
tenant.{tenantID}.cmd.{agentID}       — Commands to agent
tenant.{tenantID}.platform.>          — Platform events
msp.{mspID}.>                         — MSP-level events
platform.>                            — Platform events
```

Agents can only see their own tenant's subjects. Cross-tenant communication is impossible via NATS alone.

### 5.3 API Scoping

All API routes are scoped to `tenant_id` or `msp_id`:

| Route Pattern | Scope Source |
|--------------|--------------|
| `/api/v1/devices/{tenantID}/{deviceID}/*` | Path parameter |
| `/api/v1/tenants/{tenantID}/scripts/*` | Path parameter |
| `/api/v2/msps/{mspID}/clients/{clientID}/*` | Path parameter |
| `/api/v1/platform/customers/{tenantID}/devices` | Path parameter |

---

## 6. Audit & Compliance

### 6.1 Audit Log

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID | Unique identifier |
| `tenant_id` | UUID | Tenant scope |
| `user_id` | UUID | Actor user |
| `action` | string | Operation performed |
| `resource_type` | string | Target resource |
| `resource_id` | UUID | Target resource ID |
| `metadata` | JSONB | Additional context |
| `created_at` | timestamptz | Timestamp |

### 6.2 Immutable Audit

- Audit log table has no DELETE triggers
- WORM (Write-Once-Read-Many) storage optional for compliance
- Tamper-evident via hash chain (future)

### 6.3 SOC2 / Compliance

| Requirement | Status |
|------------|--------|
| Access control (RBAC) | ✅ Implemented |
| Audit logging | ✅ Implemented |
| Data encryption in transit | ✅ TLS 1.3 |
| Data encryption at rest | ✅ AES-256-GCM for backups |
| Incident response | ✅ Documented |
| Penetration testing | ⏳ Planned |
| SOC2 certification | ⏳ Planned |

---

## 7. Supply Chain Security

### 7.1 CI/CD

| Step | Tool | Description |
|------|------|-------------|
| Build | GoReleaser | Multi-arch builds (linux/amd64, linux/arm64, windows/amd64) |
| Signing | cosign | Keyless Sigstore signing |
| SBOM | syft | SPDX SBOM generation |
| Scanning | Trivy | SAST, dependency, image, secret scanning |

### 7.2 Agent Update

| Step | Description |
|------|-------------|
| Manifest | JSON with version, URLs, signatures |
| Verification | cosign keyless signature verification |
| Download | SHA256 checksum verification |
| Install | Atomic swap (backup current, install new, verify) |
| Rollback | Automatic on verification failure |

### 7.3 CVE Management

| Source | Frequency | Coverage |
|--------|-----------|----------|
| OSV.dev | 6h interval | Broad OSS coverage, free |
| NVD API | 24h interval | Enterprise coverage, optional |
| Local catalog | Manual | Vendor-specific packages |

---

## 8. Threat Model

### 8.1 Trusted Computing Base

| Component | Threat | Mitigation |
|-----------|--------|------------|
| JWT Secret | Theft → unauthorized access | File-based secrets, production validation |
| NATS Token | Theft → message interception | TLS, token rotation |
| DB Credentials | Theft → data breach | TLS, minimal privileges, no defaults |
| Agent Token | Theft → rogue device enrollment | Time-bound, single-use |
| Storage Keys | Theft → data exposure | KMS, file-based secrets |
| API Keys | Theft → unauthorized API access | User-scoped, revocable |

### 8.2 Attack Vectors

| Vector | Risk | Mitigation |
|--------|------|------------|
| JWT forgery | Low | HS256, min 32 char secret |
| Token replay | Low | jti blacklisting, short TTL |
| Cross-tenant data access | Medium | RLS policies, NATS subject isolation |
| Agent impersonation | Medium | Token validation, mTLS |
| Brute force login | Medium | Rate limiting (future), account lockout |
| SQL injection | Low | Parameterized queries (lib/pq, sqlx) |
| SSRF | Low | URL validation on all external URLs |
| Directory traversal | Low | Canonical path validation for secrets |
| Supply chain | Medium | cosign verification, SBOM, scanning |

### 8.3 Security Headers

| Header | Value |
|--------|-------|
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `X-XSS-Protection` | `1; mode=block` |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` |
| `Content-Security-Policy` | `default-src 'self'` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |

---

## 9. Incident Response

### 9.1 Credential Compromise

1. **JWT Secret**: Rotate, revoke all tokens via blacklist, regenerate agent tokens
2. **NATS Token**: Rotate, restart NATS cluster, re-enroll agents
3. **DB Credentials**: Rotate, audit access logs, check for unauthorized queries
4. **Storage Keys**: Rotate, revoke access, check for unauthorized reads/writes

### 9.2 Agent Compromise

1. **Isolate device**: NATS isolation command → stop telemetry
2. **Revoke token**: Remove agent record, regenerate enrollment token
3. **Investigate**: Review audit log for the compromised agent's tenant

### 9.3 Data Breach

1. **Contain**: Disable affected accounts, revoke tokens
2. **Assess**: Review audit log, identify scope
3. **Notify**: Contact affected tenants (if SaaS)
4. **Remediate**: Fix root cause, enhance controls
5. **Document**: Incident report, lessons learned

---

## 10. Penetration Testing

### 10.1 Scope

| Area | Test |
|------|------|
| API | Authorization bypass, injection, rate limiting |
| NATS | Subject isolation, message injection |
| Agent | Token validation, update verification |
| Storage | Access control, encryption |
| DB | RLS bypass, SQL injection |

### 10.2 Frequency

| Test | Frequency |
|------|-----------|
| Automated scanning | Every CI run |
| Dependency audit | Every PR |
| SAST/DAST | Every release |
| Third-party pen test | Annual |
| Bug bounty | Future |

---

*Last Updated: 2026-08-08*
