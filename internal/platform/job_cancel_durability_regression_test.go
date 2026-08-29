package platform

import (
	"os"
	"strings"
	"testing"
)

func TestJobCancellationUsesDurableOutbox(t *testing.T) {
	jobsSource, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("read jobs source: %v", err)
	}
	jobsText := string(jobsSource)

	if strings.Contains(jobsText, ".cmd.%s.cancel") || strings.Contains(jobsText, "publish job cancellation") {
		t.Fatal("request-path job cancellation must not publish directly to NATS")
	}
	if !strings.Contains(jobsText, "'job.cancel'") {
		t.Fatal("job cancellation must persist a job.cancel outbox event")
	}
	if !strings.Contains(jobsText, "BeginTx") && !strings.Contains(jobsText, ".Begin()") {
		t.Fatal("job cancellation state and outbox intent must share a database transaction")
	}
}

func TestDispatcherPublishesCancellationOutboxEvents(t *testing.T) {
	dispatcherSource, err := os.ReadFile("dispatcher.go")
	if err != nil {
		t.Fatalf("read dispatcher source: %v", err)
	}
	text := string(dispatcherSource)

	if !strings.Contains(text, "job.cancel") {
		t.Fatal("dispatcher must recognize job.cancel outbox events")
	}
	if !strings.Contains(text, ".cmd.%s.cancel") {
		t.Fatal("dispatcher must publish cancellation events on the endpoint cancellation subject")
	}
}
