package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const testJWTSecret = "test-secret-that-is-at-least-32-bytes-long"

func TestUserTokenRoundTrip(t *testing.T) {
	generator := NewTokenGenerator(testJWTSecret)
	token, err := generator.GenerateUserToken(
		"user-1",
		"tenant-1",
		"msp-1",
		"client-1",
		"site-1",
		[]string{"msp_admin"},
		time.Hour,
	)
	if err != nil {
		t.Fatalf("GenerateUserToken() error = %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("header is not valid JSON: %v", err)
	}
	if header["alg"] != "HS256" {
		t.Errorf("alg = %q, want HS256", header["alg"])
	}
	if header["typ"] != "JWT" {
		t.Errorf("typ = %q, want JWT", header["typ"])
	}

	claims, err := generator.Validate(token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if claims.Subject != "user-1" || claims.TokenUse != "user" {
		t.Fatalf("unexpected claims: sub=%q token_use=%q", claims.Subject, claims.TokenUse)
	}
}

func TestAgentTokenRoundTrip(t *testing.T) {
	generator := NewTokenGenerator(testJWTSecret)
	token, err := generator.GenerateAgentToken("tenant-1", "agent-1", 24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateAgentToken() error = %v", err)
	}
	claims, err := generator.Validate(token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if claims.Subject != "agent-1" || claims.AgentID != "agent-1" || claims.TokenUse != "agent" {
		t.Fatalf("unexpected agent claims: %+v", claims)
	}
}

func TestGeneratorRejectsUnsafeSecret(t *testing.T) {
	for _, secret := range []string{"", "short"} {
		generator := NewTokenGenerator(secret)
		if _, err := generator.GenerateUserToken("user-1", "", "", "", "", []string{"viewer"}, time.Hour); err == nil {
			t.Errorf("GenerateUserToken() accepted unsafe secret %q", secret)
		}
		if _, err := generator.Validate("a.b.c"); err == nil {
			t.Errorf("Validate() accepted unsafe secret %q", secret)
		}
	}
}

func TestAgentTokenRequiresIdentityAndTenant(t *testing.T) {
	generator := NewTokenGenerator(testJWTSecret)
	if _, err := generator.GenerateAgentToken("", "agent-1", time.Hour); err == nil {
		t.Error("GenerateAgentToken() accepted an empty tenant")
	}
	if _, err := generator.GenerateAgentToken("tenant-1", "", time.Hour); err == nil {
		t.Error("GenerateAgentToken() accepted an empty agent")
	}
}

func TestValidatorRejectsUnsupportedHeader(t *testing.T) {
	generator := NewTokenGenerator(testJWTSecret)
	tests := []struct {
		name   string
		header string
	}{
		{name: "none algorithm", header: `{"alg":"none","typ":"JWT"}`},
		{name: "wrong type", header: `{"alg":"HS256","typ":"NOT-JWT"}`},
		{name: "malformed JSON", header: `{"alg":`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := base64.RawURLEncoding.EncodeToString([]byte(test.header))
			if _, err := generator.Validate(header + ".e30.invalid"); err == nil {
				t.Fatal("Validate() accepted an unsupported header")
			}
		})
	}
}

func TestValidatorRejectsExcessiveLifetime(t *testing.T) {
	generator := NewTokenGenerator(testJWTSecret)
	token, err := generator.GenerateUserToken(
		"user-1",
		"tenant-1",
		"msp-1",
		"",
		"",
		[]string{"msp_admin"},
		25*time.Hour,
	)
	if err != nil {
		t.Fatalf("GenerateUserToken() error = %v", err)
	}
	if _, err := generator.Validate(token); err == nil {
		t.Fatal("Validate() accepted an excessive user-token lifetime")
	}
}
