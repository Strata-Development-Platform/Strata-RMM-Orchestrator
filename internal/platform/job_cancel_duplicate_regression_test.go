package platform

import (
	"os"
	"strings"
	"testing"
)

func TestCancellationPayloadUsesStableEventIdentity(t *testing.T) {
	source, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("read jobs source: %v", err)
	}
	if !strings.Contains(string(source), `fmt.Sprintf("%s:%s:cancel", jobID, target.id)`) {
		t.Fatal("cancellation event identity must be stable per job target")
	}
}
