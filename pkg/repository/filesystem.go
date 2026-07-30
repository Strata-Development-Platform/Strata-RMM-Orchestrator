package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FilesystemRepository stores artifacts as files with atomic operations.
type FilesystemRepository struct {
	root string
	mu   sync.Mutex
}

// NewFilesystemRepository creates a filesystem-backed repository.
// The root directory is created if it does not exist.
func NewFilesystemRepository(root string) (*FilesystemRepository, error) {
	if root == "" {
		root = ".stratalabs/backups"
	}
	if err := os.MkdirAll(root, 0750); err != nil {
		return nil, fmt.Errorf("create repository root: %w", err)
	}
	return &FilesystemRepository{root: root}, nil
}

// ProviderName implements Repository.
func (r *FilesystemRepository) ProviderName() string { return "filesystem" }

func (r *FilesystemRepository) basePath(backupSetID string) string {
	return filepath.Join(r.root, backupSetID)
}

func (r *FilesystemRepository) componentPath(backupSetID, componentID string) string {
	return filepath.Join(r.basePath(backupSetID), "components", componentID)
}

func (r *FilesystemRepository) manifestPath(backupSetID string) string {
	return filepath.Join(r.basePath(backupSetID), "manifest.json")
}

func (r *FilesystemRepository) CreateBackupSet(_ context.Context, set BackupSet) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := r.basePath(set.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("backup set %s already exists", set.ID)
	}
	if err := os.MkdirAll(filepath.Join(path, "components"), 0750); err != nil {
		return fmt.Errorf("create backup set directory: %w", err)
	}
	return nil
}

func (r *FilesystemRepository) WriteComponent(_ context.Context, backupSetID, componentID string, reader io.Reader) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := r.componentPath(backupSetID, componentID)
	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmpFile.Name()

	defer func() {
		tmpFile.Close()
		os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmpFile, reader); err != nil {
		return fmt.Errorf("write component: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("create component directory: %w", err)
	}

	return os.Rename(tmpName, path)
}

func (r *FilesystemRepository) FinalizeComponent(_ context.Context, backupSetID, componentID, plaintextDigest, ciphertextDigest string, encryptedSize, originalSize int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Write a component status file with digest information
	status := struct {
		ComponentID        string `json:"component_id"`
		PlaintextDigest    string `json:"plaintext_digest"`
		CiphertextDigest   string `json:"ciphertext_digest"`
		EncryptedSize      int64  `json:"encrypted_size"`
		OriginalSize       int64  `json:"original_size"`
		Status             string `json:"status"`
	}{
		ComponentID:      componentID,
		PlaintextDigest:  plaintextDigest,
		CiphertextDigest: ciphertextDigest,
		EncryptedSize:    encryptedSize,
		OriginalSize:     originalSize,
		Status:           "finalized",
	}

	statusData, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshal component status: %w", err)
	}

	return atomicWrite(filepath.Join(r.basePath(backupSetID), "components", componentID+".status.json"), statusData, 0640)
}

func (r *FilesystemRepository) FinalizeBackupSet(_ context.Context, manifest *Manifest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := manifest.VerifyManifest(); err != nil {
		return fmt.Errorf("verify manifest: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	return atomicWrite(r.manifestPath(manifest.BackupSetID), data, 0640)
}

func (r *FilesystemRepository) ListBackupSets(_ context.Context) ([]BackupSet, error) {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		return nil, err
	}

	var sets []BackupSet
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(r.root, entry.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		sets = append(sets, BackupSet{
			ID:            m.BackupSetID,
			EnvironmentID: m.EnvironmentID,
			SourceCommit:  m.SourceCommit,
			StartedAt:     m.StartedAt,
			CompletedAt:   m.CompletedAt,
			Components:    m.RequiredComponents,
			Verified:      m.VerificationStatus == "verified",
		})
	}
	return sets, nil
}

func (r *FilesystemRepository) ReadManifest(_ context.Context, backupSetID string) (*Manifest, error) {
	data, err := os.ReadFile(r.manifestPath(backupSetID))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	return &m, nil
}

func (r *FilesystemRepository) ReadComponent(_ context.Context, backupSetID, componentID string) (io.ReadCloser, error) {
	path := r.componentPath(backupSetID, componentID)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read component: %w", err)
	}
	return f, nil
}

func (r *FilesystemRepository) VerifyIntegrity(_ context.Context, backupSetID, componentID, expectedDigest string) error {
	statusFile := filepath.Join(r.basePath(backupSetID), "components", componentID+".status.json")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return fmt.Errorf("read component status: %w", err)
	}
	var status struct {
		CiphertextDigest string `json:"ciphertext_digest"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return fmt.Errorf("unmarshal component status: %w", err)
	}

	// Also verify the actual file
	path := r.componentPath(backupSetID, componentID)
	fileData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read component file: %w", err)
	}
	actualDigest := DigestBase64(fileData)
	if actualDigest != expectedDigest {
		return fmt.Errorf("integrity mismatch: expected %s, got %s", expectedDigest, actualDigest)
	}
	return nil
}

func (r *FilesystemRepository) MarkRetention(_ context.Context, backupSetID, policy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	retentionFile := filepath.Join(r.basePath(backupSetID), "retention.json")
	data, err := json.Marshal(map[string]string{"policy": policy, "backup_set_id": backupSetID})
	if err != nil {
		return fmt.Errorf("marshal retention: %w", err)
	}
	return atomicWrite(retentionFile, data, 0640)
}

func (r *FilesystemRepository) DeleteBackupSet(_ context.Context, backupSetID, policy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Verify policy is not empty
	if policy == "" {
		return fmt.Errorf("delete policy required")
	}

	path := r.basePath(backupSetID)
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove backup set: %w", err)
	}
	return nil
}

// --- Atomic write (non-racy, uses temp file + rename) ---

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmpFile.Chmod(perm); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// pathSanitizer prevents directory traversal.
func pathSanitizer(base, requested string) (string, error) {
	cleaned := filepath.Clean(filepath.Join(base, requested))
	if !strings.HasPrefix(cleaned, base) {
		return "", fmt.Errorf("path traversal detected")
	}
	return cleaned, nil
}
