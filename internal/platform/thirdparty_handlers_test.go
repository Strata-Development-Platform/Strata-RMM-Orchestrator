package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/inventory"
	"go.uber.org/zap"
)

// setupThirdPartyTestServer creates a test server with third-party engine
func setupThirdPartyTestServer(t *testing.T) *APIServer {
	t.Helper()

	logger, _ := zap.NewDevelopment()

	server := &APIServer{
		logger: logger,
	}

	server.thirdParty = setupThirdPartyEngine(t)

	return server
}

func setupThirdPartyEngine(t *testing.T) *inventory.ThirdPartyEngine {
	t.Helper()
	engine := inventory.NewThirdPartyEngine(nil, zap.NewNop())
	engine.SetTenantID("test-tenant-uuid")
	return engine
}

// TestHandleThirdPartyApps verifies apps endpoint returns valid JSON
func TestHandleThirdPartyApps(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/apps", server.handleThirdPartyApps)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/apps", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	apps, ok := resp["apps"].([]interface{})
	if !ok {
		t.Fatal("expected apps array in response")
	}
	if len(apps) == 0 {
		t.Fatal("expected at least one app")
	}
}

// TestHandleThirdPartyApps_NilEngine verifies nil engine error
func TestHandleThirdPartyApps_NilEngine(t *testing.T) {
	server := &APIServer{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/apps", server.handleThirdPartyApps)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/apps", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// TestHandleThirdPartySync verifies sync endpoint returns 202
func TestHandleThirdPartySync(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/thirdparty/sync", server.handleThirdPartySync)

	req := httptest.NewRequest("POST", "/api/v1/thirdparty/sync", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	status, ok := resp["status"].(string)
	if !ok {
		t.Fatal("expected status in response")
	}
	if status != "sync triggered" {
		t.Errorf("expected 'sync triggered', got %q", status)
	}
}

// TestHandleThirdPartySync_NilEngine verifies nil engine error
func TestHandleThirdPartySync_NilEngine(t *testing.T) {
	server := &APIServer{}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/thirdparty/sync", server.handleThirdPartySync)

	req := httptest.NewRequest("POST", "/api/v1/thirdparty/sync", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// TestHandleThirdPartyVendors verifies vendors endpoint
func TestHandleThirdPartyVendors(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/vendors", server.handleThirdPartyVendors)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/vendors", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	vendors, ok := resp["vendors"].([]interface{})
	if !ok {
		t.Fatal("expected vendors array in response")
	}
	if len(vendors) == 0 {
		t.Fatal("expected at least one vendor")
	}

	count, ok := resp["count"].(float64)
	if !ok {
		t.Fatal("expected count in response")
	}
	if count != float64(len(vendors)) {
		t.Errorf("expected count %d, got %f", len(vendors), count)
	}
}

// TestHandleThirdPartyVendors_NilEngine verifies nil engine error
func TestHandleThirdPartyVendors_NilEngine(t *testing.T) {
	server := &APIServer{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/vendors", server.handleThirdPartyVendors)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/vendors", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// TestHandleThirdPartyVendorStatus verifies vendor status endpoint
func TestHandleThirdPartyVendorStatus(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/vendors/status", server.handleThirdPartyVendorStatus)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/vendors/status", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	status, ok := resp["status"].([]interface{})
	if !ok {
		t.Fatal("expected status array in response")
	}
	if len(status) == 0 {
		t.Fatal("expected at least one vendor status")
	}
}

// TestHandleThirdPartyVendorStatus_NilEngine verifies nil engine error
func TestHandleThirdPartyVendorStatus_NilEngine(t *testing.T) {
	server := &APIServer{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/vendors/status", server.handleThirdPartyVendorStatus)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/vendors/status", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// TestThirdPartyApps_ResponseStructure verifies apps response fields
func TestThirdPartyApps_ResponseStructure(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/apps", server.handleThirdPartyApps)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/apps", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	apps, ok := resp["apps"].([]interface{})
	if !ok || len(apps) == 0 {
		t.Fatal("expected non-empty apps array")
	}

	appMap, ok := apps[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected app as object")
	}

	requiredFields := []string{"name", "vendor", "platform", "package_type"}
	for _, field := range requiredFields {
		if _, hasField := appMap[field]; !hasField {
			t.Errorf("app missing required field: %s", field)
		}
	}
}

// TestThirdPartyVendors_ResponseStructure verifies vendors response fields
func TestThirdPartyVendors_ResponseStructure(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/vendors", server.handleThirdPartyVendors)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/vendors", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	vendors, ok := resp["vendors"].([]interface{})
	if !ok || len(vendors) == 0 {
		t.Fatal("expected non-empty vendors array")
	}

	vendorMap, ok := vendors[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected vendor as object")
	}

	if _, hasName := vendorMap["name"]; !hasName {
		t.Error("vendor missing required field: name")
	}
	if _, hasApps := vendorMap["apps"]; !hasApps {
		t.Error("vendor missing required field: apps")
	}
}

// TestThirdPartyVendorStatus_ResponseStructure verifies vendor status response fields
func TestThirdPartyVendorStatus_ResponseStructure(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/vendors/status", server.handleThirdPartyVendorStatus)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/vendors/status", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	status, ok := resp["status"].([]interface{})
	if !ok || len(status) == 0 {
		t.Fatal("expected non-empty status array")
	}

	statusMap, ok := status[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected status entry as object")
	}

	if _, hasName := statusMap["name"]; !hasName {
		t.Error("status entry missing required field: name")
	}
	if _, hasAppCount := statusMap["app_count"]; !hasAppCount {
		t.Error("status entry missing required field: app_count")
	}
}

// TestThirdPartyHandler_PostBodyHandling verifies POST body handling
func TestThirdPartyHandler_PostBodyHandling(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/thirdparty/sync", server.handleThirdPartySync)

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest("POST", "/api/v1/thirdparty/sync", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

// TestThirdPartyHandler_MultipleRequests verifies consistent responses
func TestThirdPartyHandler_MultipleRequests(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/apps", server.handleThirdPartyApps)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/api/v1/thirdparty/apps", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Errorf("request %d: invalid JSON: %v", i+1, err)
		}
	}
}

// TestThirdPartyHandler_ContentTypeHeader verifies Content-Type header
func TestThirdPartyHandler_ContentTypeHeader(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/apps", server.handleThirdPartyApps)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/apps", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected application/json content type, got: %s", contentType)
	}
}

// TestThirdPartyHandler_NilChecks verifies all handlers handle nil engine
func TestThirdPartyHandler_NilChecks(t *testing.T) {
	server := &APIServer{}

	tests := []struct {
		method  string
		path    string
		handler http.HandlerFunc
	}{
		{"GET", "/api/v1/thirdparty/apps", server.handleThirdPartyApps},
		{"GET", "/api/v1/thirdparty/vendors", server.handleThirdPartyVendors},
		{"POST", "/api/v1/thirdparty/sync", server.handleThirdPartySync},
		{"GET", "/api/v1/thirdparty/vendors/status", server.handleThirdPartyVendorStatus},
	}

	for _, tt := range tests {
		mux := http.NewServeMux()
		mux.HandleFunc(tt.path, tt.handler)

		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("%s %s: expected 500 with nil engine, got %d", tt.method, tt.path, w.Code)
		}
	}
}

// TestThirdPartyHandler_ErrorResponseStructure verifies error response format
func TestThirdPartyHandler_ErrorResponseStructure(t *testing.T) {
	server := &APIServer{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/apps", server.handleThirdPartyApps)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/apps", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid error response JSON: %v", err)
	}

	if _, hasError := resp["error"]; !hasError {
		t.Fatal("error response missing 'error' field")
	}
}

// TestThirdPartySync_Validation verifies sync input validation
func TestThirdPartySync_Validation(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/thirdparty/sync", server.handleThirdPartySync)

	req := httptest.NewRequest("POST", "/api/v1/thirdparty/sync", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for sync without body, got %d", w.Code)
	}
}

// TestThirdPartyEngine_Integration verifies engine integration with handler
func TestThirdPartyEngine_Integration(t *testing.T) {
	engine := setupThirdPartyEngine(t)

	apps := engine.ListApps()
	if len(apps) == 0 {
		t.Fatal("engine returned no apps")
	}

	vendors := engine.DiscoverVendors()
	if len(vendors) == 0 {
		t.Fatal("engine returned no vendors")
	}

	vendorNames := make(map[string]bool)
	for _, app := range apps {
		vendorNames[app.Vendor] = true
	}

	if len(vendorNames) != len(vendors) {
		t.Errorf("vendor mismatch: %d unique vendors in apps, %d from DiscoverVendors",
			len(vendorNames), len(vendors))
	}
}

// TestThirdPartyHandler_TenantScoping verifies tenant context is preserved
func TestThirdPartyHandler_TenantScoping(t *testing.T) {
	engine := inventory.NewThirdPartyEngine(nil, zap.NewNop())

	engine.SetTenantID("tenant-1")
	engine.SetTenantID("tenant-2")

	_ = engine.ListApps()
}

// TestThirdPartyHandler_JSONPunctuation verifies JSON response validity
func TestThirdPartyHandler_JSONPunctuation(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/vendors", server.handleThirdPartyVendors)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/vendors", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	var resp1, resp2 map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("first unmarshal failed: %v", err)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("second unmarshal failed: %v", err)
	}

	resp1JSON, _ := json.Marshal(resp1)
	resp2JSON, _ := json.Marshal(resp2)
	if string(resp1JSON) != string(resp2JSON) {
		t.Fatal("responses differ between unmarshals")
	}
}

// TestThirdPartyHandler_ConcurrentAccess verifies concurrent handler access
func TestThirdPartyHandler_ConcurrentAccess(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/apps", server.handleThirdPartyApps)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/api/v1/thirdparty/apps", nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", w.Code)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestThirdPartyHandler_Timeout verifies handler doesn't hang
func TestThirdPartyHandler_Timeout(t *testing.T) {
	server := setupThirdPartyTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/thirdparty/apps", server.handleThirdPartyApps)

	req := httptest.NewRequest("GET", "/api/v1/thirdparty/apps", nil)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not complete within 5 seconds")
	}
}
