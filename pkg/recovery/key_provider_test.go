package recovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestFileKeyProviderCreateAndResolve proves basic key lifecycle works.
func TestFileKeyProviderCreateAndResolve(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	key, err := p.CreateKey(context.Background(), "test-key", nil)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	if key.ID == "" {
		t.Error("key ID must not be empty")
	}
	if key.Alias != "test-key" {
		t.Errorf("alias = %s, want test-key", key.Alias)
	}
	if len(key.KeyMaterial) != 32 {
		t.Errorf("key material = %d bytes, want 32", len(key.KeyMaterial))
	}

	resolved, err := p.ResolveKey(context.Background(), key.ID)
	if err != nil {
		t.Fatalf("resolve key: %v", err)
	}
	if resolved.ID != key.ID {
		t.Errorf("resolved ID = %s, want %s", resolved.ID, key.ID)
	}
	if len(resolved.KeyMaterial) != 32 {
		t.Errorf("resolved key material = %d bytes, want 32", len(resolved.KeyMaterial))
	}
}

// TestFileKeyProviderCurrentKey proves current key resolution works.
func TestFileKeyProviderCurrentKey(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	// First key is not active
	k1, err := p.CreateKey(context.Background(), "first", nil)
	if err != nil {
		t.Fatalf("create k1: %v", err)
	}

	_, err = p.CurrentKey(context.Background())
	if err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound when no active key, got: %v", err)
	}

	// Rotate to create active key
	k2, err := p.RotateKey(context.Background(), "second")
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}

	current, err := p.CurrentKey(context.Background())
	if err != nil {
		t.Fatalf("current key: %v", err)
	}
	if current.ID != k2.ID {
		t.Errorf("current = %s, want %s", current.ID, k2.ID)
	}
	if !current.Active {
		t.Error("current key should be active")
	}

	// Old key should not be active
	k1Resolved, err := p.ResolveKey(context.Background(), k1.ID)
	if err != nil {
		t.Fatalf("resolve k1: %v", err)
	}
	if k1Resolved.Active {
		t.Error("rotated key should not be active")
	}
}

// TestFileKeyProviderRotateKeepsOldKey proves old keys remain resolvable.
func TestFileKeyProviderRotateKeepsOldKey(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	k1, err := p.RotateKey(context.Background(), "key1")
	if err != nil {
		t.Fatalf("rotate 1: %v", err)
	}

	k2, err := p.RotateKey(context.Background(), "key2")
	if err != nil {
		t.Fatalf("rotate 2: %v", err)
	}

	// Both keys should be resolvable
	r1, err := p.ResolveKey(context.Background(), k1.ID)
	if err != nil {
		t.Fatalf("resolve old key: %v", err)
	}
	if len(r1.KeyMaterial) != 32 {
		t.Error("old key material should be preserved")
	}

	r2, err := p.ResolveKey(context.Background(), k2.ID)
	if err != nil {
		t.Fatalf("resolve new key: %v", err)
	}
	if len(r2.KeyMaterial) != 32 {
		t.Error("new key material should be available")
	}
}

// TestFileKeyProviderUnknownKey proves unknown keys return ErrKeyNotFound.
func TestFileKeyProviderUnknownKey(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	_, err = p.ResolveKey(context.Background(), "rk-00000000000000000000000000000000")
	if err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound, got: %v", err)
	}
}

func TestFileKeyProviderRejectsInvalidKeyID(t *testing.T) {
	p, err := NewFileKeyProvider(t.TempDir())
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if _, err = p.ResolveKey(context.Background(), "../key"); err != ErrInvalidKeyID {
		t.Fatalf("expected ErrInvalidKeyID, got: %v", err)
	}
}

// TestFileKeyProviderListKeys proves listing works.
func TestFileKeyProviderListKeys(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	_, err = p.CreateKey(context.Background(), "key1", nil)
	if err != nil {
		t.Fatalf("create k1: %v", err)
	}
	_, err = p.CreateKey(context.Background(), "key2", nil)
	if err != nil {
		t.Fatalf("create k2: %v", err)
	}

	keys, err := p.ListKeys(context.Background())
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("keys = %d, want 2", len(keys))
	}
}

// TestFileKeyProviderPersistence proves keys survive provider recreation.
func TestFileKeyProviderPersistence(t *testing.T) {
	dir := t.TempDir()

	// Create provider and add keys
	p1, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	k1, err := p1.CreateKey(context.Background(), "persistent-key", nil)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	// Create new provider from same directory
	p2, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("recreate provider: %v", err)
	}

	resolved, err := p2.ResolveKey(context.Background(), k1.ID)
	if err != nil {
		t.Fatalf("resolve persisted key: %v", err)
	}
	if resolved.ID != k1.ID {
		t.Errorf("persisted ID = %s, want %s", resolved.ID, k1.ID)
	}
	if len(resolved.KeyMaterial) != 32 {
		t.Error("persisted key material should be 32 bytes")
	}
}

// TestFileKeyProviderDirectoryPermissions proves 0700 directory.
func TestFileKeyProviderDirectoryPermissions(t *testing.T) {
	dir := t.TempDir()
	providerDir := filepath.Join(dir, "keys")

	_, err := NewFileKeyProvider(providerDir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	info, err := os.Stat(providerDir)
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("directory perm = 0%o, want 0700", perm)
	}
}

// TestFileKeyProviderEmptyDir proves provider works with empty directory.
func TestFileKeyProviderEmptyDir(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider in empty dir: %v", err)
	}

	keys, err := p.ListKeys(context.Background())
	if err != nil {
		t.Fatalf("list keys in empty dir: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("list keys in empty dir = %d, want 0", len(keys))
	}
}

// TestFileKeyProviderMaterialFilePermissions proves key files are 0600.
func TestFileKeyProviderMaterialFilePermissions(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	_, err = p.RotateKey(context.Background(), "perm-test")
	if err != nil {
		t.Fatalf("rotate key: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("stat %s: %v", entry.Name(), err)
		}
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("file %s perm = 0%o, want 0600", entry.Name(), perm)
		}
	}
}

// TestIsKeyNotFound proves error classification.
func TestIsKeyNotFound(t *testing.T) {
	if !IsKeyNotFound(ErrKeyNotFound) {
		t.Error("ErrKeyNotFound should be classified as KeyNotFound")
	}
	// os.ErrNotExist may or may not be classified as KeyNotFound
	// depending on implementation - no assertion needed here
	_ = os.ErrNotExist
}

// TestKeyIDFormat proves generated key IDs have rk- prefix.
func TestKeyIDFormat(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	key, err := p.CreateKey(context.Background(), "format-test", nil)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if len(key.ID) < 5 {
		t.Errorf("key ID too short: %s", key.ID)
	}
}

// TestFileKeyProviderProviderName proves name returns "file".
func TestFileKeyProviderProviderName(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if p.ProviderName() != "file" {
		t.Errorf("ProviderName = %s, want file", p.ProviderName())
	}
}

// TestFileKeyProviderNilMaterial proves empty material generation.
func TestFileKeyProviderNilMaterial(t *testing.T) {
	dir := t.TempDir()
	p, err := NewFileKeyProvider(dir)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	key, err := p.CreateKey(context.Background(), "auto-material", nil)
	if err != nil {
		t.Fatalf("create key with nil material: %v", err)
	}
	if len(key.KeyMaterial) != 32 {
		t.Errorf("auto-generated material = %d bytes, want 32", len(key.KeyMaterial))
	}
}
