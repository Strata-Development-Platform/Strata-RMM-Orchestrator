# External Penetration-Test Scope

## Objective

An independent tester must attempt to violate authentication, authorization,
tenant isolation, agent trust, endpoint-command safety, object access, custom
domain routing, and operator/deployment boundaries in an isolated environment
containing synthetic data only.

## In scope

- browser and API authentication, MFA, token validation/rotation, session expiry;
- all `/api/v1/admin/*`, `/api/v2/platform/*`, deployment, support-grant,
  membership, entitlement, offboarding/export, remote, recording, enrollment,
  job, approval, script, software, device-action, and audit routes;
- horizontal and vertical access across platform/MSP/client/site/device scopes;
- agent enrollment and identity replay, NATS subject/consumer permissions, job
  correlation/attempt/idempotency behavior;
- custom/subdomain host confusion and forwarded-header spoofing;
- object enumeration, signed/download paths, report/recording authorization;
- rate-limit bypass, oversized/malformed input, error disclosure, SSRF and common
  injection classes;
- build artifact, container, SBOM, dependency, deployment status, backup/recovery
  metadata, and secret/log exposure.

## Rules and exclusions

Use only the designated test environment and accounts. No production systems,
customer data, uncontrolled denial of service, destructive persistence outside
fixtures, social engineering, or third-party infrastructure testing is allowed.
Coordinate high-volume tests with operations and preserve correlation IDs.

## Finding and remediation contract

Each finding includes severity, affected requirement/asset, reproducible steps,
evidence, impact, and remediation guidance. Critical/high findings block beta.
Engineering adds a regression test, records the exact fixing SHA, and the
independent tester re-tests the original reproduction. Risk acceptance requires
security and release-owner signatures, an expiry date, and a compensating
control; it cannot be inferred from an open issue or passing CI.
