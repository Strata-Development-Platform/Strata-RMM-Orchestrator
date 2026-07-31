package repository

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestFilesystemRepositoryCreateBackupSet proves backup set creation works.
func TestFilesystemRepositoryCreateBackupSet(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	set := BackupSet{
		ID:            "bs-001",
		EnvironmentID: "prod",
		StartedAt:     time.Now(),
	}

	if err := r.CreateBackupSet(context.Background(), set); err != nil {
		t.Fatalf("create backup set: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(r.basePath(set.ID))
	if err != nil {
		t.Fatalf("backup set directory missing: %v", err)
	}
	if !info.IsDir() {
		t.Error("path should be a directory")
	}

	// Duplicate should fail
	err = r.CreateBackupSet(context.Background(), set)
	if err == nil {
		t.Fatal("expected error for duplicate backup set")
	}
}

// TestFilesystemRepositoryWriteAndReadComponent proves component roundtrip.
func TestFilesystemRepositoryWriteAndReadComponent(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	set := BackupSet{ID: "bs-001", EnvironmentID: "prod", StartedAt: time.Now()}
	if err := r.CreateBackupSet(context.Background(), set); err != nil {
		t.Fatalf("create backup set: %v", err)
	}

	payload := []byte("database backup content for testing")
	if err := r.WriteComponent(context.Background(), "bs-001", "db-backup", bytes.NewReader(payload)); err != nil {
		t.Fatalf("write component: %v", err)
	}

	body, err := r.ReadComponent(context.Background(), "bs-001", "db-backup")
	if err != nil {
		t.Fatalf("read component: %v", err)
	}
	defer body.Close() //nolint:errcheck

	buf := make([]byte, len(payload))
	n, err := body.Read(buf)
	if err != nil && n != len(payload) {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Errorf("read payload = %q, want %q", buf, payload)
	}
}

// TestFilesystemRepositoryFinalizeComponent proves component finalization.
func TestFilesystemRepositoryFinalizeComponent(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	set := BackupSet{ID: "bs-001", EnvironmentID: "prod", StartedAt: time.Now()}
	if err := r.CreateBackupSet(context.Background(), set); err != nil {
		t.Fatalf("create backup set: %v", err)
	}

	if err := r.FinalizeComponent(context.Background(), "bs-001", "db",
		"sha256-plaintext", "sha256-ciphertext", 1024, 2048); err != nil {
		t.Fatalf("finalize component: %v", err)
	}

	// Status file should exist
	statusPath := r.basePath("bs-001") + "/components/db.status.json"
	if _, err := os.Stat(statusPath); err != nil {
		t.Fatalf("status file missing: %v", err)
	}
}

// TestFilesystemRepositoryManifest proves manifest write and read.
func TestFilesystemRepositoryManifest(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	set := BackupSet{ID: "bs-001", EnvironmentID: "prod", StartedAt: time.Now()}
	if err := r.CreateBackupSet(context.Background(), set); err != nil {
		t.Fatalf("create backup set: %v", err)
	}

	manifest := &Manifest{
		Version:       ManifestVersion,
		BackupSetID:   "bs-001",
		EnvironmentID: "prod",
		StartedAt:     time.Now(),
		CompletedAt:   time.Now(),
		RequiredComponents: []ComponentRef{
			authenticatedTestComponent("db", ComponentDatabase),
			authenticatedTestComponent("js", ComponentJetStream),
		},
	}

	if err := r.FinalizeBackupSet(context.Background(), manifest); err != nil {
		t.Fatalf("finalize backup set: %v", err)
	}

	read, err := r.ReadManifest(context.Background(), "bs-001")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if read.BackupSetID != "bs-001" {
		t.Errorf("backup_set_id = %s, want bs-001", read.BackupSetID)
	}
	if len(read.RequiredComponents) != 2 {
		t.Errorf("components = %d, want 2", len(read.RequiredComponents))
	}
}

// TestFilesystemRepositoryListBackupSets proves listing works.
func TestFilesystemRepositoryListBackupSets(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	// Create two backup sets
	for _, id := range []string{"bs-001", "bs-002"} {
		set := BackupSet{ID: id, EnvironmentID: "prod", StartedAt: time.Now()}
		if err := r.CreateBackupSet(context.Background(), set); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		m := &Manifest{
			Version:            ManifestVersion,
			BackupSetID:        id,
			EnvironmentID:      "prod",
			StartedAt:          time.Now(),
			CompletedAt:        time.Now(),
			RequiredComponents: []ComponentRef{authenticatedTestComponent("db", ComponentDatabase)},
		}
		if err := r.FinalizeBackupSet(context.Background(), m); err != nil {
			t.Fatalf("finalize %s: %v", id, err)
		}
	}

	sets, err := r.ListBackupSets(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sets) != 2 {
		t.Errorf("sets = %d, want 2", len(sets))
	}
}

// TestFilesystemRepositoryIntegrity verifies digest matching.
func TestFilesystemRepositoryIntegrity(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	set := BackupSet{ID: "bs-001", EnvironmentID: "prod", StartedAt: time.Now()}
	if err := r.CreateBackupSet(context.Background(), set); err != nil {
		t.Fatalf("create backup set: %v", err)
	}

	payload := []byte("test data for integrity check")
	if err := r.WriteComponent(context.Background(), "bs-001", "comp1", bytes.NewReader(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}

	expected := DigestBase64(payload)
	if err := r.FinalizeComponent(context.Background(), "bs-001", "comp1", expected, expected, int64(len(payload)), int64(len(payload))); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	if err := r.VerifyIntegrity(context.Background(), "bs-001", "comp1", expected); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// TestFilesystemRepositoryIntegrityMismatch proves tampering is detected.
func TestFilesystemRepositoryIntegrityMismatch(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	set := BackupSet{ID: "bs-001", EnvironmentID: "prod", StartedAt: time.Now()}
	if err := r.CreateBackupSet(context.Background(), set); err != nil {
		t.Fatalf("create backup set: %v", err)
	}

	payload := []byte("test data")
	if err := r.WriteComponent(context.Background(), "bs-001", "comp1", bytes.NewReader(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Use wrong digest
	wrongDigest := "base64-wrong-digest-value"
	if err := r.FinalizeComponent(context.Background(), "bs-001", "comp1", wrongDigest, wrongDigest, 0, 0); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	err = r.VerifyIntegrity(context.Background(), "bs-001", "comp1", wrongDigest)
	if err == nil {
		t.Fatal("expected integrity error, got nil")
	}
}

// TestFilesystemRepositoryDelete proves deletion works.
func TestFilesystemRepositoryDelete(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	set := BackupSet{ID: "bs-001", EnvironmentID: "prod", StartedAt: time.Now()}
	if err := r.CreateBackupSet(context.Background(), set); err != nil {
		t.Fatalf("create backup set: %v", err)
	}

	// Delete without policy should fail
	if err := r.DeleteBackupSet(context.Background(), "bs-001", ""); err == nil {
		t.Fatal("expected error for empty policy")
	}

	// Delete with policy should succeed
	if err := r.DeleteBackupSet(context.Background(), "bs-001", "explicit-deletion"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := os.Stat(r.basePath("bs-001")); !os.IsNotExist(err) {
		t.Error("backup set directory should be removed")
	}
}

// TestFilesystemRepositoryManifestValidation proves invalid manifests are rejected.
func TestFilesystemRepositoryManifestValidation(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	// Missing backup_set_id
	badManifest := &Manifest{
		Version: ManifestVersion,
		RequiredComponents: []ComponentRef{
			{ID: "db", Type: ComponentDatabase, ArtifactLoc: "backups/bs-001/components/db", PlaintextDigest: "sha256:test", CiphertextDigest: "sha256:test", EncryptedSize: 1024, OriginalSize: 2048},
		},
	}
	err = r.FinalizeBackupSet(context.Background(), badManifest)
	if err == nil {
		t.Fatal("expected error for missing backup_set_id")
	}
	if !strings.Contains(err.Error(), "backup_set_id") {
		t.Errorf("error should mention backup_set_id, got: %v", err)
	}

	// No components
	badManifest2 := &Manifest{
		Version:            ManifestVersion,
		BackupSetID:        "bs-bad",
		RequiredComponents: []ComponentRef{},
	}
	err = r.FinalizeBackupSet(context.Background(), badManifest2)
	if err == nil {
		t.Fatal("expected error for empty components")
	}

	// Unsupported version
	badManifest3 := &Manifest{
		Version:            99,
		BackupSetID:        "bs-bad",
		RequiredComponents: []ComponentRef{{ID: "db", Type: ComponentDatabase, ArtifactLoc: "backups/bs-001/components/db", PlaintextDigest: "sha256:test", CiphertextDigest: "sha256:test", EncryptedSize: 1024, OriginalSize: 2048}},
	}
	err = r.FinalizeBackupSet(context.Background(), badManifest3)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

// TestFilesystemRepositoryAtomicWrite proves temp files are cleaned up.
func TestFilesystemRepositoryAtomicWrite(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	set := BackupSet{ID: "bs-001", EnvironmentID: "prod", StartedAt: time.Now()}
	if err := r.CreateBackupSet(context.Background(), set); err != nil {
		t.Fatalf("create backup set: %v", err)
	}

	// Write and finalize
	payload := []byte("atomic test")
	if err := r.WriteComponent(context.Background(), "bs-001", "comp", bytes.NewReader(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// No temp files should remain
	entries, _ := os.ReadDir(r.basePath("bs-001") + "/components")
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Errorf("temporary file still exists: %s", entry.Name())
		}
	}
}

// TestManifestVerify proves manifest validation.
func TestManifestVerify(t *testing.T) {
	// Valid manifest
	m := &Manifest{
		Version:            ManifestVersion,
		BackupSetID:        "bs-001",
		EnvironmentID:      "test",
		RequiredComponents: []ComponentRef{authenticatedTestComponent("db", ComponentDatabase)},
	}
	if err := m.VerifyManifest(); err != nil {
		t.Fatalf("valid manifest failed: %v", err)
	}

	// Empty component ID
	m2 := &Manifest{
		Version:            ManifestVersion,
		BackupSetID:        "bs-001",
		RequiredComponents: []ComponentRef{{ID: "", Type: ComponentDatabase}},
	}
	if err := m2.VerifyManifest(); err == nil {
		t.Fatal("expected error for empty component ID")
	}

	// Missing type
	m3 := &Manifest{
		Version:            ManifestVersion,
		BackupSetID:        "bs-001",
		RequiredComponents: []ComponentRef{{ID: "db", Type: ""}},
	}
	if err := m3.VerifyManifest(); err == nil {
		t.Fatal("expected error for empty component type")
	}

	// Missing artifact location
	m4 := &Manifest{
		Version:            ManifestVersion,
		BackupSetID:        "bs-001",
		RequiredComponents: []ComponentRef{{ID: "db", Type: ComponentDatabase, ArtifactLoc: ""}},
	}
	if err := m4.VerifyManifest(); err == nil {
		t.Fatal("expected error for missing artifact location")
	}
}

func authenticatedTestComponent(id string, componentType ComponentType) ComponentRef {
	return ComponentRef{
		ID:               id,
		Type:             componentType,
		ArtifactLoc:      id,
		PlaintextDigest:  "sha256:plain",
		CiphertextDigest: "sha256:cipher",
		EncryptedSize:    1024,
		OriginalSize:     512,
		Encryption:       "aes-256-gcm",
		KeyID:            "key-test",
		Verified:         true,
	}
}

// TestDigestBase64 proves deterministic digest computation.
func TestDigestBase64(t *testing.T) {
	data := []byte("hello world")
	d1 := DigestBase64(data)
	d2 := DigestBase64(data)
	if d1 != d2 {
		t.Errorf("digests should be deterministic: %s != %s", d1, d2)
	}
}

// TestNewFilesystemRepositoryEmptyRoot proves default root creation.
func TestNewFilesystemRepositoryEmptyRoot(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if r.ProviderName() != "filesystem" {
		t.Errorf("ProviderName = %s, want filesystem", r.ProviderName())
	}
}

func TestFilesystemRepositoryRejectsPathTraversal(t *testing.T) {
	repo, err := NewFilesystemRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := repo.CreateBackupSet(ctx, BackupSet{ID: "../escape"}); err == nil {
		t.Fatal("expected traversal backup ID to be rejected")
	}
	_, err = repo.ReadManifest(ctx, "../../manifest")
	if err == nil {
		t.Fatal("expected traversal manifest ID to be rejected")
	}
	if err := repo.WriteComponent(ctx, "safe", "../component", strings.NewReader("data")); err == nil {
		t.Fatal("expected traversal component ID to be rejected")
	}
}

// TestFilesystemRepositoryRetention proves retention marking.
func TestFilesystemRepositoryRetention(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	set := BackupSet{ID: "bs-001", EnvironmentID: "prod", StartedAt: time.Now()}
	if err := r.CreateBackupSet(context.Background(), set); err != nil {
		t.Fatalf("create backup set: %v", err)
	}

	if err := r.MarkRetention(context.Background(), "bs-001", "30-day-retention"); err != nil {
		t.Fatalf("mark retention: %v", err)
	}
}

// TestFilesystemRepositoryReadManifestNotFound proves 404 behavior.
func TestFilesystemRepositoryReadManifestNotFound(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	_, err = r.ReadManifest(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

// TestFilesystemRepositoryComponentNotFound proves component 404.
func TestFilesystemRepositoryComponentNotFound(t *testing.T) {
	root := t.TempDir()
	r, err := NewFilesystemRepository(root)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	_, err = r.ReadComponent(context.Background(), "bs-001", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing component")
	}
}
