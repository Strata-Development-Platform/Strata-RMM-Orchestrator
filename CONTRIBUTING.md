# Contributing to Strata RMM

Thank you for helping improve Strata RMM.

## Before making changes

1. Search existing issues and pull requests.
2. Open an issue before substantial behavioral, protocol, schema, security, or architectural changes.
3. Base a focused branch on the current `master`.
4. Preserve tenant isolation, authorization boundaries, durable delivery guarantees, and existing API behavior unless an approved design explicitly changes them.
5. Add or update tests and documentation with the implementation.

## Validation

Run the checks relevant to your change. The pull request is not complete until its GitHub Actions checks pass.

```bash
go test ./... -count=1 -race
go vet ./...

cd ui
npm ci
npx tsc -b
npm run lint
npm run build
```

Changes involving PostgreSQL, NATS, endpoint operations, or the technician console must also run the corresponding integration and browser acceptance jobs in GitHub Actions.

## Pull requests

- Keep each pull request focused and explain user and operator impact.
- Do not merge with unresolved review threads or failing required checks.
- Include migrations for schema changes and preserve safe rollback behavior.
- Never commit credentials, customer data, private keys, or generated secrets.
- Keep `.stratalabs/project.json` and `.stratalabs/roadmap.json` valid and aligned with the [StrataLabs integration contract](docs/stratalabs-integration.md).
