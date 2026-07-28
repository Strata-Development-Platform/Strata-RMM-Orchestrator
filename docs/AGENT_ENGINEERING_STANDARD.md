# Agent Engineering and Pull Request Standard

This standard must be included by reference in every future implementation or remediation prompt.

## Operating contract

The coordinator owns scope, integration, acceptance, and the final truthfulness audit. It may delegate bounded work to sub-agents, but delegation does not transfer accountability. The coordinator must read and verify every resulting change before accepting it.

Before editing:

1. fetch the current base and create or refresh one feature branch;
2. inspect repository instructions, architecture, schemas, runtime consumers, tests, workflows, and the open PR;
3. convert every requirement into a traceability table with an owner, files, tests, and evidence;
4. identify compatibility, tenancy, security, migration, deployment, and rollback effects;
5. record assumptions as assumptions—not completed facts.

## Delegation rules

Use sub-agents for independent, bounded work such as implementation, tests, documentation/deployment, and adversarial review. Give each agent explicit file ownership and acceptance criteria. Prevent overlapping edits. Require each agent to report:

- files and behavior changed;
- commands actually executed and their exact results;
- unresolved risks, assumptions, and deferred work;
- commit SHA or patch reviewed by the coordinator.

At least one reviewer must be adversarial and independent of the implementation. It must trace configuration to runtime consumers, exercise negative paths, check tenant isolation and authorization, and find claims unsupported by code or evidence.

## Implementation rules

- Preserve existing behavior unless the requirement explicitly changes it.
- Trace every new setting from input through validation to its runtime consumer.
- Prefer real dependency probes over booleans, startup flags, or always-success health checks.
- Fail closed for production security and tenant boundaries.
- Never silently downgrade required infrastructure.
- Keep optional capabilities explicitly disabled; dependent routes must return a clear unavailable response.
- Add a regression test that fails before each defect fix whenever practical.
- Keep migrations, rollback steps, deployment examples, and operator documentation aligned with code.
- Do not mark deferred, simulated, decorative, or untested behavior complete.

## Local verification

Run the repository-prescribed commands. At minimum, where applicable:

1. formatting and generated-file checks;
2. static analysis and lint;
3. unit tests;
4. race tests;
5. frontend type-check, lint, unit, build, and browser tests;
6. database and integration tests;
7. security, secret, dependency, and image scans;
8. build and packaging validation;
9. changed deployment-template rendering or validation.

Capture command, exit status, and meaningful output. “Should pass,” partial suites, skipped failures, and stale results are not evidence.

## Pull request discipline

- Use one branch and one PR per phase unless explicitly directed otherwise.
- Keep the PR draft while implementation or evidence is incomplete.
- Synchronize with the current base before final CI.
- Write a PR body that states scope, non-goals, architecture, security/tenancy impact, compatibility, migrations, deployment, rollback, tests, and residual risks.
- Push all fixes before starting the final evidence run.
- Record the exact head SHA.
- Monitor every required GitHub Actions job to a terminal conclusion.
- Treat skipped, cancelled, neutral, or missing required jobs as unresolved until their acceptability is proven.
- If the head changes, all prior CI evidence is stale; repeat exact-head verification.
- Do not merge, mark ready, or call the phase complete until the acceptance matrix and PR body match the exact code and evidence.
- Never merge without explicit authorization.

## Final acceptance audit

Before handoff, the coordinator must independently verify:

- the PR diff contains only intended changes;
- every acceptance row has code/test/evidence or is honestly marked partial/deferred;
- tests assert behavior rather than implementation trivia;
- runtime code consumes the configuration being documented;
- health checks perform meaningful live checks;
- documentation and deployment examples use current names and defaults;
- no secrets, debug bypasses, placeholder credentials, or unsupported completion claims remain;
- the exact-head workflow URL and job results are recorded.

If any check fails, continue remediation. “Complete” means the implementation, documentation, rollback plan, and exact-head CI evidence all agree.
