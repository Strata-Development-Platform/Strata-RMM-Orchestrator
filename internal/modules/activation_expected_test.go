package modules

import (
	"archive/tar"
	"errors"
	"path/filepath"
	"testing"
)

func TestActivateExpectedPreviousVersionIsRetrySafe(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	pkg1, payload1 := testVerifiedMaterializePayload(t, "1.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("one")}})
	pkg2, payload2 := testVerifiedMaterializePayload(t, "2.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("two")}})
	if _, err := MaterializePayload(pkg1, payload1, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializePayload(pkg2, payload2, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, pkg1.Manifest.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	expected, err := ActivateMaterializedVersion(root, pkg1.Manifest.ID, "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if expected.PreviousVersion != "1.0.0" {
		t.Fatalf("previous=%q want 1.0.0", expected.PreviousVersion)
	}

	first, err := ActivateExpectedPreviousVersion(root, pkg1.Manifest.ID, expected)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ActivateExpectedPreviousVersion(root, pkg1.Manifest.ID, expected)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if first != second || second.ActiveVersion != "1.0.0" || second.PreviousVersion != "2.0.0" {
		t.Fatalf("unexpected retry state: first=%+v second=%+v", first, second)
	}
}

func TestActivateExpectedPreviousVersionRejectsConcurrentChange(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	pkg1, payload1 := testVerifiedMaterializePayload(t, "1.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("one")}})
	pkg2, payload2 := testVerifiedMaterializePayload(t, "2.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("two")}})
	pkg3, payload3 := testVerifiedMaterializePayload(t, "3.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("three")}})
	for _, item := range []struct {
		pkg     VerifiedPackage
		payload ValidatedPayload
	}{{pkg1, payload1}, {pkg2, payload2}, {pkg3, payload3}} {
		if _, err := MaterializePayload(item.pkg, item.payload, MaterializeOptions{Root: root}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ActivateMaterializedVersion(root, pkg1.Manifest.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	expected, err := ActivateMaterializedVersion(root, pkg1.Manifest.ID, "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, pkg1.Manifest.ID, "3.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateExpectedPreviousVersion(root, pkg1.Manifest.ID, expected); !errors.Is(err, ErrActivationStateChanged) {
		t.Fatalf("error=%v want ErrActivationStateChanged", err)
	}
}

func TestActivateExpectedPreviousVersionRequiresPrevious(t *testing.T) {
	if _, err := ActivateExpectedPreviousVersion(t.TempDir(), "com.example.backup", ActivationState{SchemaVersion: activationStateSchemaVersion, ActiveVersion: "1.0.0"}); !errors.Is(err, ErrNoRollbackVersion) {
		t.Fatalf("error=%v want ErrNoRollbackVersion", err)
	}
}
