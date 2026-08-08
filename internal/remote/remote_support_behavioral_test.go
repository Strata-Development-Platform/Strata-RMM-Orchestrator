package remote

import (
	"encoding/json"
	"testing"
)

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
	if len(frame.Data) != 0 {
		t.Errorf("expected empty Data, got len=%d", len(frame.Data))
	}
	if frame.Width != 0 {
		t.Error("Width should be 0")
	}
}

func TestFrame_ZeroValues(t *testing.T) {
	frame := Frame{}
	if frame.Width != 0 {
		t.Error("Width default should be 0")
	}
	if frame.Height != 0 {
		t.Error("Height default should be 0")
	}
	if frame.Timestamp != 0 {
		t.Error("Timestamp default should be 0")
	}
	if frame.Format != "" {
		t.Error("Format default should be empty")
	}
}

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
	if decoded.Width != original.Width {
		t.Errorf("Width mismatch: %d != %d", decoded.Width, original.Width)
	}
	if decoded.Height != original.Height {
		t.Errorf("Height mismatch: %d != %d", decoded.Height, original.Height)
	}
}

func TestFrame_LargeFrame(t *testing.T) {
	frame := Frame{
		Width:  3840,
		Height: 2160,
		Format: "jpeg",
		Data:   make([]byte, 1000),
	}
	if frame.Width != 3840 {
		t.Error("Width mismatch")
	}
	if frame.Height != 2160 {
		t.Error("Height mismatch")
	}
	if len(frame.Data) != 1000 {
		t.Error("Data length mismatch")
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

func TestCaptureConfig_QualityZero(t *testing.T) {
	cfg := CaptureConfig{Quality: 0}
	if cfg.Quality != 0 {
		t.Error("Quality should accept 0")
	}
}

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
	if decoded.Height != original.Height {
		t.Errorf("Height mismatch: %d != %d", decoded.Height, original.Height)
	}
	if decoded.FPS != original.FPS {
		t.Errorf("FPS mismatch: %d != %d", decoded.FPS, original.FPS)
	}
	if decoded.Quality != original.Quality {
		t.Errorf("Quality mismatch: %d != %d", decoded.Quality, original.Quality)
	}
	if decoded.Format != original.Format {
		t.Errorf("Format mismatch: %q != %q", decoded.Format, original.Format)
	}
}

func TestCaptureConfig_EmptyJSON(t *testing.T) {
	var decoded CaptureConfig
	if err := json.Unmarshal([]byte("{}"), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Width != 0 || decoded.Height != 0 || decoded.FPS != 0 {
		t.Error("empty JSON should yield zero values")
	}
}

func TestCaptureConfig_JSONRoundTripZero(t *testing.T) {
	original := CaptureConfig{}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CaptureConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Width != 0 || decoded.Height != 0 || decoded.FPS != 0 {
		t.Error("zero values should round-trip")
	}
}

func TestCaptureConfig_JSONRoundTripHighFPS(t *testing.T) {
	original := CaptureConfig{FPS: 60, Width: 1920, Height: 1080}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CaptureConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.FPS != 60 {
		t.Errorf("FPS mismatch: %d != %d", decoded.FPS, 60)
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

func TestInputEvent_KeyDown(t *testing.T) {
	event := InputEvent{
		Type: InputKeyDown,
		Key:  "Escape",
	}
	if event.Type != InputKeyDown {
		t.Errorf("Type = %q, want %q", event.Type, InputKeyDown)
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

func TestInputEvent_KeyboardRelease(t *testing.T) {
	event := InputEvent{
		Type: InputKeyUp,
		Key:  "Enter",
	}
	if event.Type != InputKeyUp {
		t.Errorf("Type = %q, want %q", event.Type, InputKeyUp)
	}
	if event.Key != "Enter" {
		t.Errorf("Key = %q, want Enter", event.Key)
	}
}

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

func TestInputEvent_KeyPressRoundTrip(t *testing.T) {
	original := InputEvent{
		Type: InputKeyDown,
		Key:  "Tab",
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
	if decoded.Key != original.Key {
		t.Errorf("Key mismatch: %q != %q", decoded.Key, original.Key)
	}
}

func TestInputEvent_ZeroValues(t *testing.T) {
	event := InputEvent{}
	if event.Type != "" {
		t.Error("Type default should be empty")
	}
	if event.X != 0 {
		t.Error("X default should be 0")
	}
	if event.Button != 0 {
		t.Error("Button default should be 0")
	}
}

func TestInputEvent_JSONRoundTripZero(t *testing.T) {
	original := InputEvent{}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded InputEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Type != "" || decoded.X != 0 || decoded.Button != 0 {
		t.Error("zero values should round-trip")
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
	if InputKeyDown != "keydown" {
		t.Errorf("InputKeyDown = %q, want keydown", InputKeyDown)
	}
	if InputKeyUp != "keyup" {
		t.Errorf("InputKeyUp = %q, want keyup", InputKeyUp)
	}
}

func TestInputType_AllDefined(t *testing.T) {
	types := []InputType{InputMouseMove, InputMouseDown, InputMouseUp, InputKeyDown, InputKeyUp}
	for i, ty := range types {
		if ty == "" {
			t.Errorf("InputType[%d] is empty", i)
		}
	}
}

// ---------------------------------------------------------------------------
// MouseButtons — struct/const validation
// ---------------------------------------------------------------------------

func TestMouseButtons_Default(t *testing.T) {
	var mb MouseButtons
	if mb != 0 {
		t.Errorf("MouseButtons default should be 0, got %d", mb)
	}
}

func TestMouseButtons_LeftButton(t *testing.T) {
	mb := MouseButtons(1)
	if mb != 1 {
		t.Errorf("Left button should be 1, got %d", mb)
	}
}

func TestMouseButtons_RightButton(t *testing.T) {
	mb := MouseButtons(2)
	if mb != 2 {
		t.Errorf("Right button should be 2, got %d", mb)
	}
}

func TestMouseButtons_MiddleButton(t *testing.T) {
	mb := MouseButtons(4)
	if mb != 4 {
		t.Errorf("Middle button should be 4, got %d", mb)
	}
}

// ---------------------------------------------------------------------------
// ModifierKeys — const validation
// ---------------------------------------------------------------------------

func TestModifierKeys_Default(t *testing.T) {
	var mk ModifierKeys
	if mk != 0 {
		t.Errorf("ModifierKeys default should be 0, got %d", mk)
	}
}

func TestModifierKeys_Values(t *testing.T) {
	// Verify the modifier constants exist and are non-negative
	if ModifierShift < 0 {
		t.Error("ModifierShift should be non-negative")
	}
	if ModifierCtrl < 0 {
		t.Error("ModifierCtrl should be non-negative")
	}
	if ModifierAlt < 0 {
		t.Error("ModifierAlt should be non-negative")
	}
	if ModifierMeta < 0 {
		t.Error("ModifierMeta should be non-negative")
	}
}
