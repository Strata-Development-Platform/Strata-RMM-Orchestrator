# Add-on Runtime Engineering Reinforcement

This document supplements `docs/AGENT_ENGINEERING_STANDARD.md` for all add-on module and runtime work. It is mandatory context for future implementation/remediation prompts touching `internal/modules`, module APIs, package ingestion, activation, runtime execution, publisher trust, or marketplace distribution.

## Non-negotiable trust-boundary rules

- Do not execute third-party code until its package signature, manifest, payload archive, materialized tree, selected version, and runtime declaration have all passed their prior trust boundaries.
- Do not infer runtime capabilities from payload contents. Capabilities come only from the validated manifest and explicit administrator approval.
- Alpha executable modules are WASI-only. Do not add native-process, shell, container, plugin/DLL/SO, or arbitrary command execution as a shortcut.
- A module must receive no ambient host filesystem, inherited environment, host secrets, raw sockets, unrestricted DNS/network, database handle, RLS bypass, unrestricted NATS client, or secret-store handle.
- All platform access must be brokered through explicit Strata host APIs/events that independently enforce module ID, current lifecycle state, permission, MSP/client/site scope, revocation, and audit.
- Network policy `none` means no network imports. `brokered` means only reviewed Strata host calls, not general-purpose sockets.
- Fail closed when runtime policy, revocation state, durable module state, active-version metadata, health evidence, or required infrastructure cannot be verified.

## Runtime contract enforcement

The runtime adapter must revalidate, not merely trust, the manifest/runtime values it consumes:

- runtime kind must be `wasi`;
- entrypoint must remain canonical, relative, contained, and end in `.wasm`;
- selected version must match an existing immutable materialized version;
- active metadata must be valid and non-symlinked;
- memory, timeout, and concurrency must remain within manifest bounds;
- the module must still be enabled and not quarantined immediately before execution;
- token/service identity permissions must remain a subset of the currently enabled manifest.

Do not treat validation performed at package-install time as permanent authorization for later execution.

## WASI engine acceptance tests

A future WASI engine PR is incomplete unless tests prove all of the following at the actual engine boundary:

1. A known-good minimal WASI reference module executes successfully.
2. Malformed/truncated/invalid WASM is rejected without crashing the orchestrator.
3. A module trap is surfaced as a runtime failure and contributes to quarantine accounting.
4. Execution cancellation and timeout terminate the guest call and return boundedly.
5. Declared memory limits are enforced by the engine, not only parsed from the manifest.
6. Concurrency limits prevent excess simultaneous guest executions.
7. No host filesystem is available unless a later reviewed capability explicitly adds a narrow preopened directory.
8. Host environment variables and process arguments are not inherited by default.
9. Host secrets are not visible in guest environment, stdin, filesystem, error messages, or logs.
10. Raw sockets/network imports are unavailable for `network: none`.
11. `network: brokered` still exposes no raw sockets; only explicit broker functions may be called.
12. Undeclared host imports cause instantiation failure.
13. Brokered host calls re-authorize module ID, permission, scope, lifecycle state, and revocation at call time.
14. Sibling MSP/client/site and cross-tenant resource substitution are denied.
15. Disabling, quarantining, or revoking a module prevents subsequent calls without orchestrator restart.
16. Failed health checks leave prior active metadata untouched.
17. Failed runtime upgrade can return to the previous materialized version without mutating immutable version trees.
18. Guest stdout/stderr and returned errors are bounded and sanitized before logging.
19. Resource cleanup occurs after success, timeout, cancellation, trap, and instantiation failure.
20. Repeated failure cannot leak goroutines, file descriptors, guest instances, memory, or broker sessions.

Structural tests that only inspect manifest fields, constants, or engine configuration do not satisfy these runtime acceptance requirements.

## Required negative/security testing

Every executable-module slice must deliberately test hostile inputs relevant to the changed boundary, including:

- path traversal, absolute paths, backslashes, symlinks, hardlinks, and special files;
- package/signature/digest tampering;
- stale or corrupted active metadata;
- runtime kind downgrade or unsupported runtime declarations;
- memory/time/concurrency values at minimum, maximum, zero, negative-equivalent decode cases, and above-limit values;
- invalid/unknown WASI imports;
- attempts to access host filesystem/environment/network;
- expired/revoked/wrong-module service tokens;
- excessive permissions and invalid scope hierarchy;
- sibling/cross-tenant IDs supplied to broker calls;
- Redis/durable-registry outage during authorization;
- runtime timeout, cancellation, trap, repeated failure, and quarantine.

## CI and evidence methodology

For every module/runtime PR:

- keep the PR draft until implementation and evidence are complete;
- freeze the candidate SHA before the final evidence run;
- run the dedicated Add-on Modules workflow plus the entire required repository workflow matrix;
- require race tests, vet, golangci-lint, govulncheck, high-severity gosec, database isolation, security/secret scanning, and cross-platform build jobs where applicable;
- treat any head change as invalidating all previous CI evidence;
- rerun a flaky unrelated job only on the same SHA; if it reproduces, investigate rather than dismiss it;
- never suppress a high-severity scanner finding merely to obtain a green gate;
- distinguish infrastructure/action failures from actual scanner findings by reading the exact job log;
- record real engine/runtime environment limitations explicitly instead of calling contract tests execution evidence.

A green manifest/runtime-contract PR proves the contract only. It does not prove sandbox enforcement until a real engine executes a reference module under the claimed restrictions.

## Error classes that must not recur

- **Partial mature-file replacement:** Never replace a mature shared file from an incomplete read. Fetch the complete source first or use a surgical patch mechanism.
- **Enum/constant compatibility regression:** Adding a new access class must not silently renumber legacy constants whose numeric values are tested or persisted.
- **RLS semantic misread:** A successful SQL call can affect zero rows under RLS. Security tests must inspect rows affected and persisted postconditions.
- **Unchecked cleanup errors:** `Close`, cleanup, and removal paths must satisfy lint/static analysis and preserve the primary error appropriately.
- **Unsafe signed/unsigned conversion:** Do not enforce untrusted size limits through risky integer-domain conversions. Prefer bounded streaming reads and one arithmetic domain.
- **Filesystem TOCTOU hardening after write:** Do not walk attacker-influenced trees later to chmod/chown them. Apply policy while creating controlled paths and avoid race-prone post-write traversal.
- **Overbroad permissions:** Newly created module install/control roots should be least privilege by default; scanner findings such as G301 are merge blockers.
- **Stale CI reuse:** Never cite green jobs from a prior SHA after any commit, including documentation-only commits.
- **Flake assumption without proof:** A job is not a flake because the changed files seem unrelated. Re-run on the same SHA; repeated failure requires investigation.
- **Concurrent lifecycle channel close:** Never clear a lifecycle state flag and then close its stop channel outside the same synchronization boundary. Concurrent Start/Stop can otherwise double-close a channel or let an old worker observe a replacement channel. Allocate per-run stop channels, capture them in the worker, and make Stop idempotent under the lifecycle mutex; prove restart and concurrent shutdown under the race detector.
- **Contract/execution conflation:** Runtime schema validation is not sandbox proof. Health callback success is not guest-runtime proof. A mock runtime is not third-party execution evidence.
- **Ambient capability leakage:** Do not use engine defaults that inherit host environment, filesystem, stdio, networking, or process configuration without an explicit reviewed reason.
- **Authorization at install time only:** Brokered runtime/API/event calls must re-authorize current state and scope at invocation time.

## PR completion checklist for the first WASI engine

Before the first concrete engine can be called Alpha-ready, the PR must show:

- a pinned, reviewed WASI engine dependency and license/security assessment;
- no native execution fallback;
- no ambient filesystem/environment/network by default;
- manifest memory/time/concurrency enforcement at the engine boundary;
- bounded sanitized IO;
- lifecycle/revocation-aware invocation;
- broker-only host functions with scope-negative tests;
- health-gated activation using the real adapter;
- a signed reference module package that travels through verification, validation, materialization, activation, and execution;
- failure/quarantine/rollback behavior;
- exact-head 8/8 workflow evidence plus any additional engine-specific integration workflow required to make the claim durable.
