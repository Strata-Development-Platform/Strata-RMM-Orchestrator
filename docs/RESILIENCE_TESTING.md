# Strata RMM — Resilience Testing

**Version:** 2026-08-08
**Last Updated:** 2026-08-08

---

## 1. Resilience Testing Overview

Strata implements resilience patterns to handle failures gracefully. This document describes the implemented patterns and test coverage.

---

## 2. Resilience Patterns

### 2.1 Circuit Breaker

`internal/resilience/load.go` implements circuit breaker logic:
- **Closed:** Normal operation, requests pass through
- **Open:** Failures detected, requests blocked
- **Half-Open:** Limited requests allowed to test recovery

### 2.2 Retry Logic

| Component | Retry Strategy |
|-----------|----------------|
| NATS reconnect | Configurable (`NATS_MAX_RECONNECTS`, `NATS_RECONNECT_WAIT`) |
| DB connections | Automatic retry with connection pool |
| HTTP requests | Timeout-based (read/write/idle timeouts) |
| Agent commands | Job system retry (max_retries) |

### 2.3 Graceful Degradation

| Component | Degradation |
|-----------|-------------|
| Storage | Operations fail gracefully, non-critical features disabled |
| NATS | Metrics queued locally, replay on reconnect |
| DB | Connection pool retry, read replica failover |

### 2.4 Queue Persistence

Agent uses BBolt local store for offline persistence:
- Metrics queued during disconnect
- Events queued during disconnect
- Commands acknowledged after delivery

---

## 3. Resilience Tests

### 3.1 CI Workflows

| Workflow | Status | Description |
|----------|--------|-------------|
| Phase 8B — Injected Failure | ✅ Pass | Tests failure injection handling |
| Phase 8E — Resilience Validation | ✅ Pass | Tests resilience patterns |
| Phase 8B — Rollback Restoration | ✅ Pass | Tests rollback after failure |
| Phase 8B — Same-Version Idempotency | ✅ Pass | Tests idempotent upgrades |
| Phase 8B — Forward Upgrade | ✅ Pass | Tests forward upgrades |

### 3.2 Test Scenarios

| Scenario | Test | Expected |
|----------|------|----------|
| NATS disconnect | Agent reconnect queue | Metrics replay on reconnect |
| DB connection loss | Connection pool retry | Service continues |
| Storage unavailable | Graceful error | Non-critical features disabled |
| Agent offline | BBolt queue | Data preserved, replay on reconnect |
| Partial upgrade | Schema migration | Migration rolls back on failure |

---

## 4. Load Testing

### 4.1 Load Testing Framework

Uses vegeta-based load testing:
```bash
# Example load test
echo "GET http://localhost:8080/health" | vegeta attack -duration=30s -rate=100
```

### 4.2 Agent Simulation

Simulates agent telemetry:
```bash
# Simulate 1000 agents sending metrics
for i in {1..1000}; do
    curl -X POST http://localhost:8080/api/v1/agent/register \
        -H "Authorization: Bearer $token"
done
```

### 4.3 Metrics Ingestion Rate

| Metric | Target |
|--------|--------|
| Messages/second | 10,000+ |
| Batch write latency | <100ms |
| Continuous aggregate refresh | Every 1 minute |

---

## 5. Chaos Engineering

### 5.1 Chaos Scenarios

| Scenario | Tool | Frequency |
|----------|------|-----------|
| Kill orchestrator | `docker kill` | Monthly |
| Kill NATS | `docker kill nats` | Monthly |
| Kill DB | `docker kill postgres` | Quarterly |
| Network partition | `tc` | Quarterly |
| Disk full | `fallocate` | Quarterly |

### 5.2 GameDays

Regular chaos engineering exercises:
1. Define scenario
2. Execute during maintenance window
3. Monitor response
4. Document results
5. Update procedures

---

## 6. Recovery Testing

### 6.1 Backup/Restore Test

| Step | Command |
|------|---------|
| 1. Create test data | `curl -X POST /api/v1/admin/users` |
| 2. Run backup | `./strata backup run` |
| 3. Destroy database | `dropdb strata_rmm` |
| 4. Restore backup | `./strata recovery restore` |
| 5. Verify data | `curl -X GET /api/v1/admin/users` |

### 6.2 Disaster Recovery Test

| Step | Command |
|------|---------|
| 1. Simulate full outage | Stop all services |
| 2. Provision new infrastructure | Deploy fresh stack |
| 3. Restore from backup | Run recovery |
| 4. Verify data integrity | Run validation |
| 5. Resume operations | Start services |

---

## 7. Monitoring Resilience

### 7.1 Key Metrics

| Metric | Alert Threshold |
|--------|-----------------|
| Agent reconnect rate | >10/minute |
| DB connection pool exhaustion | >90% |
| NATS consumer lag | >1000 messages |
| Job failure rate | >5% |
| API error rate | >1% |

### 7.2 Health Checks

| Check | Endpoint | Failure Action |
|-------|----------|----------------|
| Liveness | `/health/live` | Restart pod |
| Readiness | `/health/ready` | Remove from LB |
| NATS | Internal | Alert on disconnect |
| DB | Internal | Alert on connection loss |

---

*Last Updated: 2026-08-08*
