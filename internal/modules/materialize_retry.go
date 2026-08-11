package modules

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// MaterializePayloadRetrySafe materializes a validated signed payload, or
// accepts an existing immutable version only when every regular file, mode, and
// byte exactly matches the validated payload. This allows an installer to
// recover after a crash between filesystem promotion and durable lifecycle
// persistence without treating an unrelated existing directory as trusted.
// Every accepted version also receives immutable signed release metadata before
// the operation is considered complete.
func MaterializePayloadRetrySafe(pkg VerifiedPackage, payload ValidatedPayload, options MaterializeOptions) (MaterializedModule, error) {
	result, err := MaterializePayload(pkg, payload, options)
	if err == nil {
		return finalizeRetrySafeMaterialization(pkg, options, result)
	}
	if !errors.Is(err, ErrMaterializedVersionExists) {
		return MaterializedModule{}, err
	}
	if err := validateMaterializeIdentity(pkg, payload); err != nil {
		return MaterializedModule{}, err
	}
	result, err = verifyExistingMaterializedPayload(pkg, payload, options)
	if err != nil {
		return MaterializedModule{}, err
	}
	return finalizeRetrySafeMaterialization(pkg, options, result)
}

func finalizeRetrySafeMaterialization(pkg VerifiedPackage, options MaterializeOptions, result MaterializedModule) (MaterializedModule, error) {
	if _, err := PersistReleaseMetadata(options.Root, pkg); err != nil {
		return MaterializedModule{}, fmt.Errorf("persist signed module release identity: %w", err)
	}
	return result, nil
}

func verifyExistingMaterializedPayload(pkg VerifiedPackage, payload ValidatedPayload, options MaterializeOptions) (MaterializedModule, error) {
	root, err := prepareInstallRoot(options.Root)
	if err != nil {
		return MaterializedModule{}, err
	}
	if err := validateInstallComponent(pkg.Manifest.ID, "module id"); err != nil {
		return MaterializedModule{}, err
	}
	if err := validateInstallComponent(pkg.Manifest.Version, "module version"); err != nil {
		return MaterializedModule{}, err
	}
	target := filepath.Join(root, pkg.Manifest.ID, pkg.Manifest.Version)
	if err := ensureContained(root, target); err != nil {
		return MaterializedModule{}, err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return MaterializedModule{}, fmt.Errorf("inspect existing materialized version: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return MaterializedModule{}, ErrMaterializedVersionExists
	}

	existingRoot, err := os.OpenRoot(target)
	if err != nil {
		return MaterializedModule{}, fmt.Errorf("open existing materialized version root: %w", err)
	}
	defer func() { _ = existingRoot.Close() }()

	expected := make(map[string]PayloadFile, len(payload.Files))
	var expanded int64
	for _, file := range payload.Files {
		clean, err := cleanPayloadPath(file.Path)
		if err != nil {
			return MaterializedModule{}, err
		}
		if _, duplicate := expected[clean]; duplicate {
			return MaterializedModule{}, fmt.Errorf("duplicate validated payload path %q", clean)
		}
		expected[clean] = file
		expanded += int64(len(file.Data))
	}

	seen := 0
	err = fs.WalkDir(existingRoot.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrMaterializedVersionExists
		}
		if entry.IsDir() {
			return nil
		}
		info, err := existingRoot.Lstat(name)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return ErrMaterializedVersionExists
		}
		file, ok := expected[filepath.ToSlash(name)]
		if !ok {
			return ErrMaterializedVersionExists
		}
		if info.Mode().Perm() != payloadFileMode(file.Mode).Perm() || info.Size() != int64(len(file.Data)) {
			return ErrMaterializedVersionExists
		}
		handle, err := existingRoot.Open(name)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(handle, int64(len(file.Data))+1))
		closeErr := handle.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !bytes.Equal(data, file.Data) {
			return ErrMaterializedVersionExists
		}
		seen++
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrMaterializedVersionExists) {
			return MaterializedModule{}, ErrMaterializedVersionExists
		}
		return MaterializedModule{}, fmt.Errorf("verify existing materialized version: %w", err)
	}
	if seen != len(expected) {
		return MaterializedModule{}, ErrMaterializedVersionExists
	}
	return MaterializedModule{
		ModuleID:      pkg.Manifest.ID,
		Version:       pkg.Manifest.Version,
		PayloadSHA256: pkg.PayloadSHA256,
		Path:          target,
		FileCount:     len(payload.Files),
		ExpandedBytes: expanded,
	}, nil
}
