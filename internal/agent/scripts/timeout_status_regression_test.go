package scripts

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyScriptTimeoutUsesCanonicalFailedStatus(t *testing.T) {
	source, err := os.ReadFile("executor.go")
	if err != nil {
		t.Fatalf("read executor.go: %v", err)
	}
	text := string(source)

	if strings.Contains(text, `result.Status = "timeout"`) {
		t.Fatal("legacy script executor must not publish timeout as a separate terminal status")
	}
	for _, required := range []string{
		`case ctx.Err() == context.DeadlineExceeded:`,
		`result.Status = "failed"`,
		`script execution timed out`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("legacy timeout normalization is missing %q", required)
		}
	}
}
