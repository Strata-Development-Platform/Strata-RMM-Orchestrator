package auth

import (
	"testing"
	"time"
)

func TestTOTPGenerateAndValidate(t *testing.T) {
	m := NewTOTPManager()

	secret, err := m.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if len(secret) < 16 {
		t.Errorf("secret too short: %s", secret)
	}

	now := time.Now()
	code, err := m.GenerateCode(secret, now)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("code length: got %d, want 6", len(code))
	}

	valid, err := m.ValidateCode(secret, code, now)
	if err != nil {
		t.Fatalf("ValidateCode: %v", err)
	}
	if !valid {
		t.Error("expected valid code")
	}

	valid, err = m.ValidateCode(secret, "000000", now)
	if err != nil {
		t.Fatalf("ValidateCode bad: %v", err)
	}
	if valid {
		t.Error("expected invalid code to fail")
	}
}

func TestTOTPWindow(t *testing.T) {
	m := NewTOTPManager()

	secret, _ := m.GenerateSecret()
	now := time.Now()

	// Code from 30s ago should still be valid (±1 window)
	past := now.Add(-30 * time.Second)
	code, _ := m.GenerateCode(secret, past)

	valid, err := m.ValidateCode(secret, code, now)
	if err != nil {
		t.Fatalf("ValidateCode past: %v", err)
	}
	if !valid {
		t.Error("expected past code within window to be valid")
	}

	// Code from 90s ago should NOT be valid (outside ±1 window)
	farPast := now.Add(-90 * time.Second)
	code, _ = m.GenerateCode(secret, farPast)

	valid, err = m.ValidateCode(secret, code, now)
	if err != nil {
		t.Fatalf("ValidateCode farPast: %v", err)
	}
	if valid {
		t.Error("expected far past code to be invalid")
	}
}

func TestTOTPProvisioningURI(t *testing.T) {
	m := NewTOTPManager()

	uri := m.ProvisioningURI("JBSWY3DPEHPK3PXP", "user@example.com", "Strata RMM")
	if uri == "" {
		t.Fatal("expected non-empty URI")
	}
	if len(uri) < 20 {
		t.Errorf("uri too short: %s", uri)
	}
}

func TestTOTPDeterministic(t *testing.T) {
	m := NewTOTPManager()

	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1700000000, 0)

	code1, _ := m.GenerateCode(secret, now)
	code2, _ := m.GenerateCode(secret, now)

	if code1 != code2 {
		t.Errorf("non-deterministic: %s != %s", code1, code2)
	}
}

func TestTOTPValidateCodeStrict(t *testing.T) {
	m := NewTOTPManager()

	secret, _ := m.GenerateSecret()
	now := time.Now()

	code, _ := m.GenerateCode(secret, now)

	// Window of 0 should only accept current code
	valid, err := m.ValidateCodeStrict(secret, code, now, 0)
	if err != nil {
		t.Fatalf("ValidateCodeStrict: %v", err)
	}
	if !valid {
		t.Error("expected valid with window 0")
	}

	// Code from 60s ago should not be valid with window 0
	past := now.Add(-60 * time.Second)
	pastCode, _ := m.GenerateCode(secret, past)
	valid, err = m.ValidateCodeStrict(secret, pastCode, now, 0)
	if err != nil {
		t.Fatalf("ValidateCodeStrict past: %v", err)
	}
	if valid {
		t.Error("expected past code to be invalid with window 0")
	}

	// Code from 60s ago should be valid with window 2
	valid, err = m.ValidateCodeStrict(secret, pastCode, now, 2)
	if err != nil {
		t.Fatalf("ValidateCodeStrict window 2: %v", err)
	}
	if !valid {
		t.Error("expected past code to be valid with window 2")
	}
}
