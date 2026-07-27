package test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

var baseURL = getEnv("API_URL", "http://localhost:8080")

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type healthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

func TestHealthEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("expected status ok, got %s", body.Status)
	}
}

func TestRootEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["service"] != "Strata RMM Orchestrator" {
		t.Errorf("unexpected service: %s", body["service"])
	}
}

func TestInstallScript(t *testing.T) {
	resp, err := http.Get(baseURL + "/install.sh")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "shellscript") && !strings.Contains(ct, "text/") {
		t.Errorf("unexpected content-type: %s", ct)
	}
}

func TestMeUnauthenticated(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAdminUsersUnauthenticated(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/v1/admin/users")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLoginEndpoint(t *testing.T) {
	body := `{"email":"test@test.com","password":"wrong"}`
	resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should return 401 for invalid credentials
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad credentials, got %d", resp.StatusCode)
	}
}

func TestPoliciesUnauthenticated(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/v1/policies")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestBrandingUnauthenticated(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/v1/branding")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
