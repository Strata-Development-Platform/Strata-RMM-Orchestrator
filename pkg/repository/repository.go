// Package repository provides external artifact storage for backup and recovery.
package repository

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"time"
)

// ManifestVersion is the current manifest schema version.
const ManifestVersion = 1

// ComponentType identifies the type of a backup component.
type ComponentType string

const (
	ComponentDatabase    ComponentType = "postgresql"
	ComponentJetStream   ComponentType = "jetstream"
	ComponentObjectStore ComponentType = "object_storage"
	ComponentManifest    ComponentType = "manifest"
)

// BackupSet represents a logical backup grouping of components.
type BackupSet struct {
	ID            string         `json:"id"`
	EnvironmentID string         `json:"environment_id"`
	SourceCommit  string         `json:"source_commit"`
	SourceRelease string         `json:"source_release"`
	SchemaVersion int            `json:"schema_version"`
	StartedAt     time.Time      `json:"started_at"`
	CompletedAt   time.Time      `json:"completed_at"`
	ProtectedAt   time.Time      `json:"protected_at"`
	Components    []ComponentRef `json:"components"`
	Verified      bool           `json:"verified"`
}

// ComponentRef identifies a component within a backup set.
type ComponentRef struct {
	ID               string        `json:"id"`
	Type             ComponentType `json:"type"`
	ArtifactLoc      string        `json:"artifact_location"`
	PlaintextDigest  string        `json:"plaintext_digest"`
	CiphertextDigest string        `json:"ciphertext_digest"`
	EncryptedSize    int64         `json:"encrypted_size"`
	OriginalSize     int64         `json:"original_size"`
	Encryption       string        `json:"encryption"`
	KeyID            string        `json:"key_id"`
	TenantScope      string        `json:"tenant_scope,omitempty"`
	Verified         bool          `json:"verified"`
}

// Manifest is the authoritative record of a finalized backup set.
type Manifest struct {
	Version            int            `json:"manifest_version"`
	BackupSetID        string         `json:"backup_set_id"`
	EnvironmentID      string         `json:"environment_id"`
	SourceCommit       string         `json:"source_commit"`
	SourceRelease      string         `json:"source_release"`
	SchemaVersion      int            `json:"schema_version"`
	StartedAt          time.Time      `json:"started_at"`
	CompletedAt        time.Time      `json:"completed_at"`
	ProtectedAt        time.Time      `json:"protected_at"`
	RequiredComponents []ComponentRef `json:"required_components"`
	VerificationStatus string         `json:"verification_status"`
	CompatibilityLow   string         `json:"compatibility_low,omitempty"`
	CompatibilityHigh  string         `json:"compatibility_high,omitempty"`
}

// VerifyManifest validates that the manifest is structurally complete.
func (m *Manifest) VerifyManifest() error {
	if m.Version != ManifestVersion {
		return fmt.Errorf("unsupported manifest version: %d", m.Version)
	}
	if m.BackupSetID == "" {
		return fmt.Errorf("missing backup_set_id")
	}
	if len(m.RequiredComponents) == 0 {
		return fmt.Errorf("no required components")
	}
	for i, c := range m.RequiredComponents {
		if c.ID == "" {
			return fmt.Errorf("component %d missing id", i)
		}
		if c.Type == "" {
			return fmt.Errorf("component %s missing type", c.ID)
		}
		if c.ArtifactLoc == "" {
			return fmt.Errorf("component %s missing artifact location", c.ID)
		}
	}
	return nil
}

// Digest computes the SHA-256 digest of data.
func Digest(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// DigestBase64 returns a base64-encoded SHA-256 digest.
func DigestBase64(data []byte) string {
	h := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(h[:])
}

// --- Repository interface ---

// Repository stores and retrieves backup artifacts with atomic visibility.
type Repository interface {
	// CreateBackupSet starts a new backup set. The set is not visible
	// until FinalizeBackupSet is called.
	CreateBackupSet(ctx context.Context, set BackupSet) error

	// WriteComponent writes a component artifact to the given location.
	// The artifact must be fully written before FinalizeComponent is called.
	WriteComponent(ctx context.Context, backupSetID, componentID string, reader io.Reader) error

	// FinalizeComponent marks a component as complete after integrity verification.
	FinalizeComponent(ctx context.Context, backupSetID, componentID string, plaintextDigest, ciphertextDigest string, encryptedSize, originalSize int64) error

	// FinalizeBackupSet makes the backup set discoverable and restorable.
	// This must only be called after all components and the manifest are written.
	FinalizeBackupSet(ctx context.Context, manifest *Manifest) error

	// ListBackupSets returns all finalized backup sets.
	ListBackupSets(ctx context.Context) ([]BackupSet, error)

	// ReadManifest returns the manifest for a given backup set.
	ReadManifest(ctx context.Context, backupSetID string) (*Manifest, error)

	// ReadComponent reads a component artifact.
	ReadComponent(ctx context.Context, backupSetID, componentID string) (io.ReadCloser, error)

	// VerifyIntegrity checks that an artifact matches its recorded digest.
	VerifyIntegrity(ctx context.Context, backupSetID, componentID, expectedDigest string) error

	// MarkRetention records the retention policy applied to a backup set.
	MarkRetention(ctx context.Context, backupSetID string, policy string) error

	// DeleteBackupSet removes a backup set under an explicit delete policy.
	DeleteBackupSet(ctx context.Context, backupSetID, policy string) error

	// ProviderName returns the repository type name.
	ProviderName() string
}
