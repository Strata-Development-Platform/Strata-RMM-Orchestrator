package probe

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestNewIPMIClient verifies client creation.
func TestNewIPMIClient(t *testing.T) {
	target := IPMITarget{
		Host:     "192.168.1.100",
		Port:     623,
		Channel:  0,
		Username: "admin",
		Password: "secret",
		AuthType: "md5",
		Timeout:  10,
	}

	client := NewIPMIClient(target)
	if client == nil {
		t.Fatal("NewIPMIClient returned nil")
	}
	if client.target.Host != "192.168.1.100" {
		t.Errorf("Host = %q, want %q", client.target.Host, "192.168.1.100")
	}
	if client.timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want %v", client.timeout, 10*time.Second)
	}
}

// TestIPMIClientDefaults verifies default port.
func TestIPMIClientDefaults(t *testing.T) {
	target := IPMITarget{
		Host: "192.168.1.100",
	}

	client := NewIPMIClient(target)
	if client.target.Port != 623 {
		t.Errorf("Port = %d, want %d", client.target.Port, 623)
	}
}

// TestIPMIClientInvalidIP verifies invalid IP handling.
func TestIPMIClientInvalidIP(t *testing.T) {
	target := IPMITarget{
		Host: "invalid-ip",
		Port: 623,
	}

	client := NewIPMIClient(target)
	ctx := context.Background()
	err := client.Connect(ctx)
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

// TestBuildSensorRequest verifies sensor request payload structure.
func TestBuildSensorRequest(t *testing.T) {
	target := IPMITarget{
		Host:    "192.168.1.100",
		Port:    623,
		Channel: 0,
	}

	client := NewIPMIClient(target)
	payload := client.buildSensorRequest()

	if len(payload) == 0 {
		t.Fatal("payload should not be empty")
	}
	if payload[0] != 0x0a {
		t.Errorf("NetFn = 0x%02x, want 0x0a", payload[0])
	}
	if payload[7] != 0x2d {
		t.Errorf("Request = 0x%02x, want 0x2d", payload[7])
	}
}

// TestBuildChassisStatusRequest verifies chassis status request payload.
func TestBuildChassisStatusRequest(t *testing.T) {
	target := IPMITarget{
		Host:    "192.168.1.100",
		Port:    623,
		Channel: 0,
	}

	client := NewIPMIClient(target)
	payload := client.buildChassisStatusRequest()

	if len(payload) == 0 {
		t.Fatal("payload should not be empty")
	}
	if payload[0] != 0x0a {
		t.Errorf("NetFn = 0x%02x, want 0x0a", payload[0])
	}
	if payload[7] != 0x01 {
		t.Errorf("Request = 0x%02x, want 0x01", payload[7])
	}
}

// TestBuildFRURequest verifies FRU request payload.
func TestBuildFRURequest(t *testing.T) {
	target := IPMITarget{
		Host:    "192.168.1.100",
		Port:    623,
		Channel: 0,
	}

	client := NewIPMIClient(target)
	payload := client.buildFRURequest()

	if len(payload) == 0 {
		t.Fatal("payload should not be empty")
	}
	if payload[0] != 0x0a {
		t.Errorf("NetFn = 0x%02x, want 0x0a", payload[0])
	}
	if payload[7] != 0x39 {
		t.Errorf("Request = 0x%02x, want 0x39", payload[7])
	}
}

// TestParseSensorResponse verifies sensor response parsing.
func TestParseSensorResponse(t *testing.T) {
	target := IPMITarget{Host: "192.168.1.100"}
	client := NewIPMIClient(target)

	data := []byte{0x00, 0x12, 0x34, 0x01}
	result := client.parseSensorResponse(data)

	if result["completion_code"] != byte(0x00) {
		t.Errorf("completion_code = %v, want %v", result["completion_code"], byte(0x00))
	}
}

// TestParseSensorResponseShort verifies short response handling.
func TestParseSensorResponseShort(t *testing.T) {
	target := IPMITarget{Host: "192.168.1.100"}
	client := NewIPMIClient(target)

	data := []byte{0x00}
	result := client.parseSensorResponse(data)

	if _, ok := result["error"]; !ok {
		t.Error("expected error for short response")
	}
}

// TestParseChassisResponse verifies chassis status response parsing.
func TestParseChassisResponse(t *testing.T) {
	target := IPMITarget{Host: "192.168.1.100"}
	client := NewIPMIClient(target)

	// Power state: on (0x01), System state: running (0x01)
	data := []byte{0x00, 0x01, 0x01}
	result := client.parseChassisResponse(data)

	if result["completion_code"] != byte(0x00) {
		t.Errorf("completion_code = %v, want %v", result["completion_code"], byte(0x00))
	}
	if result["power_state"] != "on" {
		t.Errorf("power_state = %q, want %q", result["power_state"], "on")
	}
	if result["system_state"] != "running" {
		t.Errorf("system_state = %q, want %q", result["system_state"], "running")
	}
}

// TestParseChassisResponseShort verifies short response handling.
func TestParseChassisResponseShort(t *testing.T) {
	target := IPMITarget{Host: "192.168.1.100"}
	client := NewIPMIClient(target)

	data := []byte{0x00}
	result := client.parseChassisResponse(data)

	if _, ok := result["error"]; !ok {
		t.Error("expected error for short response")
	}
}

// TestParseFRUResponse verifies FRU response parsing.
func TestParseFRUResponse(t *testing.T) {
	target := IPMITarget{Host: "192.168.1.100"}
	client := NewIPMIClient(target)

	data := []byte{0x00, 0x12, 0x34, 0x56}
	result := client.parseFRUResponse(data)

	if result["completion_code"] != byte(0x00) {
		t.Errorf("completion_code = %v, want %v", result["completion_code"], byte(0x00))
	}
}

// TestParseFRUResponseShort verifies short response handling.
func TestParseFRUResponseShort(t *testing.T) {
	target := IPMITarget{Host: "192.168.1.100"}
	client := NewIPMIClient(target)

	data := []byte{}
	result := client.parseFRUResponse(data)

	if _, ok := result["error"]; !ok {
		t.Error("expected error for short response")
	}
}

// TestDecodePowerState verifies power state decoding.
func TestDecodePowerState(t *testing.T) {
	tests := []struct {
		byteVal byte
		want    string
	}{
		{0x01, "on"},
		{0x02, "off"},
		{0x04, "powering_on"},
		{0x08, "powering_off"},
		{0x00, "unknown"},
	}

	for _, tt := range tests {
		got := decodePowerState(tt.byteVal)
		if got != tt.want {
			t.Errorf("decodePowerState(0x%02x) = %q, want %q", tt.byteVal, got, tt.want)
		}
	}
}

// TestDecodeSystemState verifies system state decoding.
func TestDecodeSystemState(t *testing.T) {
	tests := []struct {
		byteVal byte
		want    string
	}{
		{0x01, "running"},
		{0x02, "booting"},
		{0x04, "in_diagnostic_mode"},
		{0x08, "paused"},
		{0x00, "unknown"},
	}

	for _, tt := range tests {
		got := decodeSystemState(tt.byteVal)
		if got != tt.want {
			t.Errorf("decodeSystemState(0x%02x) = %q, want %q", tt.byteVal, got, tt.want)
		}
	}
}

// TestDecodeSensorValue verifies sensor value decoding.
func TestDecodeSensorValue(t *testing.T) {
	tests := []struct {
		sensorType byte
		reading    byte
		want       string
	}{
		{0x01, 25, "25°C"},
		{0x02, 255, "255 RPM"},
		{0x04, 255, "0.26V"},
		{0x05, 255, "0.26A"},
		{0x20, 0x00, "safe"},
		{0x20, 0x01, "breach"},
		{0x20, 0x02, "unsafe"},
		{0x99, 0xFF, "0xff"},
	}

	for _, tt := range tests {
		got := DecodeSensorValue(tt.sensorType, tt.reading)
		if got != tt.want {
			t.Errorf("DecodeSensorValue(0x%02x, 0x%02x) = %q, want %q", tt.sensorType, tt.reading, got, tt.want)
		}
	}
}

// TestIPMIResultJSONRoundTrip verifies IPMIResult JSON serialization.
func TestIPMIResultJSONRoundTrip(t *testing.T) {
	result := IPMIResult{
		DeviceIP:  "192.168.1.100",
		Port:      623,
		Channel:   0,
		Status:    "ok",
		Timestamp: time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed IPMIResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if parsed.DeviceIP != result.DeviceIP {
		t.Errorf("DeviceIP = %q, want %q", parsed.DeviceIP, result.DeviceIP)
	}
	if parsed.Status != result.Status {
		t.Errorf("Status = %q, want %q", parsed.Status, result.Status)
	}
}

// TestIPMIClientClose verifies Close() doesn't panic.
func TestIPMIClientClose(t *testing.T) {
	target := IPMITarget{Host: "192.168.1.100"}
	client := NewIPMIClient(target)

	// Close should not panic even without Connect
	client.Close()
}

// TestIPMIClientChannel verifies channel configuration.
func TestIPMIClientChannel(t *testing.T) {
	target := IPMITarget{
		Host:    "192.168.1.100",
		Port:    623,
		Channel: 3,
	}

	client := NewIPMIClient(target)
	if client.target.Channel != 3 {
		t.Errorf("Channel = %d, want 3", client.target.Channel)
	}
}
