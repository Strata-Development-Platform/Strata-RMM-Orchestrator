package platform

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDownloadAgentArchiveExtractsExpectedBinary(t *testing.T) {
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	binary := []byte("test-agent-binary")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "strata-agent", Mode: 0755, Size: int64(len(binary)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()

	releaseServer := NewReleaseServer(t.TempDir(), "owner", "repo")
	destination := releaseServer.cacheDir + "/agent-linux-amd64"
	path, err := releaseServer.downloadAgentArchive(context.Background(), server.URL, destination, "linux/amd64", "linux")
	if err != nil {
		t.Fatalf("downloadAgentArchive() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, binary) {
		t.Fatalf("extracted binary = %q, want %q", got, binary)
	}
	if releaseServer.cached["linux/amd64"] != path {
		t.Fatalf("cached path = %q, want %q", releaseServer.cached["linux/amd64"], path)
	}
}

func TestWindowsInstallScriptUsesVerifiedReleaseAndFailsClosed(t *testing.T) {
	for _, required := range []string{
		"Get-FileHash -Algorithm SHA256",
		"RMM_ENROLLMENT_TOKEN",
		"agent --config",
		"Start-Service",
		"agent did not consume the one-time enrollment token",
		"must use HTTPS outside local development",
	} {
		if !strings.Contains(agentWindowsInstallScript, required) {
			t.Fatalf("Windows installer missing %q", required)
		}
	}
	for _, unsafe := range []string{"ExecutionPolicy Bypass", "-SkipCertificateCheck", "latest.exe"} {
		if strings.Contains(agentWindowsInstallScript, unsafe) {
			t.Fatalf("Windows installer contains unsafe pattern %q", unsafe)
		}
	}
}

func TestDownloadAgentArchiveRejectsUnexpectedContents(t *testing.T) {
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	payload := []byte("not-the-agent")
	_ = tarWriter.WriteHeader(&tar.Header{Name: "README.md", Mode: 0644, Size: int64(len(payload)), Typeflag: tar.TypeReg})
	_, _ = tarWriter.Write(payload)
	_ = tarWriter.Close()
	_ = gzipWriter.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()

	releaseServer := NewReleaseServer(t.TempDir(), "owner", "repo")
	_, err := releaseServer.downloadAgentArchive(context.Background(), server.URL, releaseServer.cacheDir+"/agent-linux-amd64", "linux/amd64", "linux")
	if err == nil {
		t.Fatal("expected archive without agent binary to fail")
	}
}
