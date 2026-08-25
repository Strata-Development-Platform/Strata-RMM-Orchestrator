package platform

import (
	"os"
	"strings"
	"testing"
)

func TestScheduledScriptRedispatchRotatesPersistedExecutionIdentity(t *testing.T) {
	source, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("read jobs.go: %v", err)
	}
	text := string(source)

	for _, required := range []string{
		"const resetScheduleExecutionOnConflict",
		"ON CONFLICT (schedule_id, device_id) DO UPDATE SET",
		"id = EXCLUDED.id",
		"stdout = NULL",
		"stderr = NULL",
		"exit_code = NULL",
		"duration_ms = NULL",
		"next_retry_at = NULL",
		"last_retry_at = NULL",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("scheduled execution reset contract missing %q", required)
		}
	}

	if strings.Count(text, "resetScheduleExecutionOnConflict") < 3 {
		t.Fatal("both full-schedule and per-device dispatch paths must use the execution identity reset contract")
	}

	if strings.Contains(text, "ON CONFLICT (schedule_id, device_id) DO UPDATE SET status = 'pending'") {
		t.Fatal("redispatch must not retain the previous persisted execution id")
	}
}

func TestScheduledScriptResultRejectsStaleExecutionIdentity(t *testing.T) {
	source, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("read jobs.go: %v", err)
	}
	text := string(source)
	start := strings.Index(text, "func (so *ScheduleOrchestrator) ProcessScheduleDeviceResult(")
	end := strings.Index(text, "func (so *ScheduleOrchestrator) checkScheduleCompletion(")
	if start < 0 || end <= start {
		t.Fatal("could not isolate ProcessScheduleDeviceResult")
	}
	handler := text[start:end]

	for _, required := range []string{
		"RowsAffected()",
		"affected == 0",
		"execution is stale or unknown",
		"JOIN schedules s ON s.id = sde.schedule_id",
		"s.max_retries",
		"retry_count = retry_count + 1",
	} {
		if !strings.Contains(handler, required) {
			t.Fatalf("scheduled result correlation/retry contract missing %q", required)
		}
	}
}
