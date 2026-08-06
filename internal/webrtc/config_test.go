package webrtc

import (
	"testing"
	"time"
)

func TestRelayConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  RelayConfig
		wantErr bool
	}{
		{
			name:    "disabled relay",
			config:  RelayConfig{Provider: RelayGoogle, Enabled: false},
			wantErr: false,
		},
		{
			name: "valid stun only",
			config: RelayConfig{
				Provider:    RelayGoogle,
				Enabled:     true,
				STUNServers: []string{"stun:stun.l.google.com:19302"},
			},
			wantErr: false,
		},
		{
			name: "valid turn",
			config: RelayConfig{
				Provider: RelayTwilio,
				Enabled:  true,
				TURNServers: []TurnServer{
					{URL: "turn:global.turn.twilio.com:3478", Username: "user", Password: "pass"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty servers when enabled",
			config: RelayConfig{
				Provider:    RelayGoogle,
				Enabled:     true,
				STUNServers: []string{},
				TURNServers: []TurnServer{},
			},
			wantErr: true,
		},
		{
			name: "invalid stun url",
			config: RelayConfig{
				Provider:    RelayGoogle,
				Enabled:     true,
				STUNServers: []string{"not-a-valid-url"},
			},
			wantErr: true,
		},
		{
			name: "invalid turn url",
			config: RelayConfig{
				Provider: RelayTwilio,
				Enabled:  true,
				TURNServers: []TurnServer{
					{URL: "not-a-valid-url"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("RelayConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultRelayConfig(t *testing.T) {
	config := DefaultRelayConfig()
	if config.Provider != RelayGoogle {
		t.Fatalf("expected provider google, got %s", config.Provider)
	}
	if !config.Enabled {
		t.Fatal("expected relay enabled")
	}
	if len(config.STUNServers) == 0 {
		t.Fatal("expected stun servers")
	}
	if len(config.TURNServers) != 0 {
		t.Fatalf("expected 0 turn servers, got %d", len(config.TURNServers))
	}
}

func TestDefaultRelayConfigTwilio(t *testing.T) {
	config := DefaultRelayConfigTwilio()
	if config.Provider != RelayTwilio {
		t.Fatalf("expected provider twilio, got %s", config.Provider)
	}
	if !config.Enabled {
		t.Fatal("expected relay enabled")
	}
}

func TestCreateSessionRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateSessionRequest
		wantErr bool
	}{
		{
			name: "valid outbound",
			req: CreateSessionRequest{
				TenantID:      "t1",
				DeviceID:      "d1",
				SupportUserID: "u1",
				Direction:     DirectionOutbound,
			},
			wantErr: false,
		},
		{
			name: "valid inbound",
			req: CreateSessionRequest{
				TenantID:      "t1",
				DeviceID:      "d1",
				SupportUserID: "u1",
				Direction:     DirectionInbound,
			},
			wantErr: false,
		},
		{
			name:    "missing tenant_id",
			req:     CreateSessionRequest{DeviceID: "d1", SupportUserID: "u1"},
			wantErr: true,
		},
		{
			name:    "missing device_id",
			req:     CreateSessionRequest{TenantID: "t1", SupportUserID: "u1"},
			wantErr: true,
		},
		{
			name:    "missing support_user_id",
			req:     CreateSessionRequest{TenantID: "t1", DeviceID: "d1"},
			wantErr: true,
		},
		{
			name:    "invalid direction",
			req:     CreateSessionRequest{TenantID: "t1", DeviceID: "d1", SupportUserID: "u1", Direction: "invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateSessionRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStartRecordingRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     StartRecordingRequest
		wantErr bool
	}{
		{
			name:    "valid webm",
			req:     StartRecordingRequest{Format: "webm"},
			wantErr: false,
		},
		{
			name:    "valid mkv",
			req:     StartRecordingRequest{Format: "mkv"},
			wantErr: false,
		},
		{
			name:    "invalid format",
			req:     StartRecordingRequest{Format: "avi"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("StartRecordingRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStartTranscriptionRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     StartTranscriptionRequest
		wantErr bool
	}{
		{
			name:    "valid",
			req:     StartTranscriptionRequest{Language: "en", Provider: "openai_whisper"},
			wantErr: false,
		},
		{
			name:    "missing language",
			req:     StartTranscriptionRequest{Provider: "openai_whisper"},
			wantErr: true,
		},
		{
			name:    "missing provider",
			req:     StartTranscriptionRequest{Language: "en"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("StartTranscriptionRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateProvider(t *testing.T) {
	tests := []struct {
		name    string
		provider string
		wantErr bool
	}{
		{name: "openai_whisper", provider: "openai_whisper", wantErr: false},
		{name: "azure_speech", provider: "azure_speech", wantErr: false},
		{name: "google_speech", provider: "google_speech", wantErr: false},
		{name: "invalid", provider: "invalid", wantErr: true},
		{name: "empty", provider: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProvider(tt.provider)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateProvider(%q) error = %v, wantErr %v", tt.provider, err, tt.wantErr)
			}
		})
	}
}

func TestSessionStateConstants(t *testing.T) {
	states := []SessionState{
		SessionPending, SessionConnecting, SessionActive,
		SessionRecording, SessionEnded, SessionFailed,
	}
	for _, s := range states {
		s := s
		t.Run(string(s), func(t *testing.T) {
			if string(s) == "" {
				t.Fatal("expected non-empty state")
			}
		})
	}
}

func TestSessionDirectionConstants(t *testing.T) {
	directions := []SessionDirection{DirectionOutbound, DirectionInbound}
	for _, d := range directions {
		d := d
		t.Run(string(d), func(t *testing.T) {
			if string(d) == "" {
				t.Fatal("expected non-empty direction")
			}
		})
	}
}

func TestTranscriptionProviderConstants(t *testing.T) {
	providers := []TranscriptionProvider{ProviderOpenAI, ProviderAzure, ProviderGoogle}
	for _, p := range providers {
		p := p
		t.Run(string(p), func(t *testing.T) {
			if string(p) == "" {
				t.Fatal("expected non-empty provider")
			}
		})
	}
}

func TestTranscriptionStatusConstants(t *testing.T) {
	statuses := []TranscriptionStatus{
		TranscriptionPending, TranscriptionRunning,
		TranscriptionCompleted, TranscriptionFailed,
	}
	for _, s := range statuses {
		s := s
		t.Run(string(s), func(t *testing.T) {
			if string(s) == "" {
				t.Fatal("expected non-empty status")
			}
		})
	}
}

func TestSDPTypeConstants(t *testing.T) {
	types := []SDPType{SDPOffer, SDPAnswer}
	for _, tp := range types {
		tp := tp
		t.Run(string(tp), func(t *testing.T) {
			if string(tp) == "" {
				t.Fatal("expected non-empty SDP type")
			}
		})
	}
}

func TestWebRTCSessionFields(t *testing.T) {
	now := time.Now()
	session := &WebRTCSession{
		ID:              "webrtc-abc123",
		TenantID:        "tenant-1",
		DeviceID:        "device-1",
		SupportUserID:   "user-1",
		Direction:       DirectionOutbound,
		State:           SessionActive,
		RelayEnabled:    true,
		RelayProvider:   RelayGoogle,
		StartedAt:       now,
		EndedAt:         &now,
	}

	if session.ID != "webrtc-abc123" {
		t.Fatalf("expected ID webrtc-abc123, got %s", session.ID)
	}
	if session.State != SessionActive {
		t.Fatalf("expected state active, got %s", session.State)
	}
	if session.Direction != DirectionOutbound {
		t.Fatalf("expected direction outbound, got %s", session.Direction)
	}
}

func TestICECandidateFields(t *testing.T) {
	candidate := &ICECandidate{
		Candidate:     "candidate:1 1 UDP 2122260223 192.168.1.1 54321 typ host",
		SDPMid:        "0",
		SDPMLineIndex: 0,
	}

	if candidate.Candidate == "" {
		t.Fatal("expected non-empty candidate")
	}
	if candidate.SDPMid != "0" {
		t.Fatalf("expected SDPMid 0, got %s", candidate.SDPMid)
	}
}

func TestTurnServerFields(t *testing.T) {
	server := TurnServer{
		URL:      "turn:turn.example.com:3478",
		Username: "testuser",
		Password: "testpass",
	}

	if server.URL == "" {
		t.Fatal("expected non-empty URL")
	}
	if server.Username == "" {
		t.Fatal("expected non-empty username")
	}
}

func TestSDPMessageFields(t *testing.T) {
	msg := &SDPMessage{
		Type:       SDPOffer,
		SDP:        "v=0\r\no=-\r\n",
		SessionID:  "sess-123",
		SenderID:   "user-1",
		Timestamp:  time.Now(),
	}

	if msg.Type != SDPOffer {
		t.Fatalf("expected type offer, got %s", msg.Type)
	}
	if msg.SDP == "" {
		t.Fatal("expected non-empty SDP")
	}
	if msg.SessionID != "sess-123" {
		t.Fatalf("expected session_id sess-123, got %s", msg.SessionID)
	}
}

func TestTranscriptionSegmentFields(t *testing.T) {
	seg := &TranscriptionSegment{
		ID:        "seg-001",
		SessionID: "sess-123",
		Language:  "en",
		StartTime: 0.0,
		EndTime:   5.0,
		Text:      "Hello, this is a test transcription.",
		Speaker:   "agent",
		Sentiment: "neutral",
		Timestamp: time.Now(),
	}

	if seg.ID != "seg-001" {
		t.Fatalf("expected ID seg-001, got %s", seg.ID)
	}
	if seg.Text == "" {
		t.Fatal("expected non-empty text")
	}
	if seg.StartTime != 0.0 {
		t.Fatalf("expected start time 0.0, got %f", seg.StartTime)
	}
}

func TestTranscriptionResultFields(t *testing.T) {
	now := time.Now()
	tr := &TranscriptionResult{
		ID:         "trans-001",
		SessionID:  "sess-123",
		Language:   "en",
		Provider:   ProviderOpenAI,
		Status:     TranscriptionCompleted,
		DurationMs: 120000,
		StartedAt:  now,
		CompletedAt: &now,
		TranscriptText: "This is the full transcript text.",
	}

	if tr.ID != "trans-001" {
		t.Fatalf("expected ID trans-001, got %s", tr.ID)
	}
	if tr.Status != TranscriptionCompleted {
		t.Fatalf("expected status completed, got %s", tr.Status)
	}
	if tr.DurationMs != 120000 {
		t.Fatalf("expected duration 120000ms, got %d", tr.DurationMs)
	}
	if tr.TranscriptText == "" {
		t.Fatal("expected non-empty transcript text")
	}
}

func TestRecordingSessionFields(t *testing.T) {
	now := time.Now()
	rs := &RecordingSession{
		ID:            "rec-001",
		WebRTCSession: "webrtc-123",
		Format:        "webm",
		Bitrate:       "high",
		StartedAt:     now,
		EndedAt:       &now,
		FilePath:      "/tmp/recording.webm",
		StorageKey:    "s3://bucket/recording.webm",
		SizeBytes:     1048576,
		DurationMs:    300000,
	}

	if rs.ID != "rec-001" {
		t.Fatalf("expected ID rec-001, got %s", rs.ID)
	}
	if rs.Format != "webm" {
		t.Fatalf("expected format webm, got %s", rs.Format)
	}
	if rs.DurationMs != 300000 {
		t.Fatalf("expected duration 300000ms, got %d", rs.DurationMs)
	}
}

func TestRelayProviderConstants(t *testing.T) {
	providers := []RelayProvider{RelayGoogle, RelayTwilio, RelayCustom, RelayNone}
	for _, p := range providers {
		p := p
		t.Run(string(p), func(t *testing.T) {
			if string(p) == "" {
				t.Fatal("expected non-empty provider")
			}
		})
	}
}
