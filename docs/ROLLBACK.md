# Strata RMM — Recovery and Rollback Reference

**Last updated:** 2026-08-20

Rollback is not a manual version-switching shortcut. Strata RMM treats rollback as a controlled recovery action owned by the verified upgrade transaction for the deployment mode that performed the upgrade. The goal is to restore a previously proven application identity and, where required, the corresponding database recovery point without weakening provenance or tenant isolation.

For the normal upgrade procedure, start with [UPGRADE.md](UPGRADE.md). For backup and full restore operations, use [BACKUP.md](BACKUP.md), [RESTORE.md](RESTORE.md), and [DISASTER_RECOVERY.md](DISASTER_RECOVERY.md).

## Core rules

- Never use a mutable image tag as rollback identity.
- Never copy an arbitrary old executable over the installed orchestrator.
- Never run individual migration `.down.sql` files as a normal rollback procedure.
- Never delete or edit protected upgrade/recovery state to force another attempt.
- Never claim Kubernetes, patch, software, or agent rollback semantics that are not implemented and tested for that subsystem.
- Preserve tenant data, endpoint identities, object-storage data, durable jobs, credentials, audit history, and recovery evidence.
- A rollback is complete only after the previous application identity and required database state are restored and readiness passes.

## Native/bare-metal failed-upgrade rollback

The supported native updater stages a signed release only after the shared runtime preflight succeeds and a usable PostgreSQL recovery point exists. It then hands the staged candidate plus source/target schema versions to the external finalizer. The running web/API process does not overwrite its own executable.

The external finalizer owns the destructive sequence:

1. stop `strata-rmm.service`;
2. preserve the known-good installed binary;
3. preserve and verify the pre-upgrade PostgreSQL recovery point;
4. move the already verified staged candidate into place;
5. start the candidate;
6. wait for readiness;
7. if candidate acceptance fails, stop it and restore the preserved database recovery point when schema/data recovery is required;
8. restore the previous known-good binary;
9. restart the previous version and verify readiness;
10. retain evidence when recovery cannot be proven automatically.

Do not replace those steps with manual file copies, package downgrades, or ad-hoc migration commands. If the finalizer leaves unresolved recovery state, preserve it and investigate the recorded evidence before attempting another upgrade.

Operational status uses the installed service name:

```bash
systemctl status strata-rmm.service
journalctl -u strata-rmm.service --since "30 minutes ago"
```

Readiness for a locally proxied native installation is the configured `/health/ready` endpoint. When automatic or external HTTPS is configured, verify the public HTTPS readiness endpoint as well.

## Docker Compose failed-upgrade rollback

The supported Docker host-side transaction is invoked through:

```bash
sudo ./scripts/update-docker-platform.sh
```

The transaction records the exact previous and candidate `repository@sha256:<digest>` references in protected state before mutation. It never uses a mutable tag as the authority for rollback.

If candidate rollout or readiness fails, the executor restores the exact retained previous digest in the protected Compose environment, redeploys that immutable image, and verifies readiness. If it cannot prove successful rollback, it keeps the protected transaction journal instead of pretending recovery succeeded.

If a prior attempt left a journal, rerun the same host wrapper. The reconciler compares the live immutable image with the recorded current/candidate state and resolves only states it can prove. After a successful reconciliation, rerun the wrapper again if you intend to evaluate a new release.

Do not run a generic `docker compose pull`, manually rewrite the image tag, remove the journal, or mount the Docker socket into the long-running orchestrator service.

## Database restore versus application rollback

Application rollback and database restore are related but not interchangeable.

- If a candidate never mutated schema/data, restoring the previous application identity may be sufficient.
- If the failed candidate crossed a schema boundary or the controlled lifecycle identifies database recovery as required, use the pre-upgrade recovery point owned by the transaction/finalizer.
- For disaster recovery unrelated to an upgrade, follow the backup/restore runbooks rather than using upgrade rollback state as a general-purpose backup system.

The signed release manifest and runtime preflight determine schema compatibility. Operators must not infer compatibility from version names or manually advance/rewind migration files.

## Kubernetes

Strata RMM does not currently provide an accepted built-in Kubernetes automatic upgrade/rollback lifecycle. Generic `helm rollback` or `helm upgrade` commands are not authoritative Strata RMM recovery procedures. An externally managed Kubernetes environment requires its own proven immutable-image, provenance, database-recovery, readiness, and evidence process before it can be represented as supported.

## Endpoint agents, patching, and software deployment

The platform's orchestrator release rollback contract does not imply that operating-system patches, third-party software deployments, scripts, or endpoint-agent updates can always be automatically reversed. Each subsystem must follow its own implemented transaction/status semantics.

In particular, do not promise automatic reversal of an installed OS patch merely because a canary deployment failed. Where a package or patch cannot be safely reversed by its native platform mechanism, stop further rollout, preserve failure evidence, and use the subsystem-specific remediation procedure.

## Post-recovery verification

After any recovery action, verify at least:

- `/health/ready` is passing for the restored orchestrator;
- the restored application identity/version matches the intended known-good release;
- PostgreSQL and NATS health checks are passing;
- tenant/client/site/device visibility is intact and scoped correctly;
- endpoint identities and enrollment state were not regenerated unexpectedly;
- durable job/alert state remains coherent;
- configured object storage remains reachable without replacement of persisted credentials;
- no unresolved native recovery handoff or Docker transaction journal remains unless intentionally retained as a fail-closed incident state;
- sanitized logs and timestamps are retained for the acceptance evidence package.

Do not use direct production SQL queries containing credentials as a routine verification shortcut. Prefer the platform health/readiness surfaces and the dedicated backup/restore validation procedures.

## Recovery cannot be proven

If automatic rollback cannot restore a known-good state:

1. stop further mutation;
2. preserve the exact failed source/candidate identities and all protected recovery metadata;
3. preserve sanitized service/finalizer/transaction logs;
4. do not delete the backup, previous digest, previous binary, or journal;
5. follow the documented restore/disaster-recovery procedure from a verified backup;
6. validate readiness and tenant data before reopening the system;
7. record the failure and cleanup status in the hosted/internal-alpha evidence format.

A failed rollback is an incident requiring evidence and controlled recovery; repeatedly rerunning destructive commands until one succeeds is not an accepted procedure.
