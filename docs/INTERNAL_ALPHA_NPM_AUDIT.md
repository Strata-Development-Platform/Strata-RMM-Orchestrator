# Internal-alpha frontend advisory disposition

`npm audit --json` and `npm audit fix --dry-run --json` were run on 2026-07-31. npm reported four vulnerable package records: three moderate and one high. The dry run proposed no non-breaking changes.

| Package | Dependency path | Severity | Exposure | npm safe upgrade |
| --- | --- | --- | --- | --- |
| `vite@5.4.21` | root development dependency | high | Development/build server only; the production bundle does not ship Vite. The development server must not be exposed to untrusted networks. | No. npm proposes Vite `8.2.0`, a semver-major migration. |
| `esbuild@0.21.5` | root → `vite@5.4.21` → `esbuild` | moderate | Development/build server only; affected cross-origin development-server behavior is not in the static production bundle. | No. npm proposes the same semver-major Vite 8 migration. |
| `react-router-dom@6.30.4` | direct production dependency | moderate | Production navigation is exposed to the open-redirect/XSS advisory when attacker-controlled destinations reach routing APIs. | No. npm proposes React Router DOM `7.18.2`, a semver-major migration. |
| `react-router@6.30.4` | root → `react-router-dom@6.30.4` → `react-router` | moderate | Production routing is exposed to the backslash open-redirect advisory. The SSR hydration advisory is not reachable in the current client-only Vite application, but remains in the installed dependency. | No. npm proposes the same semver-major React Router 7 migration. |

No `--force` update was applied because both available remediations require framework/toolchain migrations and compatibility work outside this endpoint-agent PR. Hosted-alpha promotion requires a separate tested Vite 8 and React Router 7 upgrade or formal risk acceptance. Until then, do not expose the Vite development server, and do not pass untrusted absolute destinations to `Link` or `navigate`.
