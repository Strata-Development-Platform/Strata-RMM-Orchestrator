package update

import (
	"runtime"
	"testing"
)

func TestReleaseManifestOCICandidate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Docker promoted-release apply is supported only on linux hosts")
	}
	manifest := validManifest()
	candidate, ok, err := manifest.OCICandidate("1.9.9", manifest.OCIImages[0].Reference)
	if err != nil {
		t.Fatalf("OCICandidate() error = %v", err)
	}
	if !ok {
		t.Fatal("OCICandidate() did not select an available upgrade")
	}
	want := manifest.OCIImages[0].Reference + "@" + manifest.OCIImages[0].Digest
	if candidate.Image != want {
		t.Fatalf("candidate.Image = %q, want %q", candidate.Image, want)
	}
	if candidate.SourceSHA != manifest.SourceSHA || candidate.SchemaCompatibility != manifest.SchemaCompatibility {
		t.Fatalf("candidate lost signed release binding: %+v", candidate)
	}
}

func TestReleaseManifestOCICandidateFailsClosed(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Docker promoted-release apply is supported only on linux hosts")
	}
	base := validManifest()
	tests := []struct {
		name       string
		current    string
		repository string
		mutate     func(*ReleaseManifest)
		wantOK     bool
		wantErr    bool
	}{
		{name: "below minimum version", current: "1.1.9", repository: base.OCIImages[0].Reference, wantOK: false},
		{name: "same version", current: "1.10.0", repository: base.OCIImages[0].Reference, wantOK: false},
		{name: "wrong repository", current: "1.9.9", repository: "ghcr.io/example/other", wantErr: true},
		{name: "bad signature", current: "1.9.9", repository: base.OCIImages[0].Reference, mutate: func(m *ReleaseManifest) { m.OCIImages[0].Signature = "none" }, wantErr: true},
		{name: "bad digest", current: "1.9.9", repository: base.OCIImages[0].Reference, mutate: func(m *ReleaseManifest) { m.OCIImages[0].Digest = "latest" }, wantErr: true},
		{name: "bad schema", current: "1.9.9", repository: base.OCIImages[0].Reference, mutate: func(m *ReleaseManifest) { m.SchemaCompatibility = "" }, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifest()
			if test.mutate != nil {
				test.mutate(&manifest)
			}
			_, ok, err := manifest.OCICandidate(test.current, test.repository)
			if (err != nil) != test.wantErr {
				t.Fatalf("OCICandidate() error = %v, wantErr %v", err, test.wantErr)
			}
			if ok != test.wantOK {
				t.Fatalf("OCICandidate() ok = %v, want %v", ok, test.wantOK)
			}
		})
	}
}
