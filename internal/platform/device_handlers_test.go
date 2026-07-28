package platform

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperationRegistryUsesCanonicalTypes(t *testing.T) {
	for action, operation := range operationRegistry {
		if !strings.HasPrefix(operation.JobType, "device.") {
			t.Fatalf("%s maps to non-canonical job type %q", action, operation.JobType)
		}
	}
}

func TestEndpointRequestHashIsStable(t *testing.T) {
	request := map[string]interface{}{"device_id": "device-1", "job_type": "device.refresh"}
	first, err := endpointRequestHash(request)
	if err != nil {
		t.Fatalf("endpointRequestHash() error = %v", err)
	}
	second, err := endpointRequestHash(request)
	if err != nil {
		t.Fatalf("endpointRequestHash() error = %v", err)
	}
	if first == "" || first != second {
		t.Fatalf("hashes differ: %q != %q", first, second)
	}
}

func TestValidateIdempotencyKey(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v2/devices/device/action", nil)
	request.Header.Set("Idempotency-Key", " request-123 ")
	key, err := validateIdempotencyKey(request)
	if err != nil || key != "request-123" {
		t.Fatalf("validateIdempotencyKey() = %q, %v", key, err)
	}
	request.Header.Set("Idempotency-Key", strings.Repeat("x", 129))
	if _, err := validateIdempotencyKey(request); err == nil {
		t.Fatal("oversized idempotency key must fail")
	}
}


func TestRequestIPAddress(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v2/devices/device/action", nil)
	request.RemoteAddr = net.JoinHostPort("192.0.2.10", "4242")
	if got := requestIPAddress(request); got != "192.0.2.10" {
		t.Fatalf("requestIPAddress() = %q", got)
	}
	request.RemoteAddr = "2001:db8::1"
	if got := requestIPAddress(request); got != "2001:db8::1" {
		t.Fatalf("requestIPAddress() without port = %q", got)
	}
}


func TestPhase7AgentRoutesRequireAgentPrincipal(t *testing.T) {
	server := &APIServer{}
	for _, path := range []string{
		"/api/v2/devices/00000000-0000-0000-0000-000000000001/capabilities",
		"/api/v2/devices/00000000-0000-0000-0000-000000000001/inventory",
	} {
		if access := server.classifyRoute(http.MethodPost, path); access != AccessAgent {
			t.Fatalf("%s access = %v, want AccessAgent", path, access)
		}
	}
}
