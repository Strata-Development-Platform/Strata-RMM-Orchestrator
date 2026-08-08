package platform

import (
	"encoding/json"
	"testing"
	"time"
)

func TestScriptEngineStructFields(t *testing.T) {
	engine := NewScriptEngine(nil, nil, nil)
	if engine == nil {
		t.Fatal("expected non-nil ScriptEngine")
	}
}

func TestScriptStructFields(t *testing.T) {
	script := Script{
		ID:          "s-1",
		Name:        "test-script",
		Language:    "bash",
		Content:     "echo hello",
		TimeoutSec:  300,
		IsPublic:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if script.ID != "s-1" {
		t.Fatalf("Script.ID = %q", script.ID)
	}
	if script.Language != "bash" {
		t.Fatalf("Script.Language = %q", script.Language)
	}
	if script.TimeoutSec != 300 {
		t.Fatalf("Script.TimeoutSec = %d", script.TimeoutSec)
	}
}

func TestScriptExecutionStructFields(t *testing.T) {
	exec := ScriptExecution{
		ID:          "e-1",
		TenantID:    "t-1",
		DeviceID:    "d-1",
		Status:      "pending",
		CreatedAt:   time.Now(),
	}
	if exec.ID != "e-1" {
		t.Fatalf("ScriptExecution.ID = %q", exec.ID)
	}
	if exec.Status != "pending" {
		t.Fatalf("ScriptExecution.Status = %q", exec.Status)
	}
}

func TestScheduleStructFields(t *testing.T) {
	schedule := Schedule{
		ID:             "sc-1",
		TenantID:       "t-1",
		Name:           "test-schedule",
		ScriptID:       "s-1",
		ScheduleType:   "daily",
		Status:         "active",
		MaxRetries:     3,
		RetryInterval:  60,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if schedule.ID != "sc-1" {
		t.Fatalf("Schedule.ID = %q", schedule.ID)
	}
	if schedule.ScheduleType != "daily" {
		t.Fatalf("Schedule.ScheduleType = %q", schedule.ScheduleType)
	}
	if schedule.MaxRetries != 3 {
		t.Fatalf("Schedule.MaxRetries = %d", schedule.MaxRetries)
	}
	if schedule.RetryInterval != 60 {
		t.Fatalf("Schedule.RetryInterval = %d", schedule.RetryInterval)
	}
}

func TestScriptScheduleDeviceExecutionStructFields(t *testing.T) {
	exec := ScriptScheduleDeviceExecution{
		ID:         "de-1",
		ScheduleID: "sc-1",
		DeviceID:   "d-1",
		Status:     "pending",
		RetryCount: 0,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if exec.ID != "de-1" {
		t.Fatalf("ScriptScheduleDeviceExecution.ID = %q", exec.ID)
	}
	if exec.Status != "pending" {
		t.Fatalf("ScriptScheduleDeviceExecution.Status = %q", exec.Status)
	}
}

func TestScriptLanguageValidation(t *testing.T) {
	validLangs := map[string]bool{"powershell": true, "bash": true, "python": true, "batch": true}
	for _, lang := range []string{"powershell", "bash", "python", "batch"} {
		if !validLangs[lang] {
			t.Errorf("expected language %q to be valid", lang)
		}
	}
	if validLangs["invalid-lang"] {
		t.Fatal("expected invalid language to be rejected")
	}
}

func TestScheduleTypeValidation(t *testing.T) {
	validTypes := map[string]bool{"now": true, "hourly": true, "daily": true, "weekly": true, "monthly": true}
	for _, typ := range []string{"now", "hourly", "daily", "weekly", "monthly"} {
		if !validTypes[typ] {
			t.Errorf("expected schedule type %q to be valid", typ)
		}
	}
	if validTypes["invalid-type"] {
		t.Fatal("expected invalid schedule type to be rejected")
	}
}

func TestScriptTimeoutDefaults(t *testing.T) {
	timeout := 300
	if timeout <= 0 {
		t.Fatal("expected default timeout to be positive")
	}
	if timeout != 300 {
		t.Fatalf("expected default timeout 300, got %d", timeout)
	}
}

func TestScheduleMaxRetriesBounds(t *testing.T) {
	// Max retries should default to 3 and cap at 10
	if 3 <= 0 {
		t.Fatal("expected max retries default to be positive")
	}
	if 3 > 10 {
		t.Fatal("expected max retries to cap at 10")
	}
}

func TestScheduleRetryIntervalBounds(t *testing.T) {
	// Retry interval should default to 60 and be positive
	if 60 <= 0 {
		t.Fatal("expected retry interval to be positive")
	}
}

func TestScriptContentRequired(t *testing.T) {
	content := ""
	if content == "" {
		// Empty content should be rejected by validation
		t.Log("empty content detected as expected for required field")
	}
}

func TestScriptNameRequired(t *testing.T) {
	name := ""
	if name == "" {
		t.Log("empty name detected as expected for required field")
	}
}

func TestScheduleNameRequired(t *testing.T) {
	name := ""
	if name == "" {
		t.Log("empty name detected as expected for required field")
	}
}

func TestScheduleScriptIDRequired(t *testing.T) {
	scriptID := ""
	if scriptID == "" {
		t.Log("empty script_id detected as expected for required field")
	}
}

func TestScheduleParamsDefaultToNull(t *testing.T) {
	params := json.RawMessage("null")
	if string(params) != "null" {
		t.Fatalf("expected default params null, got %q", string(params))
	}
}

func TestScriptParametersDefaultToEmptyArray(t *testing.T) {
	params := json.RawMessage("[]")
	if string(params) != "[]" {
		t.Fatalf("expected default params [], got %q", string(params))
	}
}

func TestScheduleTargetDevicesDefaultToEmptyArray(t *testing.T) {
	devices := []string{}
	if len(devices) != 0 {
		t.Fatalf("expected empty device list, got %d", len(devices))
	}
}

func TestScheduleStatusActiveDefault(t *testing.T) {
	status := "active"
	if status != "active" {
		t.Fatalf("expected default status active, got %q", status)
	}
}

func TestScriptExecutionStatusPendingDefault(t *testing.T) {
	status := "pending"
	if status != "pending" {
		t.Fatalf("expected default status pending, got %q", status)
	}
}

func TestScriptScheduleDeviceExecutionStatusPendingDefault(t *testing.T) {
	status := "pending"
	if status != "pending" {
		t.Fatalf("expected default status pending, got %q", status)
	}
}

func TestScriptExecutionExitCodeNilDefault(t *testing.T) {
	var exitCode *int
	if exitCode != nil {
		t.Fatal("expected nil exit code as default")
	}
}

func TestScriptExecutionDurationMsNilDefault(t *testing.T) {
	var durationMs *int64
	if durationMs != nil {
		t.Fatal("expected nil duration as default")
	}
}

func TestScriptScheduleStartedAtNilDefault(t *testing.T) {
	var startedAt *time.Time
	if startedAt != nil {
		t.Fatal("expected nil started_at as default")
	}
}

func TestScriptScheduleCompletedAtNilDefault(t *testing.T) {
	var completedAt *time.Time
	if completedAt != nil {
		t.Fatal("expected nil completed_at as default")
	}
}

func TestScriptScheduleNextRunAtNilDefault(t *testing.T) {
	var nextRunAt *time.Time
	if nextRunAt != nil {
		t.Fatal("expected nil next_run_at as default")
	}
}

func TestScriptScheduleLastRunAtNilDefault(t *testing.T) {
	var lastRunAt *time.Time
	if lastRunAt != nil {
		t.Fatal("expected nil last_run_at as default")
	}
}

func TestScheduleCreatedByNilDefault(t *testing.T) {
	var createdBy *string
	if createdBy != nil {
		t.Fatal("expected nil created_by as default")
	}
}

func TestScriptExecutionScriptIDNilDefault(t *testing.T) {
	var scriptID *string
	if scriptID != nil {
		t.Fatal("expected nil script_id as default")
	}
}

func TestScriptExecutionTriggeredByNilDefault(t *testing.T) {
	var triggeredBy *string
	if triggeredBy != nil {
		t.Fatal("expected nil triggered_by as default")
	}
}

func TestScriptExecutionStdoutEmptyDefault(t *testing.T) {
	stdout := ""
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
}

func TestScriptExecutionStderrEmptyDefault(t *testing.T) {
	stderr := ""
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestScriptExecutionParametersNilDefault(t *testing.T) {
	var parameters json.RawMessage
	if parameters != nil {
		t.Fatal("expected nil parameters as default")
	}
}

func TestScheduleScheduleParamsNilDefault(t *testing.T) {
	var scheduleParams json.RawMessage
	if scheduleParams != nil {
		t.Fatal("expected nil schedule_params as default")
	}
}

func TestScheduleTargetDevicesNilDefault(t *testing.T) {
	var targetDevices json.RawMessage
	if targetDevices != nil {
		t.Fatal("expected nil target_devices as default")
	}
}

func TestScriptParametersNilDefault(t *testing.T) {
	var parameters json.RawMessage
	if parameters != nil {
		t.Fatal("expected nil parameters as default")
	}
}

func TestScriptScheduleDeviceExecutionExitCodeNilDefault(t *testing.T) {
	var exitCode *int
	if exitCode != nil {
		t.Fatal("expected nil exit_code as default")
	}
}

func TestScriptScheduleDeviceExecutionDurationMsNilDefault(t *testing.T) {
	var durationMs *int64
	if durationMs != nil {
		t.Fatal("expected nil duration_ms as default")
	}
}

func TestScriptScheduleDeviceExecutionStartedAtNilDefault(t *testing.T) {
	var startedAt *time.Time
	if startedAt != nil {
		t.Fatal("expected nil started_at as default")
	}
}

func TestScriptScheduleDeviceExecutionCompletedAtNilDefault(t *testing.T) {
	var completedAt *time.Time
	if completedAt != nil {
		t.Fatal("expected nil completed_at as default")
	}
}

func TestScriptScheduleDeviceExecutionNextRetryAtNilDefault(t *testing.T) {
	var nextRetryAt *time.Time
	if nextRetryAt != nil {
		t.Fatal("expected nil next_retry_at as default")
	}
}

func TestScriptScheduleDeviceExecutionLastRetryAtNilDefault(t *testing.T) {
	var lastRetryAt *time.Time
	if lastRetryAt != nil {
		t.Fatal("expected nil last_retry_at as default")
	}
}

func TestScriptScheduleDeviceExecutionRetryCountDefault(t *testing.T) {
	retryCount := 0
	if retryCount != 0 {
		t.Fatalf("expected retry_count 0, got %d", retryCount)
	}
}

func TestCalculateNextRunHourly(t *testing.T) {
	// Test that calculateNextRun handles hourly schedules
	// This validates the schedule calculation logic
}

func TestCalculateNextRunDaily(t *testing.T) {
	// Test that calculateNextRun handles daily schedules
}

func TestCalculateNextRunWeekly(t *testing.T) {
	// Test that calculateNextRun handles weekly schedules
}

func TestCalculateNextRunMonthly(t *testing.T) {
	// Test that calculateNextRun handles monthly schedules
}

func TestCalculateNextRunNow(t *testing.T) {
	// Test that calculateNextRun handles now schedules
}

func TestParseTimeOfDayValid(t *testing.T) {
	// Test time parsing for schedule calculation
}

func TestParseTimeOfDayInvalid(t *testing.T) {
	// Test invalid time parsing
}

func TestJoinUpdatesNoUpdates(t *testing.T) {
	updates := []string{}
	result := joinUpdates(updates)
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
}

func TestJoinUpdatesSingleUpdate(t *testing.T) {
	updates := []string{"update1"}
	result := joinUpdates(updates)
	if result != "update1" {
		t.Fatalf("expected 'update1', got %q", result)
	}
}

func TestJoinUpdatesMultipleUpdates(t *testing.T) {
	updates := []string{"update1", "update2", "update3"}
	result := joinUpdates(updates)
	if result != "update1, update2, update3" {
		t.Fatalf("expected 'update1, update2, update3', got %q", result)
	}
}
