package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/observability"
)

func TestMetricsRequireDedicatedBearerToken(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	server := &APIServer{
		metricsToken: token,
		httpMetrics:  observability.NewHTTPRegistry(),
	}

	for _, header := range []string{"", "Bearer wrong", "Basic " + token} {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("Authorization", header)
		out := httptest.NewRecorder()
		server.handleMetrics(out, req)
		if out.Code != http.StatusUnauthorized {
			t.Fatalf("header %q returned %d, want 401", header, out.Code)
		}
		if strings.Contains(out.Body.String(), token) {
			t.Fatal("metrics token leaked in authentication response")
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	out := httptest.NewRecorder()
	server.handleMetrics(out, req)
	if out.Code != http.StatusOK || !strings.Contains(out.Body.String(), "strata_http_requests_in_flight") {
		t.Fatalf("authorized scrape failed: status=%d body=%s", out.Code, out.Body.String())
	}
}

func TestMetricsAreDisabledWithoutToken(t *testing.T) {
	server := &APIServer{httpMetrics: observability.NewHTTPRegistry()}
	out := httptest.NewRecorder()
	server.handleMetrics(out, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if out.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", out.Code)
	}
}
