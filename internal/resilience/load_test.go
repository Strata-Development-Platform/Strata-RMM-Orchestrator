package resilience

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadRunnerPassesBoundedThresholdsWithoutLeakingToken(t *testing.T) {
	const token = "secret-token-that-must-not-appear"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	runner, err := NewLoadRunner(LoadConfig{
		BaseURL: server.URL, Path: "/api/v1/auth/me", Label: "authenticated_api", BearerToken: token,
		Duration: 150 * time.Millisecond, Rate: 100, Concurrency: 4,
		RequestTimeout: time.Second, MaxErrorRate: 0, MaxP95: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	report := runner.Run(context.Background())
	if !report.ThresholdPassed || report.Requests == 0 || report.Failures != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if strings.Contains(report.Target, token) || report.Target != "authenticated_api" {
		t.Fatalf("unsafe target label: %q", report.Target)
	}
}

func TestLoadRunnerFailsErrorThreshold(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	runner, err := NewLoadRunner(LoadConfig{
		BaseURL: server.URL, Path: "/health/ready", Label: "readiness", Duration: 100 * time.Millisecond,
		Rate: 50, Concurrency: 2, RequestTimeout: time.Second,
		MaxErrorRate: 0.01, MaxP95: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	report := runner.Run(context.Background())
	if report.ThresholdPassed || report.Failures == 0 {
		t.Fatalf("expected threshold failure: %+v", report)
	}
}

func TestLoadRunnerRejectsUnsafeConfiguration(t *testing.T) {
	tests := []LoadConfig{
		{BaseURL: "http://example.com", Path: "/health/live", Label: "live", Duration: time.Second, Rate: 1, Concurrency: 1, MaxP95: time.Second},
		{BaseURL: "https://user:pass@example.com", Path: "/health/live", Label: "live", Duration: time.Second, Rate: 1, Concurrency: 1, MaxP95: time.Second},
		{BaseURL: "https://example.com/path", Path: "/health/live", Label: "live", Duration: time.Second, Rate: 1, Concurrency: 1, MaxP95: time.Second},
		{BaseURL: "https://example.com", Path: "health/live", Label: "live", Duration: time.Second, Rate: 1, Concurrency: 1, MaxP95: time.Second},
		{BaseURL: "https://example.com", Path: "/health/live?credential=secret", Label: "live", Duration: time.Second, Rate: 1, Concurrency: 1, MaxP95: time.Second},
		{BaseURL: "https://example.com", Path: "/health/live", Label: "live", Duration: time.Second, Rate: 2_000_000_000, Concurrency: 1, MaxP95: time.Second},
		{BaseURL: "https://example.com", Path: "/devices/raw-id", Label: "device/123", Duration: time.Second, Rate: 1, Concurrency: 1, MaxP95: time.Second},
	}
	for _, config := range tests {
		if _, err := NewLoadRunner(config); err == nil {
			t.Fatalf("expected rejection for %+v", config)
		}
	}
}

func TestSummarizePercentiles(t *testing.T) {
	report := summarize(time.Now(), time.Second, 3, 1, []time.Duration{
		10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond, 40 * time.Millisecond,
	})
	if report.Requests != 4 || report.ErrorRate != 0.25 {
		t.Fatalf("unexpected counts: %+v", report)
	}
	if report.P50 != 20*time.Millisecond || report.P95 != 30*time.Millisecond || report.Max != 40*time.Millisecond {
		t.Fatalf("unexpected percentiles: %+v", report)
	}
}
