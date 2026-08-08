# Strata RMM — Beta Go/No-Go Criteria

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Beta Readiness Criteria

### 1.1 Code Completeness

| Criteria | Status | Details |
|----------|--------|---------|
| Core platform services | ✅ Complete | All services implemented and tested |
| Agent core | ✅ Complete | Identity, config, comms, collectors |
| Telemetry pipeline | ✅ Complete | NATS → TimescaleDB, continuous aggregates |
| Alert engine | ✅ Complete | Threshold, heartbeat, grouping, notifications |
| Policy engine | ✅ Complete | Hierarchical merge, validation, publish, rollback |
| Script vault | ✅ Complete | Git-backed, AES-256-GCM, schedule dispatch |
| Smart groups | ✅ Complete | DSL evaluator, schema, UI components |
| Remote access | ✅ Complete | WebRTC, recording, transcription, capture |
| Frontend | ✅ Complete | 20 pages, 600+ tests, real API integration |

### 1.2 Test Coverage

| Area | Status | Details |
|------|--------|---------|
| Unit tests | ✅ >90% | All major functions tested |
| Behavioral tests | ✅ >637 | All structs, JSON round-trips |
| Frontend tests | ✅ >600 | All pages and components |
| CI passing | ✅ All green | All workflows passing |
| Race detection | ✅ Clean | No race conditions detected |

### 1.3 Security

| Criteria | Status | Details |
|----------|--------|---------|
| JWT authentication | ✅ Complete | HS256, min 32 char secret |
| TOTP/MFA | ✅ Complete | Provisioning, verification, enrollment |
| API keys | ✅ Complete | User-scoped, revocable |
| RLS policies | ✅ Complete | All tenant-scoped tables |
| NATS subject isolation | ✅ Complete | Tenant-prefixed subjects |
| Audit logging | ✅ Complete | Immutable audit trail |
| TLS enforcement | ✅ Complete | Production TLS required |
| Supply chain | ✅ Complete | cosign, SBOM, scanning |

### 1.4 Deployment

| Criteria | Status | Details |
|----------|--------|---------|
| Docker | ✅ Complete | docker-compose.yml |
| Kubernetes/Helm | ✅ Complete | Helm chart, HPA, PDB |
| systemd | ✅ Complete | Linux service |
| Windows service | ✅ Complete | PowerShell installer |
| Backup/restore | ✅ Complete | AES-256-GCM, filesystem/S3 |

### 1.5 Documentation

| Criteria | Status | Details |
|----------|--------|---------|
| Architecture | ✅ Updated | Full architecture overview |
| Configuration | ✅ Updated | All env vars documented |
| API routes | ✅ Updated | 310+ routes documented |
| Security | ✅ Updated | Security model documented |
| Deployment | ✅ Updated | All deployment methods |
| Backup/restore | ✅ Updated | Procedures documented |
| Incident response | ✅ Updated | Response procedures |
| Resilience | ✅ Updated | Testing procedures |

---

## 2. Go/No-Go Checklist

### 2.1 Must-Have (Go if any fail)

| # | Criteria | Status |
|---|----------|--------|
| 1 | All CI workflows passing | ✅ Pass |
| 2 | No known security vulnerabilities | ✅ Clean |
| 3 | Backup/restore tested | ✅ Verified |
| 4 | Health checks working | ✅ /health, /health/live, /health/ready |
| 5 | Agent enrollment working | ✅ POST /api/v1/enroll |
| 6 | Metrics ingestion working | ✅ NATS → TimescaleDB |
| 7 | Alert evaluation working | ✅ Threshold, heartbeat |
| 8 | Policy CRUD working | ✅ CRUD, validation, publish |
| 9 | Script execution working | ✅ PS/Bash/Python/Batch |
| 10 | Remote access working | ✅ WebRTC sessions |
| 11 | Frontend login working | ✅ JWT auth, TOTP |
| 12 | Database migrations idempotent | ✅ Verified |

### 2.2 Should-Have (Go if acceptable)

| # | Criteria | Status |
|---|----------|--------|
| 1 | Patch management working | ✅ Partial (Windows/Linux only) |
| 2 | Software deployment working | ✅ MSI/EXE/DEB/RPM |
| 3 | Third-party catalog working | ✅ Vendor sync, version discovery |
| 4 | Vulnerability management working | ✅ OSV sync, version matching |
| 5 | Reports generation working | ✅ PDF, CSV, JSON |
| 6 | Smart groups evaluation working | ✅ DSL, nested expressions |
| 7 | LAN Cache working | ✅ 5 endpoints |
| 8 | Integration webhooks working | ✅ EDR, Backup, PSA |
| 9 | MSP lifecycle working | ✅ Billing, branding, offboarding |
| 10 | Client portal working | ✅ Support requests |

### 2.3 Nice-to-Have (Deferred)

| # | Criteria | Status |
|---|----------|--------|
| 1 | SSO/OIDC | ⏳ Deferred |
| 2 | External billing backend | ⏳ Deferred |
| 3 | Password recovery | ⏳ Deferred |
| 4 | Refresh tokens | ⏳ Deferred |
| 5 | Trusted proxy handling | ⏳ Deferred |

---

## 3. Beta Launch Plan

### 3.1 Phase 1: Internal Alpha (Current)
- Test with internal team
- Collect feedback
- Fix critical issues

### 3.2 Phase 2: Limited External Beta
- Invite 5-10 external MSPs
- Monitor usage and issues
- Gather feature requests

### 3.3 Phase 3: General Beta
- Open to all MSPs
- Full monitoring
- Iterate based on feedback

### 3.4 Phase 4: Production Release
- All critical issues resolved
- Documentation complete
- Support team trained
- SLA defined

---

## 4. Beta Metrics to Track

| Metric | Target |
|--------|--------|
| Agent uptime | >99% |
| API response time | <500ms P95 |
| Alert delivery latency | <30s |
| Script execution success rate | >95% |
| Remote session success rate | >90% |
| User satisfaction | >4/5 |
| Bug report rate | <10/week |

---

## 5. Beta Risk Register

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Data loss | Low | Critical | Backup/restore tested |
| Security breach | Low | Critical | RLS, TLS, audit logging |
| Performance degradation | Medium | Medium | Load testing, monitoring |
| Agent instability | Medium | High | Agent update rollback |
| UI usability issues | High | Low | User feedback, iterate |

---

## 6. Decision

| Role | Decision | Date | Notes |
|------|----------|------|-------|
| Engineering Lead | Go/No-Go | TBD | |
| Security Lead | Go/No-Go | TBD | |
| Product Lead | Go/No-Go | TBD | |
| Operations Lead | Go/No-Go | TBD | |

---

*Last Updated: 2026-08-08*
