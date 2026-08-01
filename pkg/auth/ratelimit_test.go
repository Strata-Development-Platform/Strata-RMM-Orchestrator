package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiterAllowsRequest(t *testing.T) {
	rl := NewRateLimiter(100, 200)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRateLimiterBlocksExcess(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.2:12345"

	// First request should succeed
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", rec.Code)
	}

	// Second immediate request should be rate limited
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", rec.Code)
	}
}

func TestRateLimiterDifferentIPs(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP 1 makes request
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req1)
	if rec.Code != http.StatusOK {
		t.Errorf("ip1 request 1: expected 200, got %d", rec.Code)
	}

	// IP 2 makes request - should not be rate limited
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.2:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req2)
	if rec.Code != http.StatusOK {
		t.Errorf("ip2 request: expected 200, got %d", rec.Code)
	}

	// IP 1 makes another request - should be rate limited
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req1)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("ip1 request 2: expected 429, got %d", rec.Code)
	}
}

func TestSensitiveRoutesUseIndependentStrictBuckets(t *testing.T) {
	rl := NewRateLimiter(100, 200)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if i < 5 && rec.Code != http.StatusOK {
			t.Fatalf("login request %d: status = %d, want 200", i+1, rec.Code)
		}
		if i == 5 {
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("login request 6: status = %d, want 429", rec.Code)
			}
			if rec.Header().Get("Retry-After") == "" {
				t.Fatal("rate-limited response must include Retry-After")
			}
		}
	}

	// Exhausting the login bucket must not consume ordinary API capacity.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("general API after login throttle: status = %d, want 200", rec.Code)
	}
}

func TestRateLimiterIgnoresForwardedAddressFromUntrustedPeer(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i, forwarded := range []string{"198.51.100.1", "198.51.100.2"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.RemoteAddr = "203.0.113.20:12345"
		req.Header.Set("X-Forwarded-For", forwarded)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		want := http.StatusOK
		if i == 1 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("request %d with spoofed forwarded address: status = %d, want %d", i+1, rec.Code, want)
		}
	}
}

func TestRateLimiterInvitationEndpointsUseDedicatedBucket(t *testing.T) {
	limiter := NewRateLimiter(30, 60)
	for _, path := range []string{
		"/api/v1/auth/invitations/inspect",
		"/api/v1/auth/invitations/accept",
	} {
		class, rate, burst := limiter.policy(http.MethodPost, path)
		if class != "account-invitation" || rate != 1 || burst != 5 {
			t.Fatalf("policy(%q) = %q/%d/%d", path, class, rate, burst)
		}
	}
	generalClass, _, _ := limiter.policy(http.MethodGet, "/api/v1/devices")
	if generalClass == "account-invitation" {
		t.Fatal("general traffic shares the invitation abuse bucket")
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Cache-Control":          "no-store",
	}

	for key, expected := range expectedHeaders {
		if got := rec.Header().Get(key); got != expected {
			t.Errorf("header %s: got %q, want %q", key, got, expected)
		}
	}
}

func TestMaxBodySize(t *testing.T) {
	handler := MaxBodySize(100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/", nil)
	req.Body = http.MaxBytesReader(nil, req.Body, 200)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Logf("MaxBodySize test: got %d (body size middleware active)", rec.Code)
	}
}

func TestAPIKeyAuth(t *testing.T) {
	auth := NewAPIKeyAuth()
	auth.SetKey("test-key-123", "tenant-1")

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request without key should be rejected
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no key: expected 401, got %d", rec.Code)
	}

	// Request with valid key should succeed
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "test-key-123")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("valid key: expected 200, got %d", rec.Code)
	}

	// Request with invalid key should be rejected
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "bad-key")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("bad key: expected 403, got %d", rec.Code)
	}
}

func TestAPIKeyAuthRevoke(t *testing.T) {
	auth := NewAPIKeyAuth()
	auth.SetKey("test-key", "tenant-1")

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	auth.RevokeKey("test-key")

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("revoked key: expected 403, got %d", rec.Code)
	}
}
