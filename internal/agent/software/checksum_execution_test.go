package software

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestChecksumMismatchNeverExecutesDownloadedPayload(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "executed.marker")
	script := fmt.Sprintf("#!/bin/sh\nprintf executed > %q\n", marker)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(script))
	}))
	defer server.Close()

	// Deliberately hash different content so the production download verifier
	// rejects the payload before chmod or any installer execution boundary.
	sum := sha256.Sum256([]byte("different payload"))
	wrongChecksum := hex.EncodeToString(sum[:])

	inst := NewInstaller(nil, zap.NewNop(), "tenant-checksum", "agent-checksum")
	cmd := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "checksum-no-exec",
		Action:       "install",
		SourceURL:    server.URL + "/install.sh",
		Checksum:     wrongChecksum,
		PackageType:  "script",
		Timeout:      30,
	}

	result := inst.executeWithContext(context.Background(), cmd)
	if result.Status != "failed" {
		t.Fatalf("expected checksum failure, got status=%s error=%q", result.Status, result.ErrorMessage)
	}
	if result.ErrorMessage == "" {
		t.Fatal("expected checksum failure detail")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("payload executed despite checksum mismatch: marker stat err=%v", err)
	}
}
