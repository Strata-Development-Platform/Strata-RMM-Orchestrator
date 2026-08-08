package remote

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
	if !old.IsExpired() {
		t.Error("expected old binding to be expired")
	}
}

func TestRemoteSessionBinding_NotExpired(t *testing.T) {
	far := remoteSessionBinding{
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if far.IsExpired() {
		t.Error("expected recent binding to not be expired")
	}
}

// ---------------------------------------------------------------------------
// Frame — struct fields
// ---------------------------------------------------------------------------

func TestFrame_StructFields(t *testing.T) {
	frame := Frame{
		Data:      []byte{0x00, 0x01},
		Width:     1920,
		Height:    1080,
		Timestamp: 12345,
		Format:    "jpeg",
	}
	if len(frame.Data) != 2 {
		t.Error("Data length mismatch")
	}
	if frame.Width != 1920 {
		t.Errorf("Width = %d, want 1920", frame.Width)
	}
	if frame.Height != 1080 {
		t.Errorf("Height = %d, want 1080", frame.Height)
	}
	if frame.Timestamp != 12345 {
		t.Errorf("Timestamp = %d, want 12345", frame.Timestamp)
	}
	if frame.Format != "jpeg" {
		t.Errorf("Format = %q, want jpeg", frame.Format)
	}
}

func TestFrame_EmptyData(t *testing.T) {
	frame := Frame{Width: 0, Height: 0, Format: ""}
	if frame.Data == nil {
		t.Error("expected nil or empty Data, not nil pointer issue")
	}
	if frame.Width != 0 {
		t.Error("Width should be 0")
	}
}

// ---------------------------------------------------------------------------
// CaptureConfig — struct fields and defaults
// ---------------------------------------------------------------------------

func TestCaptureConfig_DefaultValues(t *testing.T) {
	cfg := CaptureConfig{}
	if cfg.Width != 0 {
		t.Error("Width default should be 0")
	}
	if cfg.FPS != 0 {
		t.Error("FPS default should be 0")
	}
	if cfg.Quality != 0 {
		t.Error("Quality default should be 0")
	}
	if cfg.Format != "" {
		t.Error("Format default should be empty")
	}
}

func TestCaptureConfig_FullValues(t *testing.T) {
	cfg := CaptureConfig{
		Width:   1280,
		Height:  720,
		FPS:     30,
		Quality: 80,
		Format:  "png",
	}
	if cfg.Width != 1280 {
		t.Errorf("Width = %d, want 1280", cfg.Width)
	}
	if cfg.Height != 720 {
		t.Errorf("Height = %d, want 720", cfg.Height)
	}
	if cfg.FPS != 30 {
		t.Errorf("FPS = %d, want 30", cfg.FPS)
	}
	if cfg.Quality != 80 {
		t.Errorf("Quality = %d, want 80", cfg.Quality)
	}
	if cfg.Format != "png" {
		t.Errorf("Format = %q, want png", cfg.Format)
	}
}

func TestCaptureConfig_QualityMax(t *testing.T) {
	cfg := CaptureConfig{Quality: 100}
	if cfg.Quality != 100 {
		t.Error("Quality should accept 100")
	}
}

// ---------------------------------------------------------------------------
// InputEvent — struct fields
// ---------------------------------------------------------------------------

func TestInputEvent_MouseMove(t *testing.T) {
	event := InputEvent{
		Type: InputMouseMove,
		X:    100.5,
		Y:    200.3,
	}
	if event.Type != InputMouseMove {
		t.Errorf("Type = %q, want %q", event.Type, InputMouseMove)
	}
	if event.X != 100.5 {
		t.Errorf("X = %f, want 100.5", event.X)
	}
	if event.Y != 200.3 {
		t.Errorf("Y = %f, want 200.3", event.Y)
	}
}

func TestInputEvent_MouseDown(t *testing.T) {
	event := InputEvent{
		Type:   InputMouseDown,
		Button: MouseButtons(1),
	}
	if event.Type != InputMouseDown {
		t.Errorf("Type = %q, want %q", event.Type, InputMouseDown)
	}
	if event.Button != 1 {
		t.Errorf("Button = %d, want 1", event.Button)
	}
}

func TestInputEvent_KeyPress(t *testing.T) {
	event := InputEvent{
		Type: InputKeyPress,
		Key:  "Escape",
	}
	if event.Type != InputKeyPress {
		t.Errorf("Type = %q, want %q", event.Type, InputKeyPress)
	}
	if event.Key != "Escape" {
		t.Errorf("Key = %q, want Escape", event.Key)
	}
}

func TestInputEvent_AllFields(t *testing.T) {
	event := InputEvent{
		Type:   InputMouseMove,
		X:      50.0,
		Y:      50.0,
		Button: 0,
		Key:    "",
	}
	if event.Type != InputMouseMove {
		t.Error("Type mismatch")
	}
	if event.X != 50.0 {
		t.Error("X mismatch")
	}
	if event.Y != 50.0 {
		t.Error("Y mismatch")
	}
}

// ---------------------------------------------------------------------------
// InputType — const validation
// ---------------------------------------------------------------------------

func TestInputType_Constants(t *testing.T) {
	if InputMouseMove != "mousemove" {
		t.Errorf("InputMouseMove = %q, want mousemove", InputMouseMove)
	}
	if InputMouseDown != "mousedown" {
		t.Errorf("InputMouseDown = %q, want mousedown", InputMouseDown)
	}
	if InputMouseUp != "mouseup" {
		t.Errorf("InputMouseUp = %q, want mouseup", InputMouseUp)
	}
	if InputKeyPress != "keydown" {
		t.Errorf("InputKeyPress = %q, want keydown", InputKeyPress)
	}
	if InputKeyRelease != "keyup" {
		t.Errorf("InputKeyRelease = %q, want keyup", InputKeyRelease)
	}
}

// ---------------------------------------------------------------------------
// Handler request/response behaviour
// ---------------------------------------------------------------------------

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
// Binding expiry edge cases
// ---------------------------------------------------------------------------

func TestRemoteSessionBinding_ExactExpiryTime(t *testing.T) {
	now := time.Unix(500, 0)
	binding := remoteSessionBinding{
		ExpiresAt: now,
	}
	// At exactly the expiry time, IsExpired should return true
	if !binding.IsExpiredAt(now) {
		t.Error("binding at exact expiry should be expired")
	}
}

func TestRemoteSessionBinding_ExpiryOneSecondBefore(t *testing.T) {
	now := time.Unix(500, 0)
	binding := remoteSessionBinding{
		ExpiresAt: now.Add(time.Second),
	}
	if binding.IsExpiredAt(now) {
		t.Error("binding one second before expiry should not be expired")
	}
}

// ---------------------------------------------------------------------------
// Frame JSON round-trip
// ---------------------------------------------------------------------------

func TestFrame_JSONRoundTrip(t *testing.T) {
	original := Frame{
		Data:      []byte{0xAB, 0xCD},
		Width:     800,
		Height:    600,
		Timestamp: 999,
		Format:    "webp",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Frame
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Format != original.Format {
		t.Errorf("Format mismatch: %q != %q", decoded.Format, original.Format)
	}
	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp mismatch: %d != %d", decoded.Timestamp, original.Timestamp)
	}
}

// ---------------------------------------------------------------------------
// CaptureConfig JSON round-trip
// ---------------------------------------------------------------------------

func TestCaptureConfig_JSONRoundTrip(t *testing.T) {
	original := CaptureConfig{
		Width:   640,
		Height:  480,
		FPS:     15,
		Quality: 70,
		Format:  "webp",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CaptureConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Width != original.Width {
		t.Errorf("Width mismatch: %d != %d", decoded.Width, original.Width)
	}
	if decoded.FPS != original.FPS {
		t.Errorf("FPS mismatch: %d != %d", decoded.FPS, original.FPS)
	}
	if decoded.Quality != original.Quality {
		t.Errorf("Quality mismatch: %d != %d", decoded.Quality, original.Quality)
	}
}

// ---------------------------------------------------------------------------
// InputEvent JSON round-trip
// ---------------------------------------------------------------------------

func TestInputEvent_JSONRoundTrip(t *testing.T) {
	original := InputEvent{
		Type:   InputMouseMove,
		X:      333.0,
		Y:      444.0,
		Button: 2,
		Key:    "",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded InputEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type mismatch: %q != %q", decoded.Type, original.Type)
	}
	if decoded.X != original.X {
		t.Errorf("X mismatch: %f != %f", decoded.X, original.X)
	}
	if decoded.Button != original.Button {
		t.Errorf("Button mismatch: %d != %d", decoded.Button, original.Button)
	}
}

// ---------------------------------------------------------------------------
// remoteSessionBinding JSON round-trip
// ---------------------------------------------------------------------------

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
