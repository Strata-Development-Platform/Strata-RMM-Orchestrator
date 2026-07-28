# StrataLabs website integration

The StrataLabs website synchronizes this public repository approximately once per hour and caches the last successful result.

## Discovery contract

For this project to appear correctly:

- the repository must remain public and unarchived;
- the repository must have the GitHub topic `stratalabs-project`;
- `.stratalabs/project.json` must follow schema version 1;
- `.stratalabs/roadmap.json` must use supported roadmap and milestone statuses;
- `docs/index.md` must remain the documentation entry point; and
- published GitHub Releases provide release history and downloadable assets.

## Maintainer checklist

When public project information changes:

1. Update `.stratalabs/project.json`.
2. Update stable milestone IDs and statuses in `.stratalabs/roadmap.json`.
3. Update `README.md`, `docs/index.md`, and `CHANGELOG.md` when applicable.
4. Validate both JSON documents.
5. Confirm links use HTTPS or `null`.
6. Confirm the `stratalabs-project` topic is still present.
7. Merge only after CI succeeds.
8. Wait for the hourly synchronization and inspect the rendered StrataLabs page.

The website controls presentation. Repository metadata supplies content and cannot override the StrataLabs theme.
