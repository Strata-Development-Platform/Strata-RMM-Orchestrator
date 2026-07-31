# Phase 8C Evidence — Backup and Disaster Recovery

This document distinguishes implementation, automated verification, and operational acceptance. A green unit test is not an RPO/RTO drill.

## Requirements ledger

| ID | Implementation/runtime consumer | Negative proof | Integration proof | Status before final CI |
|---|---|---|---|---|
| BDR-01 | Independent file key provider; `recovery key-init`; runtime resolution by key ID | missing/wrong key, duplicate initialization, permissions | rotated-key tests | implemented; final CI pending |
| BDR-02 | Filesystem and S3 artifact repositories consumed by `backup.Engine` | malformed manifest, digest mismatch, path traversal | MinIO-backed S3 repository round trip | implemented; final CI pending |
| BDR-03 | `PostgreSQLRecovery` runs credential-safe `pg_dump`/`pg_restore` against distinct databases | same target, missing target, corrupted artifact, command failure redaction | seeded source to clean target; schema, tenants, devices, jobs, audit | implemented; final CI pending |
| BDR-04 | `JetStreamRecovery` captures streams, consumers, progress, headers, subjects, bytes | JetStream-disabled server, divergent non-empty target | real NATS delete/restore/verify/idempotent replay | implemented; final CI pending |
| BDR-05 | `ObjectStorageRecovery` archives exact bytes, content type, metadata, per-object digests | corrupt restored object, same-target rejection in CLI factory | real MinIO source/target restore and idempotent replay | implemented; final CI pending |
| BDR-06 | `backup.Engine` encrypts, persists, finalizes, verifies, decrypts, restores, and verifies all required components | tamper before mutation, missing component, failure cleanup | engine round trip plus service-backed component tests | implemented; final CI pending |
| BDR-07 | Migration 65 creates shared recovery mutation gate and database triggers | non-owner resume; write while quiesced | real PostgreSQL gate test | implemented; final CI pending |
| BDR-08 | Pinned session-level recovery lock | second operation times out; bounded release | real PostgreSQL concurrent lock test | implemented; final CI pending |
| BDR-09 | API and dispatcher acquire shared recovery locks; DB trigger covers other writers | API returns 503; direct SQL mutation fails | PostgreSQL quiesce test | implemented; final CI pending |
| BDR-10 | Restore verifies all artifacts before lock/quiesce/target mutation | ciphertext and manifest-associated-data tampering | engine tamper test | implemented; final CI pending |
| BDR-11 | CLI backup/preflight/status/verify/restore/key-init execute real runtime factories | missing confirmation/target/config; no `--force` | CLI build and command tests | implemented; final CI pending |
| BDR-12 | Source-independent verification and isolated restore target | unavailable source is not constructed for verify/restore | repository/key-only verification tests | implemented; final CI pending |
| BDR-13 | Authenticated encryption only; manifest identity is associated data | nonce, ciphertext, key, metadata, truncation tampering | adversarial encryption suite | implemented; final CI pending |
| BDR-14 | Exact-head CI and truthful evidence | stale/missing/non-terminal run rejected | Phase 8C and repository-wide workflows | pending final pushed head |
| BDR-15 | Beta RPO/RTO | model-only tests cannot satisfy gate | timestamped full recovery drill | **not accepted; operational drill required** |

## Runtime-wiring matrix

| Input | Validator | Runtime consumer | Failure behavior |
|---|---|---|---|
| `STRATA_BACKUP_ENVIRONMENT_ID` | required when enabled/CLI | authenticated manifest identity | preflight fails |
| `STRATA_BACKUP_KEY_PROVIDER_PATH` | active key required | file key provider | preflight fails |
| `STRATA_BACKUP_REPOSITORY_TYPE` | `filesystem`/`s3` | repository factory | preflight fails |
| filesystem/S3 repository settings | required by selected type | external artifact reads/writes | operation fails; set not finalized |
| source PostgreSQL DSN | normal DB validation | pg_dump, recovery control/gate | backup fails closed |
| source NATS/TLS/auth | production policy | JetStream backup | backup fails closed |
| source storage settings | normal storage validation | object backup | backup fails closed |
| `--target-dsn` | required, distinct identity | pg_restore target and target lock | restore fails before artifact mutation |
| recovery NATS settings | required/distinct | JetStream restore target | restore preflight fails |
| recovery storage settings | required/distinct when storage enabled | object restore target | restore preflight fails |

## Adversarial findings resolved in this remediation

| Severity | Original observation | Resolution / regression proof |
|---|---|---|
| blocker | backup CLI printed “pending” and returned success | CLI builds `backup.Engine` and executes backup; command tests |
| blocker | restore constructed nil dependencies and never restored | target runtime factory plus `Engine.Restore`; engine and service tests |
| blocker | PostgreSQL artifact was discarded | encrypted component is written/finalized in independent repository |
| blocker | JetStream/object backups fabricated metadata | real service clients and byte/message integration tests |
| blocker | transaction advisory lock was released immediately | pinned session-level lock; concurrent PostgreSQL test |
| blocker | quiesce only logged | DB mutation gate, shared advisory locks, trigger enforcement, bounded resume |
| high | associated-data replacement was accepted | expected manifest identity is recomputed and constant-time compared |
| high | filesystem backup IDs allowed traversal | repository identifier validation and traversal test |
| high | process arguments/errors could expose DSNs | `PGDATABASE` environment and redaction tests |
| high | previous CI used NATS without JetStream and structural tests | pinned real NATS `-js`, negative no-JS server, real MinIO/PostgreSQL jobs |
| high | documentation claimed automatic rollback/RPO/RTO | claims removed; isolated-target discard procedure and drill requirement documented |

## Local verification

| Check | Result | Notes |
|---|---|---|
| focused unit tests | pass | backup, recovery, repository, config, PostgreSQL, CLI, platform |
| full `go test ./...` | pass | Go 1.25.0 |
| integration-tag compile | pass | all Phase 8C integration files compile |
| real JetStream integration | pass | local pinned NATS 2.10.26 with JS and no-JS targets |
| real MinIO integration | not run locally | sandbox MinIO binary cannot enumerate interfaces; mandatory CI job remains pending |
| real PostgreSQL integration | not run locally | PostgreSQL server/client tools unavailable locally; mandatory CI job remains pending |
| gofmt/diff check | pass before push | exact command recorded in PR body |

## Operational acceptance still required

A8-06 through A8-08 can be supported by exact-head automated fixture evidence after CI. A8-09 remains unverified until an operator performs a timestamped isolated full-system drill, records actual recovery-point age and elapsed recovery time, and validates application-level tenant isolation.

## Pull request

| Field | Value |
|---|---|
| PR | [#9](https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/pull/9) |
| Branch | `agent/phase-8c-backup-disaster-recovery` |
| Base | `0368cbe5ef0a203ef2907d371e2fa4a8aadc1c6c` |
| Final head | pending push |
| Exact-head workflow | pending |
| State | draft; unmerged until final CI and review |
