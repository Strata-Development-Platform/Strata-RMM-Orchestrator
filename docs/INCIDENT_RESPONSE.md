# Strata RMM — Incident Response

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Incident Response Overview

Strata's incident response procedures cover credential compromise, agent compromise, data breach, service outage, and supply chain attacks.

---

## 2. Severity Levels

| Level | Description | Response Time |
|-------|-------------|---------------|
| P1 - Critical | Active data breach, service outage | 15 minutes |
| P2 - High | Credential compromise, agent compromise | 1 hour |
| P3 - Medium | Non-critical security issue | 4 hours |
| P4 - Low | Informational, future improvements | 1 week |

---

## 3. Credential Compromise

### 3.1 JWT Secret Compromise

**Impact:** All JWT tokens can be forged.

**Response:**
1. Generate new JWT secret
2. Blacklist all existing tokens (`rmm:auth:blacklisted_tokens` in Redis)
3. Force re-authentication for all users
4. Rotate all agent enrollment tokens
5. Audit access logs for unauthorized actions
6. Rotate NATS tokens if JWT secret was used for NATS auth

### 3.2 NATS Token Compromise

**Impact:** Unauthorized NATS message injection, agent impersonation.

**Response:**
1. Rotate NATS token
2. Restart NATS cluster
3. Re-enroll all agents with new tokens
4. Check for unauthorized message injection
5. Verify agent telemetry integrity

### 3.3 Database Credentials Compromise

**Impact:** Full database access, data breach.

**Response:**
1. Rotate database credentials
2. Audit access logs
3. Check for unauthorized queries
4. Assess data exposure scope
5. Notify affected tenants (if SaaS)
6. Enable additional monitoring

### 3.4 API Key Compromise

**Impact:** Unauthorized API access for specific user.

**Response:**
1. Revoke compromised API key
2. Audit API usage logs
3. Assess scope of unauthorized access
4. Issue new API key

### 3.5 Storage Key Compromise

**Impact:** Unauthorized access to object storage.

**Response:**
1. Rotate storage keys
2. Revoke access to old keys
3. Check for unauthorized reads/writes
4. Assess data exposure

---

## 4. Agent Compromise

### 4.1 Detection

- Unusual telemetry patterns
- Unauthorized script execution
- Unexpected network connections

### 4.2 Isolation

1. **Stop telemetry:** NATS isolation command
2. **Revoke token:** Remove agent record
3. **Regenerate:** Create new enrollment token

### 4.3 Investigation

1. Review audit log for compromised agent's tenant
2. Check script execution history
3. Review remote session recordings
4. Check for lateral movement attempts

### 4.4 Remediation

1. Revoke compromised enrollment token
2. Remove agent record from database
3. Regenerate new enrollment token
4. Notify tenant of compromise
5. Provide guidance on securing agent device

---

## 5. Data Breach

### 5.1 Detection

- Unauthorized data access detected
- Compliance monitoring alert
- User report

### 5.2 Containment

1. Disable affected accounts
2. Revoke all tokens for compromised accounts
3. Isolate affected database/tenant
4. Enable enhanced monitoring

### 5.3 Assessment

1. Review audit log for scope
2. Identify affected data
3. Identify affected tenants
4. Determine attack vector
5. Assess data classification exposure

### 5.4 Notification

1. Notify affected tenants (SaaS deployment)
2. Notify legal/compliance team
3. Notify management
4. Document findings

### 5.5 Remediation

1. Fix root cause
2. Enhance security controls
3. Implement additional monitoring
4. Update incident response procedures
5. Document lessons learned

---

## 6. Service Outage

### 6.1 Database Outage

**Symptoms:** `/health` returns degraded, API errors

**Response:**
1. Check database connectivity
2. Check disk space
3. Check connection pool exhaustion
4. Restart database if needed
5. Failover to read replica (if configured)

### 6.2 NATS Outage

**Symptoms:** Agent disconnections, command delivery failures

**Response:**
1. Check NATS server status
2. Check disk space (JetStream stores)
3. Check network connectivity
4. Restart NATS if needed
5. Check agent reconnect queue

### 6.3 Storage Outage

**Symptoms:** Remote session recordings fail, report generation fails

**Response:**
1. Check storage backend status
2. Check network connectivity
3. Check credentials
4. Failover to backup storage (if configured)

---

## 7. Supply Chain Attack

### 7.1 Agent Binary Compromise

**Symptoms:** Unusual agent behavior, signature verification failures

**Response:**
1. Block compromised binary version
2. Roll back to known good version
3. Verify all agent installations
4. Revoke and re-sign all binaries
5. Review CI/CD pipeline security

### 7.2 Dependency Compromise

**Symptoms:** Unusual network connections from application

**Response:**
1. Check dependency vulnerability scanner results
2. Update compromised dependencies
3. Review supply chain security practices
4. Update SBOM

---

## 8. Post-Incident

### 8.1 Incident Report

All incidents require an incident report covering:

| Item | Description |
|------|-------------|
| Timeline | When detected, when responded, when resolved |
| Impact | Data affected, users affected, service affected |
| Root cause | How the incident occurred |
| Response | What actions were taken |
| Lessons learned | What could be improved |
| Follow-up | What actions will be taken to prevent recurrence |

### 8.2 Follow-Up Actions

1. Implement preventive controls
2. Update security documentation
3. Conduct security training
4. Schedule security review
5. Update monitoring/alerting

---

## 9. Communication Plan

### 9.1 Internal Communication

| Channel | Purpose |
|---------|---------|
| Slack incident channel | Real-time coordination |
| Email | Formal notifications |
| Phone | P1 incidents |

### 9.2 External Communication

| Channel | Purpose |
|---------|---------|
| Status page | Service status updates |
| Email to tenants | Tenant-specific notifications |
| Legal counsel | Regulatory notifications |

---

## 10. Contact Information

| Role | Contact |
|------|---------|
| Security Team | security@strata.example.com |
| On-Call Engineer | PagerDuty |
| Management | management@strata.example.com |
| Legal | legal@strata.example.com |

---

*Last Updated: 2026-08-08*
