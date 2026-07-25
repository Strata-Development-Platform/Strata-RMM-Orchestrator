package remote

import (
	"testing"
	"time"
)

func TestRecordingResult(t *testing.T) {
	result := &RecordingResult{
		RecordingID:   "rec-1",
		SessionID:     "session-1",
		TenantID:      "tenant-1",
		DeviceID:      "device-1",
		StorageKey:    "recordings/tenant-1/device-1/rec-1.raw",
		SizeBytes:     1024,
		Duration:      time.Minute,
		Format:        FormatRaw,
		ChecksumSHA256: "abc123",
	}

	if result.RecordingID != "rec-1" {
		t.Errorf("recording id: got %s, want rec-1", result.RecordingID)
	}
	if result.Format != FormatRaw {
		t.Errorf("format: got %s, want raw", result.Format)
	}
	if result.SizeBytes != 1024 {
		t.Errorf("size: got %d, want 1024", result.SizeBytes)
	}
	if result.Duration != time.Minute {
		t.Errorf("duration: got %v, want 1m", result.Duration)
	}
}

func TestRecordingResultVideoFormat(t *testing.T) {
	result := &RecordingResult{
		Format: FormatMKV,
	}
	if result.Format != FormatMKV {
		t.Errorf("format: got %s, want mkv", result.Format)
	}
}

func TestDefaultRecorderConfig(t *testing.T) {
	cfg := DefaultRecorderConfig()
	if cfg.Format != FormatRaw {
		t.Errorf("default format: got %s, want raw", cfg.Format)
	}
	if cfg.RetentionDays != 90 {
		t.Errorf("default retention: got %d, want 90", cfg.RetentionDays)
	}
	if cfg.Width != 1920 {
		t.Errorf("default width: got %d, want 1920", cfg.Width)
	}
	if cfg.FPS != 30 {
		t.Errorf("default fps: got %d, want 30", cfg.FPS)
	}
}

func TestNewRecorder(t *testing.T) {
	recorder := NewRecorder(nil, nil)
	if recorder == nil {
		t.Fatal("expected recorder")
	}
}
