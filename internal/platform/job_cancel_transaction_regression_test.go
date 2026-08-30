package platform

import (
	"os"
	"strings"
	"testing"
)

func TestJobCancellationCommitsOnlyAfterOutboxPersistence(t *testing.T) {
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
	handler := text[start : start+end]
	begin := strings.Index(handler, "BeginTx")
	outbox := strings.Index(handler, "'job.cancel'")
	commit := strings.Index(handler, "tx.Commit()")
	if begin < 0 || outbox < 0 || commit < 0 || begin >= outbox || outbox >= commit {
		t.Fatal("cancellation must begin a transaction, persist job.cancel intent, then commit")
	}
}