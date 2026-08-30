package platform

import (
	"os"
	"strings"
	"testing"
)

func TestCancellationDispatcherUsesDedicatedAgentSubject(t *testing.T) {
	source, err := os.ReadFile("dispatcher.go")
	if err != nil {
		t.Fatalf("read dispatcher source: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, `fmt.Sprintf("tenant.%s.cmd.%s.cancel", mspID, agentID)`) {
		t.Fatal("durable cancellation must preserve the agent's dedicated cancellation subject")
	}
}
