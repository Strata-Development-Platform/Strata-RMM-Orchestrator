package modules

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

const maxCandidateHealthTimeout = 2 * time.Minute

var (
	ErrCandidateHealthCheckRequired = errors.New("module candidate health check is required")
	ErrCandidateHealthTimeout       = errors.New("module candidate health check timed out")
	ErrCandidateUnhealthy           = errors.New("module candidate failed health check")
)

// CandidateActivation describes an already-materialized immutable module
// version presented to a trusted host-side health checker. The modules package
// does not execute the candidate itself.
type CandidateActivation struct {
	ModuleID string
	Version  string
	Path     string
}

// CandidateHealthCheck is implemented by trusted host/runtime integration. It
// must honor ctx cancellation and must not mutate the immutable candidate tree.
type CandidateHealthCheck func(ctx context.Context, candidate CandidateActivation) error

// ActivateMaterializedVersionWithHealth validates an already-materialized
// candidate, serializes activation, runs a bounded trusted health check, and
// commits active-version metadata only after the check succeeds. A failed or
// timed-out health check leaves the prior activation state unchanged.
//
// This function does not launch, execute, mount, or grant credentials to module
// code. The supplied checker is a host-side integration contract for a later
// isolated runtime adapter.
func ActivateMaterializedVersionWithHealth(
	ctx context.Context,
	root, moduleID, version string,
	timeout time.Duration,
	check CandidateHealthCheck,
) (ActivationState, error) {
	if ctx == nil {
		return ActivationState{}, errors.New("activation context is required")
	}
	if check == nil {
		return ActivationState{}, ErrCandidateHealthCheckRequired
	}
	if timeout <= 0 || timeout > maxCandidateHealthTimeout {
		return ActivationState{}, fmt.Errorf("candidate health timeout must be between 1ns and %s", maxCandidateHealthTimeout)
	}

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

	candidatePath := filepath.Join(moduleDir, version)
	if err := ensureContained(moduleDir, candidatePath); err != nil {
		return ActivationState{}, err
	}
	candidate := CandidateActivation{ModuleID: moduleID, Version: version, Path: candidatePath}

	healthCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := check(healthCtx, candidate); err != nil {
		if errors.Is(healthCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return ActivationState{}, ErrCandidateHealthTimeout
		}
		return ActivationState{}, fmt.Errorf("%w: %v", ErrCandidateUnhealthy, err)
	}
	if errors.Is(healthCtx.Err(), context.DeadlineExceeded) {
		return ActivationState{}, ErrCandidateHealthTimeout
	}
	if err := healthCtx.Err(); err != nil {
		return ActivationState{}, fmt.Errorf("candidate health check canceled: %w", err)
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
