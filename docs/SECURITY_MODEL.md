# Security Model

## Current State (Pre-Rewrite)

### Critical Findings

1. **Hardcoded JWT secret**: `strata-rmm-dev-secret` in 6 locations
2. **Placeholder password hash**: `$2a$10$placeholder` in seed data
3. **In-memory enrollment tokens**: Not persisted, lost on restart
4. **Symmetric signing (HS256)**: No public-key verification
5. **No refresh tokens**: Single JWT with no rotation

### Authentication Flow

```
User → POST /api/v1/auth/login → bcrypt verify → JWT issued → stored in localStorage
API call → Authorization: <raw JWT> → server validates HMAC → identity+roles extracted
```

### Authorization Flow (Current)

- Each handler checks `r.Header.Get("Authorization")` individually
- No centralized auth middleware
- `r.PathValue("tenantID")` used directly — trust on client-provided ID
- RLS policies exist on some tables but database user is owner (`strata`)

### Required Changes

1. Externalize JWT secret via environment variable
2. Add non-owner PostgreSQL role with restricted permissions
3. Migrate to asymmetric signing (RS256/ES256) with key rotation
4. Implement centralized auth middleware with route declarations
5. Replace in-memory enrollment tokens with persisted, cryptographically secure tokens
6. Add refresh token flow with rotation
7. Audit every route for authorization correctness
