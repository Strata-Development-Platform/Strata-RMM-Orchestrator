# Strata RMM Security Architecture

## Credential Management

### Secrets That Must Be Rotated Immediately
- **GitHub PAT**: `<redacted - rotate any tokens stored in env/scripts>`
- **Local sudo password**: `<redacted>`
- **Dev JWT secret**: `strata-rmm-dev-secret` — change in production

### Production Secret Storage
- Use HashiCorp Vault or AWS Secrets Manager / GCP Secret Manager
- Never commit secrets to git
- Use Sealed Secrets or External Secrets Operator in K8s

## Authentication

### Agent → Platform
1. Agent generates ECDSA P256 keypair on first run
2. Agent requests enrollment with token (time-bound, single-use)
3. Platform issues short-lived mTLS certificate (24h)
4. Agent re-authenticates before cert expiry

### User → API
- JWT (HS256) with tenant_id + role claims
- Token expiry: 8 hours for interactive sessions
- Optional: OIDC integration (Keycloak, Google, Azure AD)

### API Key (Machine-to-Machine)
- Fixed-length tokens with tenant/scope binding
- Rate-limited per key
- Revocable via API

## Authorization

### Multi-Tenant Isolation

| Layer | Mechanism |
|-------|-----------|
| Database | PostgreSQL Row-Level Security (`current_setting('app.tenant_id')`) |
| NATS | Subject isolation (`tenant.{id}.*`) |
| API | JWT claim validation + tenant header injection |
| TimescaleDB | `tenant_id` column in all hypertables |

### RBAC Roles
| Role | Permissions |
|------|------------|
| `admin` | Full tenant access, user management, billing |
| `technician` | Device management, remote access, patching |
| `viewer` | Read-only dashboard and alert views |

## Network Security

### In-Transit Encryption
- TLS 1.3 for all HTTP endpoints
- NATS TLS with mutual authentication (agent → NATS)
- mTLS for inter-service communication within cluster

### Network Policies (K8s)
- Deny all ingress/egress by default
- Allow NATS (4222) and PostgreSQL (5432) only
- Allow DNS (UDP 53) for cluster resolution
- See: `deploy/helm/strata-rmm/templates/networkpolicy.yaml`

## Data Protection

### At Rest
- TimescaleDB: AES-256 encryption (cloud provider or LUKS)
- PostgreSQL: TDE or filesystem-level encryption
- Agent BBolt store: File permissions (0600), optional encryption

### Retention
- Raw metrics: 365 days (configurable per tenant)
- Aggregated metrics: indefinite (continuous aggregates)
- Alerts: 365 days
- Flow records: 30 days
- SNMP polls: 90 days
- Audit logs: 7 years (append-only)

### Backup
- TimescaleDB: `pg_dump` + WAL archiving to S3/GCS
- Schedule: Daily full + continuous WAL
- Retention: 30 days (production), 90 days (compliance)

## Incident Response

1. **Detect**: Alerting engine monitors platform health (internal tenant)
2. **Contain**: Isolate affected tenant namespace, revoke tokens
3. **Eradicate**: Rotate secrets, patch vulnerabilities
4. **Recover**: Restore from backup, validate data integrity
5. **Post-Mortem**: Document root cause, update runbook

## Audit

All API requests are logged with:
- Timestamp, source IP, user/token ID
- Resource accessed, action performed
- Success/failure status

Audit logs are immutable (append-only table, WORM storage).
