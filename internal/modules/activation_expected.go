package modules

import (
	"errors"
	"fmt"
)

var ErrActivationStateChanged = errors.New("module activation state changed")

// ActivateExpectedPreviousVersion performs a rollback against a specific
// activation snapshot. It is retry-safe: if the exact rollback has already
// been applied, the reversed snapshot is returned unchanged. Any unrelated
// activation change fails closed instead of selecting a stale target.
func ActivateExpectedPreviousVersion(root, moduleID string, expected ActivationState) (ActivationState, error) {
	if expected.SchemaVersion != activationStateSchemaVersion || expected.ActiveVersion == "" || expected.PreviousVersion == "" {
		return ActivationState{}, ErrNoRollbackVersion
	}
	moduleDir, err := existingModuleDirectory(root, moduleID)
	if err != nil {
		return ActivationState{}, err
	}
	if err := validateMaterializedVersionDirectory(moduleDir, expected.ActiveVersion); err != nil {
		return ActivationState{}, fmt.Errorf("validate expected active version: %w", err)
	}
	if err := validateMaterializedVersionDirectory(moduleDir, expected.PreviousVersion); err != nil {
		return ActivationState{}, fmt.Errorf("validate expected rollback version: %w", err)
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
	target := ActivationState{
		SchemaVersion:   activationStateSchemaVersion,
		ActiveVersion:   expected.PreviousVersion,
		PreviousVersion: expected.ActiveVersion,
	}
	if current == target {
		return current, nil
	}
	if current != expected {
		return ActivationState{}, fmt.Errorf("%w: expected active=%q previous=%q, found active=%q previous=%q", ErrActivationStateChanged, expected.ActiveVersion, expected.PreviousVersion, current.ActiveVersion, current.PreviousVersion)
	}
	if err := writeActivationState(moduleDir, target); err != nil {
		return ActivationState{}, err
	}
	return target, nil
}
