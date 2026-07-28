package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthReadyWhenReady(t *testing.T) {
	s := &APIServer{}
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
}

func TestHealthNotReadyReturns503(t *testing.T) {
	s := &APIServer{}
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
	s := &APIServer{}
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
	s := &APIServer{}
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
	s := &APIServer{}
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 by default, got %d", w.Code)
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
