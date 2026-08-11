package modules

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const releaseMetadataSchemaVersion = 1

var (
	ErrReleaseMetadataConflict = errors.New("module release metadata conflicts with existing version")
	ErrReleaseMetadataInvalid  = errors.New("module release metadata is invalid")
)

type ReleaseMetadata struct {
	SchemaVersion int      `json:"schema_version"`
	Manifest      Manifest `json:"manifest"`
	PayloadSHA256 string   `json:"payload_sha256"`
	Publisher     string   `json:"publisher"`
	KeyID         string   `json:"key_id"`
}

// PersistReleaseMetadata durably records the signed identity of an immutable
// materialized module version. The record lives beside, never inside, the
// payload directory so exact payload verification remains unchanged.
//
// Publication is non-overwriting: a fully-written temporary inode is hard
// linked to the final version name. If another writer wins the race, the
// existing record is accepted only when it is byte-for-byte equivalent after
// strict decoding and validation.
func PersistReleaseMetadata(root string, pkg VerifiedPackage) (ReleaseMetadata, error) {
	metadata, err := releaseMetadataFromPackage(pkg)
	if err != nil {
		return ReleaseMetadata{}, err
	}
	moduleDir, err := existingModuleDirectory(root, pkg.Manifest.ID)
	if err != nil {
		return ReleaseMetadata{}, err
	}
	moduleRoot, err := os.OpenRoot(moduleDir)
	if err != nil {
		return ReleaseMetadata{}, fmt.Errorf("open module root for release metadata: %w", err)
	}
	defer func() { _ = moduleRoot.Close() }()

	if err := moduleRoot.MkdirAll(".releases", 0o700); err != nil {
		return ReleaseMetadata{}, fmt.Errorf("create module release metadata directory: %w", err)
	}
	if info, err := moduleRoot.Lstat(".releases"); err != nil {
		return ReleaseMetadata{}, fmt.Errorf("inspect module release metadata directory: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ReleaseMetadata{}, ErrUnsafeInstallRoot
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return ReleaseMetadata{}, fmt.Errorf("encode module release metadata: %w", err)
	}
	encoded = append(encoded, '\n')
	finalName := filepath.ToSlash(filepath.Join(".releases", pkg.Manifest.Version+".json"))
	if existing, err := readReleaseMetadataFromRoot(moduleRoot, finalName); err == nil {
		if releaseMetadataEqual(existing, metadata) {
			return existing, nil
		}
		return ReleaseMetadata{}, ErrReleaseMetadataConflict
	} else if !errors.Is(err, fs.ErrNotExist) {
		return ReleaseMetadata{}, err
	}

	tempName, err := randomReleaseTempName(pkg.Manifest.Version)
	if err != nil {
		return ReleaseMetadata{}, err
	}
	tempPath := filepath.ToSlash(filepath.Join(".releases", tempName))
	handle, err := moduleRoot.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ReleaseMetadata{}, fmt.Errorf("create temporary module release metadata: %w", err)
	}
	tempExists := true
	defer func() {
		if tempExists {
			_ = moduleRoot.Remove(tempPath)
		}
	}()
	if _, err := handle.Write(encoded); err != nil {
		_ = handle.Close()
		return ReleaseMetadata{}, fmt.Errorf("write module release metadata: %w", err)
	}
	if err := handle.Sync(); err != nil {
		_ = handle.Close()
		return ReleaseMetadata{}, fmt.Errorf("sync module release metadata: %w", err)
	}
	if err := handle.Close(); err != nil {
		return ReleaseMetadata{}, fmt.Errorf("close module release metadata: %w", err)
	}

	if err := moduleRoot.Link(tempPath, finalName); err != nil {
		if existing, readErr := readReleaseMetadataFromRoot(moduleRoot, finalName); readErr == nil {
			if releaseMetadataEqual(existing, metadata) {
				return existing, nil
			}
			return ReleaseMetadata{}, ErrReleaseMetadataConflict
		}
		return ReleaseMetadata{}, fmt.Errorf("publish module release metadata: %w", err)
	}
	if err := moduleRoot.Remove(tempPath); err != nil {
		return ReleaseMetadata{}, fmt.Errorf("remove temporary module release metadata: %w", err)
	}
	tempExists = false
	return metadata, nil
}

func ReadReleaseMetadata(root, moduleID, version string) (ReleaseMetadata, error) {
	if err := validateInstallComponent(moduleID, "module id"); err != nil {
		return ReleaseMetadata{}, err
	}
	if err := validateInstallComponent(version, "module version"); err != nil {
		return ReleaseMetadata{}, err
	}
	moduleDir, err := existingModuleDirectory(root, moduleID)
	if err != nil {
		return ReleaseMetadata{}, err
	}
	moduleRoot, err := os.OpenRoot(moduleDir)
	if err != nil {
		return ReleaseMetadata{}, fmt.Errorf("open module root for release metadata: %w", err)
	}
	defer func() { _ = moduleRoot.Close() }()
	name := filepath.ToSlash(filepath.Join(".releases", version+".json"))
	return readReleaseMetadataFromRoot(moduleRoot, name)
}

func releaseMetadataFromPackage(pkg VerifiedPackage) (ReleaseMetadata, error) {
	if err := pkg.Manifest.Validate(); err != nil {
		return ReleaseMetadata{}, fmt.Errorf("%w: manifest: %v", ErrReleaseMetadataInvalid, err)
	}
	if len(pkg.PayloadSHA256) != 64 {
		return ReleaseMetadata{}, fmt.Errorf("%w: payload digest", ErrReleaseMetadataInvalid)
	}
	if _, err := hex.DecodeString(pkg.PayloadSHA256); err != nil {
		return ReleaseMetadata{}, fmt.Errorf("%w: payload digest", ErrReleaseMetadataInvalid)
	}
	if pkg.PublisherKey.Publisher != pkg.Manifest.Publisher || strings.TrimSpace(pkg.PublisherKey.KeyID) == "" {
		return ReleaseMetadata{}, fmt.Errorf("%w: publisher identity", ErrReleaseMetadataInvalid)
	}
	return ReleaseMetadata{
		SchemaVersion: releaseMetadataSchemaVersion,
		Manifest:      pkg.Manifest,
		PayloadSHA256: strings.ToLower(pkg.PayloadSHA256),
		Publisher:     pkg.PublisherKey.Publisher,
		KeyID:         pkg.PublisherKey.KeyID,
	}, nil
}

func readReleaseMetadataFromRoot(root *os.Root, name string) (ReleaseMetadata, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return ReleaseMetadata{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxManifestBytes*2 {
		return ReleaseMetadata{}, ErrReleaseMetadataInvalid
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return ReleaseMetadata{}, fmt.Errorf("read module release metadata: %w", err)
	}
	var metadata ReleaseMetadata
	if err := decodeStrictJSON(data, &metadata); err != nil {
		return ReleaseMetadata{}, fmt.Errorf("%w: decode: %v", ErrReleaseMetadataInvalid, err)
	}
	if err := validateReleaseMetadata(metadata); err != nil {
		return ReleaseMetadata{}, err
	}
	return metadata, nil
}

func validateReleaseMetadata(metadata ReleaseMetadata) error {
	if metadata.SchemaVersion != releaseMetadataSchemaVersion {
		return ErrReleaseMetadataInvalid
	}
	if err := metadata.Manifest.Validate(); err != nil {
		return fmt.Errorf("%w: manifest: %v", ErrReleaseMetadataInvalid, err)
	}
	if metadata.Publisher != metadata.Manifest.Publisher || strings.TrimSpace(metadata.KeyID) == "" {
		return ErrReleaseMetadataInvalid
	}
	if len(metadata.PayloadSHA256) != 64 {
		return ErrReleaseMetadataInvalid
	}
	if _, err := hex.DecodeString(metadata.PayloadSHA256); err != nil {
		return ErrReleaseMetadataInvalid
	}
	return nil
}

func releaseMetadataEqual(left, right ReleaseMetadata) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func randomReleaseTempName(version string) (string, error) {
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate module release metadata nonce: %w", err)
	}
	return "." + version + ".tmp-" + hex.EncodeToString(suffix[:]), nil
}
