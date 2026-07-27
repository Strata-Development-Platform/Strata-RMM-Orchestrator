# API Compatibility

## Current API Surface

66 routes across 18 categories. All routes are under `/api/v1/` or `/` and `/health`.

### Compatibility Commitment

All `/api/v1` routes will be preserved throughout the rewrite. New routes may be added under `/api/v2/` for incompatible behavior. Legacy route adapters will be maintained until all supported clients and agents migrate.

### Contract Testing

Before modifying any route:
1. Capture current request/response shape
2. Create contract tests
3. Verify backward compatibility
4. Only then modify implementation

### Route Categories

| Category | Routes | Priority |
|----------|--------|----------|
| System | 2 | 🟢 Keep |
| Agent | 5 | 🟢 Keep |
| Auth | 2 | 🟢 Keep |
| Platform | 8 | 🟢 Keep |
| Admin | 4 | 🟢 Keep |
| Metrics | 3 | 🟢 Keep |
| Alerts | 6 | 🟢 Keep |
| Vulnerabilities | 5 | 🟢 Keep |
| CVE | 7 | 🟢 Keep |
| Third-Party | 4 | 🟢 Keep |
| Reports | 5 | 🟢 Keep |
| Remote | 3 | 🟢 Keep |
| Update | 2 | 🟢 Keep |
| Keys | 5 | 🟢 Keep |
| Access Control | 3 | 🟢 Keep |
| Scripts | 7 | 🟢 Keep |
| Software | 6 | 🟢 Keep |
| MFA | 4 | 🟢 Keep |
| Recordings | 3 | 🟢 Keep |
