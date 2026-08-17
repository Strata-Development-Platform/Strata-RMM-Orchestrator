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
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSoftwareExecutionFailureMatrixIsTerminalAndBounded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script execution matrix uses POSIX shell fixtures; Windows behavior requires representative-host evidence")
	}

	installer := NewInstaller(nil, nil, "tenant-test", "agent-test")

	t.Run("download failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		defer server.Close()

		result := installer.executeWithContext(context.Background(), SoftwareCommand{
			Type: "software_install", DeploymentID: "download-failure", Action: "install",
			SourceURL: server.URL + "/package.sh", PackageType: "script", Timeout: 5,
		})
		assertTerminalSoftwareFailure(t, result, "HTTP 503")
	})

	t.Run("unsupported package rejected before download", func(t *testing.T) {
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests++
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		result := installer.executeWithContext(context.Background(), SoftwareCommand{
			Type: "software_install", DeploymentID: "unsupported", Action: "install",
			SourceURL: server.URL + "/package.bin", PackageType: "unknown", Timeout: 5,
		})
		assertTerminalSoftwareFailure(t, result, "unsupported package type")
		if requests != 0 {
			t.Fatalf("unsupported package reached download boundary: requests=%d", requests)
		}
	})

	t.Run("timeout kills execution and records timeout", func(t *testing.T) {
		script := "#!/bin/sh\nsleep 10\nexit 0\n"
		server, checksum := softwareScriptServer(t, script)
		defer server.Close()

		started := time.Now()
		result := installer.executeWithContext(context.Background(), SoftwareCommand{
			Type: "software_install", DeploymentID: "timeout", Action: "install",
			SourceURL: server.URL + "/slow.sh", Checksum: checksum, PackageType: "script", Timeout: 1,
		})
		assertTerminalSoftwareFailure(t, result, "timeout")
		if elapsed := time.Since(started); elapsed > 4*time.Second {
			t.Fatalf("timeout did not bound execution: elapsed=%v", elapsed)
		}
	})

	t.Run("parent cancellation kills in-flight execution", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "started.marker")
		script := fmt.Sprintf("#!/bin/sh\nprintf started > %q\nsleep 10\nexit 0\n", marker)
		server, checksum := softwareScriptServer(t, script)
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		resultCh := make(chan SoftwareResult, 1)
		go func() {
			resultCh <- installer.executeWithContext(ctx, SoftwareCommand{
				Type: "software_install", DeploymentID: "cancelled", Action: "install",
				SourceURL: server.URL + "/cancel.sh", Checksum: checksum, PackageType: "script", Timeout: 30,
			})
		}()

		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(marker); err == nil {
				cancel()
				select {
				case result := <-resultCh:
					assertTerminalSoftwareFailure(t, result, "cancelled")
					return
				case <-time.After(3 * time.Second):
					t.Fatal("cancelled software execution did not terminate")
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		cancel()
		t.Fatal("software script never entered execution before cancellation")
	})

	t.Run("uninstall nonzero exit is recorded", func(t *testing.T) {
		script := "#!/bin/sh\nexit 9\n"
		server, checksum := softwareScriptServer(t, script)
		defer server.Close()

		result := installer.executeWithContext(context.Background(), SoftwareCommand{
			Type: "software_uninstall", DeploymentID: "uninstall-failure", Action: "uninstall",
			SourceURL: server.URL + "/uninstall.sh", Checksum: checksum, PackageType: "script", Timeout: 5,
		})
		assertTerminalSoftwareFailure(t, result, "exit code: 9")
	})
}

func softwareScriptServer(t *testing.T, script string) (*httptest.Server, string) {
	t.Helper()
	sum := sha256.Sum256([]byte(script))
	checksum := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(script))
	}))
	return server, checksum
}

func assertTerminalSoftwareFailure(t *testing.T, result SoftwareResult, contains string) {
	t.Helper()
	if result.Status != "failed" {
		t.Fatalf("status=%q, want failed; result=%+v", result.Status, result)
	}
	if !strings.Contains(result.ErrorMessage, contains) {
		t.Fatalf("error=%q, want substring %q", result.ErrorMessage, contains)
	}
	if len(result.ErrorMessage) > maxSoftwareErrorBytes {
		t.Fatalf("error message exceeded bound: %d > %d", len(result.ErrorMessage), maxSoftwareErrorBytes)
	}
}
