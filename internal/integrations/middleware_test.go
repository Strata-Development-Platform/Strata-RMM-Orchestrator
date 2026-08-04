package integrations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHMACVerifyValidSignature(t *testing.T) {
	secret := "test-secret-key"
	body := `{"alert_id":"123","severity":"high"}`

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	signature := hex.EncodeToString(mac.Sum(nil))

	if !Verify(secret, signature, body) {
		t.Error("expected valid signature")
	}
}

func TestHMACVerifyInvalidSignature(t *testing.T) {
	secret := "test-secret-key"
	body := `{"alert_id":"123"}`

	if Verify(secret, "invalid-signature", body) {
		t.Error("expected invalid signature to be rejected")
	}
}

func TestHMACVerifyEmptySignature(t *testing.T) {
	secret := "test-secret-key"
	body := `{"alert_id":"123"}`

	if Verify(secret, "", body) {
		t.Error("expected empty signature to be rejected")
	}
}

func TestMiddlewareMissingSignature(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	verifier := NewHMACVerifier("X-Signature", []byte("secret"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/webhook", nil)

	verifier.Middleware(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing signature, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "missing signature") {
		t.Errorf("expected 'missing signature' error, got: %s", rec.Body.String())
	}
}

func TestMiddlewareValidSignature(t *testing.T) {
	secret := []byte("secret")
	body := `{"alert_id":"123","severity":"high"}`

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	signature := hex.EncodeToString(mac.Sum(nil))

	verifier := NewHMACVerifier("X-Signature", secret)

	handler := verifier.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader([]byte(body)))
	req.Header.Set("X-Signature", signature)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMiddlewareInvalidSignature(t *testing.T) {
	secret := []byte("secret")

	verifier := NewHMACVerifier("X-Signature", secret)

	handler := verifier.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader([]byte("body")))
	req.Header.Set("X-Signature", "invalid-signature")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMiddlewareExpiredTimestamp(t *testing.T) {
	secret := []byte("secret")
	body := `{"alert_id":"123"}`

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	signature := hex.EncodeToString(mac.Sum(nil))

	verifier := NewHMACVerifier("X-Signature", secret).WithClockSkew(time.Minute)

	handler := verifier.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	pastTime := time.Now().Add(-2 * time.Minute).Format(time.RFC3339)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader([]byte(body)))
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Webhook-Timestamp", pastTime)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMiddlewareValidTimestamp(t *testing.T) {
	secret := []byte("secret")
	body := `{"alert_id":"123"}`

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	signature := hex.EncodeToString(mac.Sum(nil))

	verifier := NewHMACVerifier("X-Signature", secret).WithClockSkew(5 * time.Minute)

	handler := verifier.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader([]byte(body)))
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Webhook-Timestamp", time.Now().Format(time.RFC3339))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestComputeHMACDeterministic(t *testing.T) {
	secret := []byte("my-secret")
	data := []byte("my-data")

	result1 := computeHMAC(secret, data)
	result2 := computeHMAC(secret, data)

	if string(result1) != string(result2) {
		t.Error("expected deterministic HMAC computation")
	}
}

func TestContextIntegrationMetadata(t *testing.T) {
	ctx := context.Background()

	// Test context with integration
	ctx = ContextWithIntegration(ctx, "int-123", "edr")

	ic, ok := IntegrationFromContext(ctx)
	if !ok {
		t.Error("expected integration context to be found")
	}
	if ic.ID != "int-123" {
		t.Errorf("expected int-123, got %s", ic.ID)
	}
	if ic.Integration != "edr" {
		t.Errorf("expected edr, got %s", ic.Integration)
	}
}

func TestContextMissingIntegration(t *testing.T) {
	ctx := context.Background()

	_, ok := IntegrationFromContext(ctx)
	if ok {
		t.Error("expected no integration context in plain context")
	}
}
