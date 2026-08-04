package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestIsUPSRelatedOID verifies OID classification logic
func TestIsUPSRelatedOID(t *testing.T) {
	tests := []struct {
		oid    string
		expect bool
	}{
		{OIDUPSBatteryCapacity, true},
		{OIDUPSBatteryPercent, true},
		{OIDUPSBatteryRuntime, true},
		{OIDUPSInputStatus, true},
		{OIDUPSInputVoltage, true},
		{OIDUPSOutputStatus, true},
		{OIDUPSOutputVoltage, true},
		{OIDUPSLoad, true},
		{".1.3.6.1.2.1.33.1.4.1.3", true},  // sub-OID of UPS MIB
		{".1.3.6.1.2.1.33.99", true},        // another sub-OID
		{".1.3.6.1.2.1.1.5.0", false},       // SNMP system OID, not UPS
		{".1.3.6.1.4.1.9.1.2", false},       // Cisco OID
		{"", false},                          // empty string
		{".1.3.6.1.2.1.32.1.1.1", false},    // Network device MIB
	}

	for _, tt := range tests {
		t.Run(tt.oid, func(t *testing.T) {
			got := isUPSRelatedOID(tt.oid)
			require.Equal(t, tt.expect, got, "oid=%s", tt.oid)
		})
	}
}

// TestDetermineAlertLevel verifies alert level classification
func TestDetermineAlertLevel(t *testing.T) {
	tests := []struct {
		name           string
		batteryPercent int
		batteryRuntime int
		want           string
	}{
		{"empty values normal", 0, 0, "normal"},
		{"high battery normal", 80, 7200, "normal"},
		{"medium battery warning", 35, 5400, "warning"},
		{"low battery warning", 15, 3600, "warning"},
		{"critical runtime", 50, 300, "shutdown"},
		{"borderline critical runtime", 80, 599, "shutdown"},
		{"exactly 600s critical", 80, 600, "critical"}, // 600 seconds = 10 min = boundary (>= 600 but < 1800 = critical)
		{"warning runtime", 70, 1500, "critical"},  // 25 min < 30 min threshold
		{"exactly 1800s normal", 80, 1800, "normal"}, // exactly 30 min = normal
		{"just under 30 min critical", 80, 1799, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &UPSState{
				BatteryPercent: tt.batteryPercent,
				BatteryRuntime: tt.batteryRuntime,
			}
			got := determineAlertLevel(state)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestParseNumericValue verifies SNMP value parsing
func TestParseNumericValue(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		{"100", 100.0, false},
		{"45.5", 45.5, false},
		{"0", 0.0, false},
		{"3600", 3600.0, false},
		{"", 0, true},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseNumericValue(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.InDelta(t, tt.want, got, 0.001)
			}
		})
	}
}

// TestParseTenantProbeSubject verifies NATS subject parsing
func TestParseTenantProbeSubject(t *testing.T) {
	tests := []struct {
		subject    string
		wantTenant string
		wantProbe  string
		wantErr    bool
	}{
		{"tenant.abc123.probe.upssnmp.1", "abc123", "upssnmp", false},
		{"tenant.a1b2c3.probe.x.y.z", "a1b2c3", "x", false},
		{"tenant.single.part", "", "", true}, // insufficient parts
		{"", "", "", true},                   // empty subject
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			tenant, probe, err := parseTenantProbeSubject(tt.subject)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantTenant, tenant)
				require.Equal(t, tt.wantProbe, probe)
			}
		})
	}
}

// TestNewPowerEventManager verifies constructor initialization
func TestNewPowerEventManager(t *testing.T) {
	log, _ := zap.NewDevelopment()
	pm := NewPowerEventManager(nil, nil, log)
	require.NotNil(t, pm)
	require.Nil(t, pm.js) // no NATS = no JetStream
	require.NotNil(t, pm.upses)
	require.NotNil(t, pm.events)
}

// TestNewPowerEventManagerWithNATS verifies NATS initialization
func TestNewPowerEventManagerWithNATS(t *testing.T) {
	log, _ := zap.NewDevelopment()
	pm := NewPowerEventManager(nil, &nats.Conn{}, log)
	require.NotNil(t, pm)
	// js may be nil if nats.Conn has no JetStream context, but the struct is initialized
}

// TestGetAllUPSStates verifies empty state returns empty map
func TestGetAllUPSStates(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)
	states := pm.GetAllUPSStates()
	require.NotNil(t, states)
	require.Empty(t, states)
}

// TestGetUPSStateEmpty verifies non-existent device returns nil
func TestGetUPSStateEmpty(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)
	state := pm.GetUPSState("nonexistent")
	require.Nil(t, state)
}

// TestGetActiveEventsEmpty verifies no active events initially
func TestGetActiveEventsEmpty(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)
	events := pm.GetActiveEvents("tenant-1")
	require.Empty(t, events)
}

// TestAbortShutdown verifies shutdown abortion
func TestAbortShutdown(t *testing.T) {
	log, _ := zap.NewDevelopment()
	pm := NewPowerEventManager(nil, nil, log)
	pm.events["evt-123"] = &ShutdownEvent{ID: "evt-123", Status: "initiated"}
	pm.AbortShutdown("evt-123", "manual_abort")
	require.Equal(t, "aborted", pm.events["evt-123"].Status)

	pm.AbortShutdown("evt-nonexistent", "manual_abort")
	// Non-existent event should be silent
}

// TestPtrString verifies helper function
func TestPtrString(t *testing.T) {
	s := "test"
	p := ptrString(s)
	require.NotNil(t, p)
	require.Equal(t, "test", *p)
}

// TestShutdownOrder verifies shutdown target ordering constants
func TestShutdownOrder(t *testing.T) {
	expected := []string{"hypervisor", "san", "nas", "vm_pool"}
	require.Equal(t, expected, ShutdownOrder)
}

// TestHasUPSOIDPrefix verifies UPS OID prefix matching
func TestHasUPSOIDPrefix(t *testing.T) {
	tests := []struct {
		oid    string
		expect bool
	}{
		{".1.3.6.1.2.1.33.1.4.1.2", true},
		{".1.3.6.1.2.1.33.", true},
		{".1.3.6.1.2.1.33", true},
		{".1.3.6.1.2.1.32.1", false},
		{".1.3.6.1.2.1.34.1", false},
		{".1.3.6.1.2.1.3", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.oid, func(t *testing.T) {
			got := hasUPSOIDPrefix(tt.oid)
			require.Equal(t, tt.expect, got, "oid=%s", tt.oid)
		})
	}
}

// TestShutdownTargetStruct verifies ShutdownTarget JSON marshaling
func TestShutdownTargetStruct(t *testing.T) {
	target := ShutdownTarget{
		DeviceID:     "dev-123",
		DeviceType:   "hypervisor",
		ShutdownOrder: 0,
		Command:      "shutdown",
		Timeout:      120 * time.Second,
	}

	payload, err := json.Marshal(target)
	require.NoError(t, err)

	var unmarshaled ShutdownTarget
	err = json.Unmarshal(payload, &unmarshaled)
	require.NoError(t, err)
	require.Equal(t, target.DeviceID, unmarshaled.DeviceID)
	require.Equal(t, target.DeviceType, unmarshaled.DeviceType)
	require.Equal(t, target.ShutdownOrder, unmarshaled.ShutdownOrder)
	require.Equal(t, target.Command, unmarshaled.Command)
	require.Equal(t, target.Timeout, unmarshaled.Timeout)
}

// TestShutdownEventStruct verifies ShutdownEvent JSON marshaling
func TestShutdownEventStruct(t *testing.T) {
	event := &ShutdownEvent{
		ID:          "evt-001",
		TenantID:    "tenant-abc",
		UPSDeviceID: "ups-xyz",
		BatteryMin:  7,
		InitiatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		TargetOrder: []string{"dev-1", "dev-2"},
		Status:      "initiated",
	}

	payload, err := json.Marshal(event)
	require.NoError(t, err)

	var unmarshaled ShutdownEvent
	err = json.Unmarshal(payload, &unmarshaled)
	require.NoError(t, err)
	require.Equal(t, event.ID, unmarshaled.ID)
	require.Equal(t, event.TenantID, unmarshaled.TenantID)
	require.Equal(t, event.UPSDeviceID, unmarshaled.UPSDeviceID)
	require.Equal(t, event.BatteryMin, unmarshaled.BatteryMin)
	require.Equal(t, event.Status, unmarshaled.Status)
	require.Equal(t, len(event.TargetOrder), len(unmarshaled.TargetOrder))
}

// TestUPSStatusStruct verifies UPSStatus JSON marshaling
func TestUPSStatusStruct(t *testing.T) {
	up := &UPSStatus{
		TenantID:       "tenant-abc",
		DeviceID:       "ups-001",
		ProbeID:        "probe-1",
		Timestamp:      time.Now().UTC(),
		BatteryPercent: 45,
		BatteryRuntime: 2700,
		InputStatus:    1,
		OutputStatus:   2,
		OutputLoad:     62,
		AlertLevel:     "warning",
	}

	payload, err := json.Marshal(up)
	require.NoError(t, err)
	require.Contains(t, string(payload), "tenant_id")
	require.Contains(t, string(payload), "battery_percent")
	require.Contains(t, string(payload), "battery_runtime_seconds")
	require.Contains(t, string(payload), "alert_level")
}

// TestMultipleOIDUpdates verifies UPS state updates from multiple SNMP OIDs
func TestMultipleOIDUpdates(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)

	// Simulate battery percent update
	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryPercent, 50.0)
	state := pm.GetUPSState("ups-1")
	require.NotNil(t, state)
	require.Equal(t, 50, state.BatteryPercent)
	require.Equal(t, "tenant-1", state.TenantID)
	require.Equal(t, "warning", state.AlertLevel) // 50% is the boundary

	// Simulate battery runtime update
	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryRuntime, 1500.0)
	state = pm.GetUPSState("ups-1")
	require.NotNil(t, state)
	require.Equal(t, 1500, state.BatteryRuntime)
	require.Equal(t, "critical", state.AlertLevel) // 1500s < 1800s = critical
}

// TestUPSStateUpdateSameDevice verifies state persistence across multiple OID updates
func TestUPSStateUpdateSameDevice(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)

	// First update: battery percent
	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryPercent, 75.0)
	// Second update: battery runtime
	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryRuntime, 5400.0)
	// Third update: battery percent again
	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryPercent, 60.0)

	state := pm.GetUPSState("ups-1")
	require.NotNil(t, state)
	require.Equal(t, 60, state.BatteryPercent)  // latest value
	require.Equal(t, 5400, state.BatteryRuntime) // persisted value
	require.Equal(t, "tenant-1", state.TenantID)
}

// TestUPSStateMultipleDevices verifies independent tracking per device
func TestUPSStateMultipleDevices(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)

	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryPercent, 90.0)
	pm.updateUPSState("tenant-1", "ups-2", "probe-1", OIDUPSBatteryPercent, 20.0)

	s1 := pm.GetUPSState("ups-1")
	s2 := pm.GetUPSState("ups-2")

	require.NotNil(t, s1)
	require.NotNil(t, s2)
	require.Equal(t, 90, s1.BatteryPercent)
	require.Equal(t, 20, s2.BatteryPercent)
	require.Equal(t, "normal", s1.AlertLevel)
	require.Equal(t, "warning", s2.AlertLevel)
}

// TestGetAllUPSStatesSnapshot verifies snapshot isolation
func TestGetAllUPSStatesSnapshot(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)

	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryPercent, 80.0)
	pm.updateUPSState("tenant-1", "ups-2", "probe-1", OIDUPSBatteryRuntime, 3600.0)

	snapshot := pm.GetAllUPSStates()
	require.Len(t, snapshot, 2)

	// Verify we can modify the original without affecting snapshot
	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryPercent, 40.0)

	state := pm.GetUPSState("ups-1")
	require.Equal(t, 40, state.BatteryPercent) // original updated
	require.Equal(t, 80, snapshot["ups-1"].BatteryPercent) // snapshot preserved
}

// TestGetActiveEventsFilter verifies event filtering by tenant
func TestGetActiveEventsFilter(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)
	pm.events["evt-1"] = &ShutdownEvent{ID: "evt-1", TenantID: "tenant-a", Status: "initiated"}
	pm.events["evt-2"] = &ShutdownEvent{ID: "evt-2", TenantID: "tenant-a", Status: "completed"}
	pm.events["evt-3"] = &ShutdownEvent{ID: "evt-3", TenantID: "tenant-b", Status: "in_progress"}
	pm.events["evt-4"] = &ShutdownEvent{ID: "evt-4", TenantID: "tenant-a", Status: "aborted"}

	active := pm.GetActiveEvents("tenant-a")
	require.Len(t, active, 1)
	require.Equal(t, "evt-1", active[0].ID)
}

// TestUpdateEventStatus verifies status transitions
func TestUpdateEventStatus(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)
	pm.events["evt-1"] = &ShutdownEvent{
		ID:     "evt-1",
		Status: "initiated",
	}

	pm.updateEventStatus("evt-1", "completed", nil)
	require.Equal(t, "completed", pm.events["evt-1"].Status)

	pm.updateEventStatus("evt-2", "aborted", ptrString("timeout"))
	// evt-2 doesn't exist, should be silent

	evt := &ShutdownEvent{ID: "evt-3", Status: "in_progress"}
	pm.events["evt-3"] = evt
	pm.updateEventStatus("evt-3", "aborted", ptrString("manual"))
	require.Equal(t, "aborted", evt.Status)
	require.NotNil(t, evt.AbortedAt)
	require.Equal(t, "manual", *evt.AbortedBy)
}

// TestNonUPSOIDIgnored verifies non-UPS OIDs don't affect state
func TestNonUPSOIDIgnored(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)

	pm.updateUPSState("tenant-1", "ups-1", "probe-1", ".1.3.6.1.2.1.1.5.0", 100.0)

	state := pm.GetUPSState("ups-1")
	// Device should exist (from updateUPSState creating it) but battery fields should be 0
	require.NotNil(t, state)
	require.Equal(t, 0, state.BatteryPercent)
	require.Equal(t, 0, state.BatteryRuntime)
}

// TestEmptyPayloadHandling verifies graceful handling of empty/missing fields
func TestEmptyPayloadHandling(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)

	// Test with device_id missing
	pm.updateUPSState("tenant-1", "", "probe-1", OIDUPSBatteryPercent, 50.0)
	// Should not crash, but no device registered with empty ID
	require.Len(t, pm.GetAllUPSStates(), 0)
}

// TestConcurrentStateAccess verifies thread safety
func TestConcurrentStateAccess(t *testing.T) {
	log, _ := zap.NewDevelopment()
	pm := NewPowerEventManager(nil, nil, log)

	// Writer goroutines - use safe values to avoid triggering shutdown
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				if j%2 == 0 {
					pm.updateUPSState("tenant-1", fmt.Sprintf("ups-%d", n%3), "probe-1", OIDUPSBatteryPercent, 50.0+float64(j%50))
				} else {
					pm.updateUPSState("tenant-1", fmt.Sprintf("ups-%d", n%3), "probe-1", OIDUPSBatteryRuntime, float64(3600+j))
				}
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = pm.GetAllUPSStates()
				_ = pm.GetUPSState(fmt.Sprintf("ups-%d", j%3))
				_ = pm.GetActiveEvents("tenant-1")
			}
		}()
	}

	// Let them run
	time.Sleep(200 * time.Millisecond)
}

// TestPayloadParsingWithDeviceIDFallback verifies device_id from record fallback
func TestPayloadParsingWithDeviceIDFallback(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)

	// First update creates entry with device_id
	pm.updateUPSState("tenant-1", "ups-primary", "probe-1", OIDUPSBatteryPercent, 60.0)
	// Second update with empty device_id should NOT create a new entry
	pm.updateUPSState("tenant-1", "", "probe-1", OIDUPSBatteryRuntime, 3600.0)

	states := pm.GetAllUPSStates()
	require.Len(t, states, 1)
	// The empty device_id entry was not registered
}

// TestShutdownTargetOrdering verifies the shutdown order logic
func TestShutdownTargetOrdering(t *testing.T) {
	// Verify the shutdown sequence constant is correct
	require.Equal(t, []string{"hypervisor", "san", "nas", "vm_pool"}, ShutdownOrder)

	// Verify ShutdownTarget struct has all required fields
	target := ShutdownTarget{
		DeviceID:     "hyp-1",
		DeviceType:   "hypervisor",
		ShutdownOrder: 0,
		Command:      "shutdown",
		Timeout:      120 * time.Second,
	}

	payload, err := json.Marshal(target)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(payload), "hypervisor"))
	require.True(t, strings.Contains(string(payload), "shutdown"))
}

// TestUPSStateLastSeenUpdated verifies LastSeen is updated on each state update
func TestUPSStateLastSeenUpdated(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)

	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryPercent, 80.0)
	firstSeen := pm.GetUPSState("ups-1").LastSeen

	time.Sleep(10 * time.Millisecond)
	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryRuntime, 5000.0)
	secondSeen := pm.GetUPSState("ups-1").LastSeen

	require.True(t, secondSeen.After(firstSeen), "LastSeen should advance on subsequent updates")
}

// TestUPSStateProbeIDUpdated verifies ProbeID is updated on each state update
func TestUPSStateProbeIDUpdated(t *testing.T) {
	pm := NewPowerEventManager(nil, nil, nil)

	pm.updateUPSState("tenant-1", "ups-1", "probe-1", OIDUPSBatteryPercent, 80.0)
	require.Equal(t, "probe-1", pm.GetUPSState("ups-1").ProbeID)

	pm.updateUPSState("tenant-1", "ups-1", "probe-2", OIDUPSBatteryPercent, 75.0)
	require.Equal(t, "probe-2", pm.GetUPSState("ups-1").ProbeID)
}

// TestBoundaryRuntime600Sec verifies the 600-second boundary behavior
func TestBoundaryRuntime600Sec(t *testing.T) {
	tests := []struct {
		name        string
		runtime     int
		expectLevel string
	}{
		{"exactly 600 critical", 600, "critical"},   // >= 600, < 1800 = critical
		{"599 seconds", 599, "shutdown"},             // < 600 = shutdown
		{"601 seconds", 601, "critical"},             // >= 600, < 1800 = critical
		{"1799 seconds", 1799, "critical"},           // >= 600, < 1800 = critical
		{"1800 seconds", 1800, "normal"},             // >= 1800, battery=100 > 50 = normal
		{"1801 seconds", 1801, "normal"},             // >= 1800, battery=100 > 50 = normal
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &UPSState{BatteryRuntime: tt.runtime, BatteryPercent: 100}
			got := determineAlertLevel(state)
			require.Equal(t, tt.expectLevel, got, "runtime=%d", tt.runtime)
		})
	}
}

// TestBoundaryBatteryPercent verifies battery percentage boundaries
func TestBoundaryBatteryPercent(t *testing.T) {
	tests := []struct {
		name          string
		percent       int
		expectLevel   string
	}{
		{"100%", 100, "normal"},
		{"51%", 51, "normal"},
		{"50%", 50, "warning"},
		{"49%", 49, "warning"},
		{"20%", 20, "warning"},
		{"19%", 19, "warning"},
		{"1%", 1, "warning"},
		{"0%", 0, "normal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &UPSState{BatteryPercent: tt.percent, BatteryRuntime: 0}
			got := determineAlertLevel(state)
			require.Equal(t, tt.expectLevel, got, "percent=%d", tt.percent)
		})
	}
}

// TestShutdownEventJSONRoundTrip verifies full round-trip marshaling/unmarshaling
func TestShutdownEventJSONRoundTrip(t *testing.T) {
	abortedBy := "admin@example.com"
	original := &ShutdownEvent{
		ID:          "evt-999",
		TenantID:    "tenant-x",
		UPSDeviceID: "ups-main",
		BatteryMin:  5,
		InitiatedAt: time.Date(2025, 6, 15, 3, 30, 0, 0, time.UTC),
		TargetOrder: []string{"esxi-1", "esxi-2", "san-1"},
		Status:      "aborted",
		AbortedAt:   ptrTime(time.Date(2025, 6, 15, 3, 32, 0, 0, time.UTC)),
		AbortedBy:   &abortedBy,
	}

	payload, err := json.Marshal(original)
	require.NoError(t, err)

	var restored ShutdownEvent
	err = json.Unmarshal(payload, &restored)
	require.NoError(t, err)

	require.Equal(t, original.ID, restored.ID)
	require.Equal(t, original.TenantID, restored.TenantID)
	require.Equal(t, original.UPSDeviceID, restored.UPSDeviceID)
	require.Equal(t, original.BatteryMin, restored.BatteryMin)
	require.Equal(t, original.Status, restored.Status)
	require.Equal(t, len(original.TargetOrder), len(restored.TargetOrder))
	require.NotNil(t, restored.AbortedAt)
	require.Equal(t, *original.AbortedAt, *restored.AbortedAt)
}

// ptrTime returns a pointer to the given time
func ptrTime(t time.Time) *time.Time {
	return &t
}
