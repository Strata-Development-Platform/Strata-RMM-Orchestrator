package webrtc

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/remote"
)

func newTestRecordingManager(t *testing.T) *RecordingManager {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	recorder := remote.NewRecorder(nil, logger)
	return NewRecordingManager(nil, recorder, logger)
}

func TestStartRecording(t *testing.T) {
	rm := newTestRecordingManager(t)

	rs, err := rm.StartRecording(context.Background(), "webrtc-123", &StartRecordingRequest{
		Format:  "webm",
		Bitrate: "high",
	})
	if err != nil {
		t.Fatalf("failed to start recording: %v", err)
	}

	if rs.ID == "" {
		t.Fatal("expected non-empty recording ID")
	}
	if rs.WebRTCSession != "webrtc-123" {
		t.Fatalf("expected webrtc_session webrtc-123, got %s", rs.WebRTCSession)
	}
	if rs.Format != "webm" {
		t.Fatalf("expected format webm, got %s", rs.Format)
	}
}

func TestStartRecordingWithNATS(t *testing.T) {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS not available:", err)
	}
	defer nc.Close()

	logger, _ := zap.NewDevelopment()
	recorder := remote.NewRecorder(nil, logger)
	rm := NewRecordingManager(nc, recorder, logger)

	rs, err := rm.StartRecording(context.Background(), "webrtc-123", &StartRecordingRequest{
		Format:  "webm",
		Bitrate: "high",
	})
	if err != nil {
		t.Fatalf("failed to start recording: %v", err)
	}

	if rs.ID == "" {
		t.Fatal("expected non-empty recording ID")
	}
}

func TestStopRecording(t *testing.T) {
	rm := newTestRecordingManager(t)

	rs, _ := rm.StartRecording(context.Background(), "webrtc-123", &StartRecordingRequest{
		Format:  "webm",
		Bitrate: "high",
	})

	stopped, err := rm.StopRecording(rs.ID)
	if err != nil {
		t.Fatalf("failed to stop recording: %v", err)
	}

	if stopped.ID != rs.ID {
		t.Fatalf("expected ID %s, got %s", rs.ID, stopped.ID)
	}
	if stopped.DurationMs < 0 {
		t.Fatalf("expected non-negative duration, got %d", stopped.DurationMs)
	}
	if stopped.EndedAt == nil {
		t.Fatal("expected non-nil ended_at")
	}
}

func TestGetRecording(t *testing.T) {
	rm := newTestRecordingManager(t)

	rs, _ := rm.StartRecording(context.Background(), "webrtc-123", &StartRecordingRequest{
		Format:  "webm",
		Bitrate: "high",
	})

	found, err := rm.GetRecording(rs.ID)
	if err != nil {
		t.Fatalf("failed to get recording: %v", err)
	}
	if found.ID != rs.ID {
		t.Fatalf("expected ID %s, got %s", rs.ID, found.ID)
	}
}

func TestListRecordings(t *testing.T) {
	rm := newTestRecordingManager(t)

	rm.StartRecording(context.Background(), "webrtc-123", &StartRecordingRequest{Format: "webm"})
	rm.StartRecording(context.Background(), "webrtc-123", &StartRecordingRequest{Format: "mkv"})
	rm.StartRecording(context.Background(), "webrtc-456", &StartRecordingRequest{Format: "webm"})

	recordings := rm.ListRecordings("webrtc-123")
	if len(recordings) != 2 {
		t.Fatalf("expected 2 recordings for webrtc-123, got %d", len(recordings))
	}
}

func TestSetRecordingFile(t *testing.T) {
	rm := newTestRecordingManager(t)

	rs, _ := rm.StartRecording(context.Background(), "webrtc-123", &StartRecordingRequest{
		Format:  "webm",
		Bitrate: "high",
	})

	if err := rm.SetRecordingFile(rs.ID, "/tmp/recording.webm", "s3://bucket/rec.webm", 1048576); err != nil {
		t.Fatalf("failed to set recording file: %v", err)
	}

	found, _ := rm.GetRecording(rs.ID)
	if found.FilePath != "/tmp/recording.webm" {
		t.Fatalf("expected file_path /tmp/recording.webm, got %s", found.FilePath)
	}
	if found.StorageKey != "s3://bucket/rec.webm" {
		t.Fatalf("expected storage_key s3://bucket/rec.webm, got %s", found.StorageKey)
	}
	if found.SizeBytes != 1048576 {
		t.Fatalf("expected size 1048576, got %d", found.SizeBytes)
	}
}

func TestRecordingGetRelayConfig(t *testing.T) {
	rm := newTestRecordingManager(t)

	config := rm.GetRelayConfig()
	if config == nil {
		t.Fatal("expected non-nil relay config")
	}
	if config.Provider != RelayGoogle {
		t.Fatalf("expected provider google, got %s", config.Provider)
	}
}

func TestRecordingWithNATS(t *testing.T) {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS not available:", err)
	}
	defer nc.Close()

	logger, _ := zap.NewDevelopment()
	recorder := remote.NewRecorder(nil, logger)
	rm := NewRecordingManager(nc, recorder, logger)

	rs, err := rm.StartRecording(context.Background(), "webrtc-123", &StartRecordingRequest{
		Format:  "webm",
		Bitrate: "high",
	})
	if err != nil {
		t.Fatalf("failed to start recording: %v", err)
	}

	if rs.ID == "" {
		t.Fatal("expected non-empty recording ID")
	}

	_, err = rm.StopRecording(rs.ID)
	if err != nil {
		t.Fatalf("failed to stop recording: %v", err)
	}
}

func TestRecordingEndToEnd(t *testing.T) {
	rm := newTestRecordingManager(t)

	// Start recording
	rs, err := rm.StartRecording(context.Background(), "webrtc-123", &StartRecordingRequest{
		Format:  "mkv",
		Bitrate: "medium",
	})
	if err != nil {
		t.Fatalf("failed to start recording: %v", err)
	}

	// Set recording file
	if err := rm.SetRecordingFile(rs.ID, "/tmp/test.mkv", "s3://bucket/test.mkv", 5242880); err != nil {
		t.Fatalf("failed to set recording file: %v", err)
	}

	// Stop recording
	stopped, err := rm.StopRecording(rs.ID)
	if err != nil {
		t.Fatalf("failed to stop recording: %v", err)
	}

	if stopped.FilePath != "/tmp/test.mkv" {
		t.Fatalf("expected file_path /tmp/test.mkv, got %s", stopped.FilePath)
	}
	if stopped.DurationMs < 0 {
		t.Fatalf("expected non-negative duration, got %d", stopped.DurationMs)
	}
}
