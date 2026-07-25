package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
)

func TestRecorderRecordRaw(t *testing.T) {
	backend := storage.NewMockBackend()
	logger, _ := zap.NewDevelopment()
	recorder := NewRecorder(backend, logger)

	session := &TunnelSession{
		ID:       uuid.New().String(),
		TenantID: "tenant-1",
		DeviceID: "device-1",
		Protocol: ProtocolSSH,
	}

	sessionRec, err := recorder.RecordRaw(context.Background(), session)
	if err != nil {
		t.Fatalf("RecordRaw: %v", err)
	}

	testData := "this is test session data for recording"
	h := sha256.Sum256([]byte(testData))
	expectedChecksum := hex.EncodeToString(h[:])

	_, err = sessionRec.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	result := sessionRec.Stop()
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if result.RecordingID == "" {
		t.Error("expected non-empty recording ID")
	}
	if result.SessionID != session.ID {
		t.Errorf("session id: got %s, want %s", result.SessionID, session.ID)
	}
	if result.TenantID != session.TenantID {
		t.Errorf("tenant id: got %s, want %s", result.TenantID, session.TenantID)
	}
	if result.DeviceID != session.DeviceID {
		t.Errorf("device id: got %s, want %s", result.DeviceID, session.DeviceID)
	}
	if result.StorageKey == "" {
		t.Error("expected non-empty storage key")
	}
	if result.SizeBytes != int64(len(testData)) {
		t.Errorf("size: got %d, want %d", result.SizeBytes, len(testData))
	}
	if result.ChecksumSHA256 != expectedChecksum {
		t.Errorf("checksum: got %s, want %s", result.ChecksumSHA256, expectedChecksum)
	}
	if result.Format != FormatRaw {
		t.Errorf("format: got %s, want raw", result.Format)
	}

	// Verify the data was actually uploaded
	rc, err := backend.Download(context.Background(), result.StorageKey)
	if err != nil {
		t.Fatalf("Download recorded data: %v", err)
	}
	defer rc.Close()

	uploadedData, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Read uploaded data: %v", err)
	}
	if string(uploadedData) != testData {
		t.Errorf("uploaded data mismatch: got %q, want %q", string(uploadedData), testData)
	}

	// Verify calls
	calls := backend.Calls()
	if len(calls) < 2 {
		t.Errorf("expected at least 2 calls, got %d: %v", len(calls), calls)
	}
}

func TestRecorderConcurrentWrites(t *testing.T) {
	backend := storage.NewMockBackend()
	logger, _ := zap.NewDevelopment()
	recorder := NewRecorder(backend, logger)

	session := &TunnelSession{
		ID:       uuid.New().String(),
		TenantID: "tenant-1",
		DeviceID: "device-1",
	}

	sessionRec, err := recorder.RecordRaw(context.Background(), session)
	if err != nil {
		t.Fatalf("RecordRaw: %v", err)
	}

	// Concurrent writes
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			sessionRec.Write([]byte("data"))
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 100; i++ {
			sessionRec.Write([]byte("data"))
		}
		done <- struct{}{}
	}()

	<-done
	<-done

	result := sessionRec.Stop()
	if result == nil {
		t.Fatal("expected result from concurrent writes")
	}
	if result.SizeBytes <= 0 {
		t.Errorf("expected positive size, got %d", result.SizeBytes)
	}
}

func TestRecorderMultipleSessions(t *testing.T) {
	backend := storage.NewMockBackend()
	logger, _ := zap.NewDevelopment()
	recorder := NewRecorder(backend, logger)

	for i := 0; i < 5; i++ {
		session := &TunnelSession{
			ID:       uuid.New().String(),
			TenantID: "tenant-1",
			DeviceID: "device-1",
		}

		sessionRec, err := recorder.RecordRaw(context.Background(), session)
		if err != nil {
			t.Fatalf("RecordRaw session %d: %v", i, err)
		}

		sessionRec.Write([]byte("session data"))
		result := sessionRec.Stop()
		if result == nil || result.RecordingID == "" {
			t.Errorf("session %d: expected valid result", i)
		}
	}
}

func TestRecorderEmptyRecording(t *testing.T) {
	backend := storage.NewMockBackend()
	logger, _ := zap.NewDevelopment()
	recorder := NewRecorder(backend, logger)

	session := &TunnelSession{
		ID:       uuid.New().String(),
		TenantID: "tenant-1",
		DeviceID: "device-1",
	}

	sessionRec, err := recorder.RecordRaw(context.Background(), session)
	if err != nil {
		t.Fatalf("RecordRaw: %v", err)
	}

	result := sessionRec.Stop()
	if result == nil {
		t.Fatal("expected result for empty recording")
	}
	if result.SizeBytes != 0 {
		t.Errorf("expected 0 size for empty recording, got %d", result.SizeBytes)
	}
}
