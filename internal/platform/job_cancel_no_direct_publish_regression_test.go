package platform

import (
	"os"
	"strings"
	"testing"
)

func TestCancelHandlerHasNoRequestPathNATSPublish(t *testing.T) {
	source, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("read jobs source: %v", err)
	}
	text := string(source)
	start := strings.Index(text, "func (s *APIServer) handleCancelJob")
	end := strings.Index(text[start:], "func (s *APIServer) handleRetryJobTargets")
	if start < 0 || end < 0 {
		t.Fatal("could not isolate cancellation handler")
	}
	if strings.Contains(text[start:start+end], ".Publish(") {
		t.Fatal("cancellation handler must not publish to NATS directly")
	}
}
