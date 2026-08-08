# Strata RMM — Durable Job System Reference

**Version:** 2026-08-08
**Last Updated: 2026-08-08

---

## 1. Job System Overview

Strata's durable job system provides reliable task execution with persistence, retry, and event tracking.

---

## 2. Job Types

| Type | Description |
|------|-------------|
| Script Execution | Run script on device |
| Software Deploy | Deploy software package |
| Patch Apply | Apply OS patches |
| Custom | User-defined jobs |

---

## 3. Job Lifecycle

```
Created → Queued → Running → Completed/Failed
                          ↓
                        Retrying (if configured)
                          ↓
                        Completed/Failed/Cancelled
```

---

## 4. Job API

### 4.1 Create Job

```bash
curl -X POST https://strata.example.com/api/v1/jobs \
  -H "Authorization: Bearer {token}" \
  -d '{
    "type": "script_execution",
    "target": "device-123",
    "payload": {"scriptID": "script-456", "args": "arg1 arg2"}
  }'
```

### 4.2 List Jobs

```bash
curl -X GET https://strata.example.com/api/v1/jobs \
  -H "Authorization: Bearer {token}"
```

### 4.3 Get Job

```bash
curl -X GET https://strata.example.com/api/v1/jobs/{jobID} \
  -H "Authorization: Bearer {token}"
```

### 4.4 Get Job Events

```bash
curl -X GET https://strata.example.com/api/v1/jobs/{jobID}/events \
  -H "Authorization: Bearer {token}"
```

### 4.5 Cancel Job

```bash
curl -X POST https://strata.example.com/api/v1/jobs/{jobID}/cancel \
  -H "Authorization: Bearer {token}"
```

### 4.6 Retry Job

```bash
curl -X POST https://strata.example.com/api/v1/jobs/{jobID}/retry \
  -H "Authorization: Bearer {token}"
```

---

## 5. Job Data Model

```go
type Job struct {
    ID            string      `json:"id"`
    Type          string      `json:"type"`
    Status        string      `json:"status"`
    TargetID      string      `json:"target_id"`
    Payload       JSONB       `json:"payload"`
    RetryCount    int         `json:"retry_count"`
    MaxRetries    int         `json:"max_retries"`
    CreatedAt     time.Time   `json:"created_at"`
    UpdatedAt     time.Time   `json:"updated_at"`
    CompletedAt   *time.Time  `json:"completed_at"`
}
```

### 5.1 Job Events

```go
type JobEvent struct {
    ID        string    `json:"id"`
    JobID     string    `json:"job_id"`
    Type      string    `json:"type"`
    Payload   JSONB     `json:"payload"`
    CreatedAt time.Time `json:"created_at"`
}
```

---

## 6. Job Storage

### 6.1 Database Schema

```sql
CREATE TABLE jobs (
    id            UUID PRIMARY KEY,
    type          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'created',
    target_id     UUID,
    payload       JSONB,
    retry_count   INTEGER DEFAULT 0,
    max_retries   INTEGER DEFAULT 3,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ
);

CREATE TABLE job_events (
    id            UUID PRIMARY KEY,
    job_id        UUID NOT NULL REFERENCES jobs(id),
    type          TEXT NOT NULL,
    payload       JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 6.2 Indexes

| Index | Columns | Purpose |
|-------|---------|---------|
| `jobs_status_idx` | `status` | Job status queries |
| `jobs_target_idx` | `target_id` | Target-specific queries |
| `job_events_job_idx` | `job_id` | Job event queries |

---

## 7. Job Execution

### 7.1 Dispatch

Jobs are dispatched via NATS:

```go
// Dispatch job to agent
subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, deviceID)
payload := map[string]interface{}{
    "type":   "job",
    "jobID":  job.ID,
    "payload": job.Payload,
}
nc.Publish(subject, payload)
```

### 7.2 Result Processing

Agent reports job results back:

```go
// Subscribe to job results
subject := fmt.Sprintf("tenant.%s.agent.%s.job.result", tenantID, agentID)
nc.Subscribe(subject, handleJobResult)
```

---

## 8. Job Scheduling

### 8.1 Recurring Schedules

Scripts support recurring schedules:

```bash
curl -X POST https://strata.example.com/api/v1/tenants/{tenantID}/scripts/schedule \
  -H "Authorization: Bearer {token}" \
  -d '{
    "scriptID": "script-123",
    "schedule": "0 2 * * *",
    "devices": ["device-1", "device-2"]
  }'
```

### 8.2 Schedule Types

| Type | Cron Example | Description |
|------|--------------|-------------|
| Hourly | `0 * * * *` | Every hour |
| Daily | `0 2 * * *` | Daily at 2am |
| Weekly | `0 2 * * 0` | Sunday at 2am |
| Monthly | `0 2 1 * *` | 1st of month at 2am |

---

## 9. Job Events

### 9.1 Event Types

| Event | Description |
|-------|-------------|
| `created` | Job created |
| `queued` | Job queued for execution |
| `dispatched` | Job dispatched to agent |
| `running` | Job running on agent |
| `completed` | Job completed successfully |
| `failed` | Job failed |
| `cancelled` | Job cancelled |
| `retrying` | Job retrying |

### 9.2 Event Payload

```json
{
  "type": "completed",
  "exit_code": 0,
  "output": "Script output...",
  "duration_ms": 1234
}
```

---

## 10. Error Handling

### 10.1 Retry Logic

| Condition | Action |
|-----------|--------|
| Transient error | Retry up to max_retries |
| Permanent error | Mark as failed |
| Timeout | Mark as failed |

### 10.2 Retry Backoff

```
Retry 1: 10 seconds
Retry 2: 30 seconds
Retry 3: 60 seconds
```

---

## 11. Monitoring

### 11.1 Job Metrics

| Metric | Description |
|--------|-------------|
| Jobs created | Total jobs created |
| Jobs completed | Total jobs completed |
| Jobs failed | Total jobs failed |
| Jobs active | Currently running jobs |

### 11.2 Job Health

```bash
# Check job health
curl -X GET https://strata.example.com/api/v1/jobs \
  -H "Authorization: Bearer {token}" \
  -d '{"status": "running"}'
```

---

## 12. Troubleshooting

### 12.1 Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| Job stuck in `running` | Agent disconnected | Check agent connectivity |
| Job failed | Script error | Check script output |
| Job not dispatched | NATS issue | Check NATS connectivity |

---

*Last Updated: 2026-08-08*
