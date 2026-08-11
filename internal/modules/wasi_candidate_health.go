package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// HealthCandidate executes a materialized candidate version without changing
// active-version metadata. This is the runtime bridge required by
// ActivateMaterializedVersionWithHealth: the candidate is proven executable
// under the same sandbox/resource policy before activation is committed.
func (r *WASIRuntime) HealthCandidate(ctx context.Context, module InstalledModule, candidate CandidateActivation) error {
	if r == nil {
		return errors.New("module runtime is required")
	}
	if ctx == nil {
		return errors.New("module runtime context is required")
	}
	if module.Manifest.Runtime == nil {
		return ErrRuntimeSpecRequired
	}
	if candidate.ModuleID != module.Manifest.ID || candidate.Version != module.Manifest.Version {
		return ErrActiveVersionMismatch
	}
	if err := module.Manifest.Runtime.Validate(); err != nil {
		return fmt.Errorf("revalidate module runtime: %w", err)
	}

	versionRoot := filepath.Join(r.root, candidate.ModuleID, candidate.Version)
	if filepath.Clean(candidate.Path) != filepath.Clean(versionRoot) {
		return ErrUnsafeRuntimeEntrypoint
	}
	if err := ensureContained(r.root, versionRoot); err != nil {
		return ErrUnsafeRuntimeEntrypoint
	}
	info, err := os.Lstat(versionRoot)
	if err != nil {
		return fmt.Errorf("inspect candidate runtime root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrUnsafeRuntimeEntrypoint
	}

	spec := *module.Manifest.Runtime
	entrypoint := filepath.Join(versionRoot, filepath.FromSlash(spec.Entrypoint))
	if err := ensureContained(versionRoot, entrypoint); err != nil {
		return ErrUnsafeRuntimeEntrypoint
	}
	if err := rejectSymlinkPath(versionRoot, entrypoint); err != nil {
		return err
	}
	wasm, err := readBoundedRegularFile(entrypoint, maxWASIBinaryBytes)
	if err != nil {
		return err
	}

	input, err := json.Marshal(wasiInvocationEnvelope{SchemaVersion: wasiInvocationSchema, Operation: "health"})
	if err != nil {
		return fmt.Errorf("encode WASI health envelope: %w", err)
	}
	if err := r.acquire(ctx, module.Manifest.ID, candidate.Version, spec.MaxConcurrency); err != nil {
		return err
	}
	defer r.release(module.Manifest.ID, candidate.Version)

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(spec.TimeoutSeconds)*time.Second)
	defer cancel()
	pages, err := memoryLimitPages(spec.MemoryMB)
	if err != nil {
		return err
	}
	config := wazero.NewRuntimeConfig().WithMemoryLimitPages(pages).WithCloseOnContextDone(true)
	engine := wazero.NewRuntimeWithConfig(execCtx, config)
	defer func() { _ = engine.Close(context.Background()) }()

	if _, err := wasi_snapshot_preview1.Instantiate(execCtx, engine); err != nil {
		return fmt.Errorf("instantiate WASI host: %w", err)
	}
	if spec.Network == RuntimeNetworkBrokered {
		if r.broker == nil {
			return ErrRuntimeBrokerUnavailable
		}
		if err := r.instantiateBrokerHost(execCtx, engine, module, ResourceScope{}, false); err != nil {
			return err
		}
	}
	compiled, err := engine.CompileModule(execCtx, wasm)
	if err != nil {
		return fmt.Errorf("compile WASI candidate module: %s", boundedRuntimeError(err))
	}
	defer func() { _ = compiled.Close(context.Background()) }()

	stdout := newBoundedBuffer(r.maxIOBytes)
	stderr := newBoundedBuffer(r.maxIOBytes)
	moduleConfig := wazero.NewModuleConfig().
		WithName("").
		WithStdin(bytes.NewReader(input)).
		WithStdout(stdout).
		WithStderr(stderr)
	instance, err := engine.InstantiateModule(execCtx, compiled, moduleConfig)
	if instance != nil {
		defer func() { _ = instance.Close(context.Background()) }()
	}
	if err != nil {
		if execCtx.Err() != nil {
			return execCtx.Err()
		}
		return fmt.Errorf("execute WASI candidate module: %s", boundedRuntimeError(err))
	}
	if stdout.overflow || stderr.overflow {
		return ErrRuntimeOutputTooLarge
	}
	return nil
}
