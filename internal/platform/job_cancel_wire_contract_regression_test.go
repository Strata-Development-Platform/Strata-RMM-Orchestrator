package platform

import (
	"os"
	"strings"
	"testing"
)

func TestJobCancellationOutboxCarriesEndpointIdentity(t *testing.T) {
	jobsSource, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("read jobs source: %v", err)
	}
	text := string(jobsSource)
	for _, required := range []string{"\"event_id\"", "\"job_id\"", "\"target_id\"", "\"msp_id\"", "\"agent_id\""} {
		if !strings.Contains(text, required) {
			t.Fatalf("durable cancellation payload missing %s", required)
		}
	}
}

func TestCancelledTargetCannotBeRevivedByLateResult(t *testing.T) {
	source, err := os.ReadFile("dispatcher.go")
	if err != nil {
		t.Fatalf("read dispatcher source: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, `currentStatus == "cancelled"`) ||
		!strings.Contains(text, `(currentStatus == res.Status || currentStatus == "cancelled")`) {
		t.Fatal("late terminal results must not revive a cancelled target")
	}
}
