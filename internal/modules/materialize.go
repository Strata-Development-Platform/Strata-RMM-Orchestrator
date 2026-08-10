package modules

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPayloadIdentityMismatch   = errors.New("validated payload does not match verified package")
	ErrMaterializedVersionExists = errors.New("module version is already materialized")
	ErrUnsafeInstallRoot         = errors.New("module install root is unsafe")
	ErrInstallInProgress         = errors.New("module version install is already in progress")
)

type MaterializeOwnership struct {
	UID int
	GID int
}

type MaterializeOptions struct {
	Root      string
	Ownership *MaterializeOwnership
}

type MaterializedModule struct {
	ModuleID      string
	Version       string
	PayloadSHA256 string
	Path          string
	FileCount     int
	ExpandedBytes int64
}

// MaterializePayload writes a previously validated payload into a versioned,
// module-specific directory beneath a trusted install root. Files are staged
// in a sibling temporary directory and become visible only after a successful
// final rename. This function never executes module code or changes lifecycle
// state.
func MaterializePayload(pkg VerifiedPackage, payload ValidatedPayload, options MaterializeOptions) (MaterializedModule, error) {
	if err := pkg.Manifest.Validate(); err != nil {
		return MaterializedModule{}, fmt.Errorf("validate package manifest: %w", err)
	}
	if err := validateMaterializeIdentity(pkg, payload); err != nil {
		return MaterializedModule{}, err
	}
	if strings.TrimSpace(options.Root) == "" {
		return MaterializedModule{}, fmt.Errorf("%w: install root is required", ErrUnsafeInstallRoot)
	}
	if err := validateInstallComponent(pkg.Manifest.ID, "module id"); err != nil {
		return MaterializedModule{}, err
	}
	if err := validateInstallComponent(pkg.Manifest.Version, "module version"); err != nil {
		return MaterializedModule{}, err
	}
	if options.Ownership != nil {
		if options.Ownership.UID < -1 || options.Ownership.GID < -1 || (options.Ownership.UID == -1 && options.Ownership.GID == -1) {
			return MaterializedModule{}, errors.New("materialize ownership must specify a valid uid or gid")
		}
	}

	root, err := prepareInstallRoot(options.Root)
	if err != nil {
		return MaterializedModule{}, err
	}
	moduleDir := filepath.Join(root, pkg.Manifest.ID)
	if err := ensureTrustedDirectory(moduleDir, 0o750); err != nil {
		return MaterializedModule{}, err
	}

	target := filepath.Join(moduleDir, pkg.Manifest.Version)
	if err := ensureContained(root, target); err != nil {
		return MaterializedModule{}, err
	}
	if _, err := os.Lstat(target); err == nil {
		return MaterializedModule{}, ErrMaterializedVersionExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return MaterializedModule{}, fmt.Errorf("inspect module target: %w", err)
	}

	lockPath := filepath.Join(moduleDir, "."+pkg.Manifest.Version+".install.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return MaterializedModule{}, ErrInstallInProgress
		}
		return MaterializedModule{}, fmt.Errorf("create module install lock: %w", err)
	}
	if err := lock.Close(); err != nil {
		_ = os.Remove(lockPath)
		return MaterializedModule{}, fmt.Errorf("close module install lock: %w", err)
	}
	defer os.Remove(lockPath)

	stage, err := os.MkdirTemp(moduleDir, "."+pkg.Manifest.Version+".staging-")
	if err != nil {
		return MaterializedModule{}, fmt.Errorf("create module staging directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := os.Chmod(stage, 0o750); err != nil {
		return MaterializedModule{}, fmt.Errorf("set staging directory mode: %w", err)
	}

	seen := make(map[string]struct{}, len(payload.Files))
	var expandedBytes int64
	for i, file := range payload.Files {
		clean, err := cleanPayloadPath(file.Path)
		if err != nil {
			return MaterializedModule{}, err
		}
		if _, duplicate := seen[clean]; duplicate {
			return MaterializedModule{}, fmt.Errorf("duplicate materialized payload path %q", clean)
		}
		seen[clean] = struct{}{}
		if file.Mode < 0 || file.Mode&^int64(0o777) != 0 {
			return MaterializedModule{}, fmt.Errorf("module payload file %q has unsupported mode %#o", clean, file.Mode)
		}
		if err := validatePayloadLimits(i, expandedBytes, int64(len(file.Data))); err != nil {
			return MaterializedModule{}, fmt.Errorf("module payload file %q: %w", clean, err)
		}

		destination := filepath.Join(stage, filepath.FromSlash(clean))
		if err := ensureContained(stage, destination); err != nil {
			return MaterializedModule{}, err
		}
		if err := createMaterializedFile(stage, destination, file, options.Ownership); err != nil {
			return MaterializedModule{}, err
		}
		expandedBytes += int64(len(file.Data))
	}
	if len(payload.Files) == 0 {
		return MaterializedModule{}, errors.New("validated payload contains no files")
	}
	if err := applyDirectoryPolicy(stage, options.Ownership); err != nil {
		return MaterializedModule{}, err
	}

	if _, err := os.Lstat(target); err == nil {
		return MaterializedModule{}, ErrMaterializedVersionExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return MaterializedModule{}, fmt.Errorf("recheck module target: %w", err)
	}
	if err := os.Rename(stage, target); err != nil {
		return MaterializedModule{}, fmt.Errorf("promote module staging directory: %w", err)
	}
	committed = true

	return MaterializedModule{
		ModuleID:      pkg.Manifest.ID,
		Version:       pkg.Manifest.Version,
		PayloadSHA256: pkg.PayloadSHA256,
		Path:          target,
		FileCount:     len(payload.Files),
		ExpandedBytes: expandedBytes,
	}, nil
}

func validateMaterializeIdentity(pkg VerifiedPackage, payload ValidatedPayload) error {
	digest := sha256.Sum256(pkg.Payload)
	actual := hex.EncodeToString(digest[:])
	if pkg.PayloadSHA256 == "" || !strings.EqualFold(pkg.PayloadSHA256, actual) {
		return ErrPayloadIdentityMismatch
	}
	if payload.ModuleID != pkg.Manifest.ID || payload.Version != pkg.Manifest.Version || !strings.EqualFold(payload.PayloadSHA256, pkg.PayloadSHA256) {
		return ErrPayloadIdentityMismatch
	}
	return nil
}

func prepareInstallRoot(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve module install root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return "", fmt.Errorf("create module install root: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect module install root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", ErrUnsafeInstallRoot
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve module install root symlinks: %w", err)
	}
	return filepath.Clean(canonical), nil
}

func validateInstallComponent(value, label string) error {
	if value == "" || value == "." || value == ".." || filepath.IsAbs(value) || filepath.Base(value) != value || strings.ContainsAny(value, `/\\`) || filepath.VolumeName(value) != "" {
		return fmt.Errorf("unsafe %s %q", label, value)
	}
	return nil
}

func ensureTrustedDirectory(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("create module directory %q: %w", path, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect module directory %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: module directory %q is not a trusted directory", ErrUnsafeInstallRoot, path)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("set module directory mode %q: %w", path, err)
	}
	return nil
}

func ensureContained(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return fmt.Errorf("check materialization containment: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("materialization path %q escapes root %q", candidate, root)
	}
	return nil
}

func createMaterializedFile(stage, destination string, file PayloadFile, ownership *MaterializeOwnership) error {
	parent := filepath.Dir(destination)
	if err := ensureStageDirectories(stage, parent); err != nil {
		return err
	}

	handle, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create module payload file %q: %w", file.Path, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = handle.Close()
		}
	}()
	if _, err := handle.Write(file.Data); err != nil {
		return fmt.Errorf("write module payload file %q: %w", file.Path, err)
	}
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("sync module payload file %q: %w", file.Path, err)
	}
	if err := handle.Chmod(os.FileMode(file.Mode)); err != nil {
		return fmt.Errorf("set module payload file mode %q: %w", file.Path, err)
	}
	if ownership != nil {
		if err := handle.Chown(ownership.UID, ownership.GID); err != nil {
			return fmt.Errorf("set module payload file ownership %q: %w", file.Path, err)
		}
	}
	if err := handle.Close(); err != nil {
		return fmt.Errorf("close module payload file %q: %w", file.Path, err)
	}
	closed = true
	return nil
}

func ensureStageDirectories(stage, directory string) error {
	if err := ensureContained(stage, directory); err != nil {
		return err
	}
	relative, err := filepath.Rel(stage, directory)
	if err != nil {
		return fmt.Errorf("resolve staging directory: %w", err)
	}
	current := stage
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		if err := os.Mkdir(current, 0o750); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create staging directory %q: %w", current, err)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect staging directory %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("unsafe staging directory %q", current)
		}
		if err := os.Chmod(current, 0o750); err != nil {
			return fmt.Errorf("set staging directory mode %q: %w", current, err)
		}
	}
	return nil
}

func applyDirectoryPolicy(stage string, ownership *MaterializeOwnership) error {
	return filepath.WalkDir(stage, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe symlinked staging directory %q", path)
		}
		if err := os.Chmod(path, 0o750); err != nil {
			return fmt.Errorf("set staging directory mode %q: %w", path, err)
		}
		if ownership != nil {
			if err := os.Chown(path, ownership.UID, ownership.GID); err != nil {
				return fmt.Errorf("set staging directory ownership %q: %w", path, err)
			}
		}
		return nil
	})
}
