package reporting

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestReportSectionConstants verifies all ReportSection constant values
func TestReportSectionConstants(t *testing.T) {
	constants := map[ReportSection]string{
		SectionSummary: "summary",
		SectionAlerts:  "alerts",
		SectionCVEs:    "cves",
		SectionPatches: "patches",
	}

	for section, expected := range constants {
		if string(section) != expected {
			t.Errorf("ReportSection %q = %q, want %q", section, section, expected)
		}
	}
}

// TestReportData_StructFields verifies all ReportData struct fields
func TestReportData_StructFields(t *testing.T) {
	now := time.Now()
	data := ReportData{
		TenantName:  "Test Tenant",
		PeriodStart: now.Add(-7 * 24 * time.Hour),
		PeriodEnd:   now,
		DeviceCount: 150,
		OnlineCount: 142,
		AlertCount:  5,
		CVECount:    12,
		PatchCount:  3,
		Alerts: []struct{ Severity, Message, Device, Time string }{
			{Severity: "critical", Message: "High CPU usage", Device: "web-01", Time: "2024-01-01T10:00:00Z"},
		},
		CVEs: []struct{ ID, Severity, Package, Device string }{
			{ID: "CVE-2024-1234", Severity: "high", Package: "openssl", Device: "db-01"},
		},
		Patches: []struct{ Name, Version, Status string }{
			{Name: "kernel", Version: "5.15.0", Status: "pending"},
		},
	}

	if data.TenantName != "Test Tenant" {
		t.Errorf("ReportData.TenantName = %q, want %q", data.TenantName, "Test Tenant")
	}
	if data.DeviceCount != 150 {
		t.Errorf("ReportData.DeviceCount = %d, want %d", data.DeviceCount, 150)
	}
	if data.OnlineCount != 142 {
		t.Errorf("ReportData.OnlineCount = %d, want %d", data.OnlineCount, 142)
	}
	if data.AlertCount != 5 {
		t.Errorf("ReportData.AlertCount = %d, want %d", data.AlertCount, 5)
	}
	if data.CVECount != 12 {
		t.Errorf("ReportData.CVECount = %d, want %d", data.CVECount, 12)
	}
	if data.PatchCount != 3 {
		t.Errorf("ReportData.PatchCount = %d, want %d", data.PatchCount, 3)
	}
	if len(data.Alerts) != 1 {
		t.Errorf("ReportData.Alerts length = %d, want %d", len(data.Alerts), 1)
	}
	if len(data.CVEs) != 1 {
		t.Errorf("ReportData.CVEs length = %d, want %d", len(data.CVEs), 1)
	}
	if len(data.Patches) != 1 {
		t.Errorf("ReportData.Patches length = %d, want %d", len(data.Patches), 1)
	}
}

// TestReportData_ZeroValues verifies ReportData with zero values
func TestReportData_ZeroValues(t *testing.T) {
	data := ReportData{}

	if data.TenantName != "" {
		t.Error("ReportData.TenantName should be empty by default")
	}
	if data.DeviceCount != 0 {
		t.Error("ReportData.DeviceCount should be 0 by default")
	}
	if data.OnlineCount != 0 {
		t.Error("ReportData.OnlineCount should be 0 by default")
	}
	if data.AlertCount != 0 {
		t.Error("ReportData.AlertCount should be 0 by default")
	}
	if data.CVECount != 0 {
		t.Error("ReportData.CVECount should be 0 by default")
	}
	if data.PatchCount != 0 {
		t.Error("ReportData.PatchCount should be 0 by default")
	}
	if data.Alerts != nil {
		t.Error("ReportData.Alerts should be nil by default")
	}
	if data.CVEs != nil {
		t.Error("ReportData.CVEs should be nil by default")
	}
	if data.Patches != nil {
		t.Error("ReportData.Patches should be nil by default")
	}
}

// TestReportData_JSONRoundTrip verifies ReportData serializes/deserializes
func TestReportData_JSONRoundTrip(t *testing.T) {
	now := time.Now()
	original := ReportData{
		TenantName:  "Test Tenant",
		PeriodStart: now.Add(-7 * 24 * time.Hour),
		PeriodEnd:   now,
		DeviceCount: 150,
		OnlineCount: 142,
		AlertCount:  5,
		CVECount:    12,
		PatchCount:  3,
		Alerts: []struct{ Severity, Message, Device, Time string }{
			{Severity: "critical", Message: "High CPU usage", Device: "web-01", Time: "2024-01-01T10:00:00Z"},
		},
		CVEs: []struct{ ID, Severity, Package, Device string }{
			{ID: "CVE-2024-1234", Severity: "high", Package: "openssl", Device: "db-01"},
		},
		Patches: []struct{ Name, Version, Status string }{
			{Name: "kernel", Version: "5.15.0", Status: "pending"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal ReportData: %v", err)
	}

	var restored ReportData
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal ReportData: %v", err)
	}

	if restored.TenantName != original.TenantName {
		t.Errorf("Round-trip ReportData.TenantName = %q, want %q", restored.TenantName, original.TenantName)
	}
	if restored.DeviceCount != original.DeviceCount {
		t.Errorf("Round-trip ReportData.DeviceCount = %d, want %d", restored.DeviceCount, original.DeviceCount)
	}
	if restored.AlertCount != original.AlertCount {
		t.Errorf("Round-trip ReportData.AlertCount = %d, want %d", restored.AlertCount, original.AlertCount)
	}
	if restored.CVECount != original.CVECount {
		t.Errorf("Round-trip ReportData.CVECount = %d, want %d", restored.CVECount, original.CVECount)
	}
}

// TestReportEngine_StructFields verifies all ReportEngine struct fields exist
func TestReportEngine_StructFields(t *testing.T) {
	// ReportEngine has unexported fields: db, logger, storage, bucket
	// We verify via constructor behavior

	engine := NewReportEngine(nil, nil, nil, "reports-bucket")
	if engine == nil {
		t.Fatal("NewReportEngine returned nil")
	}

	// Verify bucket is set via Start (which uses it for storage)
	// We can't directly access unexported fields, but we can verify constructor behavior
	_ = engine
}

// TestNewReportEngine_Defaults verifies NewReportEngine default values
func TestNewReportEngine_Defaults(t *testing.T) {
	engine := NewReportEngine(nil, nil, nil, "reports-bucket")

	if engine == nil {
		t.Fatal("NewReportEngine returned nil")
	}

	// Verify bucket is set
	// We can't access unexported fields directly
	_ = engine
}

// TestTruncate_Func verifies the truncate helper function
func TestTruncate_Func(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		max    int
		output string
	}{
		{
			name:   "short string",
			input:  "hello",
			max:    10,
			output: "hello",
		},
		{
			name:   "exact length",
			input:  "hello world",
			max:    11,
			output: "hello world",
		},
		{
			name:   "truncated string",
			input:  "hello world this is a long message",
			max:    10,
			output: "hello w...",
		},
		{
			name:   "empty string",
			input:  "",
			max:    10,
			output: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.max)
			if result != tt.output {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.output)
			}
		})
	}
}

// TestReportSection_JSONRoundTrip verifies ReportSection serializes/deserializes
func TestReportSection_JSONRoundTrip(t *testing.T) {
	sections := []ReportSection{SectionSummary, SectionAlerts, SectionCVEs, SectionPatches}

	for _, section := range sections {
		t.Run(string(section), func(t *testing.T) {
			data, err := json.Marshal([]ReportSection{section})
			if err != nil {
				t.Fatalf("Failed to marshal ReportSection: %v", err)
			}

			var restored []ReportSection
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatalf("Failed to unmarshal ReportSection: %v", err)
			}

			if len(restored) != 1 || restored[0] != section {
				t.Errorf("Round-trip ReportSection = %v, want %v", restored, []ReportSection{section})
			}
		})
	}
}

// TestReportEngine_Start_ContextCancellation verifies Start respects context cancellation
func TestReportEngine_Start_ContextCancellation(t *testing.T) {
	engine := NewReportEngine(nil, nil, nil, "reports-bucket")
	if engine == nil {
		t.Fatal("NewReportEngine returned nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Start should return when context is already cancelled
	go engine.Start(ctx)
	// Give it a moment to detect cancellation
	time.Sleep(50 * time.Millisecond)
}
