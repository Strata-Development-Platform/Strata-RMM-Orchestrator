package platform

import (
	"os"
	"strings"
	"testing"
)

func TestScheduledTimeoutIsRetryableAndTerminal(t *testing.T) {
	source, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("read jobs.go: %v", err)
	}
	text := string(source)

	for _, required := range []string{
		`func isScheduleExecutionFailure(status string) bool`,
		`return status == "failed" || status == "timeout"`,
		`func isScheduleExecutionTerminal(status string) bool`,
		`isScheduleExecutionFailure(status) || status == "completed" || status == "success" || status == "succeeded"`,
		`isScheduleExecutionFailure(status), execID`,
		`if isScheduleExecutionFailure(execStatus) && retryCount < maxRetries`,
		`if isScheduleExecutionFailure(execStatus) && retryCount >= maxRetries`,
		`if isScheduleExecutionTerminal(execStatus)`,
		`status IN ('failed', 'timeout')`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("scheduled timeout accounting contract missing %q", required)
		}
	}
}

func TestScheduledTimeoutPreservesExecutionIdentityGuards(t *testing.T) {
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
		"WHERE id = $8",
		"RowsAffected()",
		"affected == 0",
		"execution is stale or unknown",
		"JOIN schedules s ON s.id = sde.schedule_id",
		"s.max_retries",
	} {
		if !strings.Contains(handler, required) {
			t.Fatalf("scheduled timeout change weakened correlation guard %q", required)
		}
	}
}
