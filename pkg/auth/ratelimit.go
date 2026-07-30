package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int
	burst    int
}

type visitor struct {
	tokens   int
	lastSeen time.Time
}

func NewRateLimiter(rate, burst int) *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		burst:    burst,
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	go rl.cleanup()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		rate, burst := rl.rate, rl.burst
		if strings.HasPrefix(r.URL.Path, "/health") {
			rate, burst = 100, 200
		} else if strings.HasPrefix(r.URL.Path, "/api/v1/enroll") {
			rate, burst = 5, 10
		} else if strings.HasPrefix(r.URL.Path, "/api/v1/mfa") {
			rate, burst = 10, 20
		} else if strings.HasPrefix(r.URL.Path, "/api/v1/recordings") {
			rate, burst = 20, 40
		}

		rl.mu.Lock()
		v, exists := rl.visitors[ip]
		if !exists {
			v = &visitor{tokens: burst}
			rl.visitors[ip] = v
		}

		elapsed := time.Since(v.lastSeen)
		v.lastSeen = time.Now()
		v.tokens += int(elapsed.Seconds()) * rate
		if v.tokens > burst {
			v.tokens = burst
		}

		if v.tokens <= 0 {
			rl.mu.Unlock()
			http.Error(w, "429 too many requests", http.StatusTooManyRequests)
			return
		}
		v.tokens--

		if v.tokens < burst/4 {
			w.Header().Set("X-RateLimit-Warning", "approaching limit")
		}

		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		threshold := time.Now().Add(-10 * time.Minute)
		for ip, v := range rl.visitors {
			if v.lastSeen.Before(threshold) {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

type APIKeyAuth struct {
	apiKeys map[string]string
	mu      sync.RWMutex
}

func NewAPIKeyAuth() *APIKeyAuth {
	return &APIKeyAuth{
		apiKeys: make(map[string]string),
	}
}

func (a *APIKeyAuth) SetKey(key, tenantID string) {
	a.mu.Lock()
	a.apiKeys[key] = tenantID
	a.mu.Unlock()
}

func (a *APIKeyAuth) RevokeKey(key string) {
	a.mu.Lock()
	delete(a.apiKeys, key)
	a.mu.Unlock()
}

func (a *APIKeyAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" {
			http.Error(w, "API key required", http.StatusUnauthorized)
			return
		}
		a.mu.RLock()
		_, valid := a.apiKeys[key]
		a.mu.RUnlock()
		if !valid {
			http.Error(w, "invalid API key", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
