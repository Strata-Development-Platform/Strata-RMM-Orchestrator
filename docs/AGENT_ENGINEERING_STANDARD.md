# Agent Engineering and Pull Request Standard

This standard must be included by reference in every future implementation or remediation prompt.

## Operating contract

The coordinator owns scope, integration, acceptance, and the final truthfulness audit. Delegation never transfers accountability. Every change must be read, tested, and verified against the actual runtime boundary being claimed.

Before editing:

1. fetch the current base and create or refresh one feature branch;
2. inspect repository instructions, architecture, schemas, runtime consumers, tests, workflows, and the open PR;
3. convert every requirement into a traceability table with an owner, files, tests, and evidence;
4. identify compatibility, tenancy, security, migration, deployment, upgrade, backup, restore, and rollback effects;
5. record assumptions as assumptions, never as completed facts.

## Implementation rules

- Preserve existing behavior unless the requirement explicitly changes it.
- Trace every new setting from input through validation to its runtime consumer.
- Prefer real dependency probes over booleans, startup flags, or always-success health checks.
- Fail closed for production security and tenant boundaries.
- Never silently downgrade required infrastructure.
- Keep optional capabilities explicitly disabled; dependent routes must return a clear unavailable response.
- Add a regression test that fails before each defect fix whenever practical.
- Keep migrations, rollback steps, deployment examples, and operator documentation aligned with code.
- Do not mark deferred, simulated, decorative, structural-only, or untested behavior complete.
- Optional/vendor-specific functionality should preferentially use the add-on module framework instead of increasing coupling in the core.
- Modules may not bypass core authorization, RLS, auditing, secret storage, tenancy, compatibility, or lifecycle controls.

## Required testing methodology

A green compile is not acceptance. Tests must be layered and must exercise the boundary being claimed.

### Unit tests

Cover happy paths, invalid input, malformed payloads, nil/zero values where meaningful, boundary values, duplicate operations, timeouts, idempotency, and deterministic error handling.

### Integration tests

Use real infrastructure where practical. PostgreSQL, TimescaleDB, NATS JetStream, Redis, object storage, and browser/API flows must not be replaced by mocks when the claim depends on their real semantics. Integration tests must prove schema behavior, RLS, transactions, acknowledgement semantics, persistence, retries, and recovery.

### Behavioral tests

Behavioral tests must validate user/system workflows, not merely struct fields, constant values, JSON round-trips, reflection counts, or string presence. Structural contract tests are useful but must be labeled as structural/contract tests and never counted as workflow evidence.

Examples of acceptable behavioral paths include:

- provider -> MSP -> client -> site -> scoped enrollment -> agent heartbeat;
- telemetry -> persistence -> monitoring evaluation -> alert -> ticket/notification;
- durable command -> agent acknowledgement -> execution -> persisted result -> audit trail;
- patch/software deployment -> maintenance policy -> execution -> result -> reboot/reconnect where applicable;
- backup -> isolated restore -> integrity verification -> tenant authorization preserved;
- module install -> manifest validation -> permission approval -> enable -> health -> disable/uninstall.

### Failure injection

Every critical subsystem must deliberately fail in tests or controlled environment exercises. Required examples include database outage, NATS outage/restart, object storage outage, invalid/expired credentials, duplicate messages, partial writes, timeout, process restart, network loss, dependency recovery, and rollback after failed upgrade. The expected behavior must be explicit: retry, NAK, fail closed, preserve state, or surface a terminal error.

### Durability and messaging

Never acknowledge a durable message before the claimed durable side effect is committed. For JetStream or similar systems, the default rule is validate -> persist/commit -> ACK. On transient persistence failure, NAK/retry. Poison/malformed messages may be terminated only when replay cannot make them valid and the failure is observable/audited. Tests must prove no silent loss and must consider duplicate delivery.

### Security and tenancy

Every privileged route or module capability requires positive-role tests and negative tests for missing credentials, wrong role, wrong MSP/client/site/tenant, sibling scope, parent escalation, inactive scope, malformed identifiers, and direct resource-ID substitution. RLS claims require real PostgreSQL evidence when possible.

### Recovery and resilience

For stateful features, prove restart/reconnect behavior, retry bounds, idempotency, duplicate suppression, rollback, and restoration. Alpha acceptance requires environment-level exercises for reconnect storms, dependency outages, clean-host lifecycle, and measured recovery.

### Load and soak

Performance claims require versioned load evidence. Run bounded CI load contracts plus representative environment tests. Alpha requires a documented soak exercise with memory, goroutine/thread, connection, queue, latency, failure, and telemetry-loss observations.

Load generators must cross the same trust boundary as production clients. Do not bypass enrollment, authorization, RLS, scoped service identity, or NATS subject credentials merely to generate traffic. Synthetic tenants/agents are valid only when they are provisioned through supported control-plane paths or a dedicated simulator that enforces equivalent scope. Readiness-only soak samples are useful observations but do not prove resource stability without corresponding server metrics.

### Evidence automation

Automation may collect, hash, and summarize evidence, but it must not self-certify an acceptance gate. Every evidence artifact must identify the exact candidate SHA, environment, timestamps, inputs, and limitations. A script exiting zero means the scripted observations passed; it does not mean the broader Phase 8 row is accepted unless the row's complete required proof is satisfied.

## Local verification

Run the repository-prescribed commands. At minimum, where applicable:

1. formatting and generated-file checks;
2. static analysis and lint;
3. unit tests;
4. race tests;
5. frontend type-check, lint, unit, build, and browser tests;
6. database and integration tests;
7. security, secret, dependency, SBOM, and image scans;
8. build and packaging validation;
9. changed deployment-template rendering or validation;
10. focused failure-injection and recovery tests for the changed subsystem.

Capture command, exit status, and meaningful output. “Should pass,” partial suites, skipped failures, stale results, or unrelated green workflows are not evidence.

## Common mistakes that must not recur

- Do not replace a mature shared source file from a partial read or reconstructed excerpt. Before a whole-file update, fetch and preserve the complete file, compare against the base, and verify that unrelated exported types/functions remain present. Prefer a minimal patch when the tool supports one; if only whole-file replacement is available, treat preservation of every unaffected declaration as an explicit acceptance check.
- Do not create a duplicate test function or helper name across the same Go package. Run the full package/suite, not only the newly added test file.
- Do not call CI green from documentation or memory. The exact current head SHA and GitHub checks are authoritative.
- Do not equate a high test count with high assurance. Separate unit, structural contract, integration, browser, fault, load, soak, and hosted lifecycle evidence.
- Do not acknowledge telemetry/jobs before durable persistence. This can create silent data loss during database failure.
- Do not clear an in-memory batch before successful persistence unless a durable replay source still owns the message and acknowledgement is withheld.
- Do not duplicate simplified production authorization logic in test-only mocks and then claim production authorization coverage.
- Do not assume FORCE ROW LEVEL SECURITY enables RLS; verify ENABLE and FORCE semantics against the actual migration and PostgreSQL.
- Do not interpret a nil PostgreSQL DML error as proof that an authorization-sensitive mutation succeeded or failed. Under RLS, UPDATE/DELETE may validly return no error while affecting zero rows. Security tests must inspect rows affected and verify the persisted postcondition; when a trigger is intended as defense in depth beyond RLS, test it separately through an authorized RLS-bypass/migration-owner connection.
- Do not write RLS policies or migrations from memory. Verify real table/column names and execute migrations in both directions where supported.
- Do not use route classification as a substitute for handler/resource authorization.
- Do not fix only the reported symptom when the same defect class can exist elsewhere; search for siblings and add regression coverage.
- Do not update only embedded or standalone schema copies. Reconcile every source of migration/schema truth mechanically.
- Do not mark a feature complete because routes, structs, UI controls, or tests exist. Acceptance requires functional wiring and evidence.
- Do not let documentation claim capabilities beyond code or environment proof. Environment-pending work must remain explicitly pending.
- Do not add third-party/vendor-specific core coupling when the module API is the appropriate extension boundary.
- Do not use arbitrary fixed sleeps as readiness proof for asynchronous workers. Poll the actual state with a bounded deadline and print diagnostics on timeout.
- Do not generate Alpha load by publishing directly to tenant NATS subjects with hard-coded tenant/device identifiers. Use enrolled/scoped identities or a simulator that preserves the production trust model.
- Do not let an evidence collector write “accepted”, “passed Phase 8”, or equivalent merely because its own probes succeeded. Acceptance is a reviewed decision against the complete matrix.

## Pull request discipline

- Use one branch and one PR per phase unless explicitly directed otherwise.
- Keep the PR draft while implementation or evidence is incomplete.
- Synchronize with the current base before final CI.
- Write a PR body that states scope, non-goals, architecture, security/tenancy impact, compatibility, migrations, deployment, rollback, tests, and residual risks.
- Push all fixes before starting the final evidence run.
- Record the exact head SHA.
- Monitor every required GitHub Actions job to a terminal conclusion.
- Treat skipped, cancelled, neutral, missing, or infrastructure-dependent required jobs as unresolved until their acceptability is proven.
- If the head changes, all prior CI evidence is stale; repeat exact-head verification.
- Do not merge, mark ready, or call the phase complete until the acceptance matrix and PR body match the exact code and evidence.
- Never merge without explicit authorization.

## Final acceptance audit

Before handoff, independently verify:

- the PR diff contains only intended changes;
- every acceptance row has code/test/evidence or is honestly marked partial/deferred;
- tests assert behavior rather than implementation trivia;
- runtime code consumes the configuration being documented;
- health checks perform meaningful live checks;
- documentation and deployment examples use current names and defaults;
- no secrets, debug bypasses, placeholder credentials, or unsupported completion claims remain;
- the exact-head workflow URL and job results are recorded;
- new optional functionality uses the module boundary where appropriate;
- any discovered agent error has been added to `docs/agents.ms` or this standard so the project learns permanently.

If any check fails, continue remediation. “Complete” means implementation, tests, documentation, recovery/rollback plan, and exact-head CI evidence all agree.
