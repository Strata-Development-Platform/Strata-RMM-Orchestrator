# Phase 8G Security and Alpha-Readiness Evidence

## Decision

**Internal alpha: pending exact-head CI and a bounded hosted smoke exercise.**

**Production beta: Not accepted.** Mandatory environment exercises, a signed
incident-response tabletop, an external penetration test, and final operational
sign-off remain outstanding. Code or CI presence is not evidence that those
human/environment gates occurred.

## Implementation ledger

| Gate | Implementation and negative proof | Current status |
|---|---|---|
| A8-22 Authorization | privileged route inventory; platform/deployment prefixes fail closed; every inventoried admin route denies non-platform admins and accepts platform admins | implemented; exact-head CI pending |
| A8-23 Security pipeline | Go/race authorization contracts, govulncheck, gosec, frontend production dependency audit, image scan, SPDX SBOM, repository-history secret scan, existing browser CI | implemented; exact-head CI pending |
| A8-24 Incident response | response roles/procedure and four-scenario tabletop template | Not accepted; signed exercise required |
| A8-25 Operations | existing alert/runbook ownership plus incident escalation procedure | partial; hosted paging/runbook audit pending |
| A8-26 Beta decision | truthful gate inventory and explicit stop conditions | Not accepted; mandatory evidence incomplete |

## Security changes

- Deployment state/history are platform-admin only.
- Unknown routes in privileged namespaces cannot fall through to ordinary user
  access.
- Login, enrollment, remote access, privileged mutations, public downloads,
  probes, and general requests have isolated source-address buckets. Untrusted
  forwarded headers never select the bucket identity.
- JWT rotation accepts one explicitly configured previous secret only during a
  bounded overlap; new tokens are always signed with the current secret.
- The tracked startup wrapper contains no JWT or database credential and refuses
  to start without operator-supplied values.
- React Router is updated to the latest compatible 6.x release. Two moderate
  advisories remain without an advisory-free upgrade path: the application does
  not use the affected SSR/RSC features and all navigation targets are
  application-generated same-origin paths. High/critical production dependency
  findings remain CI blockers; this residual risk is R8-20.

## Evidence to populate after the final push

| Evidence | Exact head | Workflow URL | Result |
|---|---|---|---|
| Phase 8G Security Gate | pending | pending | pending |
| Full repository CI | pending | pending | pending |
| Earlier Phase 8 workflows | pending | pending | pending |

## Remaining mandatory environment work

- A8-09 timestamped recovery RPO/RTO drill;
- A8-10/A8-12 hosted dashboard, synthetic, and paging delivery exercise;
- A8-14–A8-17 baseline load, 24-hour soak, reconnect storm, and dependency
  failure matrix;
- A8-18–A8-21 hosted MSP lifecycle exercise;
- A8-24 signed tabletop and independent penetration test/re-test;
- A8-25 escalation/status/support audit and A8-26 signed go/no-go record.
