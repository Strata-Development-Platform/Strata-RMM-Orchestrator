# Strata RMM — Upgrade Reference

**Last updated:** 2026-08-20

This document describes only upgrade behavior currently enforced by the repository. It intentionally does not provide manual shortcuts around release provenance, compatibility checks, database recovery, or health validation.

## Supported automatic update path

The built-in authenticated update flow currently supports **native/bare-metal orchestrator deployments only**.

The update service uses GitHub release metadata for discovery, but release metadata is not a trust anchor. Before an update can be staged, Strata RMM requires all of the following:

- a signed `release-manifest.json` and its Sigstore bundle;
- Sigstore identity bound to this repository's protected `publish-release.yml` workflow and the candidate release tag;
- a release tag that matches the version in the verified manifest;
- valid semantic-version ordering and the manifest's `minimum_upgrade_version` policy;
- a platform-specific orchestrator artifact declared by the verified manifest;
- the exact artifact size and SHA-256 declared by the manifest;
- successful artifact Sigstore verification;
- the shared runtime preflight, including schema compatibility and a usable pre-upgrade PostgreSQL recovery point.

If any required manifest, signature, checksum, compatibility, preflight, or provenance check is missing or invalid, the upgrade fails closed.

## Native/bare-metal lifecycle

For native installations, use the authenticated operator update surface or the supported CLI update path. Both consume the same verified update service; do not replace the orchestrator binary manually.

After a candidate is verified and staged, an external upgrade finalizer owns the destructive lifecycle. It stops the service, preserves the known-good binary and database recovery state, swaps the verified candidate into place, starts the candidate, validates readiness, and restores the previous binary/database state when the candidate cannot be accepted.

An operator should not manually run migration SQL as part of a normal upgrade. Schema changes and recovery are part of the controlled lifecycle. A failed automatic upgrade should be investigated from the retained update/recovery evidence rather than bypassed with ad-hoc database mutation.

## Docker Compose status

Automatic/in-app upgrade of a Docker Compose deployment is **not yet a supported pre-beta path**. The web-facing orchestrator intentionally does not receive Docker-socket or equivalent host-root access merely to expose an update button.

The repository does contain the privileged host-side Docker promoted-release transaction implemented by PR #158. That transaction:

1. selects the authoritative OCI candidate from the verified signed release manifest;
2. requires an immutable `repository@sha256:<digest>` reference and verifies candidate Sigstore provenance;
3. reuses the authoritative semantic-version compatibility and runtime preflight policy;
4. reconciles the live image with protected Compose state before mutation;
5. records crash-safe transaction state in a protected journal;
6. rolls out the immutable candidate and validates readiness;
7. restores the exact retained previous digest if candidate health cannot be accepted; and
8. reconciles interrupted/replayed states rather than blindly reapplying destructive work.

Those transaction primitives are implemented and tested, but the legacy `orchestrator update` deployment-mode branch is **not yet an accepted Docker operator entrypoint**. Issue #162 tracks wiring a privileged host-side command to the verified transaction and removing the legacy manual Docker/Kubernetes guidance. Until #162 is closed and exact-head evidence is retained, do not treat a CLI log message, manual Compose pull/redeploy, local repository rebuild, or mutable image tag as a supported Docker upgrade procedure.

A promoted Docker installation must remain pinned to immutable image provenance. Do not mount the host Docker socket into the web-facing orchestrator to work around the missing operator entrypoint.

## Kubernetes status

The built-in automatic updater does not currently apply Kubernetes releases. An externally managed Kubernetes deployment must not be represented as a supported Strata RMM automatic upgrade unless its own immutable-digest, provenance, compatibility, rollback, and evidence process has been separately proven. Generic `helm upgrade` guidance is not a substitute for that lifecycle.

## Pre-upgrade operator checks

Before initiating a supported native update:

- confirm the current installation is healthy;
- confirm the update/recovery storage locations have sufficient capacity;
- ensure PostgreSQL and required dependencies are reachable;
- review the candidate release notes and supported upgrade range;
- confirm no unresolved prior upgrade/recovery handoff remains;
- plan a maintenance window appropriate to the environment.

For Docker validation work, additionally preserve the protected Compose environment and transaction journal and confirm the running image is an immutable digest reference. These checks do not convert the still-unwired operator entrypoint into an accepted production upgrade path.

The runtime preflight remains authoritative even when operator checks appear satisfactory.

## Post-upgrade acceptance

An update is not accepted merely because a new binary or container starts. Native lifecycle acceptance requires the candidate to pass configured readiness/health validation after mutation. The Docker transaction machinery likewise treats candidate health as an acceptance boundary and retains/restores the previous digest when a candidate cannot be accepted.

For hosted internal-alpha promotion, retain the exact source SHA/release identity, timestamps, expected and observed results, relevant sanitized logs, recovery/rollback evidence, and cleanup status required by Issue #15. Code presence and unit tests do not replace representative-host evidence.

## Recovery and rollback boundaries

For native updates, use the state retained by the verified update service and external finalizer. Do not overwrite backup binaries, remove staged recovery metadata, or manually advance/rewind schema state while a recovery handoff is unresolved.

For Docker transaction testing, do not delete or edit an unresolved protected journal to force another attempt. The #158 executor/reconciler is designed to compare protected state with the live immutable image and either complete or fail closed. A supported operator command for that transaction remains tracked by Issue #162.

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
