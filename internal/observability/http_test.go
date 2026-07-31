package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPRegistryUsesMatchedPatternAndStatus(t *testing.T) {
	registry := NewHTTPRegistry()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/devices/{deviceID}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed", http.StatusServiceUnavailable)
	})
	handler := registry.Middleware(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/customer-secret-device", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	out := httptest.NewRecorder()
	registry.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := out.Body.String()
	if strings.Contains(body, "customer-secret-device") {
		t.Fatal("raw resource identifier leaked into metric labels")
	}
	if !strings.Contains(body, `route="GET /api/v1/devices/{deviceID}",status="503"} 1`) {
		t.Fatalf("matched route/status metric missing:\n%s", body)
	}
	if !strings.Contains(body, "strata_http_request_duration_seconds_count") {
		t.Fatal("latency histogram missing")
	}
}

func TestHTTPRegistryEscapesLabels(t *testing.T) {
	registry := NewHTTPRegistry()
	registry.observe("G\"ET", "line\nroute", http.StatusOK, 0.01)
	out := httptest.NewRecorder()
	registry.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := out.Body.String()
	if !strings.Contains(body, `method="G\"ET"`) || !strings.Contains(body, `route="line\nroute"`) {
		t.Fatalf("metric labels were not escaped: %s", body)
	}
}

func TestStatusRecorderKeepsFirstStatusAndUnwraps(t *testing.T) {
	target := httptest.NewRecorder()
	recorder := &statusRecorder{ResponseWriter: target, status: http.StatusOK}
	recorder.WriteHeader(http.StatusCreated)
	recorder.WriteHeader(http.StatusInternalServerError)
	if recorder.status != http.StatusCreated || target.Code != http.StatusCreated {
		t.Fatalf("first status was not preserved: recorder=%d response=%d", recorder.status, target.Code)
	}
	if recorder.Unwrap() != target {
		t.Fatal("wrapped response writer was not exposed")
	}
}
