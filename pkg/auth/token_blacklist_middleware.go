package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/redis"
)

const (
	tokenBlacklistKey = "rmm:auth:blacklisted_tokens"
	tokenBlacklistTTL = 24 * time.Hour
)

// TokenBlacklistMiddleware checks if a JWT token is blacklisted in Redis.
type TokenBlacklistMiddleware struct {
	rdb *goredis.Client
}

// NewTokenBlacklistMiddleware creates a new token blacklist middleware.
func NewTokenBlacklistMiddleware(rdb *redis.Client) *TokenBlacklistMiddleware {
	return &TokenBlacklistMiddleware{rdb: rdb.RDB()}
}

// Middleware returns an HTTP middleware that checks token blacklists.
func (tb *TokenBlacklistMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			next.ServeHTTP(w, r)
			return
		}

		token := parts[1]
		// Extract token ID from JWT for blacklist check
		tokenID := extractTokenIDFromJWT(token)
		if tokenID == "" {
			next.ServeHTTP(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 50*time.Millisecond)
		defer cancel()

		isBlacklisted, err := tb.rdb.SIsMember(ctx, tokenBlacklistKey, tokenID).Result()
		if err != nil {
			// If Redis fails, allow the request (fail-open)
			next.ServeHTTP(w, r)
			return
		}

		if isBlacklisted {
			http.Error(w, "401 token revoked", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractTokenIDFromJWT extracts the token ID from a JWT token.
// JWT structure: header.payload.signature (base64url encoded)
// The token ID is typically in the payload's "jti" claim.
func extractTokenIDFromJWT(token string) string {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return ""
	}

	payload := parts[1]
	// Decode base64url
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}

	// Look for "jti" claim in the decoded JSON
	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}

	if jti, ok := claims["jti"].(string); ok {
		return jti
	}

	return ""
}
