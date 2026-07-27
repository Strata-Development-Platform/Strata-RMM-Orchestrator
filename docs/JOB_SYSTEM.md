# Job System

## Delivery and recovery contract

Phase 5 uses at-least-once transport with idempotent processing:

1. The orchestrator writes the command to PostgreSQL before publication.
2. An agent validates tenant and agent ownership, then durably records the
   command in its BoltDB receipt ledger before publishing `accepted`.
3. The agent persists the complete immutable result envelope before publishing
   it. A publish failure leaves the envelope pending for restart replay.
4. The orchestrator atomically claims each `(msp_id, message_id)`, validates the
   subject and stored target identity, updates target/job state, and marks the
   inbox row processed in one serializable PostgreSQL transaction.
5. Only after that transaction commits does the orchestrator publish a result
   receipt on `tenant.<msp>.agent.<agent>.result.ack`.
6. The agent retains and periodically replays a result until that receipt is
   durably stored.

Duplicate commands never execute twice. Duplicate results are harmless and
still receive a result receipt so agent retransmission terminates.

Each dispatched command carries a deterministic `event_id` scoped to the job,
target, and attempt, plus a `command_type` consumed by the agent registry.

Running commands receive cancellation on
`tenant.<msp>.cmd.<agent>.cancel`. Handlers inherit the agent shutdown context,
the command expiry deadline, and explicit job cancellation.

Operational recovery:

- Restarting an agent replays all complete, unacknowledged result envelopes.
- Restarting the orchestrator is safe because inbox claims and state changes
  commit atomically.
- Never delete an unacknowledged ledger record during cleanup.
- Investigate repeated pending results as a NATS connectivity or server inbox
  processing failure; do not manually mark them successful.

CI exercises the protocol against real PostgreSQL and NATS containers with the
`jobintegration` build tag. It covers the complete round trip, duplicate
delivery, restart replay, result receipts, and running-command cancellation.

## Overview

The durable job system provides reliable, asynchronous execution of operations across managed devices. It supports dispatch via NATS, per-device status tracking, retry with backoff, idempotency enforcement, and automatic expiration.

---

## Job States

### State Machine

```
Queued -> Dispatched -> Acknowledged -> Running -> Succeeded
                                               |-> Failed
                                               |-> Cancelled
```

### State Descriptions

| State | Description | Transitions To |
|-------|-------------|----------------|
| `queued` | Job created and persisted to database, not yet dispatched | `dispatched`, `cancelled` |
| `dispatched` | NATS message published to target devices | `acknowledged`, `failed`, `cancelled` |
| `acknowledged` | Agent received the command and is processing it | `running`, `failed` |
| `running` | Agent is actively executing the operation | `succeeded`, `failed` |
| `succeeded` | Operation completed successfully on all targets | (terminal) |
| `failed` | Operation failed on one or more targets (after retries exhausted) | (terminal) |
| `cancelled` | Job was cancelled by user before completion | (terminal) |

### State Diagram

```
        ┌──────────┐
        │  Queued  │
        └────┬─────┘
             │ dispatch
             ▼
      ┌──────────────┐
      │  Dispatched   │
      └──────┬───────┘
             │ acknowledge
             ▼
     ┌────────────────┐
     │ Acknowledged    │
     └───────┬────────┘
             │ start execution
             ▼
      ┌────────────┐      ┌───────────┐
      │  Running   │ ───> │ Succeeded │
      └──────┬─────┘      └───────────┘
             │ error
             ▼
       ┌────────┐
       │ Failed │
       └────────┘

Cancelled may transition from: Queued, Dispatched, Acknowledged
```

---

## Job Targets with Per-Device Status

### Model

Each job targets multiple devices via `job_targets` table. Each target maintains its own status independent of the job-level status.

### `job_targets` Schema

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID | Target record ID |
| `job_id` | UUID | Parent job ID |
| `device_id` | UUID | Target device |
| `status` | text | pending, queued, dispatched, running, acknowledged, succeeded, failed |
| `error_message` | text | Error details if failed |
| `result` | jsonb | Execution result data |
| `attempts` | int | Number of retry attempts |
| `started_at` | timestamptz | When execution started |
| `completed_at` | timestamptz | When execution completed |
| `created_at` | timestamptz | Record creation time |

### Job Completion Logic

The job-level status transitions to `completed` only when ALL targets have reached a terminal state (succeeded, failed, or cancelled). Intermediate transitions update `completed_count` and `failed_count` on the `jobs` table:

```sql
UPDATE jobs SET
  completed_count = (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status = 'succeeded'),
  failed_count = (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status = 'failed'),
  status = CASE
    WHEN (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status IN ('pending','queued','dispatched','running')) = 0
    THEN 'completed' ELSE status END,
  completed_at = CASE
    WHEN (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status IN ('pending','queued','dispatched','running')) = 0
    THEN NOW() ELSE NULL END
WHERE id = $1
```

---

## Retry Limits

### Retry Configuration

- **Default max retries**: 3
- **Max allowed**: 10
- **Per-job**: Set via `max_retries` field on job creation
- **Per-target**: Tracked via `attempts` counter in `job_targets`

### Retry Policy

| Attempt | Backoff Duration | Behavior |
|---------|-----------------|----------|
| 1 | 30s | Immediate retry after failure |
| 2 | 2m | Wait 2 minutes |
| 3 | 5m | Wait 5 minutes |
| 4 | 15m | Wait 15 minutes |
| 5 | 30m | Wait 30 minutes |
| 6+ | 1h | Cap at 1 hour between retries |

### Retry Flow

1. Agent reports failure via NATS result subject.
2. Platform updates `job_targets.attempts += 1`.
3. If `attempts < max_retries`, re-dispatch after backoff.
4. If `attempts >= max_retries`, mark target as `failed` and proceed.

---

## Idempotency

### Purpose
Prevent duplicate execution of the same operation when NATS messages are delivered more than once (at-least-once semantics).

### Mechanism

- **Idempotency Key**: Client-generated unique string sent with job creation.
- **Storage**: Key is stored in `jobs.idempotency_key` with a UNIQUE constraint.
- **Check**: Before creating a new job, platform checks if the idempotency key already exists.
- **Conflict Response**: If key exists, return the existing job ID rather than creating a duplicate.

### Scope

- Idempotency keys are scoped per MSP+Client.
- Keys expire after 24 hours (matches job expiration).
- Supported for: script execution, software deployment, patch deployment, custom commands.

### Example

```http
POST /api/v1/jobs
Content-Type: application/json
X-MSP-ID: <msp_id>

{
  "type": "script_exec",
  "device_ids": ["device-1"],
  "payload": {},
  "idempotency_key": "unique-op-identifier-abc123"
}
```

On retry with same key:

```http
HTTP/1.1 409 Conflict
{
  "error": "job already exists",
  "existing_job_id": "existing-job-uuid",
  "status": "queued"
}
```

---

## Expiration

### Job Expiration

- Default TTL: 24 hours from creation.
- Configurable per job via `expires_at` field.
- Expired jobs are automatically cancelled by a background cleanup goroutine.
- Expired targets that were never dispatched are marked as `failed` with reason "job expired".

### Cleanup

```sql
-- Background job (runs every 15 minutes)
UPDATE jobs SET status = 'cancelled', completed_at = NOW()
WHERE status IN ('pending', 'queued', 'dispatched')
  AND expires_at < NOW();
```

---

## NATS Dispatch Mechanism

### Dispatch Flow

```
1. Client  ──POST /api/v1/jobs──>  API Server
2. API Server  ──INSERT jobs + job_targets──>  PostgreSQL
3. API Server  ──nats.Publish──>  NATS (tenant.*.cmd.<device_id>)
4. Agent  ──Subscribe──>  NATS (receives command)
5. Agent  ──executes──>  Local execution (script/software/patch/etc.)
6. Agent  ──nats.Publish──>  NATS (tenant.*.agent.*.result subject)
7. API Server  ──NATS subscription──>  Updates job_targets status
8. API Server  ──UPDATE jobs──>  PostgreSQL (aggregate status)
```

### NATS Subjects

| Subject Pattern | Direction | Purpose |
|----------------|-----------|---------|
| `tenant.{msp_id}.cmd.{device_id}` | Platform -> Agent | Job command dispatch |
| `tenant.{msp_id}.agent.{device_id}.script.result` | Agent -> Platform | Script execution result |
| `tenant.{msp_id}.agent.{device_id}.software.result` | Agent -> Platform | Software install result |
| `tenant.{msp_id}.agent.{device_id}.patch.result` | Agent -> Platform | Patch install result |

### Command Payload

```json
{
  "job_id": "uuid",
  "type": "script_exec|software_install|patch_install|custom",
  "payload": { ... }
}
```

### Result Payload

```json
{
  "job_id": "uuid",
  "device_id": "uuid",
  "status": "succeeded|failed",
  "error": ""
}
```

---

## Compatibility with Existing Systems

### Script Dispatch

- Uses same NATS command/result subjects (`tenant.*.cmd.*`, `tenant.*.agent.*.script.result`).
- Existing `ScriptCommand`/`ScriptResult` structures map directly to job payload/result.
- Scripts can be dispatched as jobs for retry, idempotency, and tracking benefits.

### Software Dispatch

- Uses same NATS pattern (`tenant.*.cmd.*`, `tenant.*.agent.*.software.result`).
- Existing `SoftwareCommand`/`SoftwareResult` structures integrate with job system.
- Job system adds retry, expiration, and multi-target orchestration.

### Patch Dispatch

- Patch manager already uses NATS dispatches via `patch_install` commands.
- Job system integration provides idempotency and per-device retry tracking.
- `patch.result` subject feeds into both the patch manager and job system simultaneously.

### Migration Path

1. Existing ad-hoc dispatches continue working without job system.
2. New "Create Job" API wraps any dispatch type with job tracking.
3. Backward compatibility maintained with existing NATS subjects.
4. Job system optionally retroactive: existing dispatches can be registered as jobs.
