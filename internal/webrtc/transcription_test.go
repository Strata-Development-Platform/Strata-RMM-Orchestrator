package webrtc

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

func newTestTranscriptionManager(t *testing.T) *TranscriptionManager {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return NewTranscriptionManager(nil, logger)
}

func TestStartTranscription(t *testing.T) {
	tm := newTestTranscriptionManager(t)

	tr, err := tm.StartTranscription("webrtc-123", "en", "openai_whisper")
	if err != nil {
		t.Fatalf("failed to start transcription: %v", err)
	}

	if tr.ID == "" {
		t.Fatal("expected non-empty transcription ID")
	}
	if tr.SessionID != "webrtc-123" {
		t.Fatalf("expected session_id webrtc-123, got %s", tr.SessionID)
	}
	if tr.Language != "en" {
		t.Fatalf("expected language en, got %s", tr.Language)
	}
	if tr.Provider != ProviderOpenAI {
		t.Fatalf("expected provider openai_whisper, got %s", tr.Provider)
	}
	if tr.Status != TranscriptionPending {
		t.Fatalf("expected status pending, got %s", tr.Status)
	}
}

func TestStartTranscriptionWithNATS(t *testing.T) {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS not available:", err)
	}
	defer nc.Close()

	logger, _ := zap.NewDevelopment()
	tm := NewTranscriptionManager(nc, logger)

	tr, err := tm.StartTranscription("webrtc-123", "en", "openai_whisper")
	if err != nil {
		t.Fatalf("failed to start transcription: %v", err)
	}

	if tr.ID == "" {
		t.Fatal("expected non-empty transcription ID")
	}
}

func TestStartTranscriptionMissingLanguage(t *testing.T) {
	tm := newTestTranscriptionManager(t)

	_, err := tm.StartTranscription("webrtc-123", "", "openai_whisper")
	if err == nil {
		t.Fatal("expected error for missing language")
	}
}

func TestStartTranscriptionMissingProvider(t *testing.T) {
	tm := newTestTranscriptionManager(t)

	_, err := tm.StartTranscription("webrtc-123", "en", "")
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestStopTranscription(t *testing.T) {
	tm := newTestTranscriptionManager(t)

	tr, _ := tm.StartTranscription("webrtc-123", "en", "openai_whisper")

	stopped, err := tm.StopTranscription(tr.ID)
	if err != nil {
		t.Fatalf("failed to stop transcription: %v", err)
	}

	if stopped.Status != TranscriptionCompleted {
		t.Fatalf("expected status completed, got %s", stopped.Status)
	}
	if stopped.CompletedAt == nil {
		t.Fatal("expected non-nil completed_at")
	}
	if stopped.TranscriptText == "" && len(stopped.Segments) == 0 {
		t.Logf("no segments added, transcript text is empty (expected)")
	}
}

func TestGetTranscription(t *testing.T) {
	tm := newTestTranscriptionManager(t)

	tr, _ := tm.StartTranscription("webrtc-123", "en", "openai_whisper")

	found, err := tm.GetTranscription(tr.ID)
	if err != nil {
		t.Fatalf("failed to get transcription: %v", err)
	}
	if found.ID != tr.ID {
		t.Fatalf("expected ID %s, got %s", tr.ID, found.ID)
	}
}

func TestListTranscriptions(t *testing.T) {
	tm := newTestTranscriptionManager(t)

	tm.StartTranscription("webrtc-123", "en", "openai_whisper")
	tm.StartTranscription("webrtc-123", "es", "azure_speech")
	tm.StartTranscription("webrtc-456", "en", "openai_whisper")

	transcriptions := tm.ListTranscriptions("webrtc-123")
	if len(transcriptions) != 2 {
		t.Fatalf("expected 2 transcriptions for webrtc-123, got %d", len(transcriptions))
	}
}

func TestAddSegment(t *testing.T) {
	tm := newTestTranscriptionManager(t)

	tr, _ := tm.StartTranscription("webrtc-123", "en", "openai_whisper")

	segment := &TranscriptionSegment{
		ID:        "seg-001",
		SessionID: "webrtc-123",
		Language:  "en",
		StartTime: 0.0,
		EndTime:   5.0,
		Text:      "Hello, welcome to the remote support session.",
		Speaker:   "agent",
		Timestamp: time.Now(),
	}

	if err := tm.AddSegment(tr.ID, segment); err != nil {
		t.Fatalf("failed to add segment: %v", err)
	}

	found, _ := tm.GetTranscription(tr.ID)
	if len(found.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(found.Segments))
	}
	if found.Segments[0].Text != "Hello, welcome to the remote support session." {
		t.Fatalf("unexpected segment text: %s", found.Segments[0].Text)
	}
}

func TestProcessAudioChunk(t *testing.T) {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS not available:", err)
	}
	defer nc.Close()

	logger, _ := zap.NewDevelopment()
	tm := NewTranscriptionManager(nc, logger)

	tr, _ := tm.StartTranscription("webrtc-123", "en", "openai_whisper")

	subject := "tenant.t1.webrtc.webrtc-123.audio"
	data, _ := json.Marshal([]byte{0x01, 0x02, 0x03, 0x04})
	if err := nc.Publish(subject, data); err != nil {
		t.Fatalf("failed to publish audio chunk: %v", err)
	}

	tm.processAudioChunk(tr.ID, &nats.Msg{Subject: subject, Data: data})

	found, _ := tm.GetTranscription(tr.ID)
	if found.Status != TranscriptionRunning {
		t.Fatalf("expected status running, got %s", found.Status)
	}
}

func TestTranscriptionWithMultipleProviders(t *testing.T) {
	tm := newTestTranscriptionManager(t)

	providers := []string{"openai_whisper", "azure_speech", "google_speech"}
	for _, provider := range providers {
		tr, err := tm.StartTranscription("webrtc-123", "en", provider)
		if err != nil {
			t.Fatalf("failed to start transcription for %s: %v", provider, err)
		}

		_, err = tm.StopTranscription(tr.ID)
		if err != nil {
			t.Fatalf("failed to stop transcription for %s: %v", provider, err)
		}
	}
}

func TestTranscriptionEndToEnd(t *testing.T) {
	tm := newTestTranscriptionManager(t)

	tr, err := tm.StartTranscription("webrtc-123", "en", "openai_whisper")
	if err != nil {
		t.Fatalf("failed to start transcription: %v", err)
	}

	for i := 0; i < 3; i++ {
		segment := &TranscriptionSegment{
			ID:        fmt.Sprintf("seg-%d", i),
			SessionID: "webrtc-123",
			Language:  "en",
			StartTime: float64(i) * 5.0,
			EndTime:   float64(i+1) * 5.0,
			Text:      fmt.Sprintf("Transcribed segment %d", i+1),
			Speaker:   "agent",
			Timestamp: time.Now(),
		}
		if err := tm.AddSegment(tr.ID, segment); err != nil {
			t.Fatalf("failed to add segment: %v", err)
		}
	}

	stopped, err := tm.StopTranscription(tr.ID)
	if err != nil {
		t.Fatalf("failed to stop transcription: %v", err)
	}

	if stopped.Status != TranscriptionCompleted {
		t.Fatalf("expected status completed, got %s", stopped.Status)
	}
	if len(stopped.Segments) < 3 {
		t.Fatalf("expected at least 3 segments, got %d", len(stopped.Segments))
	}
	if stopped.DurationMs <= 0 {
		t.Fatalf("expected positive duration, got %d", stopped.DurationMs)
	}
}
