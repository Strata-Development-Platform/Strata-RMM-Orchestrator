# Compatibility Matrix

## Production Host Compatibility

| Component | Current | Target |
|-----------|---------|--------|
| API base URL | `https://rmm.stratadevplatform.com` | `https://rmm.stratadevplatform.com` (preserved) |
| API version | `/api/v1` | `/api/v1` (preserved) + `/api/v2` |
| Agent enrollment | `POST /api/v1/agent/register` | Preserved + scoped enrollment |
| Agent config | `POST /api/v1/agent/config` | Preserved |
| Install script | `GET /install.sh` | Preserved |
| Releases | `GET /releases/latest/agent/{os}/{arch}` | Preserved |
| Health | `GET /health` | Preserved |
| NATS URL | `nats://{host}:4222` | Preserved |

## Agent Compatibility

| Agent Version | Server Compatibility | Notes |
|---------------|---------------------|-------|
| Current (pre-rewrite) | ✅ | Will be preserved throughout rewrite |
| Post-rewrite v2 | ✅ | Forward-compatible with v1 subjects |

## Database Compatibility

| Migration ID | Description | Forward-Compatible |
|-------------|-------------|-------------------|
| 1-26 | All existing migrations | ✅ Additive only |
| 27+ | New SaaS model | ✅ New tables, backfill old data |
