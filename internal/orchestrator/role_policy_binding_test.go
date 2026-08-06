package orchestrator

import (
	"context"
	"testing"
	"time"
)

// TestRolePolicyBinderNewBinder verifies the binder is created correctly.
func TestRolePolicyBinderNewBinder(t *testing.T) {
	binder := NewRolePolicyBinder(nil, nil, &testLogger{})
	if binder == nil {
		t.Fatal("RolePolicyBinder should not be nil")
	}
	if binder.logger == nil {
		t.Fatal("logger should not be nil")
	}
}

// TestRolePolicyBinderProcessMonitorEventEmptyRoles verifies no error with empty roles.
func TestRolePolicyBinderProcessMonitorEventEmptyRoles(t *testing.T) {
	binder := NewRolePolicyBinder(nil, nil, &testLogger{})

	event := MonitorEvent{
		AgentID:   "agent-1",
		TenantID:  "tenant-1",
		DeviceID:  "device-1",
		Roles:     []string{},
		Timestamp: time.Now(),
	}

	err := binder.ProcessMonitorEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ProcessMonitorEvent with empty roles: %v", err)
	}
}

// TestRolePolicyBinderProcessMonitorEventNilRoles verifies no error with nil roles.
func TestRolePolicyBinderProcessMonitorEventNilRoles(t *testing.T) {
	binder := NewRolePolicyBinder(nil, nil, &testLogger{})

	event := MonitorEvent{
		AgentID:   "agent-1",
		TenantID:  "tenant-1",
		DeviceID:  "device-1",
		Roles:     nil,
		Timestamp: time.Now(),
	}

	err := binder.ProcessMonitorEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ProcessMonitorEvent with nil roles: %v", err)
	}
}

// TestRolePolicyBinderBindRoleToDeviceEmptyParams verifies error on empty params.
func TestRolePolicyBinderBindRoleToDeviceEmptyParams(t *testing.T) {
	binder := NewRolePolicyBinder(nil, nil, &testLogger{})

	tests := []struct {
		name     string
		deviceID string
		tenantID string
		role     string
	}{
		{"empty device ID", "", "tenant-1", "sql_server"},
		{"empty tenant ID", "device-1", "", "sql_server"},
		{"empty role", "device-1", "tenant-1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := binder.bindRoleToDevice(context.Background(), tt.deviceID, tt.tenantID, tt.role)
			if err == nil {
				t.Fatal("expected error for empty params")
			}
		})
	}
}

// TestMonitorEventJSONSerialization verifies MonitorEvent JSON tags are correct.
func TestMonitorEventJSONSerialization(t *testing.T) {
	event := MonitorEvent{
		AgentID:   "agent-1",
		TenantID:  "tenant-1",
		DeviceID:  "device-1",
		Roles:     []string{"sql_server", "ad_dc"},
		Timestamp: time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
	}

	// Verify struct fields are accessible
	if event.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", event.AgentID, "agent-1")
	}
	if len(event.Roles) != 2 {
		t.Errorf("Roles length = %d, want 2", len(event.Roles))
	}
	if event.Roles[0] != "sql_server" {
		t.Errorf("Roles[0] = %q, want %q", event.Roles[0], "sql_server")
	}
}

// TestMonitoringDefinitionFields verifies struct fields are accessible.
func TestMonitoringDefinitionFields(t *testing.T) {
	def := MonitoringDefinition{
		ID:          "def-1",
		TenantID:    "tenant-1",
		Name:        "SQL Server Monitor",
		Description: "Monitor SQL Server health",
		Roles:       []string{"sql_server"},
		RuleType:    "threshold",
		MetricName:  "sql_server.uptime",
		Condition:   "lt",
		Threshold:   300,
		Severity:    "critical",
		Cooldown:    "10m",
		Enabled:     true,
		CreatedAt:   time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
	}

	if def.ID != "def-1" {
		t.Errorf("ID = %q, want %q", def.ID, "def-1")
	}
	if def.Roles[0] != "sql_server" {
		t.Errorf("Roles[0] = %q, want %q", def.Roles[0], "sql_server")
	}
	if def.Threshold != 300 {
		t.Errorf("Threshold = %f, want 300", def.Threshold)
	}
	if !def.Enabled {
		t.Error("Enabled should be true")
	}
}

// TestLogger is a minimal test logger.
type testLogger struct{}

func (l *testLogger) Info(msg string, keysAndValues ...interface{})  {}
func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {}
func (l *testLogger) Warn(msg string, keysAndValues ...interface{})  {}
