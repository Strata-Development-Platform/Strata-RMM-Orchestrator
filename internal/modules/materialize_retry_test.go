package modules

import (
	"archive/tar"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializePayloadRetrySafeAcceptsExactExistingVersion(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "8.0.0", []payloadArchiveEntry{
		{name: "bin/module.wasm", typeflag: tar.TypeReg, mode: 0o500, data: []byte("wasm-bytes")},
		{name: "config/default.json", typeflag: tar.TypeReg, mode: 0o400, data: []byte(`{"safe":true}`)},
	})
	root := filepath.Join(t.TempDir(), "modules")
	first, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	second, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root})
	if err != nil {
		t.Fatalf("retry returned error: %v", err)
	}
	if second.Path != first.Path || second.PayloadSHA256 != first.PayloadSHA256 || second.FileCount != first.FileCount {
		t.Fatalf("retry result=%+v, first=%+v", second, first)
	}
}

func TestMaterializePayloadRetrySafeRejectsChangedExistingVersion(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "8.0.1", []payloadArchiveEntry{
		{name: "module.wasm", typeflag: tar.TypeReg, mode: 0o500, data: []byte("trusted")},
	})
	root := filepath.Join(t.TempDir(), "modules")
	result, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.Path, "module.wasm"), []byte("changed"), 0o500); err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); !errors.Is(err, ErrMaterializedVersionExists) {
		t.Fatalf("error=%v, want ErrMaterializedVersionExists", err)
	}
}

func TestMaterializePayloadRetrySafeRejectsUnexpectedExistingFile(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "8.0.2", []payloadArchiveEntry{
		{name: "module.wasm", typeflag: tar.TypeReg, mode: 0o500, data: []byte("trusted")},
	})
	root := filepath.Join(t.TempDir(), "modules")
	result, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.Path, "unexpected"), []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); !errors.Is(err, ErrMaterializedVersionExists) {
		t.Fatalf("error=%v, want ErrMaterializedVersionExists", err)
	}
}
