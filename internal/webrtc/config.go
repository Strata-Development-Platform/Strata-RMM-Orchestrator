package webrtc

import (
	"fmt"
	"net/url"
	"time"
)

// RelayProvider represents a TURN/STUN relay service.
type RelayProvider string

const (
	RelayGoogle RelayProvider = "google"
	RelayTwilio RelayProvider = "twilio"
	RelayCustom RelayProvider = "custom"
	RelayNone   RelayProvider = "none"
)

// RelayConfig holds TURN/STUN server configuration for NAT traversal.
type RelayConfig struct {
	Provider    RelayProvider `json:"provider"`
	Enabled     bool          `json:"enabled"`
	STUNServers []string      `json:"stun_servers"`
	TURNServers []TurnServer  `json:"turn_servers"`
}

// TurnServer represents a single TURN/STUN server.
type TurnServer struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// ICECandidate represents a WebRTC ICE candidate.
type ICECandidate struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdp_mid"`
	SDPMLineIndex uint16 `json:"sdp_mline_index"`
	UserFragment  string `json:"user_fragment,omitempty"`
}

// SDPType represents the type of SDP offer/answer.
type SDPType string

const (
	SDPOffer  SDPType = "offer"
	SDPAnswer SDPType = "answer"
)

// SDPMessage represents a WebRTC SDP exchange message.
type SDPMessage struct {
	Type       SDPType   `json:"type"`
	SDP        string    `json:"sdp"`
	SessionID  string    `json:"session_id"`
	SenderID   string    `json:"sender_id"`
	ReceiverID string    `json:"receiver_id"`
	Timestamp  time.Time `json:"timestamp"`
}

// SessionState represents the state of a WebRTC session.
type SessionState string

const (
	SessionPending    SessionState = "pending"
	SessionConnecting SessionState = "connecting"
	SessionActive     SessionState = "active"
	SessionRecording  SessionState = "recording"
	SessionEnded      SessionState = "ended"
	SessionFailed     SessionState = "failed"
)

// SessionDirection represents the direction of a remote support session.
type SessionDirection string

const (
	DirectionOutbound SessionDirection = "outbound" // support agent to device
	DirectionInbound  SessionDirection = "inbound"  // device to support agent
)

// WebRTCSession represents a WebRTC remote support session.
type WebRTCSession struct {
	ID              string           `json:"id"`
	TenantID        string           `json:"tenant_id"`
	DeviceID        string           `json:"device_id"`
	SupportUserID   string           `json:"support_user_id"`
	DeviceUserID    string           `json:"device_user_id,omitempty"`
	Direction       SessionDirection `json:"direction"`
	State           SessionState     `json:"state"`
	RelayEnabled    bool             `json:"relay_enabled"`
	RelayProvider   RelayProvider    `json:"relay_provider"`
	ICECandidates   []ICECandidate   `json:"ice_candidates"`
	SDPOffer        *SDPMessage      `json:"sdp_offer,omitempty"`
	SDPAnswer       *SDPMessage      `json:"sdp_answer,omitempty"`
	RecordingID     string           `json:"recording_id,omitempty"`
	TranscriptionID string           `json:"transcription_id,omitempty"`
	StartedAt       time.Time        `json:"started_at"`
	EndedAt         *time.Time       `json:"ended_at,omitempty"`
	Error           string           `json:"error,omitempty"`
}

// CreateSessionRequest is the request body for creating a WebRTC session.
type CreateSessionRequest struct {
	TenantID      string           `json:"tenant_id"`
	DeviceID      string           `json:"device_id"`
	SupportUserID string           `json:"support_user_id"`
	DeviceUserID  string           `json:"device_user_id,omitempty"`
	Direction     SessionDirection `json:"direction"`
	RelayEnabled  bool             `json:"relay_enabled"`
	RelayProvider RelayProvider    `json:"relay_provider,omitempty"`
}

// StartRecordingRequest is the request body for starting a recording.
type StartRecordingRequest struct {
	Format  string `json:"format"`            // "webm", "mkv"
	Bitrate string `json:"bitrate,omitempty"` // "high", "medium", "low"
}

// StartTranscriptionRequest is the request body for starting transcription.
type StartTranscriptionRequest struct {
	Language string `json:"language"`
	Provider string `json:"provider"` // "openai_whisper", "azure_speech", "google_speech"
}

// Validate checks the CreateSessionRequest for required fields.
func (r *CreateSessionRequest) Validate() error {
	if r.TenantID == "" {
		return fmt.Errorf("missing tenant_id")
	}
	if r.DeviceID == "" {
		return fmt.Errorf("missing device_id")
	}
	if r.SupportUserID == "" {
		return fmt.Errorf("missing support_user_id")
	}
	if r.Direction != DirectionOutbound && r.Direction != DirectionInbound {
		return fmt.Errorf("invalid direction: %s", r.Direction)
	}
	return nil
}

// Validate checks the StartRecordingRequest for required fields.
func (r *StartRecordingRequest) Validate() error {
	if r.Format != "webm" && r.Format != "mkv" {
		return fmt.Errorf("invalid format: %s (must be webm or mkv)", r.Format)
	}
	return nil
}

// Validate checks the StartTranscriptionRequest for required fields.
func (r *StartTranscriptionRequest) Validate() error {
	if r.Language == "" {
		return fmt.Errorf("missing language")
	}
	if r.Provider == "" {
		return fmt.Errorf("missing provider")
	}
	return nil
}

// Validate checks the RelayConfig for valid servers.
func (c *RelayConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.STUNServers) == 0 && len(c.TURNServers) == 0 {
		return fmt.Errorf("at least one STUN or TURN server required when relay is enabled")
	}
	for _, stun := range c.STUNServers {
		parsed, err := url.Parse(stun)
		if err != nil || (parsed.Scheme != "stun" && parsed.Scheme != "turn" && parsed.Scheme != "stuns" && parsed.Scheme != "turns") {
			return fmt.Errorf("invalid STUN server URL: %s", stun)
		}
	}
	for _, turn := range c.TURNServers {
		parsed, err := url.Parse(turn.URL)
		if err != nil || (parsed.Scheme != "stun" && parsed.Scheme != "turn" && parsed.Scheme != "stuns" && parsed.Scheme != "turns") {
			return fmt.Errorf("invalid TURN server URL: %s", turn.URL)
		}
	}
	return nil
}

// DefaultRelayConfig returns the default relay configuration (Google STUN servers).
func DefaultRelayConfig() RelayConfig {
	return RelayConfig{
		Provider: RelayGoogle,
		Enabled:  true,
		STUNServers: []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		},
		TURNServers: []TurnServer{},
	}
}

// DefaultRelayConfigTwilio returns a relay config with Twilio TURN servers.
func DefaultRelayConfigTwilio() RelayConfig {
	return RelayConfig{
		Provider: RelayTwilio,
		Enabled:  true,
		STUNServers: []string{
			"stun:global.turn.twilio.com:3478",
			"stun:udp.turn.twilio.com:3478",
		},
		TURNServers: []TurnServer{},
	}
}
