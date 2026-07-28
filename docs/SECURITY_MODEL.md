# Security Model

## Current verified state after Phase 7

The original prototype findings below drove Phases 1–7. The following controls are now part of the verified baseline:

- the hardcoded JWT-secret fallback was removed and required configuration fails closed;
- route classification and authentication/authorization middleware are centralized;
- MSP, client, site, device, and agent scope is propagated through authenticated requests and database transactions;
- PostgreSQL restricted-role/RLS and cross-tenant negative tests exercise isolation without relying on an owner bypass;
- enrollment tokens are cryptographically secure, database-backed, scoped, expiring, revocable, and single-use;
- authenticated agent transactions establish tenant scope before accessing RLS-protected state;
- endpoint operations require capability, policy, approval, lifecycle, and idempotency checks;
- immutable endpoint audit evidence is insert/select-only for application roles;
- GitHub CI includes dedicated database, isolation, integration, security, durable-job, endpoint-operation, and browser acceptance coverage.

This is a verified engineering baseline, not a declaration of unrestricted production readiness.

## Phase 8 security work

The following remain mandatory before a hosted production beta can be approved:

1. complete an updated threat model and data-flow inventory;
2. migrate signing/session design to an approved asymmetric and refresh-token rotation model, or document and approve a compensating design;
3. centralize production secret storage, access auditing, rotation, escrow, and recovery;
4. audit every privileged route and background worker against an explicit authorization matrix;
5. harden remote access, recordings, public endpoints, custom-domain routing, and abuse controls;
6. enforce dependency, container, secret, SBOM, static-analysis, and browser security gates;
7. perform incident-response/tabletop exercises and define external penetration-test scope;
8. close every launch-blocking item in the [Phase 8 risk register](PHASE_8_RISK_REGISTER.md) and [acceptance matrix](PHASE_8_ACCEPTANCE_MATRIX.md).

## Authentication flow

```text
User login
  → credential and MFA verification
  → signed access token
  → centralized route classification
  → authenticated identity and membership
  → scoped database transaction
  → authorization and RLS enforcement
```

Agent enrollment establishes a persistent, scoped agent identity. Subsequent agent traffic must authenticate that identity and match MSP, client, site, device, agent, subject, correlation, attempt, and expiry constraints.

## Authorization and isolation invariants

- Platform, MSP, client, site, device, and agent scopes are explicit; request path identifiers are never trusted alone.
- Application queries run under restricted database roles with RLS scope set inside the transaction.
- Background workers and messaging consumers carry the same ownership context as synchronous API requests.
- Cross-scope reads and writes fail closed and are covered by negative tests.
- Privileged endpoint work requires policy evaluation and, where applicable, an unexpired approval from a different authorized actor.
- Duplicate or replayed destructive work cannot execute twice.
- Audit evidence may be appended and read by authorized roles but not mutated or deleted through application paths.
- Secrets, tokens, credentials, sensitive command payloads, and customer content must not appear in logs or artifacts.

## Historical pre-rewrite findings

The following findings describe the starting prototype and are retained for traceability:

1. a hardcoded `strata-rmm-dev-secret` appeared in multiple locations;
2. seed data contained a placeholder password hash;
3. enrollment tokens were kept only in memory;
4. HS256 signing had no public-key verification or rotation;
5. refresh-token/session rotation was absent;
6. handlers performed inconsistent per-route checks;
7. client-supplied tenant identifiers could be trusted too early;
8. RLS coverage was incomplete and owner connections could bypass it.

Items 1–3 and the route/RLS containment portions of 6–8 were remediated in completed phases. Items 4–5 and the full production audit remain Phase 8 gates.
