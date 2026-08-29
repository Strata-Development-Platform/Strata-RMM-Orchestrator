package platform

import (
	"os"
	"strings"
	"testing"
)

func TestCancellationOutboxFinalizesOnlyAfterBrokerFlush(t *testing.T) {
	source, err := os.ReadFile("dispatcher.go")
	if err != nil {
		t.Fatalf("read dispatcher source: %v", err)
	}
	text := string(source)
	cancelStart := strings.Index(text, `if eventType == "job.cancel"`)
	if cancelStart < 0 {
		t.Fatal("missing job.cancel branch")
	}
	cancelText := text[cancelStart:]
	flush := strings.Index(cancelText, "FlushTimeout")
	published := strings.Index(cancelText, "published_at = NOW()")
	if flush < 0 || published < 0 || flush > published {
		t.Fatal("cancellation outbox must flush broker delivery before marking event published")
	}
}
