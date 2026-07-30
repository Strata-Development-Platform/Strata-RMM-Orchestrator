package recovery

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// KeyProvider resolves recovery encryption keys independently of the source database.
type KeyProvider interface {
	// ResolveKey returns the key material for the given key ID.
	// Returns ErrKeyNotFound if the key does not exist.
	// Returns ErrProviderUnavailable if the provider is temporarily unavailable.
	ResolveKey(ctx context.Context, keyID string) (*RecoveryKey, error)

	// CurrentKey returns the key currently used for new encryption operations.
	CurrentKey(ctx context.Context) (*RecoveryKey, error)

	// ListKeys returns all known keys, ordered newest first.
	ListKeys(ctx context.Context) ([]*RecoveryKey, error)

	// RotateKey creates a new current key and marks the previous as historical.
	// The previous key remains available via ResolveKey for restoration.
	RotateKey(ctx context.Context, alias string) (*RecoveryKey, error)

	// ProviderName returns a human-readable provider name.
	ProviderName() string
}

// RecoveryKey represents a single encryption key managed by the provider.
type RecoveryKey struct {
	ID          string    `json:"id"`
	Alias       string    `json:"alias"`
	KeyMaterial []byte    `json:"-"` // Never serialized
	CreatedAt   time.Time `json:"created_at"`
	Active      bool      `json:"active"`
	Provider    string    `json:"provider"`
}

// Key errors.
var (
	ErrKeyNotFound         = fmt.Errorf("key not found")
	ErrProviderUnavailable = fmt.Errorf("key provider unavailable")
	ErrInvalidKeyID        = fmt.Errorf("invalid key ID format")
	ErrMalformedKeyFile    = fmt.Errorf("malformed key file")
)

// IsKeyNotFound reports whether err indicates a missing key.
func IsKeyNotFound(err error) bool {
	return err == ErrKeyNotFound || strings.Contains(err.Error(), ErrKeyNotFound.Error())
}

// IsProviderUnavailable reports whether err indicates temporary provider failure.
func IsProviderUnavailable(err error) bool {
	return err == ErrProviderUnavailable || strings.Contains(err.Error(), ErrProviderUnavailable.Error())
}

// --- File-based provider for development/testing ---

// FileKeyProvider stores keys in a directory as individual JSON+key files.
type FileKeyProvider struct {
	dir  string
	mu   sync.RWMutex
	keys map[string]*RecoveryKey // keyID -> RecoveryKey (without material)
}

// NewFileKeyProvider creates a provider backed by files in the given directory.
// The directory is created if it does not exist. Permissions are set to 0700.
func NewFileKeyProvider(dir string) (*FileKeyProvider, error) {
	if dir == "" {
		dir = ".stratalabs/recovery-keys"
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}
	p := &FileKeyProvider{dir: dir, keys: make(map[string]*RecoveryKey)}
	if err := p.loadKeys(); err != nil {
		return nil, fmt.Errorf("load keys from %s: %w", dir, err)
	}
	return p, nil
}

// ProviderName implements KeyProvider.
func (p *FileKeyProvider) ProviderName() string { return "file" }

// ResolveKey implements KeyProvider.
func (p *FileKeyProvider) ResolveKey(_ context.Context, keyID string) (*RecoveryKey, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	k, ok := p.keys[keyID]
	if !ok {
		// Check for a file on disk (may have been added by another process)
		data, err := os.ReadFile(keyFile(p.dir, keyID))
		if err != nil {
			return nil, ErrKeyNotFound
		}
		var meta keyMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, ErrMalformedKeyFile
		}
		keyData, err := os.ReadFile(keyMaterialFile(p.dir, keyID))
		if err != nil {
			return nil, ErrKeyNotFound
		}
		k = &RecoveryKey{
			ID:          keyID,
			Alias:       meta.Alias,
			KeyMaterial: keyData,
			CreatedAt:   meta.CreatedAt,
			Active:      meta.Active,
			Provider:    "file",
		}
		p.keys[keyID] = k
	}

	if len(k.KeyMaterial) == 0 {
		data, err := os.ReadFile(keyMaterialFile(p.dir, keyID))
		if err != nil {
			return nil, ErrKeyNotFound
		}
		k.KeyMaterial = data
	}

	return k, nil
}

// CurrentKey implements KeyProvider.
func (p *FileKeyProvider) CurrentKey(ctx context.Context) (*RecoveryKey, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, k := range p.keys {
		if k.Active {
			return p.ResolveKey(ctx, k.ID)
		}
	}
	return nil, ErrKeyNotFound
}

// ListKeys implements KeyProvider.
func (p *FileKeyProvider) ListKeys(ctx context.Context) ([]*RecoveryKey, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*RecoveryKey, 0, len(p.keys))
	for _, k := range p.keys {
		result = append(result, k)
	}
	return result, nil
}

// RotateKey implements KeyProvider.
func (p *FileKeyProvider) RotateKey(ctx context.Context, alias string) (*RecoveryKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Deactivate current active key
	for _, k := range p.keys {
		if k.Active {
			k.Active = false
			if err := p.persistKeyMeta(k); err != nil {
				return nil, fmt.Errorf("deactivate previous key: %w", err)
			}
		}
	}

	// Generate new key
	material := make([]byte, 32)
	if _, err := rand.Read(material); err != nil {
		return nil, fmt.Errorf("generate key material: %w", err)
	}

	keyID := newKeyID()
	k := &RecoveryKey{
		ID:          keyID,
		Alias:       alias,
		KeyMaterial: material,
		CreatedAt:   time.Now().UTC(),
		Active:      true,
		Provider:    "file",
	}
	p.keys[keyID] = k

	if err := p.persistKeyMeta(k); err != nil {
		return nil, fmt.Errorf("persist new key metadata: %w", err)
	}
	if err := p.persistKeyMaterial(k); err != nil {
		return nil, fmt.Errorf("persist new key material: %w", err)
	}

	return k, nil
}

// CreateKey adds a new key without rotating.
func (p *FileKeyProvider) CreateKey(ctx context.Context, alias string, material []byte) (*RecoveryKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(material) == 0 {
		material = make([]byte, 32)
		if _, err := rand.Read(material); err != nil {
			return nil, fmt.Errorf("generate key material: %w", err)
		}
	}

	keyID := newKeyID()
	k := &RecoveryKey{
		ID:          keyID,
		Alias:       alias,
		KeyMaterial: material,
		CreatedAt:   time.Now().UTC(),
		Active:      false,
		Provider:    "file",
	}
	p.keys[keyID] = k

	if err := p.persistKeyMeta(k); err != nil {
		return nil, fmt.Errorf("persist key metadata: %w", err)
	}
	if err := p.persistKeyMaterial(k); err != nil {
		return nil, fmt.Errorf("persist key material: %w", err)
	}

	return k, nil
}

type keyMeta struct {
	ID        string    `json:"id"`
	Alias     string    `json:"alias"`
	CreatedAt time.Time `json:"created_at"`
	Active    bool      `json:"active"`
	Provider  string    `json:"provider"`
}

func (p *FileKeyProvider) loadKeys() error {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".meta.json") {
			continue
		}
		path := filepath.Join(p.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var meta keyMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		k := &RecoveryKey{
			ID:          meta.ID,
			Alias:       meta.Alias,
			CreatedAt:   meta.CreatedAt,
			Active:      meta.Active,
			Provider:    meta.Provider,
			KeyMaterial: nil, // loaded on demand
		}
		p.keys[meta.ID] = k
	}
	return nil
}

func (p *FileKeyProvider) persistKeyMeta(k *RecoveryKey) error {
	meta := keyMeta{
		ID:        k.ID,
		Alias:     k.Alias,
		CreatedAt: k.CreatedAt,
		Active:    k.Active,
		Provider:  k.Provider,
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(p.dir, k.ID+".meta.json"), data, 0600)
}

func (p *FileKeyProvider) persistKeyMaterial(k *RecoveryKey) error {
	return atomicWriteFile(filepath.Join(p.dir, k.ID+".key"), k.KeyMaterial, 0600)
}

// --- Atomic file write helpers ---

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
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

func keyFile(dir, keyID string) string {
	return filepath.Join(dir, keyID+".meta.json")
}

func keyMaterialFile(dir, keyID string) string {
	return filepath.Join(dir, keyID+".key")
}

// --- Shared helpers ---

func newKeyID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("rk-%x", b)
}
