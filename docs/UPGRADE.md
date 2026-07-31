# Upgrade and Release Packages

Strata RMM releases use immutable semantic-version tags such as `v0.3.0`. A published release contains SHA-256 checksums, Sigstore signatures, build provenance, raw orchestrator binaries for the application updater, portable agent/probe archives, and native Linux packages.

Do not install from `latest`, an unpinned container tag, or an asset whose checksum is unavailable.

## Artifact choices

| Deployment | Supported artifact | Upgrade mechanism |
|---|---|---|
| Debian/Ubuntu host | `strata-rmm-orchestrator_<version>_<arch>.deb` | `apt`/local package install with an explicit version |
| RPM-based host | `strata-rmm-orchestrator_<version>_<arch>.rpm` | `dnf`/local package install with an explicit version |
| Endpoint host | `strata-rmm-agent` package or OS archive | Managed agent rollout; do not mass-upgrade outside a deployment ring |
| Application updater | `strata-rmm-orchestrator-<version>-linux-<arch>` | Check, download, checksum verification, staged apply |
| Docker/Kubernetes | Image pinned by version and digest | Deployment controller or orchestrator-specific rollout |

The secure clean-installer will configure the service account, environment file, database, broker, storage, and first administrator. Installing a package alone intentionally does not invent credentials or start an unconfigured production service.

## Native package upgrade

1. Read the release notes and supported source-version boundary.
2. Back up PostgreSQL and `/etc/strata-rmm`.
3. Download the package and `checksums.txt` from the same immutable GitHub release.
4. Verify the exact filename:

   ```bash
   sha256sum --ignore-missing --check checksums.txt
   ```

5. Verify the Sigstore signature and certificate according to the release policy.
6. Install the explicit package file:

   ```bash
   sudo apt install ./strata-rmm-orchestrator_VERSION_ARCH.deb
   # or
   sudo dnf install ./strata-rmm-orchestrator_VERSION_ARCH.rpm
   ```

7. Run the documented preflight/migration procedure for that release.
8. Restart `strata-rmm.service`, then verify readiness and authenticated smoke tests.

Never pipe a network download directly into a privileged shell.

## Application and CLI upgrades

The updater accepts only a newer valid semantic version. It requires the exact platform artifact and an exact SHA-256 entry from `checksums.txt`; missing or malformed provenance fails closed. Numeric ordering is used, so `1.10.0` correctly follows `1.9.0`, and prereleases sort below their final release.

The raw binary is staged under `/var/lib/strata-rmm/updates`. Applying it must remain controlled by the deployment lifecycle: backup, schema compatibility check, migration lock, service restart, bounded readiness verification, and rollback or forward-fix decision. The administrative UI and CLI must call that same lifecycle controller rather than independently replacing files.

Until the lifecycle controller is exposed through an operator-authorized API/command, use the explicit native package or digest-pinned container procedure. This document does not claim an unexposed in-app button is production-ready.

## Container upgrades

Pin both the release version and image digest. Preserve the current healthy digest as the rollback target. Pull and validate the candidate, run schema compatibility checks, deploy it, and switch traffic only after readiness and authenticated smoke tests pass.

A mutable `latest` tag is not release evidence.

## Rollback safety

A binary rollback is allowed only when the previous binary is compatible with the current schema. Never run destructive down migrations merely to make an older binary start.

If compatibility is not proven:

- retain the new schema;
- stop the failed rollout;
- deploy a forward fix built for that schema;
- preserve database, audit, and durable-job records;
- record the failed deployment and recovery result.

If compatibility is proven, restore the previously verified package or image digest, verify readiness, and retain an audit record. A deployment failure remains a failed result even when rollback succeeds.

## Release workflow

A semantic-version tag triggers `Publish Versioned Release`. The workflow:

- proves the tag commit is on `master`;
- runs updater contract tests;
- builds platform artifacts and Debian/RPM packages;
- publishes checksums, Sigstore signatures, and certificates;
- attaches GitHub build-provenance attestations.

The GitHub `release` environment should require appropriate reviewer approval for production releases. Snapshot artifacts built in pull requests are test evidence only and must not be promoted as releases.
