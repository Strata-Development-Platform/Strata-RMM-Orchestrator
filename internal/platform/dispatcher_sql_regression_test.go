package platform

import (
	"os"
	"strings"
	"testing"
)

func TestDispatcherSQLAvoidsInboxForeignKeyLockUpgradeDeadlock(t *testing.T) {
	source, err := os.ReadFile("dispatcher.go")
	if err != nil {
		t.Fatalf("read dispatcher source: %v", err)
	}
	text := string(source)

	// ACK and result transactions insert job_inbox rows before locking the
	// referenced job target. The FK insert holds KEY SHARE, so FOR UPDATE would
	// require a conflicting lock upgrade when ACK and result arrive together.
	// NO KEY UPDATE is sufficient because these transactions change state only,
	// not the target primary/foreign-key identity, and remains compatible with
	// the FK KEY SHARE lock.
	if got := strings.Count(text, "FOR NO KEY UPDATE"); got < 2 {
		t.Fatalf("dispatcher target lock count=%d, want at least 2 FOR NO KEY UPDATE locks", got)
	}
}

func TestDispatcherRetrySQLQualifiesRetryCount(t *testing.T) {
	source, err := os.ReadFile("dispatcher.go")
	if err != nil {
		t.Fatalf("read dispatcher source: %v", err)
	}
	text := string(source)

	if strings.Contains(text, "retry_count = retry_count + 1") {
		t.Fatal("reconciliation SQL must qualify retry_count in UPDATE ... FROM jobs")
	}
	if !strings.Contains(text, "retry_count = jt.retry_count + 1") {
		t.Fatal("reconciliation SQL is missing qualified jt.retry_count increment")
	}
}
