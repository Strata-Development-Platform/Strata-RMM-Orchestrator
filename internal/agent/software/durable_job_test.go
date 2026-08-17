package software

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRunDurableJobRejectsMalformedPayload(t *testing.T) {
	inst := &Installer{}
	status, code, _, err := inst.RunDurableJob(context.Background(), json.RawMessage(`{"type":`), "install")
	if err == nil {
		t.Fatal("expected malformed durable payload to fail")
	}
	if status != "failed" || code == 0 {
		t.Fatalf("status/code = %q/%d, want failed/nonzero", status, code)
	}
}

func TestRunDurableJobRejectsActionMismatch(t *testing.T) {
	inst := &Installer{}
	payload := json.RawMessage(`{
		"type":"software_uninstall",
		"deployment_id":"deployment-1",
		"action":"uninstall",
		"source_url":"https://example.invalid/package.msi",
		"checksum":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"package_type":"msi",
		"timeout":600
	}`)
	status, code, _, err := inst.RunDurableJob(context.Background(), payload, "install")
	if err == nil {
		t.Fatal("expected durable handler action mismatch to fail")
	}
	if status != "failed" || code == 0 {
		t.Fatalf("status/code = %q/%d, want failed/nonzero", status, code)
	}
}
