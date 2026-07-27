//go:build integration

package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

var baseURL = getEnv("API_URL", "http://localhost:8080")
var testAdminEmail = getEnv("TEST_ADMIN_EMAIL", "")
var testAdminPassword = getEnv("TEST_ADMIN_PASSWORD", "")

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// --- Baseline contract tests ---

func TestHealthEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
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

func TestInstallScriptPublic(t *testing.T) {
	resp, err := http.Get(baseURL + "/install.sh")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// --- Auth boundary tests ---

func TestProtectedRoutesRejectNoAuth(t *testing.T) {
	protected := []string{
		"/api/v1/auth/me",
		"/api/v1/platform/overview",
		"/api/v1/platform/customers",
		"/api/v1/admin/users",
		"/api/v1/branding",
		"/api/v1/jobs",
		"/api/v1/policies",
		"/api/v2/platform/msps",
	}
	for _, path := range protected {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(baseURL + path)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s: expected 401, got %d", path, resp.StatusCode)
			}
		})
	}
}

func TestPublicRoutesAccessible(t *testing.T) {
	public := []struct {
		method string
		path   string
		code   int
	}{
		{"GET", "/health", 200},
		{"GET", "/", 200},
		{"GET", "/install.sh", 200},
	}
	for _, tc := range public {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp, err := http.Get(baseURL + tc.path)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.code {
				t.Errorf("%s: expected %d, got %d", tc.path, tc.code, resp.StatusCode)
			}
		})
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	body := `{"email":"nonexistent@test.com","password":"wrong"}`
	resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad creds, got %d", resp.StatusCode)
	}
}

func requireTestCredentials(t *testing.T) {
	if testAdminEmail == "" || testAdminPassword == "" {
		t.Skip("TEST_ADMIN_EMAIL and TEST_ADMIN_PASSWORD not set")
	}
}

func adminLoginBody() string {
	return fmt.Sprintf(`{"email":"%s","password":"%s"}`, testAdminEmail, testAdminPassword)
}

func TestLoginAndMeFlow(t *testing.T) {
	requireTestCredentials(t)
	resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json", strings.NewReader(adminLoginBody()))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login expected 200, got %d", resp.StatusCode)
	}

	var loginResp struct {
		Token  string `json:"token"`
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if loginResp.Token == "" {
		t.Fatal("login token is empty")
	}
	if loginResp.UserID == "" {
		t.Fatal("login user_id is empty")
	}
	if loginResp.Email != testAdminEmail {
		t.Errorf("expected email %s, got %s", testAdminEmail, loginResp.Email)
	}

	// Test /auth/me with Bearer token
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	meResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("me request failed: %v", err)
	}
	defer meResp.Body.Close()
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("me expected 200, got %d", meResp.StatusCode)
	}

	var meData struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&meData); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if meData.UserID != loginResp.UserID {
		t.Errorf("me user_id %s != login user_id %s", meData.UserID, loginResp.UserID)
	}
	if meData.Email != loginResp.Email {
		t.Errorf("me email %s != login email %s", meData.Email, loginResp.Email)
	}
}

func TestBearerTokenAuth(t *testing.T) {
	requireTestCredentials(t)
	body := adminLoginBody()
	resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer resp.Body.Close()

	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Bearer token should work
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/platform/overview", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == http.StatusUnauthorized {
		t.Error("Bearer token rejected")
	}
}

func TestAdminRoutesWithBearer(t *testing.T) {
	requireTestCredentials(t)
	body := adminLoginBody()
	resp, err := http.Post(baseURL+"/api/v1/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer resp.Body.Close()

	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	req, _ := http.NewRequest("GET", baseURL+"/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("admin/users with admin token expected 200, got %d", resp2.StatusCode)
	}
}

func TestMalformedTokenRejected(t *testing.T) {
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for malformed token, got %d", resp.StatusCode)
	}
}

func TestEmptyTokenRejected(t *testing.T) {
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer ")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty token, got %d", resp.StatusCode)
	}
}

func TestNoAuthHeaderRejected(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestPoliciesRequireAuth(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/v1/policies")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestPlatformMSPSRequireAuth(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/v2/platform/msps")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestBrandingRequiresAuth(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/v1/branding")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}
