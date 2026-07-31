package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLoggingUsesRoutePatternNotResourceID(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/devices/{deviceID}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v2/devices/customer-secret-device-id", nil)
	withLogging(mux, logger).ServeHTTP(httptest.NewRecorder(), req)

	if logs.Len() != 1 {
		t.Fatalf("log count=%d, want 1", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["route"] != "GET /api/v2/devices/{deviceID}" {
		t.Fatalf("route=%v", fields["route"])
	}
	if _, exists := fields["path"]; exists {
		t.Fatal("raw path must not be logged")
	}
	for _, value := range fields {
		if value == "customer-secret-device-id" {
			t.Fatal("resource identifier leaked into request log")
		}
	}
}
