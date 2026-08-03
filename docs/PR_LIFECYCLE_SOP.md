# Pull Request Lifecycle — Controlled Development and Verification Standard

Applies to: Every feature, defect, migration, deployment, security, and documentation PR.  
Authority: Human owner controls merge authorization.

**Core rule:** A pull request is not ready because code exists or a status says implemented. It is ready only when the exact final head has terminal evidence, limitations are truthful, and the owner explicitly authorizes merge.

---

## 1. Purpose and Governing Principles

This lifecycle keeps development reviewable, tenant-safe, recoverable, and evidence-based. It applies from the first fetch of master through post-merge verification.

- One focused development PR at a time unless the owner explicitly authorizes parallel work.
- Every branch starts from verified current remote master.
- A coherent vertical slice includes every layer required for one real outcome.
- Tests prove runtime behavior; routes, schemas, mocks, controls, and documentation alone do not.
- Only workflows associated with the exact final head count as CI evidence.
- Unavailable representative infrastructure remains Environment pending and is never marked passed.
- No merge occurs without explicit human authorization after final evidence is reported.

## 2. Lifecycle at a Glance

| Gate | State | Required Outcome |
|------|-------|-----------------|
| 1 | Orient | Fetch remote master, verify SHA and merged history, inspect open PRs, read instructions. |
| 2 | Audit | Trace existing behavior and evidence; identify the smallest coherent gap without duplication. |
| 3 | Plan | Define acceptance criteria, non-goals, risks, migration/rollback, tests, and PR boundary. |
| 4 | Implement | Build the full vertical outcome with security, tenancy, observability, tests, and documentation. |
| 5 | Validate locally | Run every locally available relevant unit, race, lint, integration, UI, browser, and packaging check. |
| 6 | Open draft PR | Publish one branch and create a draft PR with base SHA, scope, impacts, and evidence plan. |
| 7 | Verify exact head | Monitor every applicable workflow to terminal; inspect and correct real failures. |
| 8 | Complete evidence | Run required representative-host or operational exercises and document limitations accurately. |
| 9 | Request authorization | Present final SHA, workflow URLs/job totals, failures, migrations, rollback, and blockers. |
| 10 | Merge and verify | After explicit approval, merge pinned head, fetch master, verify merge commit and open-PR state. |

## 3. Gate 1 — Starting-State Verification

No file may be changed until the agent establishes remote truth.

1. Fetch origin and resolve the current `origin/master` SHA.
2. Inspect recent commits and confirm the expected prior PRs are merged.
3. List open PRs and determine whether another authorized development PR exists.
4. Read `CONTRIBUTING.md`, `AGENTS.md` when present, and the architecture, security, compatibility, deployment, upgrade, rollback, feature matrix, and acceptance documents relevant to the slice.
5. Report the base SHA, open-PR state, instructions found, and proposed audit scope before implementation.

**Stop condition:** If master, branch ownership, PR state, or instructions are ambiguous, stop and obtain direction. Do not create a speculative branch or PR.

## 4. Gate 2 — Evidence-First Audit

Trace the intended behavior end to end before deciding what to build. The audit must include entry points, identity, authorization, tenant hierarchy, persistence, messaging, object storage, endpoint execution, UI state, audit records, metrics, alerts, runbooks, and existing tests.

| Classification | Meaning |
|----------------|---------|
| Verified | Real runtime behavior is exercised by automated behavioral or integration evidence. |
| Partial | Useful behavior exists, but advertised functionality or proof is incomplete. |
| Not implemented | No real runtime implementation exists; a stub, empty result, or no-op does not count. |
| Environment pending | Implementation and deterministic automation exist, but representative infrastructure is still required. |

The audit must identify the smallest coherent missing behavior and reuse existing primitives. A new subsystem is justified only when the existing design cannot safely support the required behavior.

## 5. Gate 3 — Plan the Bounded Vertical Slice

The plan becomes the PR contract. It must be specific enough that a reviewer can determine whether the PR is complete.

- User or operator outcome and exact acceptance criteria.
- Explicit non-goals and remaining work after this slice.
- Threat, authorization, tenant, client, site, and endpoint boundary analysis.
- API, protocol, agent, database, and backward-compatibility impact.
- Migration, backfill, locking, upgrade, downgrade, rollback, and data-loss analysis.
- Local, GitHub Actions, browser, fault, and representative-infrastructure test matrix.
- Observability, audit, failure-state, and runbook requirements.

## 6. Gate 4 — Branch and Implementation

1. Create a new named branch from the verified `origin/master`. Never reuse a stale branch.
2. Preserve unrelated user changes and keep the working tree intentional.
3. Implement the complete vertical behavior, not only a handler, schema, UI control, or test double.
4. Use database-proven authorization, forced RLS where required, transactions, concurrency control, durable idempotency, bounded inputs/outputs/retries, and secret-safe logging.
5. Add metrics, audit records, user-visible state, errors, and runbook guidance with the implementation.
6. Commit intentionally with descriptive messages and no credentials, customer data, generated secrets, or private keys.

## 7. Gate 5 — Local Verification

Run every relevant check available in the local environment before publishing. A missing tool or unavailable service must be reported, not silently converted into a pass.

| Layer | Minimum Proof When Applicable |
|-------|-------------------------------|
| Go | Formatting, unit tests, race detector, vet, lint, builds, error-path tests. |
| PostgreSQL/TimescaleDB | Clean and upgrade migrations, constraints, transactions, concurrency, forced RLS, cross-scope denial, rollback. |
| NATS JetStream | Dispatch, acknowledgment, outage, bounded queue, reconnect, replay, expiry, duplicate delivery, restart. |
| Object storage | Authorized write/read/list/delete, integrity, metadata, outage, restart, restore, cross-tenant denial. |
| Agents | Parsing, validation, capability, execution, timeout, cancellation, output bounds, identity, service lifecycle, OS support. |
| Frontend/browser | Types, lint, unit tests, build, complete positive journey, persisted state, negative authorization, accessibility. |
| Deployment/security | Compose/Helm/systemd/PowerShell validation, secret/SAST/dependency/image/container scans, SBOM. |

## 8. Gate 6 — Draft PR Creation

Open exactly one draft PR when the coherent implementation and initial validation are ready for remote verification. Keep it draft and unmerged until all evidence gates are complete.

| PR Body Section | Required Content |
|-----------------|-----------------|
| Scope | Outcome, bounded behavior, non-goals, user/operator impact. |
| Revision | Base SHA, exact current head SHA, branch, draft/unmerged statement. |
| Security and tenancy | Threats, authorization, RLS, cross-scope behavior, secret handling. |
| Compatibility | API/protocol/agent impact and supported upgrade path. |
| Migration and rollback | Migration ID, backfill, locking, downgrade behavior, precise rollback and preservation steps. |
| Testing | Tests added, what they prove, local limitations, hosted requirements. |
| Workflow evidence | Every applicable exact-head URL, per-workflow job totals, overall result. |
| Failures and reruns | Original run/job, root cause, corrective commit, external-transient evidence and any permitted rerun. |
| Limitations | Environment-pending items, unsupported behavior, remaining blockers. |

## 9. Gate 7 — Exact-Head Workflow Verification

**Exact-head rule:** Evidence from an older commit does not transfer to a newer PR head. After every push, resolve the new remote SHA and monitor only workflows associated with that SHA.

1. Enumerate every workflow applicable to the changed paths and behavior.
2. Monitor each workflow and job until terminal; queued and in-progress are not passes.
3. If a job fails, inspect its logs and classify the failure before acting.
4. Fix deterministic code, test, configuration, race, flake, or packaging failures in a new commit. Do not rerun them.
5. Allow at most one rerun only for a documented external transient such as a registry timeout, runner outage, or provider network failure.
6. Never repeatedly rerun a flaky job. Treat flakiness as a defect and remove the source of nondeterminism.
7. Record all failures, fixes, reruns, workflow URLs, and job totals in the PR body.

## 10. Failure Decision Matrix

| Observed Cause | Classification | Required Action | Rerun? |
|----------------|----------------|-----------------|--------|
| Compilation, lint, unit, race, migration, fixture, authorization, browser, or scan defect | Deterministic | Fix root cause, strengthen regression proof, push new head, verify all applicable workflows. | No |
| Intermittent assertion, timing race, order dependency, leaked state | Product/test defect | Remove nondeterminism; do not normalize flakiness. | No |
| Registry authentication/network timeout with unchanged code and external evidence | External transient | Document evidence; one targeted rerun may be used. | Once |
| Runner or GitHub service outage | External transient | Document service evidence; one rerun may be used after recovery. | Once |
| Unknown | Unresolved | Investigate logs and reproduce; do not guess or rerun until classified. | No |

## 11. Gate 8 — Representative and Operational Evidence

Some claims require real infrastructure even after CI passes. Examples include native systemd installation, Windows service installation, public ACME certificate issuance, live dashboard observation, live provider delivery, RPO/RTO drills, load, soak, reconnect storms, incident tabletop, and go/no-go signoff.

If infrastructure is unavailable, the repository may provide deterministic automation and an evidence procedure, but the PR must retain an Environment pending limitation. It must not claim the hosted gap is closed.

## 12. Gate 9 — Final Pre-Merge Review

Before requesting authorization, the agent must present a concise final report proving each item below.

- The base is current or deliberately reconciled with current master.
- The local and remote final head SHA are exact and unchanged.
- The PR is the sole authorized development PR and remains draft/unmerged.
- All review threads are resolved.
- Every applicable exact-head workflow is terminal and successful, with URLs and job totals recorded.
- Positive, malformed, unauthorized, cross-tenant, cross-endpoint, concurrency, retry, duplicate, outage, restart, and rollback paths relevant to the slice passed.
- Migrations passed clean install, supported upgrade, same-version redeploy, and rollback/downgrade analysis.
- Documentation, feature matrix, threat model, compatibility, evidence index, and runbooks match runtime truth.
- Security and supply-chain evidence is terminal.
- Representative infrastructure evidence is complete, or remaining claims are explicitly Environment pending.

**Mandatory pause:** Stop after the final report. Do not mark the PR ready and do not merge until the human owner explicitly authorizes merge.

## 13. Gate 10 — Authorized Merge and Post-Merge Verification

1. Confirm the owner explicitly authorized the specific PR after the final report.
2. Verify the authorized head SHA has not changed.
3. Mark the draft ready if the repository requires it.
4. Merge using the established method and expected-head protection so a moved head is rejected.
5. Fetch `origin/master` and verify it resolves to or contains the returned merge commit.
6. Confirm the PR is merged and no unintended development PR remains open.
7. Report the merge SHA, master verification, open-PR state, and any post-merge workflow status.
8. Start no new slice until the owner directs the next action.

## 14. Prohibited Shortcuts

- Do not count implemented status, documentation, a route, schema, UI control, stub, mock, compile-only build, or empty result as acceptance.
- Do not use workflow evidence from an earlier head.
- Do not rerun deterministic failures or repeatedly rerun flaky tests.
- Do not merge while workflows, jobs, hosted exercises, review threads, migrations, or rollback evidence are incomplete.
- Do not mark unavailable infrastructure passed.
- Do not create a second development PR while the current one is being verified unless explicitly authorized.
- Do not merge without explicit authorization, even when every check is green.

## 15. Standard Handoff Record

At every pause or transfer to another agent, record the following so work can resume without guessing:

| Field | Required Value |
|-------|---------------|
| Repository and branch | Owner/repository and exact branch name. |
| Base and head | Verified base SHA and exact remote head SHA. |
| PR | Number, URL, draft/ready/merged state, merge authorization state. |
| Scope | Completed behavior, current work, explicit non-goals. |
| Files and migrations | Changed paths, migration IDs, compatibility and rollback. |
| Tests | Local commands/results and what could not run locally. |
| Workflows | Exact-head run IDs/URLs, status, job totals, failures, reruns. |
| Remaining evidence | Review, infrastructure, operational, security, or documentation blockers. |
| Next action | One precise next step; state whether merge is prohibited or authorized. |

## 16. Definition of a Closed PR Lifecycle

A lifecycle is closed only when the intended code is merged under explicit authorization, remote master is verified, the PR state is confirmed, evidence remains durable and linked, limitations are truthful, and no unintended development PR remains open. Green checks alone do not close the lifecycle.
