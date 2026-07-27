# Credential Rotation and Git History Remediation

## Exposed Credential Incident Response

A production-format email address and plaintext password were committed in `test/api_test.go` at commit `90c7b78`.

### Immediate Actions Taken
1. Hardcoded credentials removed from `test/api_test.go` (commit `XXXXXX`)
2. Test now reads credentials from environment variables `TEST_ADMIN_EMAIL` and `TEST_ADMIN_PASSWORD`
3. Secret scanning added to CI pipeline via TruffleHog
4. `.env.example` added with placeholder values only

### Credential Rotation Instructions
1. Change the password for the affected account immediately
2. Review authentication logs for any suspicious activity
3. If the account has any API keys or tokens, revoke and recreate them
4. Update `TEST_ADMIN_EMAIL` and `TEST_ADMIN_PASSWORD` in any CI secrets

### Git History Remediation
**Warning**: Deleting the credential from the latest commit does NOT remove it from Git history. Anyone with repository access can see it.

To fully remove from history using `git filter-repo`:
```bash
pip install git-filter-repo
git filter-repo --path test/api_test.go --invert-paths
git push origin --force --all
```

Alternative using BFG:
```bash
java -jar bfg.jar --replace-text passwords.txt
git reflog expire --expire=now --all && git gc --prune=now --aggressive
git push origin --force --all
```

**Important**: Force-pushing rewritten history will invalidate all existing clones, open PRs, and CI runs. Coordinate with all collaborators before proceeding.

### Prevention
- Pre-commit hooks with `trufflehog` or `gitleaks`
- CI secret scanning (already added)
- Environment variables for all test credentials
- Code review for hardcoded secrets
