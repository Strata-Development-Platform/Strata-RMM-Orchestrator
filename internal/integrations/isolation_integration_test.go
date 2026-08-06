//go:build integration
// +build integration

package integrations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// natsJetStreamWrapper adapts nats.JetStreamContext to JetStreamPublisher.
type natsJetStreamWrapper struct {
	nats.JetStreamContext
}

func (w natsJetStreamWrapper) Publish(subject string, data []byte) (*PublishAck, error) {
	ack, err := w.JetStreamContext.Publish(subject, data)
	if err != nil {
		return nil, err
	}
	return &PublishAck{
		Subject:  subject,
		Sequence: ack.Sequence,
		Domain:   ack.Domain,
		Stream:   ack.Stream,
	}, nil
}

// TestIsolationFullFlow traces the complete alert → NATS dispatch flow.
// Requires NATS running at localhost:4222 (set TEST_NATS_URL).
func TestIsolationFullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	natsURL := "nats://localhost:4222"
	if env := os.Getenv("TEST_NATS_URL"); env != "" {
		natsURL = env
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Skipf("NATS not available at %s: %v", natsURL, err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	require.NoError(t, err, "JetStream not available")

	secret := []byte("integration-test-secret")
	hmacVerifier := NewHMACVerifier("X-Signature", secret)

	// Subscribe to the isolation subject before sending the request
	_, err = nc.SubscribeSync("tenant.tenant-123.cmd.isolate")
	require.NoError(t, err)

	jsWrapper := natsJetStreamWrapper{js}
	handler := NewIsolationHandler(jsWrapper, nil)

	// Register the handler through the middleware chain
	handlerFunc := http.HandlerFunc(handler.HandleIsolation)
	middlewareHandler := hmacVerifier.Middleware(handlerFunc)

	server := httptest.NewServer(middlewareHandler)
	defer server.Close()

	// Build the isolation payload
	action := IsolationAction{
		DeviceID: "device-456",
		TenantID: "tenant-123",
		Reason:   "Malware detected on endpoint",
		Severity: "critical",
		AlertID:  "edr-alert-789",
		Provider: "crowdstrike",
	}

	payload, err := json.Marshal(action)
	require.NoError(t, err)

	// Compute HMAC signature
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))

	// Send the request
	req, err := http.NewRequest("POST", server.URL+"/api/v1/integrations/isolate", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK")

	var respBody map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&respBody)
	require.NoError(t, err)
	require.Equal(t, "isolated", respBody["status"])

	// Verify NATS message was published by subscribing and receiving
	msgChan := make(chan *nats.Msg, 1)
	sub, err := nc.Subscribe("tenant.tenant-123.cmd.isolate", func(msg *nats.Msg) {
		select {
		case msgChan <- msg:
		default:
		}
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case msg := <-msgChan:
		var cmd IsolationCommand
		err = json.Unmarshal(msg.Data, &cmd)
		require.NoError(t, err)

		require.Equal(t, "device-456", cmd.DeviceID)
		require.Equal(t, "tenant-123", cmd.TenantID)
		require.Equal(t, "Malware detected on endpoint", cmd.Reason)
		require.Equal(t, "critical", cmd.Severity)
		require.Equal(t, "edr-alert-789", cmd.AlertID)
		require.Equal(t, "crowdstrike", cmd.Provider)
		require.True(t, cmd.Isolate, "isolate flag should be true")
		require.Equal(t, "iso-edr-alert-789", cmd.EventID)

	case <-ctx.Done():
		t.Fatal("timeout waiting for NATS isolation message")
	}
}

// TestIsolationFlowRejectsInvalidSignature verifies that an invalid HMAC
// signature prevents the isolation command from being dispatched.
func TestIsolationFlowRejectsInvalidSignature(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	natsURL := "nats://localhost:4222"
	if env := os.Getenv("TEST_NATS_URL"); env != "" {
		natsURL = env
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Skipf("NATS not available at %s: %v", natsURL, err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	require.NoError(t, err)

	jsWrapper := natsJetStreamWrapper{js}
	secret := []byte("integration-test-secret")
	hmacVerifier := NewHMACVerifier("X-Signature", secret)

	handler := NewIsolationHandler(jsWrapper, nil)

	handlerFunc := http.HandlerFunc(handler.HandleIsolation)
	middlewareHandler := hmacVerifier.Middleware(handlerFunc)

	server := httptest.NewServer(middlewareHandler)
	defer server.Close()

	action := IsolationAction{
		DeviceID: "device-456",
		TenantID: "tenant-123",
	}
	payload, _ := json.Marshal(action)

	// Send with INVALID signature
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/integrations/isolate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", "invalid-signature")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
