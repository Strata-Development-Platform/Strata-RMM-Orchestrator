package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/redis"
)

const (
	rateLimiterKeyPrefix = "rmm:ratelimit:" // #nosec G101 -- not a credential, it's a Redis key prefix
	rateLimiterTTL       = 60 * time.Second
)

// RedisRateLimiter implements rate limiting using Redis for horizontal scaling.
type RedisRateLimiter struct {
	rdb   *goredis.Client
	rate  int
	burst int
}

// NewRedisRateLimiter creates a new Redis-backed rate limiter.
func NewRedisRateLimiter(rdb *redis.Client, rate, burst int) *RedisRateLimiter {
	return &RedisRateLimiter{
		rdb:   rdb.RDB(),
		rate:  rate,
		burst: burst,
	}
}

// Middleware returns an HTTP middleware that enforces rate limiting using Redis.
func (rl *RedisRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		class, burst := rl.policy(r.Method, r.URL.Path)
		key := rateLimiterKeyPrefix + class + ":" + ip

		ctx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
		defer cancel()

		// Use Redis sorted set for sliding window rate limiting.
		// Each request gets a timestamp as score, member is unique.
		now := float64(time.Now().UnixNano())
		window := float64(time.Second.Nanoseconds())

		// Use pipeline to batch operations
		countCmd := rl.rdb.ZCount(ctx, key, strconv.FormatFloat(now-window, 'f', -1, 64), "+inf")
		rl.rdb.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatFloat(now-window, 'f', -1, 64))
		rl.rdb.ZAdd(ctx, key, goredis.Z{
			Score:  now,
			Member: fmt.Sprintf("%d:%x", int64(now), time.Now().UnixNano()%0xFFFF),
		})
		rl.rdb.Expire(ctx, key, rateLimiterTTL)

		count, err := countCmd.Result()
		if err != nil {
			// If Redis fails, allow the request (fail-open)
			// The caller should log this
			next.ServeHTTP(w, r)
			return
		}

		if count > int64(burst) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "429 too many requests", http.StatusTooManyRequests)
			return
		}

		if count > int64(burst*3/4) {
			w.Header().Set("X-RateLimit-Warning", "approaching limit")
		}

		next.ServeHTTP(w, r)
	})
}

// policy returns an isolated bucket class and its burst limit. Sensitive
// operations cannot borrow capacity from the general API bucket. RemoteAddr is
// intentionally used as the network identity: forwarded headers are untrusted.
func (rl *RedisRateLimiter) policy(method, path string) (string, int) {
	switch {
	case method == "POST" && strings.HasPrefix(path, "/api/v1/admin/"):
		return "admin", 20
	case method == "POST" && strings.HasPrefix(path, "/api/v1/auth/"):
		return "auth", 30
	case method == "POST":
		return "write", 60
	case method == "GET":
		return "read", 180
	default:
		return "default", 90
	}
}
