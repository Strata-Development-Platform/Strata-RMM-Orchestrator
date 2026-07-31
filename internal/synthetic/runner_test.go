package synthetic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunnerExercisesAllPathsWithoutLeakingSecrets(t *testing.T) {
	const token = "synthetic-session-secret"
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = true
		switch r.URL.Path {
		case "/health/live", "/health/ready":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/auth/login":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["email"] != "synthetic@example.test" || body["password"] != "password-secret" {
				http.Error(w, "bad credentials", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
		default:
			if r.Header.Get("Authorization") != "Bearer "+token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	runner, err := New(Config{
		BaseURL: server.URL, Email: "synthetic@example.test", Password: "password-secret",
		TenantID: "tenant-id", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	results := runner.Run(context.Background())
	if len(results) != 6 {
		t.Fatalf("result count=%d, want 6", len(results))
	}
	for _, result := range results {
		if !result.Success {
			t.Fatalf("check failed: %+v", result)
		}
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), "password-secret") || strings.Contains(string(encoded), token) {
			t.Fatalf("secret leaked in result: %s", encoded)
		}
	}
	for _, path := range []string{
		"/health/live", "/health/ready", "/api/v1/auth/login", "/api/v1/auth/me",
		"/api/v2/devices", "/api/v1/recordings/tenant-id",
	} {
		if !seen[path] {
			t.Errorf("path was not checked: %s", path)
		}
	}
}

func TestRunnerStopsAuthenticatedChecksAfterLoginFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	runner, err := New(Config{
		BaseURL: server.URL, Email: "synthetic@example.test", Password: "bad",
		TenantID: "tenant-id", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	results := runner.Run(context.Background())
	if len(results) != 3 || results[2].Success {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestRunnerRejectsRemotePlaintextURL(t *testing.T) {
	_, err := New(Config{
		BaseURL: "http://example.com", Email: "a", Password: "b", TenantID: "c",
	})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("expected HTTPS rejection, got %v", err)
	}
}

func TestRunnerDoesNotFollowCredentialRedirect(t *testing.T) {
	redirectReached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectReached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer source.Close()

	runner, err := New(Config{
		BaseURL: source.URL, Email: "synthetic@example.test", Password: "password-secret",
		TenantID: "tenant-id", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	results := runner.Run(context.Background())
	if len(results) != 3 || results[2].Success {
		t.Fatalf("redirected login should fail: %+v", results)
	}
	if redirectReached {
		t.Fatal("login credentials were followed to a redirect target")
	}
}
