# Internal-alpha endpoint agent acceptance status

The endpoint agent is intended for controlled, ephemeral internal-alpha environments only. It is not evidence of hosted or production operational readiness.

## Verified locally by package tests

- Enrollment consumes a database-backed, expiring, bounded-use credential in the registration transaction.
- The issued application credential is bound to a tenant and endpoint, and the generated NATS subject policy contains only endpoint-specific subjects.
- Identity state is stored with owner-only file modes on Unix. Incomplete, corrupt, certificate/key-mismatched, or tenant-mismatched state fails closed instead of silently creating a replacement identity.
- The BoltDB offline queue survives restart, retains entries until publish acknowledgement, and enforces a combined metric/event capacity (`store.queue_max_items`, default `10000`). Capacity exhaustion rejects new entries rather than allowing unbounded growth.
- Production configuration rejects missing, plaintext, credential-bearing, loopback, and container-local advertised NATS URLs.
- Remote session input and stop operations must match the tenant, device, and agent binding established when the session started. Bindings intentionally disappear on orchestrator restart, causing old sessions to fail closed.
- Linux and Windows installers validate TLS, verify SHA-256 checksums, bound download attempts, preserve the data directory during binary replacement, and fail if enrollment material remains in the runtime configuration.

## Supported internal-alpha matrix

| Platform | Architecture | Build coverage | Installer coverage | Status |
| --- | --- | --- | --- | --- |
| Linux | AMD64 | Native CI build | Shell syntax/static contract; isolated systemd execution still required | Partial |
| Linux | ARM64 | Cross-compile CI build | Shell syntax/static contract; ARM64 host execution still required | Partial |
| Windows | AMD64 | Cross-compile CI build | PowerShell parse/static contract; Windows service execution still required | Partial |
| Windows | ARM64 | Cross-compile CI build | PowerShell parse/static contract; Windows ARM64 service execution still required | Partial |
| macOS | AMD64/ARM64 | Cross-compile CI build | No installer | Partial |

## Mandatory hosted-alpha prerequisites

Before promotion, capture exact-head CI evidence and run the end-to-end exercise against ephemeral PostgreSQL/TimescaleDB, NATS with JetStream, and object storage. The exercise must cover enrollment replay, process restarts, a broker outage and queue replay, duplicate ingestion, harmless job dispatch, and cross-tenant/cross-endpoint denials. Linux systemd and Windows service installation must also execute on representative supported hosts. Vulnerability, container, and secret scans must complete successfully.

Release downloads currently resolve the configured GitHub release before being cached and served with a same-origin checksum. Promotion requires pinning and recording immutable release/tag identity in deployment evidence; a mutable `latest` response must not be treated as authoritative provenance.

## Rollback

Stop and disable the agent service, restore the previously verified agent binary, and retain the data directory and identity directory. Restart only after the prior binary checksum and configuration have been verified. Revoking the endpoint credential and re-enrolling creates a new endpoint identity; do not delete identity state as a routine binary rollback step.
