# Strata RMM — GitHub Project Board

## Views

1. **Roadmap** (Timeline) — Milestones + sprints
2. **Sprint Board** (Kanban) — Backlog → Ready → In Progress → Review → Done
3. **By Component** (Table) — Group by Agent, Platform, Probe, Infra, Security, Docs
4. **By Owner** (Board) — Eng 1, Eng 2, Eng 3, Unassigned
5. **SOC2 Tracking** (Table) — Filter `label:soc2`

## Custom Fields

| Field | Type | Values |
|-------|------|--------|
| Sprint | Iteration | Sprint 1–6 |
| Component | Single Select | Agent, Platform, Probe, Infra, Security, Docs, CI/CD |
| Priority | Single Select | P0-Critical, P1-High, P2-Medium, P3-Low |
| Story Points | Number | 1, 2, 3, 5, 8, 13 |
| SOC2 Control | Text | CC6.1, CC7.2, etc. |
| Risk | Single Select | Low, Medium, High, Critical |

## Milestones

| Milestone | Target | Scope |
|-----------|--------|-------|
| M1: Agent Hardening | Week 2 | Auto-update core, cosign CI, recording pipeline |
| M2: Session Recording + SOC2 | Week 4 | Full recording + playback, MFA, retention, tamper-proof |
| M3: CVE Automation + Data Residency | Week 6 | NVD/OSV sync, vuln remediation, tenant encryption keys |
| M4: Self-Hosted Packaging | Week 8 | KOTS console, air-gap bundle, installers |
| M5: Beta Launch | Week 10 | Load test, pen test, SOC2 evidence, runbooks |

## Sprint 1 Scope (Weeks 1–2)

| Area | Items | Owner |
|------|-------|-------|
| Agent Auto-Update | Update manifest, cosign pipeline, update client, staged rollout, rollback, BBolt state | Eng 2 |
| Session Recording | StorageBackend interface, MinIO/S3 backends, recording pipeline, tests | Eng 2 + Eng 1 |
| Supporting Infra | Cosign CI, SBOM, Helm CRD, integration tests | Eng 1 + Eng 3 |
