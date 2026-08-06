package webrtc

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

func newTestPeerManager(t *testing.T) *PeerManager {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return NewPeerManager(nil, logger)
}

func TestCreateSession(t *testing.T) {
	pm := newTestPeerManager(t)

	session, err := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if session.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if session.TenantID != "t1" {
		t.Fatalf("expected tenant_id t1, got %s", session.TenantID)
	}
	if session.DeviceID != "d1" {
		t.Fatalf("expected device_id d1, got %s", session.DeviceID)
	}
	if session.State != SessionPending {
		t.Fatalf("expected state pending, got %s", session.State)
	}
}

func TestCreateSessionWithNATS(t *testing.T) {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS not available:", err)
	}
	defer nc.Close()

	logger, _ := zap.NewDevelopment()
	pm := NewPeerManager(nc, logger)

	session, err := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if session.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
}

func TestGetSession(t *testing.T) {
	pm := newTestPeerManager(t)

	session, _ := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})

	found, err := pm.GetSession(session.ID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if found.ID != session.ID {
		t.Fatalf("expected ID %s, got %s", session.ID, found.ID)
	}

	_, err = pm.GetSession("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestListSessions(t *testing.T) {
	pm := newTestPeerManager(t)

	pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})
	pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d2",
		SupportUserID: "u2",
		Direction:     DirectionInbound,
	})
	pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t2",
		DeviceID:      "d3",
		SupportUserID: "u3",
		Direction:     DirectionOutbound,
	})

	sessions := pm.ListSessions("t1")
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions for t1, got %d", len(sessions))
	}
}

func TestCreateOffer(t *testing.T) {
	pm := newTestPeerManager(t)

	session, _ := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})

	offer, err := pm.CreateOffer(session.ID, "agent-1")
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}

	if offer.Type != SDPOffer {
		t.Fatalf("expected type offer, got %s", offer.Type)
	}
	if offer.SenderID != "agent-1" {
		t.Fatalf("expected sender_id agent-1, got %s", offer.SenderID)
	}
	if offer.SDP == "" {
		t.Fatal("expected non-empty SDP")
	}
	if session.State != SessionConnecting {
		t.Fatalf("expected state connecting, got %s", session.State)
	}
}

func TestHandleAnswer(t *testing.T) {
	pm := newTestPeerManager(t)

	session, _ := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})

	// Create offer first
	pm.CreateOffer(session.ID, "agent-1")

	answer := &SDPMessage{
		Type:       SDPAnswer,
		SDP:        "v=0\r\n",
		SessionID:  session.ID,
		ReceiverID: "device-1",
	}

	if err := pm.HandleAnswer(session.ID, answer); err != nil {
		t.Fatalf("failed to handle answer: %v", err)
	}

	if session.State != SessionActive {
		t.Fatalf("expected state active, got %s", session.State)
	}
}

func TestAddICECandidate(t *testing.T) {
	pm := newTestPeerManager(t)

	session, _ := pm.CreateSession(&CreateSessionRequest{
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

	if err := pm.AddICECandidate(session.ID, candidate); err != nil {
		t.Fatalf("failed to add ICE candidate: %v", err)
	}

	if len(session.ICECandidates) != 1 {
		t.Fatalf("expected 1 ICE candidate, got %d", len(session.ICECandidates))
	}
}

func TestEndSession(t *testing.T) {
	pm := newTestPeerManager(t)

	session, _ := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})

	ended, err := pm.EndSession(session.ID, "connection lost")
	if err != nil {
		t.Fatalf("failed to end session: %v", err)
	}

	if ended.State != SessionEnded {
		t.Fatalf("expected state ended, got %s", ended.State)
	}
	if ended.Error != "connection lost" {
		t.Fatalf("expected error 'connection lost', got %s", ended.Error)
	}
	if ended.EndedAt == nil {
		t.Fatal("expected non-nil ended_at")
	}
}

func TestGetRelayConfig(t *testing.T) {
	pm := newTestPeerManager(t)

	// Session with relay enabled
	session, _ := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
		RelayEnabled:  true,
	})

	config, err := pm.GetRelayConfig(session.ID)
	if err != nil {
		t.Fatalf("failed to get relay config: %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil relay config")
	}

	// Session with relay disabled
	session2, _ := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d2",
		SupportUserID: "u2",
		Direction:     DirectionOutbound,
		RelayEnabled:  false,
	})

	config2, err := pm.GetRelayConfig(session2.ID)
	if err != nil {
		t.Fatalf("failed to get relay config: %v", err)
	}
	if config2 != nil {
		t.Fatal("expected nil relay config when relay disabled")
	}
}

func TestDefaultRelayConfigWithTurn(t *testing.T) {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS not available:", err)
	}
	defer nc.Close()

	logger, _ := zap.NewDevelopment()
	pm := NewPeerManager(nc, logger)

	session, _ := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
		RelayEnabled:  true,
		RelayProvider: RelayTwilio,
	})

	config, err := pm.GetRelayConfig(session.ID)
	if err != nil {
		t.Fatalf("failed to get relay config: %v", err)
	}
	if config.Provider != RelayTwilio {
		t.Fatalf("expected provider twilio, got %s", config.Provider)
	}
}

func TestHandleSignalingMessage(t *testing.T) {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS not available:", err)
	}
	defer nc.Close()

	logger, _ := zap.NewDevelopment()
	pm := NewPeerManager(nc, logger)

	session, err := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Publish a signaling message
	subject := "tenant.t1.webrtc." + session.ID + ".signaling"
	data, _ := json.Marshal(map[string]interface{}{
		"type":    "offer",
		"sdp":     "v=0\r\n",
		"session": session.ID,
	})
	if err := nc.Publish(subject, data); err != nil {
		t.Fatalf("failed to publish signaling message: %v", err)
	}

	// Give it time to be processed
	pm.handleSignalingMessage(session.ID, &nats.Msg{Subject: subject, Data: data})
}

func TestGenerateMockSDPOffer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	pm := NewPeerManager(nil, logger)

	offer := pm.generateMockSDPOffer("agent-1")
	if offer == "" {
		t.Fatal("expected non-empty SDP offer")
	}
	if !contains(offer, "v=0") {
		t.Fatal("expected SDP to contain v=0")
	}
	if !contains(offer, "agent-1") {
		t.Fatal("expected SDP to contain agent ID")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSessionIDFormat(t *testing.T) {
	pm := newTestPeerManager(t)

	session, err := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if len(session.ID) < 10 {
		t.Fatalf("expected session ID to be at least 10 chars, got %d", len(session.ID))
	}
	if !contains(session.ID, "webrtc-") {
		t.Fatalf("expected session ID to contain webrtc- prefix, got %s", session.ID)
	}
}

func TestPeerManagerWithNilNATS(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	pm := NewPeerManager(nil, logger)

	session, err := pm.CreateSession(&CreateSessionRequest{
		TenantID:      "t1",
		DeviceID:      "d1",
		SupportUserID: "u1",
		Direction:     DirectionOutbound,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	offer, err := pm.CreateOffer(session.ID, "agent-1")
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}
	if offer == nil {
		t.Fatal("expected non-nil offer")
	}
}

// Test helper to generate a UUID-based session ID
func generateUUID() string {
	return uuid.New().String()
}
