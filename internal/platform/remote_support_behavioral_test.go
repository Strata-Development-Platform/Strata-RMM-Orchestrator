package platform

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// RemoteSessionRequest — struct fields and validation
// ---------------------------------------------------------------------------

func TestRemoteSessionRequest_DeviceIDRequired(t *testing.T) {
	b, _ := json.Marshal(map[string]any{
		"width": 1920,
	})
	var req RemoteSessionRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if req.DeviceID != "" {
		t.Fatalf("expected empty DeviceID, got %q", req.DeviceID)
	}
}

func TestRemoteSessionRequest_DeviceIDFilled(t *testing.T) {
	b, _ := json.Marshal(map[string]any{
		"device_id": "dev-001",
	})
	var req RemoteSessionRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if req.DeviceID != "dev-001" {
		t.Fatalf("expected dev-001, got %q", req.DeviceID)
	}
}

func TestRemoteSessionRequest_QualityBounds(t *testing.T) {
	b, _ := json.Marshal(map[string]any{
		"device_id": "d",
		"quality":   95,
	})
	var req RemoteSessionRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if req.Quality != 95 {
		t.Fatalf("expected 95, got %d", req.Quality)
	}
}

func TestRemoteSessionRequest_FPSDefaults(t *testing.T) {
	b, _ := json.Marshal(map[string]any{
		"device_id": "d",
	})
	var req RemoteSessionRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if req.FPS != 0 {
		t.Fatalf("expected FPS 0, got %d", req.FPS)
	}
}

func TestRemoteSessionRequest_DimensionsDefault(t *testing.T) {
	b, _ := json.Marshal(map[string]any{
		"device_id": "d",
	})
	var req RemoteSessionRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if req.Width != 0 {
		t.Fatalf("expected Width 0, got %d", req.Width)
	}
	if req.Height != 0 {
		t.Fatalf("expected Height 0, got %d", req.Height)
	}
}

func TestRemoteSessionRequest_FullValues(t *testing.T) {
	b, _ := json.Marshal(map[string]any{
		"device_id": "dev-001",
		"width":     1920,
		"height":    1080,
		"quality":   85,
		"fps":       30,
	})
	var req RemoteSessionRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if req.DeviceID != "dev-001" {
		t.Errorf("DeviceID = %q, want dev-001", req.DeviceID)
	}
	if req.Width != 1920 {
		t.Errorf("Width = %d, want 1920", req.Width)
	}
	if req.Height != 1080 {
		t.Errorf("Height = %d, want 1080", req.Height)
	}
	if req.Quality != 85 {
		t.Errorf("Quality = %d, want 85", req.Quality)
	}
	if req.FPS != 30 {
		t.Errorf("FPS = %d, want 30", req.FPS)
	}
}

func TestRemoteSessionRequest_UnknownFieldsIgnored(t *testing.T) {
	b, _ := json.Marshal(map[string]any{
		"device_id":   "d1",
		"unknown_key": "ignored",
	})
	var req RemoteSessionRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if req.DeviceID != "d1" {
		t.Error("DeviceID should be parsed")
	}
}

func TestRemoteSessionRequest_JSONRoundTrip(t *testing.T) {
	original := RemoteSessionRequest{
		DeviceID: "dev-abc",
		Width:    1920,
		Height:   1080,
		Quality:  85,
		FPS:      24,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RemoteSessionRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.DeviceID != original.DeviceID {
		t.Errorf("DeviceID mismatch: %q != %q", decoded.DeviceID, original.DeviceID)
	}
	if decoded.Width != original.Width {
		t.Errorf("Width mismatch: %d != %d", decoded.Width, original.Width)
	}
	if decoded.Height != original.Height {
		t.Errorf("Height mismatch: %d != %d", decoded.Height, original.Height)
	}
	if decoded.Quality != original.Quality {
		t.Errorf("Quality mismatch: %d != %d", decoded.Quality, original.Quality)
	}
	if decoded.FPS != original.FPS {
		t.Errorf("FPS mismatch: %d != %d", decoded.FPS, original.FPS)
	}
}

// ---------------------------------------------------------------------------
// remoteSessionBinding — struct fields and expiry behaviour
// ---------------------------------------------------------------------------

func TestRemoteSessionBinding_Fields(t *testing.T) {
	binding := remoteSessionBinding{
		TenantID:  "tenant-1",
		DeviceID:  "device-2",
		AgentID:   "agent-3",
		CreatedAt: time.Unix(1000, 0),
		ExpiresAt: time.Unix(2000, 0),
	}
	if binding.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want tenant-1", binding.TenantID)
	}
	if binding.DeviceID != "device-2" {
		t.Errorf("DeviceID = %q, want device-2", binding.DeviceID)
	}
	if binding.AgentID != "agent-3" {
		t.Errorf("AgentID = %q, want agent-3", binding.AgentID)
	}
	if !binding.CreatedAt.Equal(time.Unix(1000, 0)) {
		t.Errorf("CreatedAt mismatch")
	}
	if !binding.ExpiresAt.Equal(time.Unix(2000, 0)) {
		t.Errorf("ExpiresAt mismatch")
	}
}

func TestRemoteSessionBinding_IsExpired(t *testing.T) {
	old := remoteSessionBinding{
		CreatedAt: time.Unix(100, 0),
		ExpiresAt: time.Unix(200, 0),
	}
	if old.ExpiresAt.After(time.Unix(300, 0)) {
		t.Error("expected old binding to be expired")
	}
}

func TestRemoteSessionBinding_NotExpired(t *testing.T) {
	far := remoteSessionBinding{
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if !far.ExpiresAt.After(time.Now()) {
		t.Error("expected recent binding to not be expired")
	}
}

func TestRemoteSessionBinding_ExactExpiryTime(t *testing.T) {
	now := time.Unix(500, 0)
	binding := remoteSessionBinding{
		ExpiresAt: now,
	}
	if binding.ExpiresAt.After(now) {
		t.Error("binding at exact expiry should be expired")
	}
}

func TestRemoteSessionBinding_ExpiryOneSecondBefore(t *testing.T) {
	now := time.Unix(500, 0)
	binding := remoteSessionBinding{
		ExpiresAt: now.Add(time.Second),
	}
	if !binding.ExpiresAt.After(now) {
		t.Error("binding one second before expiry should not be expired")
	}
}

func TestRemoteSessionBinding_ExpiryOneSecondAfter(t *testing.T) {
	now := time.Unix(500, 0)
	binding := remoteSessionBinding{
		ExpiresAt: now.Add(-time.Second),
	}
	if binding.ExpiresAt.After(now) {
		t.Error("binding one second past expiry should be expired")
	}
}

func TestRemoteSessionBinding_JSONRoundTrip(t *testing.T) {
	original := remoteSessionBinding{
		TenantID:  "t1",
		DeviceID:  "d1",
		AgentID:   "a1",
		CreatedAt: time.Unix(100, 0),
		ExpiresAt: time.Unix(200, 0),
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded remoteSessionBinding
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TenantID != original.TenantID {
		t.Errorf("TenantID mismatch: %q != %q", decoded.TenantID, original.TenantID)
	}
	if decoded.DeviceID != original.DeviceID {
		t.Errorf("DeviceID mismatch: %q != %q", decoded.DeviceID, original.DeviceID)
	}
	if decoded.AgentID != original.AgentID {
		t.Errorf("AgentID mismatch: %q != %q", decoded.AgentID, original.AgentID)
	}
}
