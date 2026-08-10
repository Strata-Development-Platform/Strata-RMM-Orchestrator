package modules

import (
	"archive/tar"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMaterializePayloadPromotesValidatedPackageAtomically(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "1.0.0", []payloadArchiveEntry{
		{name: "bin/module", typeflag: tar.TypeReg, mode: 0o755, data: []byte("binary")},
		{name: "config/default.json", typeflag: tar.TypeReg, mode: 0o640, data: []byte(`{"enabled":true}`)},
	})
	root := filepath.Join(t.TempDir(), "modules")

	result, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root})
	if err != nil {
		t.Fatalf("MaterializePayload returned error: %v", err)
	}
	wantTarget := filepath.Join(root, pkg.Manifest.ID, pkg.Manifest.Version)
	if result.Path != wantTarget {
		t.Fatalf("materialized path = %q, want %q", result.Path, wantTarget)
	}
	if result.FileCount != 2 || result.ExpandedBytes != int64(len("binary")+len(`{"enabled":true}`)) {
		t.Fatalf("materialized result = %#v", result)
	}

	binaryPath := filepath.Join(result.Path, "bin", "module")
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "binary" {
		t.Fatalf("binary contents = %q", data)
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("binary mode = %#o, want 0755", info.Mode().Perm())
	}
	assertNoInstallScratch(t, filepath.Dir(result.Path), pkg.Manifest.Version)
}

func TestMaterializePayloadRejectsMutationAfterValidation(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "1.0.1", []payloadArchiveEntry{
		{name: "bin/module", typeflag: tar.TypeReg, mode: 0o755, data: []byte("binary")},
	})
	payload.Files[0].Data = []byte("tampered after validation")
	root := filepath.Join(t.TempDir(), "modules")

	_, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root})
	if !errors.Is(err, ErrPayloadIdentityMismatch) {
		t.Fatalf("MaterializePayload error = %v, want ErrPayloadIdentityMismatch", err)
	}
	if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("install root unexpectedly created after identity failure: %v", statErr)
	}
}

func TestMaterializePayloadRejectsDifferentPackageIdentity(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "1.0.2", []payloadArchiveEntry{
		{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")},
	})
	pkg.Manifest.Version = "1.0.3"

	_, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: filepath.Join(t.TempDir(), "modules")})
	if !errors.Is(err, ErrPayloadIdentityMismatch) {
		t.Fatalf("MaterializePayload error = %v, want ErrPayloadIdentityMismatch", err)
	}
}

func TestMaterializePayloadNeverOverwritesExistingVersion(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "2.0.0", []payloadArchiveEntry{
		{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("first")},
	})
	root := filepath.Join(t.TempDir(), "modules")
	first, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	_, err = MaterializePayload(pkg, payload, MaterializeOptions{Root: root})
	if !errors.Is(err, ErrMaterializedVersionExists) {
		t.Fatalf("second MaterializePayload error = %v, want ErrMaterializedVersionExists", err)
	}
	data, err := os.ReadFile(filepath.Join(first.Path, "module"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first" {
		t.Fatalf("existing target changed to %q", data)
	}
}

func TestMaterializePayloadRejectsConcurrentInstallLock(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "3.0.0", []payloadArchiveEntry{
		{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")},
	})
	root := filepath.Join(t.TempDir(), "modules")
	moduleDir := filepath.Join(root, pkg.Manifest.ID)
	if err := os.MkdirAll(moduleDir, 0o750); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(moduleDir, "."+pkg.Manifest.Version+".install.lock")
	if err := os.WriteFile(lockPath, []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root})
	if !errors.Is(err, ErrInstallInProgress) {
		t.Fatalf("MaterializePayload error = %v, want ErrInstallInProgress", err)
	}
}

func TestMaterializePayloadCleansStageOnFailure(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "4.0.0", []payloadArchiveEntry{
		{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")},
	})
	// Corrupt both the public data and the private seal so this test reaches the
	// filesystem loop and proves staging cleanup rather than identity rejection.
	payload.Files[0].Path = "../escape"
	payload.validationFingerprint = fingerprintValidatedPayload(payload.ModuleID, payload.Version, payload.PayloadSHA256, payload.Files)
	root := filepath.Join(t.TempDir(), "modules")

	_, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "unsafe module payload path") {
		t.Fatalf("MaterializePayload error = %v, want unsafe path rejection", err)
	}
	moduleDir := filepath.Join(root, pkg.Manifest.ID)
	assertNoInstallScratch(t, moduleDir, pkg.Manifest.Version)
	if _, statErr := os.Stat(filepath.Join(moduleDir, pkg.Manifest.Version)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("target unexpectedly exists after failed materialization: %v", statErr)
	}
}

func TestMaterializePayloadRejectsSymlinkedInstallRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	pkg, payload := testVerifiedMaterializePayload(t, "5.0.0", []payloadArchiveEntry{
		{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")},
	})
	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	if err := os.Mkdir(realRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	linkRoot := filepath.Join(base, "link")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatal(err)
	}

	_, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: linkRoot})
	if !errors.Is(err, ErrUnsafeInstallRoot) {
		t.Fatalf("MaterializePayload error = %v, want ErrUnsafeInstallRoot", err)
	}
}

func TestMaterializePayloadRejectsSymlinkedModuleDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	pkg, payload := testVerifiedMaterializePayload(t, "6.0.0", []payloadArchiveEntry{
		{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")},
	})
	root := filepath.Join(t.TempDir(), "modules")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, pkg.Manifest.ID)); err != nil {
		t.Fatal(err)
	}

	_, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root})
	if !errors.Is(err, ErrUnsafeInstallRoot) {
		t.Fatalf("MaterializePayload error = %v, want ErrUnsafeInstallRoot", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target was modified: %v", entries)
	}
}

func TestMaterializePayloadRejectsUnsafeVersionComponent(t *testing.T) {
	pkg, payload := testVerifiedMaterializePayload(t, "7.0.0", []payloadArchiveEntry{
		{name: "module", typeflag: tar.TypeReg, mode: 0o700, data: []byte("x")},
	})
	pkg.Manifest.Version = "../7.0.0"
	payload.Version = pkg.Manifest.Version
	payload.validationFingerprint = fingerprintValidatedPayload(payload.ModuleID, payload.Version, payload.PayloadSHA256, payload.Files)

	_, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: filepath.Join(t.TempDir(), "modules")})
	if err == nil {
		t.Fatal("MaterializePayload accepted unsafe version component")
	}
}

func testVerifiedMaterializePayload(t *testing.T, version string, entries []payloadArchiveEntry) (VerifiedPackage, ValidatedPayload) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testPackageManifest()
	manifest.Version = version
	archive := makePayloadArchive(t, entries)
	packageBytes := makeSignedPackage(t, manifest, archive, "materialize-key", privateKey, nil)
	trust := StaticPublisherTrustStore{
		manifest.Publisher + "\x00materialize-key": {
			Publisher: manifest.Publisher,
			KeyID:     "materialize-key",
			PublicKey: publicKey,
		},
	}
	pkg, err := VerifyPackage(packageBytes, trust)
	if err != nil {
		t.Fatalf("VerifyPackage returned error: %v", err)
	}
	payload, err := ValidatePayload(pkg)
	if err != nil {
		t.Fatalf("ValidatePayload returned error: %v", err)
	}
	return pkg, payload
}

func assertNoInstallScratch(t *testing.T, moduleDir, version string) {
	t.Helper()
	entries, err := os.ReadDir(moduleDir)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "."+version+".install.lock" || strings.HasPrefix(entry.Name(), "."+version+".staging-") {
			t.Fatalf("scratch artifact left behind: %s", entry.Name())
		}
	}
}
