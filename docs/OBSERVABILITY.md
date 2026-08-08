# Strata RMM — Observability Reference

**Version:** 2026-08-08
**Last Updated: 2026-08-08

---

## 1. Observability Overview

Strata provides three pillars of observability: metrics, logs, and health checks.

---

## 2. Health Checks

### 2.1 Endpoints

| Endpoint | Purpose | Auth |
|----------|---------|------|
| `GET /health` | Readiness (DB, NATS, storage) | None |
| `GET /health/live` | Liveness (process alive) | None |

### 2.2 Health Response

```json
{
  "status": "ready",
  "database": "connected",
  "nats": "connected",
  "storage": "connected",
  "uptime": "1h23m"
}
```

### 2.3 Health Failures

| Component | Failure State |
|-----------|---------------|
| Database | `status: degraded` |
| NATS | `status: degraded` |
| Storage | `status: degraded` |
| All | `status: unhealthy` |

---

## 3. Metrics

### 3.1 Prometheus Metrics

```bash
# Get metrics
curl https://strata.example.com/metrics \
  -H "Authorization: Bearer {metrics-token}"
```

### 3.2 Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `strata_db_connections_open` | Gauge | Open DB connections |
| `strata_db_connections_in_use` | Gauge | In-use DB connections |
| `strata_nats_connected` | Gauge | NATS connection status |
| `strata_nats_reconnects_total` | Counter | NATS reconnect count |
| `strata_requests_total` | Counter | Total HTTP requests |
| `strata_request_duration_seconds` | Histogram | Request duration |
| `strata_errors_total` | Counter | Total errors |

### 3.3 Metrics Token

```bash
# Set metrics token
export STRATA_METRICS_TOKEN=your-32-char-metrics-token

# Access metrics
curl https://strata.example.com/metrics \
  -H "Authorization: Bearer your-32-char-metrics-token"
```

---

## 4. Logging

### 4.1 Structured Logging

```go
// zap JSON logging
logger := zap.NewProduction()
logger.Info("request started",
    zap.String("method", "GET"),
    zap.String("path", "/api/v1/devices"),
    zap.String("tenant_id", "tenant-123"),
)
```

### 4.2 Log Levels

| Level | Usage |
|-------|-------|
| Debug | Development troubleshooting |
| Info | Normal operation |
| Warn | Recoverable issues |
| Error | Failures requiring attention |
| Fatal | Unrecoverable errors |

### 4.3 Log Context

All logs include:
- Request ID
- Tenant ID
- User ID (if authenticated)
- Timestamp

---

## 5. Grafana Dashboards

### 5.1 Provisioned Dashboards

| Dashboard | Description |
|-----------|-------------|
| Platform Overview | System health, metrics |
| Device Health | Device status, telemetry |
| Alert Summary | Active alerts, trends |
| Patch Status | Patch deployment status |

### 5.2 Dashboard Data Sources

| Data Source | Type |
|-------------|------|
| Prometheus | Metrics |
| Loki | Logs |
| PostgreSQL | Business data |

---

## 6. Synthetic Monitoring

### 6.1 Check Types

| Check | Description |
|-------|-------------|
| HTTP | URL availability, response time |
| TCP | Port reachability |
| DNS | Resolution verification |
| ICMP | Ping reachability |

### 6.2 Synthetic Runner

```go
// synthetic/runner.go
func (r *Runner) RunChecks(ctx context.Context) {
    for _, check := range r.checks {
        result := r.executeCheck(check)
        r.reportResult(result)
    }
}
```

---

## 7. Alerting Integration

### 7.1 Alert Channels

| Channel | Configuration |
|---------|---------------|
| Email | SMTP host, port, credentials |
| Slack | Webhook URL |
| Teams | Webhook URL |
| PagerDuty | Integration key |
| Generic Webhook | Webhook URL |

### 7.2 Alert Delivery

```bash
# Configure alert channels
STRATA_ALERT_SLACK_URL=https://hooks.slack.com/...
STRATA_ALERT_TEAMS_URL=https://teams.webhook/...
STRATA_ALERT_PAGERDUTY_KEY=PD_KEY
STRATA_ALERT_EMAIL_RECIPIENTS=admin@example.com
```

---

## 8. Audit Logging

### 8.1 Audit Trail

All significant events are logged to `audit_log` table:

| Field | Description |
|-------|-------------|
| `id` | Unique identifier |
| `tenant_id` | Tenant scope |
| `user_id` | Actor user |
| `action` | Operation performed |
| `resource_type` | Target resource |
| `resource_id` | Target resource ID |
| `metadata` | Additional context |
| `created_at` | Timestamp |

### 8.2 Audit Queries

```bash
# Get audit log
curl -X GET https://strata.example.com/api/v1/access/audit/{tenantID} \
  -H "Authorization: Bearer {token}"
```

---

## 9. Performance Monitoring

### 9.1 Database Performance

| Metric | Description |
|--------|-------------|
| Query duration | Slow query tracking |
| Connection pool | Pool utilization |
| Table sizes | Storage growth |

### 9.2 NATS Performance

| Metric | Description |
|--------|-------------|
| Message rate | Messages per second |
| Consumer lag | Message backlog |
| Reconnect count | Connection stability |

### 9.3 API Performance

| Metric | Description |
|--------|-------------|
| Request duration | P50, P95, P99 |
| Error rate | 5xx responses |
| Throughput | Requests per second |

---

## 10. Troubleshooting

### 10.1 Health Check Failures

| Issue | Cause | Solution |
|-------|-------|----------|
| `/health` fails | DB connection error | Check DB DSN |
| `/health` fails | NATS connection error | Check NATS URL |
| `/health` fails | Storage connection error | Check storage config |

### 10.2 Metrics Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| No metrics | Token not set | Set `STRATA_METRICS_TOKEN` |
| Metrics empty | No requests | Check API usage |
| High error rate | Application error | Check logs |

---

*Last Updated: 2026-08-08*
