package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthReadyWhenReady(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.setReadiness(true)
	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when ready, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Ready {
		t.Error("expected ready=true")
	}
	if body.Status != "ok" {
		t.Errorf("expected status ok, got %q", body.Status)
	}
}

func TestHealthNotReadyReturns503(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.setReadiness(false)
	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when not ready, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Ready {
		t.Error("expected ready=false when not ready")
	}
	if body.Status != "not ready" {
		t.Errorf("expected status 'not ready', got %q", body.Status)
	}
}

func TestHealthLiveReturnsAlwaysAlive(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()
	s.handleHealthLive(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body healthLivenessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "alive" {
		t.Errorf("expected status alive, got %s", body.Status)
	}
}

func TestHealthLivenessQueryParam(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	req := httptest.NewRequest("GET", "/health?liveness=1", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body healthLivenessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "alive" {
		t.Errorf("expected status alive, got %s", body.Status)
	}
}

func TestHealthNotReadyByDefault(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 by default, got %d", w.Code)
	}
}

func TestHealthReadyAllChecksPass(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.RegisterHealth("db", func(ctx context.Context) error { return nil })
	s.RegisterHealth("nats", func(ctx context.Context) error { return nil })
	s.setReadiness(true)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when all checks pass, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Ready {
		t.Error("expected ready=true")
	}
	if body.Status != "ok" {
		t.Errorf("expected status ok, got %q", body.Status)
	}
}

func TestHealthReadyDBFailure(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.RegisterHealth("db", func(ctx context.Context) error {
		return fmt.Errorf("unreachable")
	})
	s.RegisterHealth("nats", func(ctx context.Context) error { return nil })
	s.setReadiness(true)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on DB failure, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Ready {
		t.Error("expected ready=false")
	}
	if body.Status != "degraded" {
		t.Errorf("expected status degraded, got %q", body.Status)
	}
	if body.Components == nil {
		t.Fatal("expected components map")
	}
	if body.Components["db"] != "failed: unreachable" {
		t.Errorf("expected db component failure, got %q", body.Components["db"])
	}
	if body.Components["nats"] != "ok" {
		t.Errorf("expected nats ok, got %q", body.Components["nats"])
	}
}

func TestHealthReadyNATSFailure(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.RegisterHealth("db", func(ctx context.Context) error { return nil })
	s.RegisterHealth("nats", func(ctx context.Context) error {
		return fmt.Errorf("disconnected")
	})
	s.setReadiness(true)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on NATS failure, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Ready {
		t.Error("expected ready=false")
	}
	if body.Status != "degraded" {
		t.Errorf("expected status degraded, got %q", body.Status)
	}
	if body.Components["nats"] != "failed: disconnected" {
		t.Errorf("expected nats failure, got %q", body.Components["nats"])
	}
}

func TestHealthReadyDegradedAllComponentsReported(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.RegisterHealth("db", func(ctx context.Context) error {
		return fmt.Errorf("connection refused")
	})
	s.RegisterHealth("nats", func(ctx context.Context) error {
		return fmt.Errorf("timeout")
	})
	s.RegisterHealth("storage", func(ctx context.Context) error { return nil })
	s.setReadiness(true)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on multi-component failure, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "degraded" {
		t.Errorf("expected status degraded, got %q", body.Status)
	}
	if body.Components["db"] != "failed: connection refused" {
		t.Errorf("unexpected db status: %q", body.Components["db"])
	}
	if body.Components["nats"] != "failed: timeout" {
		t.Errorf("unexpected nats status: %q", body.Components["nats"])
	}
	if body.Components["storage"] != "ok" {
		t.Errorf("expected storage ok, got %q", body.Components["storage"])
	}
}

func TestHealthLiveAlwaysOKRegardlessOfHealth(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.RegisterHealth("db", func(ctx context.Context) error {
		return fmt.Errorf("unreachable")
	})
	s.setReadiness(true)

	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()
	s.handleHealthLive(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on liveness even with failing checks, got %d", w.Code)
	}

	var body healthLivenessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "alive" {
		t.Errorf("expected status alive, got %s", body.Status)
	}
}

func TestHealthReadyNoRegisteredCheckStillHealthy(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.setReadiness(true)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with no checks, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Ready {
		t.Error("expected ready=true")
	}
	if body.Status != "ok" {
		t.Errorf("expected status ok, got %q", body.Status)
	}
}

func TestRootResponse(t *testing.T) {
	s := &APIServer{}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.handleRoot(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["service"] != "Strata RMM Orchestrator" {
		t.Errorf("unexpected service: %s", body["service"])
	}
}

func TestHealthReadyDispatcherHealthy(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.setReadiness(true)
	s.SetDispatcherHealthy(true)
	s.RegisterHealth("dispatcher", func(ctx context.Context) error {
		if !s.DispatcherHealthy() {
			return fmt.Errorf("dispatcher not started")
		}
		return nil
	})

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when dispatcher healthy, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Ready {
		t.Error("expected ready=true")
	}
	if body.Components["dispatcher"] != "ok" {
		t.Errorf("expected dispatcher ok, got %q", body.Components["dispatcher"])
	}
}

func TestHealthReadyDispatcherNotStarted(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.setReadiness(true)
	s.SetDispatcherHealthy(false)
	s.RegisterHealth("dispatcher", func(ctx context.Context) error {
		if !s.DispatcherHealthy() {
			return fmt.Errorf("dispatcher not started")
		}
		return nil
	})

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when dispatcher not started, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Ready {
		t.Error("expected ready=false")
	}
	if body.Components["dispatcher"] != "failed: dispatcher not started" {
		t.Errorf("expected dispatcher failure, got %q", body.Components["dispatcher"])
	}
}

func TestHealthReadyMigrationsComplete(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.setReadiness(true)
	s.SetMigrationsComplete(true)
	s.RegisterHealth("migrations", func(ctx context.Context) error {
		if !s.MigrationsComplete() {
			return fmt.Errorf("migrations not complete")
		}
		return nil
	})

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when migrations complete, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Ready {
		t.Error("expected ready=true")
	}
	if body.Components["migrations"] != "ok" {
		t.Errorf("expected migrations ok, got %q", body.Components["migrations"])
	}
}

func TestHealthReadyMigrationsNotComplete(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.setReadiness(true)
	s.SetMigrationsComplete(false)
	s.RegisterHealth("migrations", func(ctx context.Context) error {
		if !s.MigrationsComplete() {
			return fmt.Errorf("migrations not complete")
		}
		return nil
	})

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when migrations not complete, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Ready {
		t.Error("expected ready=false")
	}
	if body.Components["migrations"] != "failed: migrations not complete" {
		t.Errorf("expected migrations failure, got %q", body.Components["migrations"])
	}
}

func TestHealthReadyStorageRequiredAndHealthy(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.setReadiness(true)
	s.RegisterHealth("storage", func(ctx context.Context) error { return nil })

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when storage healthy, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Ready {
		t.Error("expected ready=true")
	}
	if body.Components["storage"] != "ok" {
		t.Errorf("expected storage ok, got %q", body.Components["storage"])
	}
}

func TestHealthReadyStorageRequiredAndFailed(t *testing.T) {
	s := &APIServer{healthRegistry: NewHealthRegistry()}
	s.setReadiness(true)
	s.RegisterHealth("storage", func(ctx context.Context) error {
		return fmt.Errorf("storage backend unreachable: connection refused")
	})

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	s.handleHealthReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when storage fails, got %d", w.Code)
	}

	var body healthReadinessResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Ready {
		t.Error("expected ready=false")
	}
	if body.Status != "degraded" {
		t.Errorf("expected status degraded, got %q", body.Status)
	}
	if body.Components["storage"] != "failed: storage backend unreachable: connection refused" {
		t.Errorf("expected storage failure, got %q", body.Components["storage"])
	}
}

func TestLoginValidation(t *testing.T) {
	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"email and password required"}`, http.StatusBadRequest)
	}).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
