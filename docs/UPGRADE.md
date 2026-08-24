# Strata RMM — Upgrade Reference

**Last updated:** 2026-08-20

This document describes only upgrade behavior currently enforced by the repository. It intentionally does not provide manual shortcuts around release provenance, compatibility checks, database recovery, or health validation.

## Supported automatic update path

The built-in authenticated/in-process update flow currently supports **native/bare-metal orchestrator deployments only**. Docker Compose uses the separate privileged host-side transaction documented below. Kubernetes automatic apply remains unsupported.

The update service uses GitHub release metadata for discovery, but release metadata is not a trust anchor. Before an update can be staged, Strata RMM requires all of the following:

- a signed `release-manifest.json` and its Sigstore bundle;
- Sigstore identity bound to this repository's protected `publish-release.yml` workflow and the candidate release tag;
- a release tag that matches the version in the verified manifest;
- valid semantic-version ordering and the manifest's `minimum_upgrade_version` policy;
- a platform-specific artifact or OCI digest declared by the verified manifest;
- exact signed artifact metadata and successful Sigstore verification;
- the shared runtime preflight, including schema compatibility and a usable pre-upgrade PostgreSQL recovery point.

If any required manifest, signature, checksum/digest, compatibility, preflight, or provenance check is missing or invalid, the upgrade fails closed.

## Native/bare-metal lifecycle

For native installations, use the authenticated operator update surface or the supported CLI update path. Both consume the same verified update service; do not replace the orchestrator binary manually.

After a candidate is verified and staged, an external upgrade finalizer owns the destructive lifecycle. It stops the service, preserves the known-good binary and database recovery state, swaps the verified candidate into place, starts the candidate, validates readiness, and restores the previous binary/database state when the candidate cannot be accepted.

An operator should not manually run migration SQL as part of a normal upgrade. Schema changes and recovery are part of the controlled lifecycle. A failed automatic upgrade should be investigated from the retained update/recovery evidence rather than bypassed with ad-hoc database mutation.

## Docker Compose host-side lifecycle

Automatic/in-app Docker mutation is intentionally **not** performed by the web-facing orchestrator. The long-running service has no Docker-socket or equivalent host-root access. Docker upgrades are owned by a separate root-only, one-shot host utility.

For a Docker installation whose current immutable release contains the host updater utility, run from the verified release checkout that owns the installed Compose bundle:

```bash
sudo ./scripts/update-docker-platform.sh
```

The wrapper validates the protected Compose files, root-only environment state, Docker socket, canonical OCI repository, and current immutable image. It mounts the Docker socket only into a transient utility container and invokes `docker-update-host`; the normal orchestrator container remains unchanged and unprivileged with respect to the host Docker daemon.

The host-side transaction:

1. reconciles any retained crash journal before allowing a new mutation;
2. selects the authoritative OCI candidate from the verified signed release manifest;
3. requires an immutable `repository@sha256:<digest>` reference and verifies candidate Sigstore provenance against the protected release workflow and release tag;
4. reuses the authoritative semantic-version compatibility and runtime preflight policy, including a PostgreSQL recovery point;
5. reconciles the live image with protected Compose state before mutation;
6. records current/candidate identity in a protected, crash-safe transaction journal;
7. pulls and rolls out only the exact immutable candidate digest;
8. validates orchestrator readiness after rollout;
9. restores the exact retained previous digest when candidate health cannot be accepted; and
10. preserves fail-closed recovery state when rollback cannot be proven.

The shipped `orchestrator update` command refuses Docker apply from inside the service container and directs control to this privileged boundary. It also refuses Kubernetes apply rather than printing generic Helm instructions.

### Upgrade boundary for older Docker releases

Do not infer backward upgrade support for a Docker image that predates the host updater utility. The wrapper can adopt a missing protected image variable only when it can prove the live container already uses the expected immutable repository and digest, but the current image must also contain the one-shot updater command and its pinned verification/runtime dependencies. If it does not, treat that installation as outside the supported automatic Docker source range and use a controlled fresh/redeployment exercise with retained data and explicit evidence rather than inventing a manual pull/redeploy shortcut.

A promoted Docker installation must remain pinned to immutable image provenance. Never mount the host Docker socket into the web-facing orchestrator to bypass this boundary.

## Kubernetes status

The built-in automatic updater does not currently apply Kubernetes releases. An externally managed Kubernetes deployment must not be represented as a supported Strata RMM automatic upgrade unless its own immutable-digest, provenance, compatibility, rollback, and evidence process has been separately proven. Generic `helm upgrade` guidance is not a substitute for that lifecycle.

## Pre-upgrade operator checks

Before initiating a supported update:

- confirm the current installation is healthy;
- confirm the update/recovery storage locations have sufficient capacity;
- ensure PostgreSQL and required dependencies are reachable;
- review the candidate release notes and supported upgrade range;
- confirm no unresolved prior upgrade/recovery handoff remains;
- plan a maintenance window appropriate to the environment.

For Docker, additionally confirm the current orchestrator is running from an immutable digest and preserve the protected Compose environment and transaction journal. The host wrapper and runtime preflight remain authoritative even when operator checks appear satisfactory.

## Post-upgrade acceptance

An update is not accepted merely because a new binary or container starts. Native lifecycle acceptance requires the candidate to pass configured readiness/health validation after mutation. The Docker transaction likewise treats candidate readiness as an acceptance boundary and restores the previous immutable digest when the candidate cannot be accepted.

For hosted internal-alpha promotion, retain the exact source SHA/release identity, timestamps, expected and observed results, relevant sanitized logs, recovery/rollback evidence, and cleanup status required by Issue #15. Code presence and unit tests do not replace representative-host evidence.

## Recovery and rollback boundaries

For native updates, use the state retained by the verified update service and external finalizer. Do not overwrite backup binaries, remove staged recovery metadata, or manually advance/rewind schema state while a recovery handoff is unresolved.

For Docker, do not delete or edit an unresolved protected journal to force another attempt. Re-run the supported host wrapper: the executor/reconciler compares protected state with the live immutable image and either resolves the prior transaction or fails closed. After a successful reconciliation, run the wrapper again to evaluate a new release.

Kubernetes automatic rollback remains outside the built-in updater's current support claim.

## Prohibited shortcuts

The following are not supported upgrade procedures:

- pulling or deploying a mutable container tag as the authoritative candidate;
- running a generic `docker compose pull` / `docker compose up -d` sequence as promoted-release provenance;
- rebuilding the orchestrator image locally and treating it as a promoted release;
- mounting the Docker socket into the web-facing orchestrator to gain update privileges;
- manually replacing the running orchestrator binary outside the verified update service;
- applying migration files manually to force an unsupported source version forward;
- treating generic Helm guidance as an accepted Kubernetes upgrade lifecycle;
- deleting protected transaction/recovery state to bypass reconciliation;
- ignoring a failed manifest, signature, digest, checksum, compatibility, preflight, or health check;
- repeatedly rerunning a failed destructive upgrade until it happens to pass.

When the supported update path rejects a candidate, preserve the evidence and remediate the underlying defect rather than weakening the gate.
