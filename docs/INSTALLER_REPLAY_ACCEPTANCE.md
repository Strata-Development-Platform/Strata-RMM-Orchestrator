# Installer Replay Acceptance

Issue #154 closes the deterministic Docker installer replay gap under Issues #17 and #149.

## Repository-proven behavior

- A fresh Docker preparation creates the complete protected credential and local TLS identity set once.
- A coherent replay validates the expected secret files and `.install.env` without regenerating or overwriting them.
- Replay requires the same domain, ACME email, and ACME CA. Conflicting inputs fail closed and require an explicit reconfiguration path.
- Partial installation state fails closed instead of generating replacement credentials into an incomplete installation.
- `--prepare-only` replay does not require a new bootstrap password.
- Full replay only invokes initial-administrator bootstrap while the protected bootstrap password artifact still exists. Once that artifact has been consumed and removed, replay preserves the existing administrator state.
- Docker Compose configuration is revalidated on every run.

## CI evidence

The `Secure Installer` workflow:

1. performs a first Docker `--prepare-only` run;
2. records SHA-256 digests for all generated installation material and `.install.env`;
3. performs a second replay without supplying bootstrap password input;
4. proves every digest is unchanged;
5. removes one required secret and proves the installer rejects the partial state without mutation;
6. restores the secret and proves the original digest set is intact;
7. supplies a conflicting domain and proves the installer rejects it without mutation;
8. continues to validate Compose, Caddy, and image builds.

The existing PostgreSQL bootstrap integration test remains the authoritative database-backed proof that initial administrator creation succeeds exactly once and is audited.

## Environment-required evidence

A complete non-`--prepare-only` replay against a running Compose deployment remains representative-environment evidence. It must prove service reconciliation, preserved volumes/data, unchanged endpoint identity, and unchanged administrator credentials before Issue #17 can claim full installer replay acceptance.
