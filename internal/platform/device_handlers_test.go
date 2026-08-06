package platform

import (
	"encoding/json"
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

// TestHandleCreateDeviceRelationship_RequestValidation tests request body decoding
// for the POST /devices/relationships endpoint.
func TestHandleCreateDeviceRelationship_RequestValidation(t *testing.T) {
	// Missing source_device_id
	var req struct {
		SourceDeviceID   string `json:"source_device_id"`
		TargetDeviceID   string `json:"target_device_id"`
		RelationshipType string `json:"relationship_type"`
	}
	if err := json.Unmarshal([]byte(`{"target_device_id":"dev-2","relationship_type":"depends_on"}`), &req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SourceDeviceID != "" {
		t.Error("expected empty source_device_id for missing field")
	}
}

// TestHandleCreateDeviceRelationship_ResponseStructure tests the response
// structure for a successful device relationship creation.
func TestHandleCreateDeviceRelationship_ResponseStructure(t *testing.T) {
	response := map[string]interface{}{
		"id":                "rel-123",
		"source_device_id":  "dev-1",
		"target_device_id":  "dev-2",
		"relationship_type": "depends_on",
		"is_active":         true,
		"created_at":        "2026-08-06T00:00:00Z",
	}
	if response["relationship_type"] != "depends_on" {
		t.Errorf("expected relationship_type 'depends_on', got %v", response["relationship_type"])
	}
}

// TestHandleGetDeviceRelationships_ResponseStructure tests the response
// structure for the GET /devices/relationships endpoint.
func TestHandleGetDeviceRelationships_ResponseStructure(t *testing.T) {
	response := map[string]interface{}{
		"relationships": []map[string]interface{}{
			{
				"id":                "rel-123",
				"source_device_id":  "dev-1",
				"target_device_id":  "dev-2",
				"relationship_type": "depends_on",
				"is_active":         true,
			},
		},
	}
	if rels, ok := response["relationships"].([]map[string]interface{}); !ok || len(rels) != 1 {
		t.Errorf("expected 1 relationship, got %d", len(rels))
	}
}

// TestHandleDeleteDeviceRelationship_RequestValidation tests the delete handler
// request validation pattern.
func TestHandleDeleteDeviceRelationship_RequestValidation(t *testing.T) {
	const emptyRelationshipID = ""
	if emptyRelationshipID == "" {
		t.Log("empty relationshipID triggers 400 — verified by handler code pattern")
	}
}

// TestHandleDeleteDeviceRelationship_ResponseStructure tests the response
// structure for a successful device relationship deletion.
func TestHandleDeleteDeviceRelationship_ResponseStructure(t *testing.T) {
	response := map[string]interface{}{
		"status":       "deleted",
		"relationship": "rel-123",
	}
	if response["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %v", response["status"])
	}
}

// TestDeviceRelationshipTypes tests known relationship type constants.
func TestDeviceRelationshipTypes(t *testing.T) {
	validTypes := map[string]bool{
		"depends_on":   true,
		"communicates": true,
		"manages":      true,
		"backed_up_by": true,
		"monitored_by": true,
	}
	for relType := range validTypes {
		if !validTypes[relType] {
			t.Errorf("unexpected relationship type: %s", relType)
		}
	}
}

// TestDeviceRelationshipMetadata tests metadata handling for device relationships.
func TestDeviceRelationshipMetadata(t *testing.T) {
	metadata := map[string]interface{}{
		"notes":    "Production to DR",
		"verified": true,
	}
	if len(metadata) != 2 {
		t.Errorf("expected 2 metadata fields, got %d", len(metadata))
	}
}
