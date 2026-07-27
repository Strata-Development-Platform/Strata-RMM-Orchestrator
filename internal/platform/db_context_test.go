package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransactionResponseFlushesBufferedResponse(t *testing.T) {
	buffered := newTransactionResponse()
	buffered.Header().Set("Content-Type", "application/json")
	buffered.WriteHeader(http.StatusCreated)
	if _, err := buffered.Write([]byte(`{"status":"created"}`)); err != nil {
		t.Fatalf("write buffered response: %v", err)
	}

	recorder := httptest.NewRecorder()
	buffered.flushTo(recorder)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
	if got := recorder.Body.String(); got != `{"status":"created"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestAgentRegistrationIsPublicBootstrapRoute(t *testing.T) {
	server := &APIServer{}
	if got := server.classifyRoute(http.MethodPost, "/api/v1/agent/register"); got != AccessPublic {
		t.Fatalf("registration access = %v, want public enrollment-token bootstrap", got)
	}
	if got := server.classifyRoute(http.MethodPost, "/api/v1/agent/config"); got != AccessAgent {
		t.Fatalf("agent config access = %v, want agent token", got)
	}
}
