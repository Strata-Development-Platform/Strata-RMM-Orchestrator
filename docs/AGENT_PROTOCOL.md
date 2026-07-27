# Agent Protocol

## Current Protocol

### Enrollment

1. Agent receives `deployment_id` (installed or via token)
2. Agent calls `POST /api/v1/agent/register` with hostname, OS, arch, public key
3. Server creates device record, returns `device_id`, `agent_id`, `token`, `nats_urls`
4. Agent connects to NATS and begins publishing heartbeats

### NATS Subjects

#### Agent → Platform (Publish)

| Subject | Payload | Frequency |
|---------|---------|-----------|
| `tenant.{tenantID}.agent.{agentID}.heartbeat` | Status + timestamp | Every 60s |
| `tenant.{tenantID}.agent.{agentID}.metrics` | CPU, memory, disk, net samples | Every 60s |
| `tenant.{tenantID}.agent.{agentID}.events` | System events | On occurrence |
| `tenant.{tenantID}.agent.{agentID}.script.result` | Script output | After execution |
| `tenant.{tenantID}.agent.{agentID}.software.result` | Install/uninstall result | After execution |
| `tenant.{tenantID}.agent.{agentID}.patch.result` | Patch result | After patching |
| `tenant.{tenantID}.agent.{agentID}.patch.inventory` | Installed patches | On schedule |

#### Platform → Agent (Subscribe)

| Subject | Action |
|---------|--------|
| `tenant.{tenantID}.cmd.{agentID}` | Command dispatch |
| `tenant.{tenantID}.config.{agentID}` | Configuration update |
| `tenant.{tenantID}.rollout.{agentID}` | Update rollout |
| `tenant.{tenantID}.tunnel.{sessionID}.input` | Remote input events |
| `tenant.{tenantID}.tunnel.{sessionID}.ctrl` | Remote control events |

### Future Protocol

After Phase 4, a new subject namespace will be introduced:

```
msp.{mspID}.client.{clientID}.device.{deviceID}.>
```

Legacy `tenant.*` subjects will be preserved through a compatibility adapter.
