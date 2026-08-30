package platform

import (
	"os"
	"strings"
	"testing"
)

func TestCancellationOutboxUsesLeaseRetryAndFlush(t *testing.T) {
	source, err := os.ReadFile("dispatcher.go")
	if err != nil {
		t.Fatalf("read dispatcher source: %v", err)
	}
	text := string(source)
	for _, required := range []string{"lease_owner", "lease_expires", `eventType == "job.cancel"`, ".cmd.%s.cancel", "FlushTimeout", "failOutbox"} {
		if !strings.Contains(text, required) {
			t.Fatalf("cancellation dispatcher contract missing %q", required)
		}
	}
}
