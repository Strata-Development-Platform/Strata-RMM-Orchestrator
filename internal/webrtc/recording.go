package webrtc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/remote"
)

// RecordingManager manages WebRTC session recordings.
type RecordingManager struct {
	nats     *nats.Conn
	recorder *remote.Recorder
	logger   *zap.Logger
	mu       sync.RWMutex
	sessions map[string]*RecordingSession
}

// RecordingSession tracks an active recording.
type RecordingSession struct {
	ID            string            `json:"id"`
	WebRTCSession string            `json:"webrtc_session_id"`
	Recording     *remote.Recording `json:"recording,omitempty"`
	Format        string            `json:"format"` // "webm", "mkv"
	Bitrate       string            `json:"bitrate"`
	StartedAt     time.Time         `json:"started_at"`
	EndedAt       *time.Time        `json:"ended_at,omitempty"`
	FilePath      string            `json:"file_path,omitempty"`
	StorageKey    string            `json:"storage_key,omitempty"`
	SizeBytes     int64             `json:"size_bytes,omitempty"`
	DurationMs    int64             `json:"duration_ms,omitempty"`
}

// NewRecordingManager creates a new recording manager.
func NewRecordingManager(nc *nats.Conn, recorder *remote.Recorder, logger *zap.Logger) *RecordingManager {
	return &RecordingManager{
		nats:     nc,
		recorder: recorder,
		logger:   logger,
		sessions: make(map[string]*RecordingSession),
	}
}

// StartRecording starts recording a WebRTC session.
func (rm *RecordingManager) StartRecording(ctx context.Context, sessionID string, req *StartRecordingRequest) (*RecordingSession, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	recordingSession := &RecordingSession{
		ID:            fmt.Sprintf("rec-%s", uuid.New().String()[:8]),
		WebRTCSession: sessionID,
		Format:        req.Format,
		Bitrate:       req.Bitrate,
		StartedAt:     time.Now(),
	}

	rm.mu.Lock()
	rm.sessions[recordingSession.ID] = recordingSession
	rm.mu.Unlock()

	if rm.logger != nil {
		rm.logger.Info("started WebRTC recording",
			zap.String("recording_id", recordingSession.ID),
			zap.String("webrtc_session_id", sessionID),
			zap.String("format", req.Format),
			zap.String("bitrate", req.Bitrate),
		)
	}

	// Publish recording started event via NATS
	if rm.nats != nil {
		subject := fmt.Sprintf("tenant.*.webrtc.%s.recording.started", sessionID)
		data, _ := json.Marshal(map[string]interface{}{
			"recording_id": recordingSession.ID,
			"session_id":   sessionID,
			"format":       req.Format,
			"bitrate":      req.Bitrate,
			"started_at":   time.Now().Format(time.RFC3339),
		})
		if err := rm.nats.Publish(subject, data); err != nil {
			rm.logger.Warn("failed to publish recording started event", zap.Error(err))
		}
	}

	return recordingSession, nil
}

// StopRecording stops recording a WebRTC session.
func (rm *RecordingManager) StopRecording(recordingID string) (*RecordingSession, error) {
	rm.mu.Lock()
	rs, ok := rm.sessions[recordingID]
	if !ok {
		rm.mu.Unlock()
		return nil, fmt.Errorf("recording not found: %s", recordingID)
	}

	now := time.Now()
	rs.EndedAt = &now
	rs.DurationMs = now.Sub(rs.StartedAt).Milliseconds()
	rm.mu.Unlock()

	if rm.logger != nil {
		rm.logger.Info("stopped WebRTC recording",
			zap.String("recording_id", recordingID),
			zap.Duration("duration", now.Sub(rs.StartedAt)),
		)
	}

	// Publish recording stopped event via NATS
	if rm.nats != nil {
		subject := fmt.Sprintf("tenant.*.webrtc.%s.recording.stopped", rs.WebRTCSession)
		data, _ := json.Marshal(map[string]interface{}{
			"recording_id": recordingID,
			"session_id":   rs.WebRTCSession,
			"duration_ms":  rs.DurationMs,
			"file_path":    rs.FilePath,
			"size_bytes":   rs.SizeBytes,
		})
		if err := rm.nats.Publish(subject, data); err != nil {
			rm.logger.Warn("failed to publish recording stopped event", zap.Error(err))
		}
	}

	return rs, nil
}

// GetRecording returns a recording session by ID.
func (rm *RecordingManager) GetRecording(recordingID string) (*RecordingSession, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	rs, ok := rm.sessions[recordingID]
	if !ok {
		return nil, fmt.Errorf("recording not found: %s", recordingID)
	}
	return rs, nil
}

// ListRecordings returns all recordings for a WebRTC session.
func (rm *RecordingManager) ListRecordings(sessionID string) []*RecordingSession {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	var sessions []*RecordingSession
	for _, rs := range rm.sessions {
		if rs.WebRTCSession == sessionID {
			sessions = append(sessions, rs)
		}
	}
	return sessions
}

// SetRecordingFile sets the file path and metadata for a recording.
func (rm *RecordingManager) SetRecordingFile(recordingID, filePath, storageKey string, sizeBytes int64) error {
	rm.mu.Lock()
	rs, ok := rm.sessions[recordingID]
	if !ok {
		rm.mu.Unlock()
		return fmt.Errorf("recording not found: %s", recordingID)
	}
	rs.FilePath = filePath
	rs.StorageKey = storageKey
	rs.SizeBytes = sizeBytes
	rm.mu.Unlock()

	if rm.logger != nil {
		rm.logger.Info("set recording file",
			zap.String("recording_id", recordingID),
			zap.String("file_path", filePath),
			zap.Int64("size_bytes", sizeBytes),
		)
	}

	return nil
}

// GetRelayConfig returns the relay configuration for a session.
func (rm *RecordingManager) GetRelayConfig() *RelayConfig {
	{
		cfg := DefaultRelayConfig()
		return &cfg
	}
}
