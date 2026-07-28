# Security Policy

## Reporting a vulnerability

Do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.

Email **security@stratadevplatform.com** with:

- the affected component, branch, release, or commit;
- reproduction steps or a proof of concept;
- the security and tenant-isolation impact;
- relevant logs with secrets and customer data removed; and
- any suggested mitigation.

We will acknowledge the report, investigate it, and coordinate disclosure after an appropriate fix is available.

## Supported versions

Strata RMM is currently incubating. Until the first stable release, security fixes are applied to the current `master` branch and incorporated into the next published release.

## Security-sensitive contributions

Changes affecting authentication, authorization, tenancy, agent identity, command execution, cryptography, audit evidence, update verification, or secret handling require focused security tests and successful GitHub Actions security checks.
