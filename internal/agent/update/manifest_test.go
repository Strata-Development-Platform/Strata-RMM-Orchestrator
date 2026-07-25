package update

import (
	"testing"
	"time"
)

func TestParseManifest(t *testing.T) {
	data := []byte(`{
		"version": "1.2.3",
		"release_date": "2026-07-25T00:00:00Z",
		"channel": "stable",
		"min_agent_version": "1.0.0",
		"platforms": {
			"linux/amd64": {
				"url": "https://releases.example.com/agent-linux-amd64",
				"checksum": "abc123",
				"checksum_type": "sha256",
				"size": 4194304
			}
		},
		"changelog": "Bug fixes",
		"signature": {
			"bundle_url": "https://releases.example.com/agent-linux-amd64.sig",
			"algorithm": "cosign"
		}
	}`)

	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}

	if m.Version != "1.2.3" {
		t.Errorf("version: got %s, want 1.2.3", m.Version)
	}
	if m.Channel != ChannelStable {
		t.Errorf("channel: got %s, want stable", m.Channel)
	}
	if !m.ReleaseDate.Equal(time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("release_date: got %v", m.ReleaseDate)
	}

	art, err := m.ArtifactFor("linux", "amd64")
	if err != nil {
		t.Fatalf("ArtifactFor: %v", err)
	}
	if art.URL != "https://releases.example.com/agent-linux-amd64" {
		t.Errorf("artifact url: got %s", art.URL)
	}
}

func TestParseManifestInvalid(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty version", `{"version": "", "platforms": {"linux/amd64": {}}}`},
		{"no platforms", `{"version": "1.0.0"}`},
		{"invalid json", `{version: 1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tt.data))
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestArtifactFor(t *testing.T) {
	m := &Manifest{
		Version: "1.0.0",
		Platforms: map[string]PlatformArtifact{
			"linux/amd64":   {URL: "linux-url"},
			"windows/amd64": {URL: "windows-url"},
		},
	}

	if _, err := m.ArtifactFor("linux", "amd64"); err != nil {
		t.Errorf("linux/amd64: %v", err)
	}
	if _, err := m.ArtifactFor("windows", "amd64"); err != nil {
		t.Errorf("windows/amd64: %v", err)
	}
	if _, err := m.ArtifactFor("darwin", "amd64"); err == nil {
		t.Error("expected error for unsupported platform")
	}
}
