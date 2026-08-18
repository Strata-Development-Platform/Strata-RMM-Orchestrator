package update

import "testing"

func validManifest() ReleaseManifest {
	return ReleaseManifest{
		SchemaVersion:         ReleaseManifestSchemaVersion,
		Version:               "1.10.0",
		SourceSHA:             "0123456789abcdef0123456789abcdef01234567",
		BuildTimestamp:        "2026-08-17T20:00:00Z",
		SchemaCompatibility:   "00096",
		MinimumUpgradeVersion: "1.2.0",
		Channel:               "stable",
		Artifacts: []ReleaseArtifact{{
			Name:           "strata-rmm-orchestrator-1.10.0-linux-amd64",
			Kind:           "orchestrator",
			OS:             "linux",
			Arch:           "amd64",
			Size:           123,
			SHA256:         "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SigstoreBundle: "strata-rmm-orchestrator-1.10.0-linux-amd64.sigstore.json",
		}},
		OCIImages: []ReleaseOCIImage{{
			Reference: "ghcr.io/strata-development-platform/strata-rmm-orchestrator",
			Digest:    "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Platforms: []string{"linux/amd64", "linux/arm64"},
			Signature: "sigstore-keyless",
		}},
	}
}

func TestReleaseManifestValidate(t *testing.T) {
	manifest := validManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestReleaseManifestValidateFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReleaseManifest)
	}{
		{name: "schema", mutate: func(m *ReleaseManifest) { m.SchemaVersion = 2 }},
		{name: "version", mutate: func(m *ReleaseManifest) { m.Version = "latest" }},
		{name: "source sha", mutate: func(m *ReleaseManifest) { m.SourceSHA = "master" }},
		{name: "timestamp", mutate: func(m *ReleaseManifest) { m.BuildTimestamp = "yesterday" }},
		{name: "schema compatibility", mutate: func(m *ReleaseManifest) { m.SchemaCompatibility = "" }},
		{name: "minimum version", mutate: func(m *ReleaseManifest) { m.MinimumUpgradeVersion = "1.2" }},
		{name: "channel", mutate: func(m *ReleaseManifest) { m.Channel = "latest" }},
		{name: "no artifacts", mutate: func(m *ReleaseManifest) { m.Artifacts = nil }},
		{name: "bad checksum", mutate: func(m *ReleaseManifest) { m.Artifacts[0].SHA256 = "deadbeef" }},
		{name: "missing bundle", mutate: func(m *ReleaseManifest) { m.Artifacts[0].SigstoreBundle = "" }},
		{name: "no oci image", mutate: func(m *ReleaseManifest) { m.OCIImages = nil }},
		{name: "mutable oci reference", mutate: func(m *ReleaseManifest) { m.OCIImages[0].Reference += "@sha256:deadbeef" }},
		{name: "bad oci digest", mutate: func(m *ReleaseManifest) { m.OCIImages[0].Digest = "latest" }},
		{name: "bad oci platforms", mutate: func(m *ReleaseManifest) { m.OCIImages[0].Platforms = []string{"linux/amd64"} }},
		{name: "missing oci signature", mutate: func(m *ReleaseManifest) { m.OCIImages[0].Signature = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifest()
			test.mutate(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid release manifest")
			}
		})
	}
}

func TestReleaseManifestAllowsUpgradeFrom(t *testing.T) {
	manifest := validManifest()
	for _, test := range []struct {
		current string
		want    bool
	}{
		{current: "1.2.0", want: true},
		{current: "1.9.9", want: true},
		{current: "1.10.0", want: false},
		{current: "1.11.0", want: false},
		{current: "1.1.9", want: false},
	} {
		got, err := manifest.AllowsUpgradeFrom(test.current)
		if err != nil {
			t.Fatalf("AllowsUpgradeFrom(%q) error = %v", test.current, err)
		}
		if got != test.want {
			t.Fatalf("AllowsUpgradeFrom(%q) = %v, want %v", test.current, got, test.want)
		}
	}
}

func TestReleaseManifestArtifact(t *testing.T) {
	manifest := validManifest()
	artifact, ok := manifest.Artifact("strata-rmm-orchestrator-1.10.0-linux-amd64")
	if !ok || artifact.Arch != "amd64" {
		t.Fatalf("Artifact() = %+v, %v", artifact, ok)
	}
	if _, ok := manifest.Artifact("missing"); ok {
		t.Fatal("Artifact() found unknown artifact")
	}
}
