# Hosted internal-alpha end-to-end evidence

Candidate base: `05e6a9e8b1cc7a17177e9445c9670fccdcb95e57`.

The broader product-completeness audit is maintained in
[`FEATURE_COMPLETENESS_MATRIX.md`](FEATURE_COMPLETENESS_MATRIX.md). Hosted
internal-alpha acceptance is intentionally narrower and must not be used as
evidence that every capability in `FEATURE_SPECIFICATION.md` is complete.

This record separates repository capability, automated execution, and hosted
acceptance. A row is not accepted merely because its implementation exists.
Exact-head run URLs and terminal results must be added to the draft pull request
after the candidate head stops changing.

## Audit result

The repository already provides database-backed provider setup, MSP owner
activation, client/site lifecycle, enrollment, tenant isolation, telemetry
deduplication, durable-job, agent identity, bounded queue, NATS replay,
deployment idempotency, installer parsing, and security pipeline tests. The
smallest blocking defect in the authoritative clean-install path was that it
disabled object storage and polled an unregistered `/ready` route. This change
configures a pinned MinIO service with file-backed credentials and polls the
registered `/health/ready` endpoint.

## Traceability and evidence boundary

| Exercise | Existing executable evidence | Candidate addition | Acceptance state |
|---|---|---|---|
| Clean isolated installation | `scripts/install-platform.sh`, `deploy/docker/docker-compose.install.yml`, Phase 8B deployment jobs | Generate the authoritative configuration and start/probe its isolated MinIO dependency | Pending exact-head hosted run; the new job does not claim a public ACME issuance |
| Administrator bootstrap and provider setup | bootstrap command; `provider_profile_integration_test.go`; provider Playwright suite | No duplicate implementation | Pending one continuous hosted API/browser exercise |
| Provider-created MSP and owner activation | `owner_invitations_integration_test.go` and UI component tests | No duplicate implementation | Pending live mailbox delivery and activation in hosted environment |
| MSP client/site and scoped enrollment | `phase8f_integration_test.go`, `agent_enrollment_test.go`, lifecycle browser contracts | No duplicate implementation | Pending continuous hosted exercise |
| Agent install, enrollment, identity restart | agent core tests and Linux/Windows installers | Authoritative storage/config contract only | Linux systemd host execution and Windows service host execution remain required |
| PostgreSQL/TimescaleDB, JetStream, object storage | PostgreSQL integration workflows, live NATS replay, Phase 8C MinIO integrations | Object storage is now part of the authoritative install and receives a live health probe | Pending full topology clean-install run |
| Telemetry, outage, bounded queue, reconnect/replay/duplicates | monitoring identity tests, BoltDB queue tests, live NATS replay test, database duplicate-ingestion test | No duplicate implementation | Pending continuous process-level hosted exercise and dashboard observation |
| Harmless durable job and visible result | durable-job and endpoint-operation integration jobs | No duplicate implementation | Pending continuous process-level hosted exercise |
| Cross-tenant/cross-endpoint denials | isolation, scope-authorization, enrollment and remote binding suites | No duplicate implementation | Pending hosted adversarial run |
| Orchestrator restart/readiness and same-version redeploy | health contracts and Phase 8B idempotency jobs | Correct readiness route in authoritative installer | Pending full topology restart/redeploy evidence |
| Native Linux systemd | package and unit validation | None | Not executed on a representative systemd host yet |
| Windows PowerShell/service | parse contract and Windows builds | None | Parse can run in CI; service installation needs a Windows host that can reach the isolated control plane |
| Security and supply chain | Phase 8G secret, vulnerability, dependency, image, SBOM and container jobs | File-backed object-storage secrets | Pending exact-head terminal workflows |

## Migration, compatibility, and rollback

There is no schema migration or protocol change. Existing installations with
`STORAGE_BACKEND=none` remain valid; the behavioral change applies to newly
generated authoritative Docker installations. Agent identity and enrollment
formats are unchanged.

Rollback is to redeploy the prior compose/install files. If MinIO has already
accepted objects, retain the `minio_data` volume and its generated credential
files until the objects are exported or the installation is intentionally
destroyed. Do not remove that volume as an application rollback step.

## Remaining terminal evidence

- exact-head workflow URLs and per-job conclusions;
- one uninterrupted hosted lifecycle record from bootstrap through agent/job
  result, including broker and orchestrator restart timestamps;
- Linux systemd execution record;
- Windows service execution record when a connected Windows host is available;
- dashboard capture proving ingested telemetry and durable result visibility;
- limitations and any single external-transient rerun recorded in the draft PR.

Until those items exist, this document and the draft PR must not describe the
hosted internal-alpha gap as closed.
