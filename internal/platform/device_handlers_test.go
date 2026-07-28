package platform

import (
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
