package integrations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"
)

// HMACVerifier validates HMAC-SHA256 signatures on incoming webhook requests.
type HMACVerifier struct {
	signatureHeader string
	secret          []byte
	clockSkew       time.Duration
}

// NewHMACVerifier creates a verifier for the given header and secret.
func NewHMACVerifier(signatureHeader string, secret []byte) *HMACVerifier {
	if signatureHeader == "" {
		signatureHeader = "X-Signature"
	}
	return &HMACVerifier{
		signatureHeader: signatureHeader,
		secret:          secret,
	}
}

// WithClockSkew sets the allowed clock skew for X-Webhook-Timestamp validation.
func (v *HMACVerifier) WithClockSkew(skew time.Duration) *HMACVerifier {
	v.clockSkew = skew
	return v
}

// Middleware returns an HTTP middleware that validates HMAC signatures.
func (v *HMACVerifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signature := r.Header.Get(v.signatureHeader)
		if signature == "" {
			http.Error(w, `{"error":"missing signature"}`, http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		expected := computeHMAC(v.secret, body)
		if !hmac.Equal([]byte(signature), expected) {
			http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
			return
		}

		if v.clockSkew > 0 {
			ts := r.Header.Get("X-Webhook-Timestamp")
			if ts != "" {
				t, err := time.Parse(time.RFC3339, ts)
				if err != nil {
					http.Error(w, `{"error":"invalid timestamp format"}`, http.StatusBadRequest)
					return
				}
				if time.Since(t) > v.clockSkew {
					http.Error(w, `{"error":"webhook expired"}`, http.StatusUnauthorized)
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

// Verify returns true if the signature matches the body and secret.
func Verify(secret, signature, body string) bool {
	expected := computeHMAC([]byte(secret), []byte(body))
	return hmac.Equal([]byte(signature), expected)
}

// computeHMAC computes HMAC-SHA256 of data with secret.
func computeHMAC(secret, data []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return []byte(hex.EncodeToString(mac.Sum(nil)))
}

// IntegrationContext carries integration metadata through the request context.
type IntegrationContext struct {
	ID          string
	Integration string
}

// ContextKey is the context key type for integration context.
type ContextKey string

const IntegrationContextKey ContextKey = "integration_context"

// ContextWithIntegration returns a new context with integration metadata.
func ContextWithIntegration(ctx context.Context, id, integration string) context.Context {
	return context.WithValue(ctx, IntegrationContextKey, IntegrationContext{
		ID:          id,
		Integration: integration,
	})
}

// IntegrationFromContext returns the integration context from the request.
func IntegrationFromContext(ctx context.Context) (IntegrationContext, bool) {
	v := ctx.Value(IntegrationContextKey)
	if v == nil {
		return IntegrationContext{}, false
	}
	ic, ok := v.(IntegrationContext)
	return ic, ok
}
