package webrtc

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// PeerManager manages WebRTC peer connections for remote support sessions.
type PeerManager struct {
	nats     *nats.Conn
	logger   *zap.Logger
	mu       sync.RWMutex
	sessions map[string]*WebRTCSession
}

// NewPeerManager creates a new peer connection manager.
func NewPeerManager(nc *nats.Conn, logger *zap.Logger) *PeerManager {
	return &PeerManager{
		nats:     nc,
		logger:   logger,
		sessions: make(map[string]*WebRTCSession),
	}
}

// CreateSession creates a new WebRTC session and subscribes to signaling subjects.
func (pm *PeerManager) CreateSession(req *CreateSessionRequest) (*WebRTCSession, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	session := &WebRTCSession{
		ID:            fmt.Sprintf("webrtc-%s", uuid.New().String()[:8]),
		TenantID:      req.TenantID,
		DeviceID:      req.DeviceID,
		SupportUserID: req.SupportUserID,
		DeviceUserID:  req.DeviceUserID,
		Direction:     req.Direction,
		State:         SessionPending,
		RelayEnabled:  req.RelayEnabled,
		RelayProvider: req.RelayProvider,
		StartedAt:     time.Now(),
	}

	if req.RelayProvider == "" {
		session.RelayProvider = DefaultRelayConfig().Provider
	}

	pm.mu.Lock()
	pm.sessions[session.ID] = session
	pm.mu.Unlock()

	if pm.logger != nil {
		pm.logger.Info("created WebRTC session",
			zap.String("session_id", session.ID),
			zap.String("device_id", req.DeviceID),
			zap.String("support_user_id", req.SupportUserID),
			zap.String("direction", string(req.Direction)),
		)
	}

	// Subscribe to signaling subjects for this session
	if pm.nats != nil {
		signalingSubject := fmt.Sprintf("tenant.%s.webrtc.%s.signaling", req.TenantID, session.ID)
		_, err := pm.nats.Subscribe(signalingSubject, func(m *nats.Msg) {
			pm.handleSignalingMessage(session.ID, m)
		})
		if err != nil {
			pm.logger.Warn("failed to subscribe to signaling subject",
				zap.String("session_id", session.ID),
				zap.Error(err),
			)
		}
	}

	return session, nil
}

// GetSession returns a session by ID.
func (pm *PeerManager) GetSession(sessionID string) (*WebRTCSession, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	session, ok := pm.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return session, nil
}

// ListSessions returns all sessions for a tenant.
func (pm *PeerManager) ListSessions(tenantID string) []*WebRTCSession {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var sessions []*WebRTCSession
	for _, s := range pm.sessions {
		if s.TenantID == tenantID {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

// CreateOffer generates an SDP offer for a session.
func (pm *PeerManager) CreateOffer(sessionID string, senderID string) (*SDPMessage, error) {
	session, err := pm.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	offer := &SDPMessage{
		Type:      SDPOffer,
		SDP:       pm.generateMockSDPOffer(senderID),
		SessionID: sessionID,
		SenderID:  senderID,
		Timestamp: time.Now(),
	}

	session.SDPOffer = offer
	session.State = SessionConnecting

	pm.mu.Lock()
	pm.sessions[sessionID] = session
	pm.mu.Unlock()

	if pm.logger != nil {
		pm.logger.Info("created SDP offer",
			zap.String("session_id", sessionID),
			zap.String("sender_id", senderID),
		)
	}

	// Publish offer to signaling subject
	if pm.nats != nil {
		subject := fmt.Sprintf("tenant.%s.webrtc.%s.signaling", session.TenantID, sessionID)
		data, _ := json.Marshal(offer)
		if err := pm.nats.Publish(subject, data); err != nil {
			pm.logger.Warn("failed to publish SDP offer", zap.Error(err))
		}
	}

	return offer, nil
}

// HandleAnswer processes an SDP answer for a session.
func (pm *PeerManager) HandleAnswer(sessionID string, answer *SDPMessage) error {
	session, err := pm.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.SDPAnswer = answer
	session.State = SessionActive

	pm.mu.Lock()
	pm.sessions[sessionID] = session
	pm.mu.Unlock()

	if pm.logger != nil {
		pm.logger.Info("session connected",
			zap.String("session_id", sessionID),
			zap.String("receiver_id", answer.ReceiverID),
		)
	}

	// Publish answer to signaling subject
	if pm.nats != nil {
		subject := fmt.Sprintf("tenant.%s.webrtc.%s.signaling", session.TenantID, sessionID)
		data, _ := json.Marshal(answer)
		if err := pm.nats.Publish(subject, data); err != nil {
			pm.logger.Warn("failed to publish SDP answer", zap.Error(err))
		}
	}

	return nil
}

// AddICECandidate adds an ICE candidate to a session.
func (pm *PeerManager) AddICECandidate(sessionID string, candidate *ICECandidate) error {
	session, err := pm.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.ICECandidates = append(session.ICECandidates, *candidate)

	pm.mu.Lock()
	pm.sessions[sessionID] = session
	pm.mu.Unlock()

	if pm.logger != nil {
		pm.logger.Info("added ICE candidate",
			zap.String("session_id", sessionID),
			zap.String("candidate", candidate.Candidate),
		)
	}

	// Publish candidate to signaling subject
	if pm.nats != nil {
		subject := fmt.Sprintf("tenant.%s.webrtc.%s.signaling", session.TenantID, sessionID)
		data, _ := json.Marshal(candidate)
		if err := pm.nats.Publish(subject, data); err != nil {
			pm.logger.Warn("failed to publish ICE candidate", zap.Error(err))
		}
	}

	return nil
}

// EndSession ends a WebRTC session.
func (pm *PeerManager) EndSession(sessionID string, error string) (*WebRTCSession, error) {
	session, err := pm.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session.State = SessionEnded
	session.EndedAt = &now
	session.Error = error

	pm.mu.Lock()
	pm.sessions[sessionID] = session
	pm.mu.Unlock()

	if pm.logger != nil {
		pm.logger.Info("ended WebRTC session",
			zap.String("session_id", sessionID),
			zap.Duration("duration", session.EndedAt.Sub(session.StartedAt)),
			zap.String("error", error),
		)
	}

	// Publish session ended event
	if pm.nats != nil {
		subject := fmt.Sprintf("tenant.%s.webrtc.%s.ended", session.TenantID, sessionID)
		data, _ := json.Marshal(map[string]interface{}{
			"session_id":  sessionID,
			"ended_at":    now.Format(time.RFC3339),
			"duration_ms": session.EndedAt.Sub(session.StartedAt).Milliseconds(),
			"error":       error,
		})
		if err := pm.nats.Publish(subject, data); err != nil {
			pm.logger.Warn("failed to publish session ended event", zap.Error(err))
		}
	}

	return session, nil
}

// GetRelayConfig returns the relay configuration for a session.
func (pm *PeerManager) GetRelayConfig(sessionID string) (*RelayConfig, error) {
	session, err := pm.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	if !session.RelayEnabled {
		return nil, nil
	}

	var relayCfg RelayConfig
	switch session.RelayProvider {
	case RelayGoogle:
		relayCfg = DefaultRelayConfig()
	case RelayTwilio:
		relayCfg = DefaultRelayConfigTwilio()
	default:
		relayCfg = DefaultRelayConfig()
	}

	return &relayCfg, nil
}

// handleSignalingMessage processes an incoming signaling message for a session.
func (pm *PeerManager) handleSignalingMessage(sessionID string, msg *nats.Msg) {
	var raw map[string]interface{}
	if err := json.Unmarshal(msg.Data, &raw); err != nil {
		pm.logger.Warn("failed to decode signaling message", zap.Error(err))
		return
	}

	msgType, _ := raw["type"].(string)
	switch msgType {
	case string(SDPOffer):
		pm.logger.Debug("received SDP offer via NATS", zap.String("session_id", sessionID))
	case string(SDPAnswer):
		pm.logger.Debug("received SDP answer via NATS", zap.String("session_id", sessionID))
	default:
		pm.logger.Debug("received signaling message via NATS", zap.String("session_id", sessionID), zap.String("type", msgType))
	}
}

// generateMockSDPOffer generates a mock SDP offer string.
func (pm *PeerManager) generateMockSDPOffer(senderID string) string {
	return fmt.Sprintf("v=0\r\n"+
		"o=- 0 0 IN IP4 127.0.0.1\r\n"+
		"s=Strata RMM WebRTC Remote Support\r\n"+
		"t=0 0\r\n"+
		"a=ice-ufrag:mock-ufrag-%s\r\n"+
		"a=ice-pwd:mock-pwd-%s\r\n",
		senderID, senderID,
	)
}
