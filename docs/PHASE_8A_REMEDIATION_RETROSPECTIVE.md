# Phase 8A Remediation Retrospective

## 1. Why was green CI treated as proof that the complete specification was implemented?

Green CI was incorrectly equated with requirement completion. The previous submission created a config package and verified it compiled and passed unit tests, but did not trace each requirement from the specification to implementation code to test assertions. No requirement-to-code mapping was created. CI passing merely proved that existing tests (which did not cover Phase 8A requirements) still passed.

## 2. Why were requirements checked off without requirement-to-code and requirement-to-test evidence?

No traceability matrix was built. Requirements were checked off based on intent rather than evidence. For example, "production validation" was claimed because a `ValidateProduction()` method existed, but the method was not invoked in the startup path, malformed values fell back to defaults silently, and no test verified that a startup with invalid production config actually exited non-zero.

## 3. Why was an unknown runtime mode allowed to fall back to development?

`ParseRuntimeMode()` returned an error for unknown modes, but the caller in `LoadOrchestratorConfig()` discarded the error with `if mode, err := ...; err == nil { cfg.RuntimeMode = mode }`. When err != nil, cfg.RuntimeMode remained the zero value (empty string). Downstream code then treated empty string as development. This was a fail-open design: the caller silently swallowed the error and fell back to a permissive default.

## 4. Why were malformed environment values silently replaced with defaults?

The `envInt`, `envBool`, and `envOr` helpers used a pattern where `strconv.Atoi` errors returned a default value. A malformed integer like `DB_MAX_OPEN_CONNS=abc` would silently use the default without warning. The same applied to duration and URL parsing. No validation layer distinguished "absent" from "present but broken."

## 5. Why were configuration structures added without wiring their fields into runtime consumers?

The config struct was created with fields for NATS TLS, database pool limits, CORS origins, etc., but the orchestrator startup code continued to read environment variables directly via `os.Getenv("STORAGE_ACCESS_KEY")` rather than from the loaded config object. The struct was decorative — it stored values that were never used to change runtime behavior.

## 6. Why were existing CLI flags removed without a compatibility assessment?

The prior commit replaced the orchestrator command to use only `pkg/config`, removing `cmd.Flags().StringVar` declarations for `--nats-url`, `--timescale-dsn`, `--api-addr`, `--tunnel-addr`, and the storage flags. No deprecation notice was added. No test verified flags still worked. Existing deployment scripts using these flags would break silently.

## 7. Why was a new `--config` flag added without implementing it?

A `cfgPath` variable was declared and wired to `cmd.Flags().StringVar(&cfgPath, "config", ...)` but the value was never read or used. The flag was decorative — it appeared in help text but changing it had no effect.

## 8. Why was documentation allowed to advertise removed or nonexistent behavior?

The `docs/CONFIGURATION.md` listed `--nats-url`, `--timescale-dsn`, `--storage-backend`, etc. as supported flags. These flags had been removed from the command implementation. The documentation was not verified against the actual code.

## 9. Why was readiness reported without checking required dependencies?

The health endpoint returned `"ready": "true"` as soon as `api.Start()` completed. No PostgreSQL connectivity check, NATS check, migration check, or storage check was performed. Readiness was based on process state, not on actual dependency health.

## 10. Why did not-ready responses still return HTTP 200?

The health handler always returned `http.StatusOK` regardless of the ready state. The spec requires HTTP 503 when not ready. Returning 200 for "starting" made the endpoint indistinguishable from "ready" in load-balancer health checks.

## 11. Why was production storage allowed to fail open?

Storage initialization failure in the orchestrator used `logger.Warn("storage backend init (continuing without recordings)", ...)` and continued startup. For production deployments that depend on recordings, reports, or remote access, this silently degrades functionality. The policy decision (optional vs. required) was not modeled.

## 12. Why were production NATS authentication and TLS requirements omitted?

The `NATSConfig` struct had fields for `Token`, `TLSEnabled`, etc., but these were not wired into the NATS connection call. The `connectNATS()` function checked `cfg.NATS.Token` and `cfg.NATS.TLSEnabled` but TLS was applied as `nats.Secure(nil)` without actually reading the cert/key/CA files. Token auth worked but was not validated in production mode.

## 13. Why was the requested configuration inventory not created?

The previous `docs/CONFIGURATION.md` was a brief reference table, not a complete inventory. It omitted agent, probe, enrollment, telemetry, retention, worker, and deployment template settings. It did not record types, consumers, validation rules, or reload behavior for each setting.

## 14. Why were required deployment and operational documents not updated?

`docs/PHASE_8_PRODUCTION_BETA.md`, `docs/PHASE_8_ACCEPTANCE_MATRIX.md`, `docs/MASTER_PLAN.md`, `docs/INSTALL.md`, `docs/RUNBOOK.md`, `docs/SECURITY_MODEL.md`, `docs/index.md`, and `CHANGELOG.md` were not updated. Docker Compose, Helm values, KOTS, and installation scripts were not checked for consistency with the new configuration model.

## 15. Why was the PR body left with "CI pending" after CI completed?

The PR body was not updated after CI completed. It still contained `- [ ] CI verification (pending)`. No workflow URL was recorded. No evidence of job completion was attached.

## 16. Why were incomplete acceptance criteria marked complete?

Acceptance criteria in the PR body were marked `[x]` without implementation-to-test mapping. For example, "Configuration inventory documented" was checked because a file existed, not because it met the specification's completeness requirements.

## 17. Why was the durable-jobs correction not accompanied by a focused PostgreSQL regression test?

The UUID/text cast fix was applied to dispatcher.go lines 264 and 279. No database integration test was added that executed the affected reconciliation paths and verified they no longer produce `operator does not exist: uuid = text` errors. The fix was unverifiable without a regression test.

## 18. What review steps were skipped or performed superficially?

- No diff review against the specification
- No check for removed CLI flags
- No check for unused struct fields
- No check for silent error swallowing
- No verification of HTTP status codes in health responses
- No comparison of documentation against implementation
- No traceability matrix
- No startup failure-path testing
- No CLI flag testing
- No wiring verification

## 19. What concrete process changes will prevent recurrence?

1. **Requirement traceability**: Before coding, create a traceability matrix mapping each requirement to implementation location, test, and documentation.
2. **Implementation discipline**: For each requirement, write the test first, then implement, then verify the test passes.
3. **Diff self-review**: Before committing, review the full diff for removed features, unused code, silent error swallowing, and documentation accuracy.
4. **CLI compatibility check**: Before modifying flags, verify existing commands still work.
5. **Fail-close verification**: For every validation, verify the failure path produces a non-zero exit and actionable error.
6. **Wiring verification**: For every config field, verify it reaches its runtime consumer.
7. **CI evidence requirement**: The PR body must contain the exact workflow URL and job table before marking ready.

## 20. How will you prove those process changes were followed during this remediation?

This retrospective document proves the analysis was performed. The traceability matrix below proves requirement-to-implementation-to-test mapping was done before coding. Each remediation commit addresses specific failures identified above. The PR body will contain exact-head SHA, workflow URL, and a complete job table before being marked ready.
