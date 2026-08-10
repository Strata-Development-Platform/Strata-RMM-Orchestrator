package modules

import (
	"archive/tar"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestActivateMaterializedVersionAndRollback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	pkg1, payload1 := testVerifiedMaterializePayload(t, "1.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("one")}})
	if _, err := MaterializePayload(pkg1, payload1, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	pkg2, payload2 := testVerifiedMaterializePayload(t, "2.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("two")}})
	if _, err := MaterializePayload(pkg2, payload2, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}

	state, err := ActivateMaterializedVersion(root, pkg1.Manifest.ID, pkg1.Manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveVersion != "1.0.0" || state.PreviousVersion != "" {
		t.Fatalf("first activation = %#v", state)
	}
	state, err = ActivateMaterializedVersion(root, pkg2.Manifest.ID, pkg2.Manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveVersion != "2.0.0" || state.PreviousVersion != "1.0.0" {
		t.Fatalf("second activation = %#v", state)
	}

	read, err := ReadActiveVersion(root, pkg1.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read != state {
		t.Fatalf("read state = %#v, want %#v", read, state)
	}

	rolled, err := RollbackActiveVersion(root, pkg1.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.ActiveVersion != "1.0.0" || rolled.PreviousVersion != "2.0.0" {
		t.Fatalf("rollback state = %#v", rolled)
	}
	rolledAgain, err := RollbackActiveVersion(root, pkg1.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rolledAgain.ActiveVersion != "2.0.0" || rolledAgain.PreviousVersion != "1.0.0" {
		t.Fatalf("second rollback state = %#v", rolledAgain)
	}
}

func TestActivateMaterializedVersionIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	pkg, payload := testVerifiedMaterializePayload(t, "3.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")}})
	if _, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	first, err := ActivateMaterializedVersion(root, pkg.Manifest.ID, pkg.Manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ActivateMaterializedVersion(root, pkg.Manifest.ID, pkg.Manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || second.PreviousVersion != "" {
		t.Fatalf("idempotent activation changed state: first=%#v second=%#v", first, second)
	}
}

func TestActivateMaterializedVersionRejectsMissingVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	pkg, payload := testVerifiedMaterializePayload(t, "4.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")}})
	if _, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	_, err := ActivateMaterializedVersion(root, pkg.Manifest.ID, "4.0.1")
	if err == nil {
		t.Fatal("activation accepted a version that is not materialized")
	}
	if _, statErr := os.Stat(filepath.Join(root, pkg.Manifest.ID, ".active.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("activation state unexpectedly written: %v", statErr)
	}
}

func TestRollbackRequiresPreviousVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	pkg, payload := testVerifiedMaterializePayload(t, "5.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")}})
	if _, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, pkg.Manifest.ID, pkg.Manifest.Version); err != nil {
		t.Fatal(err)
	}
	_, err := RollbackActiveVersion(root, pkg.Manifest.ID)
	if !errors.Is(err, ErrNoRollbackVersion) {
		t.Fatalf("rollback error = %v, want ErrNoRollbackVersion", err)
	}
}

func TestActivationLockBlocksConcurrentSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	pkg, payload := testVerifiedMaterializePayload(t, "6.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")}})
	materialized, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(filepath.Dir(materialized.Path), ".activation.lock")
	if err := os.WriteFile(lockPath, []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ActivateMaterializedVersion(root, pkg.Manifest.ID, pkg.Manifest.Version)
	if !errors.Is(err, ErrActivationInProgress) {
		t.Fatalf("activation error = %v, want ErrActivationInProgress", err)
	}
}

func TestReadActiveVersionRejectsSymlinkState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	root := filepath.Join(t.TempDir(), "modules")
	pkg, payload := testVerifiedMaterializePayload(t, "7.0.0", []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")}})
	materialized, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	moduleDir := filepath.Dir(materialized.Path)
	outside := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(outside, []byte(`{"schema_version":1,"active_version":"7.0.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(moduleDir, ".active.json")); err != nil {
		t.Fatal(err)
	}
	_, err = ReadActiveVersion(root, pkg.Manifest.ID)
	if !errors.Is(err, ErrInvalidActivationState) {
		t.Fatalf("read error = %v, want ErrInvalidActivationState", err)
	}
}
