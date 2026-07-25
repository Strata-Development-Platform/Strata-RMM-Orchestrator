package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestClientShouldCheck(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	s.Init()
	c := NewClient(ClientOptions{
		Store:         s,
		CheckInterval: time.Hour,
	})

	if !c.ShouldCheck(time.Now().Add(-2 * time.Hour)) {
		t.Error("should need check")
	}
	if c.ShouldCheck(time.Now()) {
		t.Error("should not need check")
	}
}

func TestClientCheckAndDownload(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	s.Init()
	s.SetCurrentVersion("1.0.0")

	binaryContent := []byte("#!/bin/sh\necho hello")
	checksum := sha256hex(binaryContent)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/update-manifest-stable.json" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"version": "2.0.0",
				"release_date": "2026-07-25T00:00:00Z",
				"channel": "stable",
				"min_agent_version": "1.0.0",
				"platforms": {
					"linux/amd64": {
						"url": "` + serverURL(r) + `/binary",
						"checksum": "` + checksum + `",
						"checksum_type": "sha256",
						"size": ` + itoa(len(binaryContent)) + `
					}
				},
				"changelog": "New version",
				"signature": {
					"bundle_url": "https://example.com/sig",
					"algorithm": "cosign"
				}
			}`))
			return
		}
		if r.URL.Path == "/binary" {
			w.Write(binaryContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	dataDir := t.TempDir()

	c := NewClient(ClientOptions{
		ManifestURL: server.URL,
		Store:       s,
		DataDir:     dataDir,
		Channel:     ChannelStable,
	})

	manifest, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if manifest == nil {
		t.Fatal("expected manifest, got nil")
	}
	if manifest.Version != "2.0.0" {
		t.Errorf("version: got %s, want 2.0.0", manifest.Version)
	}

	binaryPath, err := c.Download(context.Background(), manifest)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	data, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read downloaded: %v", err)
	}
	if string(data) != string(binaryContent) {
		t.Errorf("content mismatch")
	}

	state, _ := s.GetState()
	if state.PendingVersion != "2.0.0" {
		t.Errorf("pending: got %s, want 2.0.0", state.PendingVersion)
	}
}

func TestClientCurrentVersion(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	s.Init()

	c := NewClient(ClientOptions{Store: s})

	if v := c.CurrentVersion(); v != "" {
		t.Errorf("expected empty, got %s", v)
	}

	s.SetCurrentVersion("1.5.0")
	if v := c.CurrentVersion(); v != "1.5.0" {
		t.Errorf("got %s, want 1.5.0", v)
	}
}

func TestRolloutConfigShouldApply(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	s.Init()

	c := NewClient(ClientOptions{Store: s})

	rm := NewRolloutManager(nil, s, c, "test-agent", "test-tenant")
	rm.config.Paused = true

	if rm.ShouldApply("1.0.0") {
		t.Error("should not apply when paused")
	}

	rm.config.Paused = false
	rm.config.BlockedVersions = []string{"1.0.0"}
	if rm.ShouldApply("1.0.0") {
		t.Error("should not apply blocked version")
	}

	rm.config.BlockedVersions = nil
	rm.config.ApprovedVersion = "2.0.0"
	if rm.ShouldApply("1.0.0") {
		t.Error("should not apply non-approved version")
	}
	if !rm.ShouldApply("2.0.0") {
		t.Error("should apply approved version")
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
