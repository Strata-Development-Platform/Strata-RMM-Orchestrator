# Strata RMM — Upgrade Reference

**Last updated:** 2026-08-19

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

Automatic/in-app upgrade of a Docker Compose deployment is **not yet a supported pre-beta path**. The API intentionally refuses container-mode apply until the platform has a digest-pinned promoted-release transaction with retained previous-digest rollback.

Do not use mutable image tags as an upgrade mechanism. Do not treat a local rebuild of the repository checkout as promoted-release provenance. Until the Docker lifecycle tracked by Issue #156 is implemented and validated, use a representative test environment for development-only redeployments and do not claim them as accepted production upgrades.

The required future Docker path is:

1. select the authoritative OCI image digest from the verified signed release manifest;
2. verify candidate image provenance/signature;
3. retain both current and candidate image digests in protected transaction state;
4. run the same compatibility and runtime preflight policy used by the native update service;
5. roll out the immutable candidate digest;
6. validate readiness and application health;
7. restore the retained previous digest on failure;
8. reconcile an interrupted/replayed transaction without blindly reapplying it.

Those steps are requirements, not instructions for manual execution today.

## Kubernetes status

The built-in automatic updater does not currently apply Kubernetes releases. An externally managed Kubernetes deployment must not be represented as a supported Strata RMM automatic upgrade unless its own immutable-digest, provenance, compatibility, rollback, and evidence process has been separately proven.

## Pre-upgrade operator checks

Before initiating a supported native update:

- confirm the current installation is healthy;
- confirm the update/recovery storage locations have sufficient capacity;
- ensure PostgreSQL and required dependencies are reachable;
- review the candidate release notes and supported upgrade range;
- confirm no unresolved prior upgrade/recovery handoff remains;
- plan a maintenance window appropriate to the environment.

The runtime preflight remains authoritative even when these operator checks appear satisfactory.

## Post-upgrade acceptance

An update is not accepted merely because a new binary starts. The supported lifecycle requires the candidate to pass the configured readiness/health validation after mutation. If acceptance fails, the finalizer owns rollback to the known-good state.

For hosted internal-alpha promotion, retain the exact source SHA/release identity, timestamps, expected and observed results, relevant sanitized logs, recovery/rollback evidence, and cleanup status required by Issue #15.

## Recovery and rollback boundaries

For native updates, use the state retained by the verified update service and external finalizer. Do not overwrite backup binaries, remove staged recovery metadata, or manually advance/rewind schema state while a recovery handoff is unresolved.

Docker Compose automatic rollback remains unaccepted until the digest-pinned transaction in Issue #156 is implemented. Kubernetes automatic rollback remains outside the built-in updater's current support claim.

## Prohibited shortcuts

The following are not supported upgrade procedures:

- pulling or deploying a mutable container tag as the authoritative candidate;
- manually replacing the running orchestrator binary outside the verified update service;
- applying migration files manually to force an unsupported source version forward;
- ignoring a failed manifest, signature, digest, checksum, compatibility, preflight, or health check;
- repeatedly rerunning a failed destructive upgrade until it happens to pass.

When the supported update path rejects a candidate, preserve the evidence and remediate the underlying defect rather than weakening the gate.
