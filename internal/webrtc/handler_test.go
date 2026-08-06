package webrtc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/remote"
)

func newTestHandler(t *testing.T) *Handler {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	recorder := remote.NewRecorder(nil, logger)
	return NewHandler(nil, recorder, logger)
}

func TestHandleCreateSession(t *testing.T) {
	h := newTestHandler(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantField  string
	}{
		{
			name:       "valid outbound session",
			body:       `{"tenant_id":"t1","device_id":"d1","support_user_id":"u1","direction":"outbound"}`,
			wantStatus: http.StatusCreated,
			wantField:  "id",
		},
		{
			name:       "valid inbound session",
			body:       `{"tenant_id":"t1","device_id":"d1","support_user_id":"u1","direction":"inbound"}`,
			wantStatus: http.StatusCreated,
			wantField:  "id",
		},
		{
			name:       "missing tenant_id",
			body:       `{"device_id":"d1","support_user_id":"u1"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing device_id",
			body:       `{"tenant_id":"t1","support_user_id":"u1"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing support_user_id",
			body:       `{"tenant_id":"t1","device_id":"d1"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid direction",
			body:       `{"tenant_id":"t1","device_id":"d1","support_user_id":"u1","direction":"invalid"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webrtc/sessions", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.HandleCreateSession(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if tt.wantStatus == http.StatusCreated && tt.wantField != "" {
				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if _, ok := resp[tt.wantField]; !ok {
					t.Fatalf("expected response field %s, not found in %v", tt.wantField, resp)
				}
			}
		})
	}
}

func TestHandleGetSession(t *testing.T) {
	h := newTestHandler(t)

	session, err := h.peerManager.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Test GetSession directly
	found, err := h.peerManager.GetSession(session.ID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if found.ID != session.ID {
		t.Fatalf("expected ID %s, got %s", session.ID, found.ID)
	}

	// Test unknown session
	_, err = h.peerManager.GetSession("unknown-session")
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
}

func TestHandleCreateOffer(t *testing.T) {
	h := newTestHandler(t)

	session, _ := h.peerManager.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})

	// Test CreateOffer directly
	offer, err := h.peerManager.CreateOffer(session.ID, "agent-1")
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}
	if offer.Type != SDPOffer {
		t.Fatalf("expected type offer, got %s", offer.Type)
	}
}

func TestHandleHandleAnswer(t *testing.T) {
	h := newTestHandler(t)

	session, _ := h.peerManager.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})

	tests := []struct {
		name       string
		sessionID  string
		body       string
		wantStatus int
	}{
		{
			name:       "valid answer",
			sessionID:  session.ID,
			body:       `{"sdp":"v=0\r\n","receiver_id":"device-1"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing session ID",
			sessionID:  "",
			body:       `{"sdp":"v=0\r\n","receiver_id":"device-1"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing sdp",
			sessionID:  session.ID,
			body:       `{"receiver_id":"device-1"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webrtc/sessions/"+tt.sessionID+"/answer", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.HandleHandleAnswer(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleAddICECandidate(t *testing.T) {
	h := newTestHandler(t)

	session, _ := h.peerManager.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})

	candidate := &ICECandidate{
		Candidate:     "candidate:1 1 UDP 1 1.2.3.4 54321 typ host",
		SDPMid:        "0",
		SDPMLineIndex: 0,
	}

	if err := h.peerManager.AddICECandidate(session.ID, candidate); err != nil {
		t.Fatalf("failed to add ICE candidate: %v", err)
	}

	if len(session.ICECandidates) != 1 {
		t.Fatalf("expected 1 ICE candidate, got %d", len(session.ICECandidates))
	}
}

func TestHandleEndSession(t *testing.T) {
	h := newTestHandler(t)

	session, _ := h.peerManager.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})

	ended, err := h.peerManager.EndSession(session.ID, "connection lost")
	if err != nil {
		t.Fatalf("failed to end session: %v", err)
	}

	if ended.State != SessionEnded {
		t.Fatalf("expected state ended, got %s", ended.State)
	}
	if ended.Error != "connection lost" {
		t.Fatalf("expected error 'connection lost', got %s", ended.Error)
	}
}

func TestHandleListSessions(t *testing.T) {
	h := newTestHandler(t)

	// Create sessions for a tenant
	h.peerManager.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})
	h.peerManager.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d2",
		SupportUserID: "u2",
		Direction:     DirectionInbound,
	})

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{name: "with tenant_id", query: "tenant_id=t1", wantStatus: http.StatusOK},
		{name: "missing tenant_id", query: "", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/webrtc/sessions?"+tt.query, nil)
			w := httptest.NewRecorder()
			h.HandleListSessions(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleGetRelayConfig(t *testing.T) {
	h := newTestHandler(t)

	session, _ := h.peerManager.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
		RelayEnabled:  true,
	})

	// Get relay config directly from peer manager
	config, err := h.peerManager.GetRelayConfig(session.ID)
	if err != nil {
		t.Fatalf("failed to get relay config: %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil relay config")
	}
}

func TestNATSConnection(t *testing.T) {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS not available:", err)
	}
	defer nc.Close()

	logger, _ := zap.NewDevelopment()
	recorder := remote.NewRecorder(nil, logger)
	h := NewHandler(nc, recorder, logger)

	_, err = h.peerManager.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})
	if err != nil {
		t.Fatalf("failed to create session with NATS: %v", err)
	}
}
