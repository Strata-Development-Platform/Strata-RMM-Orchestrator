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

// TranscriptionProvider represents a speech-to-text provider.
type TranscriptionProvider string

const (
	ProviderOpenAI   TranscriptionProvider = "openai_whisper"
	ProviderAzure    TranscriptionProvider = "azure_speech"
	ProviderGoogle   TranscriptionProvider = "google_speech"
)

// TranscriptionStatus represents the status of a transcription.
type TranscriptionStatus string

const (
	TranscriptionPending   TranscriptionStatus = "pending"
	TranscriptionRunning   TranscriptionStatus = "running"
	TranscriptionCompleted TranscriptionStatus = "completed"
	TranscriptionFailed    TranscriptionStatus = "failed"
)

// TranscriptionSegment represents a segment of transcribed text.
type TranscriptionSegment struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Language  string    `json:"language"`
	StartTime float64   `json:"start_time"`
	EndTime   float64   `json:"end_time"`
	Text      string    `json:"text"`
	Speaker   string    `json:"speaker,omitempty"`
	Sentiment string    `json:"sentiment,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// TranscriptionResult represents the full result of a transcription.
type TranscriptionResult struct {
	ID             string                  `json:"id"`
	SessionID      string                  `json:"session_id"`
	Language       string                  `json:"language"`
	Provider       TranscriptionProvider   `json:"provider"`
	Status         TranscriptionStatus     `json:"status"`
	Segments       []TranscriptionSegment  `json:"segments"`
	DurationMs     int64                   `json:"duration_ms"`
	Error          string                  `json:"error,omitempty"`
	StartedAt      time.Time               `json:"started_at"`
	CompletedAt    *time.Time              `json:"completed_at,omitempty"`
	TranscriptText string                  `json:"transcript_text,omitempty"`
}

// TranscriptionManager manages live transcription for WebRTC sessions.
type TranscriptionManager struct {
	nats      *nats.Conn
	logger    *zap.Logger
	mu        sync.RWMutex
	transcriptions map[string]*TranscriptionResult
}

// NewTranscriptionManager creates a new transcription manager.
func NewTranscriptionManager(nc *nats.Conn, logger *zap.Logger) *TranscriptionManager {
	return &TranscriptionManager{
		nats:           nc,
		logger:         logger,
		transcriptions: make(map[string]*TranscriptionResult),
	}
}

// StartTranscription starts live transcription for a session.
func (tm *TranscriptionManager) StartTranscription(sessionID, language, provider string) (*TranscriptionResult, error) {
	if language == "" {
		return nil, fmt.Errorf("missing language")
	}
	if provider == "" {
		return nil, fmt.Errorf("missing provider")
	}

	tr := &TranscriptionResult{
		ID:        fmt.Sprintf("trans-%s", uuid.New().String()[:8]),
		SessionID: sessionID,
		Language:  language,
		Provider:  TranscriptionProvider(provider),
		Status:    TranscriptionPending,
		Segments:  []TranscriptionSegment{},
		StartedAt: time.Now(),
	}

	tm.mu.Lock()
	tm.transcriptions[tr.ID] = tr
	tm.mu.Unlock()

	if tm.logger != nil {
		tm.logger.Info("started transcription",
			zap.String("transcription_id", tr.ID),
			zap.String("session_id", sessionID),
			zap.String("language", language),
			zap.String("provider", string(provider)),
		)
	}

	// Subscribe to audio stream via NATS for real-time transcription
	if tm.nats != nil {
		subject := fmt.Sprintf("tenant.*.webrtc.%s.audio", sessionID)
		_, err := tm.nats.Subscribe(subject, func(m *nats.Msg) {
			tm.processAudioChunk(tr.ID, m)
		})
		if err != nil {
			tm.logger.Warn("failed to subscribe to audio stream",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	return tr, nil
}

// StopTranscription stops transcription for a session.
func (tm *TranscriptionManager) StopTranscription(transcriptionID string) (*TranscriptionResult, error) {
	tm.mu.Lock()
	tr, ok := tm.transcriptions[transcriptionID]
	if !ok {
		tm.mu.Unlock()
		return nil, fmt.Errorf("transcription not found: %s", transcriptionID)
	}

	tr.Status = TranscriptionCompleted
	now := time.Now()
	tr.CompletedAt = &now
	tm.mu.Unlock()

	if tm.logger != nil {
		tm.logger.Info("stopped transcription",
			zap.String("transcription_id", transcriptionID),
			zap.Int("segments", len(tr.Segments)),
		)
	}

	// Compile full transcript text
	var transcriptText string
	for _, seg := range tr.Segments {
		transcriptText += seg.Text + " "
	}
	tr.TranscriptText = transcriptText

	// Publish transcription completed event
	if tm.nats != nil {
		subject := fmt.Sprintf("tenant.*.webrtc.%s.transcription.completed", tr.SessionID)
		data, _ := json.Marshal(map[string]interface{}{
			"transcription_id": transcriptionID,
			"session_id":       tr.SessionID,
			"language":         tr.Language,
			"provider":         string(tr.Provider),
			"segments":         len(tr.Segments),
			"duration_ms":      tr.DurationMs,
			"transcript_text":  tr.TranscriptText,
		})
		if err := tm.nats.Publish(subject, data); err != nil {
			tm.logger.Warn("failed to publish transcription completed event", zap.Error(err))
		}
	}

	return tr, nil
}

// GetTranscription returns a transcription by ID.
func (tm *TranscriptionManager) GetTranscription(transcriptionID string) (*TranscriptionResult, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	tr, ok := tm.transcriptions[transcriptionID]
	if !ok {
		return nil, fmt.Errorf("transcription not found: %s", transcriptionID)
	}
	return tr, nil
}

// ListTranscriptions returns all transcriptions for a session.
func (tm *TranscriptionManager) ListTranscriptions(sessionID string) []*TranscriptionResult {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	var results []*TranscriptionResult
	for _, tr := range tm.transcriptions {
		if tr.SessionID == sessionID {
			results = append(results, tr)
		}
	}
	return results
}

// AddSegment adds a transcribed segment to a transcription result.
func (tm *TranscriptionManager) AddSegment(transcriptionID string, segment *TranscriptionSegment) error {
	tm.mu.Lock()
	tr, ok := tm.transcriptions[transcriptionID]
	if !ok {
		tm.mu.Unlock()
		return fmt.Errorf("transcription not found: %s", transcriptionID)
	}

	tr.Segments = append(tr.Segments, *segment)
	tr.DurationMs += int64((segment.EndTime - segment.StartTime) * 1000)

	// Publish segment update via NATS
	if tm.nats != nil {
		subject := fmt.Sprintf("tenant.*.webrtc.%s.transcription.segment", tr.SessionID)
		data, _ := json.Marshal(map[string]interface{}{
			"transcription_id": transcriptionID,
			"segment":          segment,
		})
		if err := tm.nats.Publish(subject, data); err != nil {
			tm.logger.Warn("failed to publish segment update", zap.Error(err))
		}
	}

	return nil
}

// processAudioChunk processes an incoming audio chunk for real-time transcription.
func (tm *TranscriptionManager) processAudioChunk(transcriptionID string, msg *nats.Msg) {
	tm.mu.RLock()
	tr, ok := tm.transcriptions[transcriptionID]
	tm.mu.RUnlock()

	if !ok {
		return
	}

	// In production, this would send the audio chunk to the STT provider.
	// For now, we simulate transcription with mock data.
	tr.Status = TranscriptionRunning

	segment := &TranscriptionSegment{
		ID:        fmt.Sprintf("seg-%s", uuid.New().String()[:6]),
		SessionID: tr.SessionID,
		Language:  tr.Language,
		StartTime: float64(len(tr.Segments)) * 5.0,
		EndTime:   float64(len(tr.Segments)+1) * 5.0,
		Text:      fmt.Sprintf("Transcribed segment %d", len(tr.Segments)+1),
		Sentiment: "neutral",
		Timestamp: time.Now(),
	}

	tm.AddSegment(transcriptionID, segment)
}

// ValidateProvider checks if a transcription provider is supported.
func ValidateProvider(provider string) error {
	switch TranscriptionProvider(provider) {
	case ProviderOpenAI, ProviderAzure, ProviderGoogle:
		return nil
	default:
		return fmt.Errorf("unsupported transcription provider: %s (valid: openai_whisper, azure_speech, google_speech)", provider)
	}
}
