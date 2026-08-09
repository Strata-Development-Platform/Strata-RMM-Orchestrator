package auth

import (
	"testing"
	"time"
)

func TestModuleTokenClaimsAndLifetime(t *testing.T) {
	generator := NewTokenGenerator("module-token-test-secret-32-bytes-minimum-value")
	token, err := generator.GenerateModuleToken(
		"com.example.backup",
		"msp-1",
		"client-1",
		"site-1",
		[]string{"devices.read"},
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("generate module token: %v", err)
	}
	claims, err := generator.Validate(token)
	if err != nil {
		t.Fatalf("validate module token: %v", err)
	}
	if claims.TokenUse != "module" || claims.Subject != "module:com.example.backup" || claims.ModuleID != "com.example.backup" {
		t.Fatalf("unexpected module claims: %+v", claims)
	}
	if claims.MSPID != "msp-1" || claims.ClientID != "client-1" || claims.SiteID != "site-1" {
		t.Fatalf("unexpected module scope: %+v", claims)
	}
	if len(claims.Permissions) != 1 || claims.Permissions[0] != "devices.read" {
		t.Fatalf("unexpected module permissions: %v", claims.Permissions)
	}
	if claims.ExpiresAt-claims.IssuedAt > int64((5 * time.Minute).Seconds()) {
		t.Fatalf("module token lifetime too long: %d", claims.ExpiresAt-claims.IssuedAt)
	}
}

func TestModuleTokenRejectsLongOrInvalidLifetime(t *testing.T) {
	generator := NewTokenGenerator("module-token-test-secret-32-bytes-minimum-value")
	for _, ttl := range []time.Duration{0, -time.Minute, 16 * time.Minute} {
		if _, err := generator.GenerateModuleToken("com.example.backup", "", "", "", nil, ttl); err == nil {
			t.Fatalf("ttl %s unexpectedly accepted", ttl)
		}
	}
}
