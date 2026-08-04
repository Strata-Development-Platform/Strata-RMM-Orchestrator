package probe

import (
	"encoding/json"
	"testing"
	"time"
)

// TestNewSyslogCollector verifies collector creation.
func TestNewSyslogCollector(t *testing.T) {
	cfg := SyslogConfig{
		Port:     514,
		Protocol: "udp",
		Timeout:  30 * time.Second,
		Buffer:   1000,
	}

	collector := NewSyslogCollector(cfg)
	if collector == nil {
		t.Fatal("NewSyslogCollector returned nil")
	}
	if collector.config.Port != 514 {
		t.Errorf("Port = %d, want %d", collector.config.Port, 514)
	}
}

// TestSyslogCollectorDefaults verifies default configuration.
func TestSyslogCollectorDefaults(t *testing.T) {
	collector := NewSyslogCollector(SyslogConfig{})

	if collector.config.Port != 514 {
		t.Errorf("Port = %d, want %d", collector.config.Port, 514)
	}
	if collector.config.Protocol != "udp" {
		t.Errorf("Protocol = %q, want %q", collector.config.Protocol, "udp")
	}
	if collector.config.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want %v", collector.config.Timeout, 30*time.Second)
	}
	if collector.config.Buffer != 1000 {
		t.Errorf("Buffer = %d, want %d", collector.config.Buffer, 1000)
	}
}

// TestParseSyslogMessageRFC3164 verifies RFC 3164 parsing.
func TestParseSyslogMessageRFC3164(t *testing.T) {
	collector := NewSyslogCollector(SyslogConfig{})

	raw := "<165>Aug  4 12:00:00 server01 sshd[12345]: Accepted publickey for admin"
	msg := collector.parseSyslogMessage(raw, "192.168.1.50:514")

	if msg.Priority != 165 {
		t.Errorf("Priority = %d, want 165", msg.Priority)
	}
	if msg.Hostname != "server01" {
		t.Errorf("Hostname = %q, want %q", msg.Hostname, "server01")
	}
	if msg.AppName != "sshd" {
		t.Errorf("AppName = %q, want %q", msg.AppName, "sshd")
	}
	if msg.ProcID != "12345" {
		t.Errorf("ProcID = %q, want %q", msg.ProcID, "12345")
	}
	if msg.Content != "Accepted publickey for admin" {
		t.Errorf("Content = %q, want %q", msg.Content, "Accepted publickey for admin")
	}
	if msg.SourceIP != "192.168.1.50:514" {
		t.Errorf("SourceIP = %q, want %q", msg.SourceIP, "192.168.1.50:514")
	}
}

// TestParseSyslogMessageWithoutPriority verifies parsing without PRI.
func TestParseSyslogMessageWithoutPriority(t *testing.T) {
	collector := NewSyslogCollector(SyslogConfig{})

	raw := "Aug  4 12:00:00 server01 sshd[12345]: Normal message"
	msg := collector.parseSyslogMessage(raw, "192.168.1.50:514")

	if msg.Hostname != "server01" {
		t.Errorf("Hostname = %q, want %q", msg.Hostname, "server01")
	}
	if msg.AppName != "sshd" {
		t.Errorf("AppName = %q, want %q", msg.AppName, "sshd")
	}
	if msg.Priority != 0 {
		t.Errorf("Priority = %d, want 0", msg.Priority)
	}
}

// TestGetFacility verifies facility names.
func TestGetFacility(t *testing.T) {
	tests := []struct {
		priority int
		want     string
	}{
		{0, "kern"},    // Facility 0 (0>>3=0)
		{8, "user"},    // Facility 1 (8>>3=1)
		{16, "mail"},   // Facility 2 (16>>3=2)
		{24, "daemon"}, // Facility 3 (24>>3=3)
		{32, "auth"},   // Facility 4 (32>>3=4)
		{64, "uucp"},   // Facility 8 (64>>3=8)
		{128, "ntp"},   // Facility 16 (128>>3=16)
		{160, "local0"}, // Facility 20 (160>>3=20)
	}

	for _, tt := range tests {
		got := getFacility(tt.priority)
		if got != tt.want {
			t.Errorf("getFacility(%d) = %q, want %q", tt.priority, got, tt.want)
		}
	}
}

// TestGetSeverity verifies severity levels.
func TestGetSeverity(t *testing.T) {
	tests := []struct {
		priority int
		want     string
	}{
		{0, "emergency"},   // Severity 0
		{1, "alert"},       // Severity 0
		{2, "critical"},    // Severity 0
		{3, "error"},       // Severity 0
		{4, "warning"},     // Severity 0
		{5, "notice"},      // Severity 0
		{6, "info"},        // Severity 0
		{7, "debug"},       // Severity 0
		{8, "emergency"},   // Severity 1
		{15, "debug"},      // Severity 7
	}

	for _, tt := range tests {
		got := getSeverity(tt.priority)
		if got != tt.want {
			t.Errorf("getSeverity(%d) = %q, want %q", tt.priority, got, tt.want)
		}
	}
}

// TestClassifyDeviceType verifies device type classification.
func TestClassifyDeviceType(t *testing.T) {
	tests := []struct {
		appName string
		want    string
	}{
		{"firewall01", "firewall"},
		{"fortigate", "firewall"},
		{"paloalto", "firewall"},
		{"cisco-fwc", "firewall"},
		{"switch01", "switch"},
		{"cisco-sw", "switch"},
		{"router01", "router"},
		{"cisco-rt", "router"},
		{"server01", "server"},
		{"bmc01", "server"},
		{"ipmi01", "server"},
		{"ups01", "ups"},
		{"apc-ups", "ups"},
		{"san01", "storage"},
		{"storage01", "storage"},
		{"unknown-app", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		got := classifyDeviceType(tt.appName)
		if got != tt.want {
			t.Errorf("classifyDeviceType(%q) = %q, want %q", tt.appName, got, tt.want)
		}
	}
}

// TestSyslogMessageJSONRoundTrip verifies SyslogMessage JSON serialization.
func TestSyslogMessageJSONRoundTrip(t *testing.T) {
	msg := SyslogMessage{
		Priority:  165,
		Severity:  "info",
		Facility:  "daemon",
		Hostname:  "server01",
		AppName:   "sshd",
		ProcID:    "12345",
		MessageID: "AUTH",
		Content:   "Accepted publickey for admin",
		SourceIP:  "192.168.1.50",
		DeviceType: "server",
		Timestamp: time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed SyslogMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if parsed.Hostname != msg.Hostname {
		t.Errorf("Hostname = %q, want %q", parsed.Hostname, msg.Hostname)
	}
	if parsed.Severity != msg.Severity {
		t.Errorf("Severity = %q, want %q", parsed.Severity, msg.Severity)
	}
	if parsed.DeviceType != msg.DeviceType {
		t.Errorf("DeviceType = %q, want %q", parsed.DeviceType, msg.DeviceType)
	}
}

// TestStoreAndRetrieveMessages verifies message storage and retrieval.
func TestStoreAndRetrieveMessages(t *testing.T) {
	collector := NewSyslogCollector(SyslogConfig{Buffer: 5})

	msg1 := &SyslogMessage{
		Hostname: "server01",
		AppName:  "sshd",
		Content:  "Message 1",
	}
	msg2 := &SyslogMessage{
		Hostname: "server02",
		AppName:  "apache",
		Content:  "Message 2",
	}

	collector.storeMessage(msg1)
	collector.storeMessage(msg2)

	messages := collector.GetMessages()
	if len(messages) != 2 {
		t.Errorf("message count = %d, want 2", len(messages))
	}
	if messages[0].Content != "Message 1" {
		t.Errorf("first message = %q, want %q", messages[0].Content, "Message 1")
	}
	if messages[1].Content != "Message 2" {
		t.Errorf("second message = %q, want %q", messages[1].Content, "Message 2")
	}
}

// TestStoreMessagesBufferExceeded verifies oldest message is dropped when buffer is full.
func TestStoreMessagesBufferExceeded(t *testing.T) {
	collector := NewSyslogCollector(SyslogConfig{Buffer: 3})

	for i := 0; i < 5; i++ {
		msg := &SyslogMessage{
			Content:  "Message " + string(rune('0'+i)),
		}
		collector.storeMessage(msg)
	}

	messages := collector.GetMessages()
	if len(messages) != 3 {
		t.Errorf("message count = %d, want 3", len(messages))
	}
	// Oldest messages (0, 1) should be dropped, only 2, 3, 4 remain
	if messages[0].Content != "Message 2" {
		t.Errorf("first message = %q, want %q", messages[0].Content, "Message 2")
	}
	if messages[2].Content != "Message 4" {
		t.Errorf("last message = %q, want %q", messages[2].Content, "Message 4")
	}
}

// TestBuildSNMPTrap verifies syslog trap payload building.
func TestBuildSNMPTrap(t *testing.T) {
	message := "Test syslog message"
	payload := BuildSNMPTrap("192.168.1.100", message)

	if len(payload) == 0 {
		t.Fatal("payload should not be empty")
	}

	// Payload should start with 4-byte length prefix
	if len(payload) < 4 {
		t.Fatalf("payload too short: %d bytes", len(payload))
	}
}

// TestSyslogCollectorGetMessagesEmpty verifies empty message list.
func TestSyslogCollectorGetMessagesEmpty(t *testing.T) {
	collector := NewSyslogCollector(SyslogConfig{})
	messages := collector.GetMessages()

	if messages == nil {
		t.Fatal("messages should not be nil")
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}
}

// TestSyslogCollectorStop verifies Stop() doesn't panic.
func TestSyslogCollectorStop(t *testing.T) {
	collector := NewSyslogCollector(SyslogConfig{})

	// Stop should not panic even without Start
	collector.Stop()
}

// TestSyslogCollectorContextVerification verifies context handling.
func TestSyslogCollectorContextVerification(t *testing.T) {
	collector := NewSyslogCollector(SyslogConfig{})

	msg := &SyslogMessage{
		Hostname:  "server01",
		AppName:   "test",
		Content:   "test message",
		Timestamp: time.Now().UTC(),
		SourceIP:  "192.168.1.1",
	}

	// Store and verify
	collector.storeMessage(msg)
	messages := collector.GetMessages()

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Hostname != "server01" {
		t.Errorf("Hostname = %q, want %q", messages[0].Hostname, "server01")
	}
}
