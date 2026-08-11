package modules

import (
	"archive/tar"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistReleaseMetadataIsIdempotentForSameSignedRelease(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "9.0.0", []payloadArchiveEntry{
		{name: "module.wasm", typeflag: tar.TypeReg, mode: 0o500, data: []byte("trusted")},
	})
	root := filepath.Join(t.TempDir(), "modules")
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	first, err := PersistReleaseMetadata(root, pkg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PersistReleaseMetadata(root, pkg)
	if err != nil {
		t.Fatalf("idempotent persist: %v", err)
	}
	if !releaseMetadataEqual(first, second) {
		t.Fatalf("metadata changed on retry: first=%+v second=%+v", first, second)
	}
	read, err := ReadReleaseMetadata(root, pkg.Manifest.ID, pkg.Manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !releaseMetadataEqual(read, first) {
		t.Fatalf("read metadata=%+v want=%+v", read, first)
	}
}

func TestPersistReleaseMetadataRejectsConflictingExistingRecord(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "9.0.1", []payloadArchiveEntry{
		{name: "module.wasm", typeflag: tar.TypeReg, mode: 0o500, data: []byte("trusted")},
	})
	root := filepath.Join(t.TempDir(), "modules")
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := PersistReleaseMetadata(root, pkg); err != nil {
		t.Fatal(err)
	}

	metadataPath := filepath.Join(root, pkg.Manifest.ID, ".releases", pkg.Manifest.Version+".json")
	if err := os.WriteFile(metadataPath, []byte(`{"schema_version":1,"manifest":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PersistReleaseMetadata(root, pkg); err == nil {
		t.Fatal("corrupt existing release metadata was accepted")
	}
}

func TestReadReleaseMetadataRejectsSymlink(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	pkg, payload := testVerifiedMaterializePayload(t, "9.0.2", []payloadArchiveEntry{
		{name: "module.wasm", typeflag: tar.TypeReg, mode: 0o500, data: []byte("trusted")},
	})
	root := filepath.Join(t.TempDir(), "modules")
	// Plain materialization creates only the immutable payload. The test then
	// supplies a hostile release-metadata pathname without the retry-safe layer
	// pre-populating the legitimate metadata record first.
	if _, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	releaseDir := filepath.Join(root, pkg.Manifest.ID, ".releases")
	if err := os.MkdirAll(releaseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(releaseDir, pkg.Manifest.Version+".json")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReleaseMetadata(root, pkg.Manifest.ID, pkg.Manifest.Version); !errors.Is(err, ErrReleaseMetadataInvalid) {
		t.Fatalf("error=%v, want ErrReleaseMetadataInvalid", err)
	}
}
