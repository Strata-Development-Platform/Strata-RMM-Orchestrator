package auth

import (
	"testing"
)

func TestRedisRateLimiterPolicy(t *testing.T) {
	rl := &RedisRateLimiter{rdb: nil, rate: 10, burst: 30}

	tests := []struct {
		method    string
		path      string
		want      string
		wantBurst int
	}{
		{"POST", "/api/v1/admin/users", "admin", 20},
		{"POST", "/api/v1/admin/settings", "admin", 20},
		{"POST", "/api/v1/auth/login", "auth", 30},
		{"POST", "/api/v1/auth/register", "auth", 30},
		{"POST", "/api/v1/jobs", "write", 60},
		{"POST", "/api/v1/devices", "write", 60},
		{"GET", "/api/v1/jobs", "read", 180},
		{"GET", "/api/v1/devices", "read", 180},
		{"PUT", "/api/v1/jobs", "default", 90},
		{"DELETE", "/api/v1/devices", "default", 90},
	}

	for _, tt := range tests {
		t.Run(tt.method+"-"+tt.path, func(t *testing.T) {
			class, burst := rl.policy(tt.method, tt.path)
			if class != tt.want {
				t.Errorf("policy(%q, %q) class = %q, want %q", tt.method, tt.path, class, tt.want)
			}
			if burst != tt.wantBurst {
				t.Errorf("policy(%q, %q) burst = %d, want %d", tt.method, tt.path, burst, tt.wantBurst)
			}
		})
	}
}

func TestExtractTokenIDFromJWT(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		wantID string
	}{
		{
			name: "valid JWT with jti",
			token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
				"eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyLCJqdGkiOiJ0b2tlbi1hYmMxMjMifQ." +
				"fake_signature",
			wantID: "token-abc123",
		},
		{
			name:   "malformed JWT",
			token:  "not.a.valid.jwt",
			wantID: "",
		},
		{
			name:   "empty token",
			token:  "",
			wantID: "",
		},
		{
			name: "JWT without jti",
			token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
				"eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ." +
				"fake_signature",
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTokenIDFromJWT(tt.token)
			if got != tt.wantID {
				t.Errorf("extractTokenIDFromJWT() = %q, want %q", got, tt.wantID)
			}
		})
	}
}
