package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const activationStateSchemaVersion = 1

var (
	ErrNoActiveVersion       = errors.New("module has no active version")
	ErrNoRollbackVersion     = errors.New("module has no rollback version")
	ErrActivationInProgress  = errors.New("module activation is already in progress")
	ErrInvalidActivationState = errors.New("module activation state is invalid")
)

// ActivationState records which immutable materialized version is selected for
// a module and the single previous version available for rollback. Selection
// does not execute module code or change lifecycle state.
type ActivationState struct {
	SchemaVersion   int    `json:"schema_version"`
	ActiveVersion   string `json:"active_version"`
	PreviousVersion string `json:"previous_version,omitempty"`
}

// ActivateMaterializedVersion atomically selects an already-materialized
// immutable module version. The prior active version is retained as the
// rollback target. Selecting the already-active version is idempotent.
func ActivateMaterializedVersion(root, moduleID, version string) (ActivationState, error) {
	moduleDir, err := existingModuleDirectory(root, moduleID)
	if err != nil {
		return ActivationState{}, err
	}
	if err := validateMaterializedVersionDirectory(moduleDir, version); err != nil {
		return ActivationState{}, err
	}

	unlock, err := acquireActivationLock(moduleDir)
	if err != nil {
		return ActivationState{}, err
	}
	defer unlock()

	current, err := readActivationState(moduleDir)
	if err != nil && !errors.Is(err, ErrNoActiveVersion) {
		return ActivationState{}, err
	}
	if err == nil && current.ActiveVersion == version {
		return current, nil
	}

	next := ActivationState{SchemaVersion: activationStateSchemaVersion, ActiveVersion: version}
	if err == nil {
		next.PreviousVersion = current.ActiveVersion
	}
	if err := writeActivationState(moduleDir, next); err != nil {
		return ActivationState{}, err
	}
	return next, nil
}

// RollbackActiveVersion atomically swaps the active version with the recorded
// previous version. The swap is reversible because the displaced active
// version becomes the new rollback target.
func RollbackActiveVersion(root, moduleID string) (ActivationState, error) {
	moduleDir, err := existingModuleDirectory(root, moduleID)
	if err != nil {
		return ActivationState{}, err
	}

	unlock, err := acquireActivationLock(moduleDir)
	if err != nil {
		return ActivationState{}, err
	}
	defer unlock()

	current, err := readActivationState(moduleDir)
	if err != nil {
		return ActivationState{}, err
	}
	if current.PreviousVersion == "" {
		return ActivationState{}, ErrNoRollbackVersion
	}
	if err := validateMaterializedVersionDirectory(moduleDir, current.PreviousVersion); err != nil {
		return ActivationState{}, fmt.Errorf("validate rollback version: %w", err)
	}

	next := ActivationState{
		SchemaVersion:   activationStateSchemaVersion,
		ActiveVersion:   current.PreviousVersion,
		PreviousVersion: current.ActiveVersion,
	}
	if err := writeActivationState(moduleDir, next); err != nil {
		return ActivationState{}, err
	}
	return next, nil
}

// ReadActiveVersion returns the durable active-version selection for a module.
func ReadActiveVersion(root, moduleID string) (ActivationState, error) {
	moduleDir, err := existingModuleDirectory(root, moduleID)
	if err != nil {
		return ActivationState{}, err
	}
	state, err := readActivationState(moduleDir)
	if err != nil {
		return ActivationState{}, err
	}
	if err := validateMaterializedVersionDirectory(moduleDir, state.ActiveVersion); err != nil {
		return ActivationState{}, fmt.Errorf("validate active version: %w", err)
	}
	if state.PreviousVersion != "" {
		if err := validateMaterializedVersionDirectory(moduleDir, state.PreviousVersion); err != nil {
			return ActivationState{}, fmt.Errorf("validate previous version: %w", err)
		}
	}
	return state, nil
}

func existingModuleDirectory(root, moduleID string) (string, error) {
	if err := validateInstallComponent(moduleID, "module id"); err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve module install root: %w", err)
	}
	rootInfo, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect module install root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", ErrUnsafeInstallRoot
	}
	canonicalRoot, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve module install root symlinks: %w", err)
	}
	moduleDir := filepath.Join(canonicalRoot, moduleID)
	if err := ensureContained(canonicalRoot, moduleDir); err != nil {
		return "", err
	}
	info, err := os.Lstat(moduleDir)
	if err != nil {
		return "", fmt.Errorf("inspect module directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", ErrUnsafeInstallRoot
	}
	return moduleDir, nil
}

func validateMaterializedVersionDirectory(moduleDir, version string) error {
	if err := validateInstallComponent(version, "module version"); err != nil {
		return err
	}
	target := filepath.Join(moduleDir, version)
	if err := ensureContained(moduleDir, target); err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return fmt.Errorf("inspect materialized module version: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("materialized module version %q is not a trusted directory", version)
	}
	return nil
}

func acquireActivationLock(moduleDir string) (func(), error) {
	lockPath := filepath.Join(moduleDir, ".activation.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, ErrActivationInProgress
		}
		return nil, fmt.Errorf("create module activation lock: %w", err)
	}
	if err := lock.Close(); err != nil {
		if removeErr := os.Remove(lockPath); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			return nil, fmt.Errorf("close activation lock: %v; remove failed lock: %w", err, removeErr)
		}
		return nil, fmt.Errorf("close module activation lock: %w", err)
	}
	return func() {
		_ = os.Remove(lockPath)
	}, nil
}

func readActivationState(moduleDir string) (ActivationState, error) {
	statePath := filepath.Join(moduleDir, ".active.json")
	info, err := os.Lstat(statePath)
	if errors.Is(err, fs.ErrNotExist) {
		return ActivationState{}, ErrNoActiveVersion
	}
	if err != nil {
		return ActivationState{}, fmt.Errorf("inspect module activation state: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ActivationState{}, ErrInvalidActivationState
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		return ActivationState{}, fmt.Errorf("read module activation state: %w", err)
	}
	var state ActivationState
	if err := json.Unmarshal(data, &state); err != nil {
		return ActivationState{}, fmt.Errorf("%w: decode state: %v", ErrInvalidActivationState, err)
	}
	if state.SchemaVersion != activationStateSchemaVersion || state.ActiveVersion == "" {
		return ActivationState{}, ErrInvalidActivationState
	}
	if err := validateInstallComponent(state.ActiveVersion, "active module version"); err != nil {
		return ActivationState{}, fmt.Errorf("%w: %v", ErrInvalidActivationState, err)
	}
	if state.PreviousVersion != "" {
		if err := validateInstallComponent(state.PreviousVersion, "previous module version"); err != nil {
			return ActivationState{}, fmt.Errorf("%w: %v", ErrInvalidActivationState, err)
		}
	}
	return state, nil
}

func writeActivationState(moduleDir string, state ActivationState) error {
	if state.SchemaVersion != activationStateSchemaVersion || state.ActiveVersion == "" {
		return ErrInvalidActivationState
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode module activation state: %w", err)
	}
	data = append(data, '\n')

	temp, err := os.CreateTemp(moduleDir, ".active.json.tmp-")
	if err != nil {
		return fmt.Errorf("create temporary module activation state: %w", err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set temporary activation state mode: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary activation state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary activation state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary activation state: %w", err)
	}

	statePath := filepath.Join(moduleDir, ".active.json")
	if info, err := os.Lstat(statePath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return ErrInvalidActivationState
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect existing activation state: %w", err)
	}
	if err := os.Rename(tempPath, statePath); err != nil {
		return fmt.Errorf("promote module activation state: %w", err)
	}
	cleanup = false
	return nil
}
