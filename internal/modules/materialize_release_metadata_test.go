package modules

import (
	"archive/tar"
	"path/filepath"
	"testing"
)

func TestMaterializePayloadRetrySafePersistsReleaseMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	pkg, payload := testVerifiedMaterializePayload(t, "8.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("release")}})
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	metadata, err := ReadReleaseMetadata(root, pkg.Manifest.ID, pkg.Manifest.Version)
	if err != nil {
		t.Fatalf("read release metadata: %v", err)
	}
	if metadata.Manifest.Version != pkg.Manifest.Version || metadata.PayloadSHA256 != pkg.PayloadSHA256 || metadata.KeyID != pkg.PublisherKey.KeyID {
		t.Fatalf("metadata=%+v package=%+v", metadata, pkg)
	}

	// A crash/retry path that encounters the existing immutable payload must
	// converge on the same release record rather than replacing it.
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatalf("retry materialization: %v", err)
	}
	retried, err := ReadReleaseMetadata(root, pkg.Manifest.ID, pkg.Manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !releaseMetadataEqual(metadata, retried) {
		t.Fatalf("release metadata changed across retry: first=%+v retry=%+v", metadata, retried)
	}
}
