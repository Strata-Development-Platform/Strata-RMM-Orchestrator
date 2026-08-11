package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const (
	defaultWASIIOBytes   = 1 << 20
	maxWASIBinaryBytes   = 64 << 20
	wasmPageBytes        = 64 << 10
	wasmPagesPerMiB      = (1024 * 1024) / wasmPageBytes
	wasiInvocationSchema = 1
)

var (
	ErrRuntimeSpecRequired       = errors.New("module runtime declaration is required")
	ErrActiveVersionMismatch     = errors.New("active module version does not match registry manifest")
	ErrUnsafeRuntimeEntrypoint   = errors.New("module runtime entrypoint is unsafe")
	ErrRuntimeInputTooLarge      = errors.New("module runtime input exceeds limit")
	ErrRuntimeOutputTooLarge     = errors.New("module runtime output exceeds limit")
	ErrRuntimeConcurrencyLimited = errors.New("module runtime concurrency wait canceled")
	ErrRuntimeMemoryLimit        = errors.New("module runtime memory limit is invalid")
	ErrRuntimeBrokerUnavailable  = errors.New("module brokered runtime capability is unavailable")
)

type WASIRuntimeOptions struct {
	Root       string
	MaxIOBytes int
	Broker     *CapabilityBroker
}

type wasiLimiter struct {
	limit int
	sem   chan struct{}
}

// WASIRuntime executes the active immutable module version with wazero. Each
// invocation receives a fresh wazero runtime configured without ambient host
// filesystem, argv, environment, or raw network access. Brokered modules may
// receive only the reviewed strata_broker host ABI backed by CapabilityBroker.
type WASIRuntime struct {
	root       string
	maxIOBytes int
	broker     *CapabilityBroker

	mu       sync.Mutex
	limiters map[string]*wasiLimiter
}

type wasiInvocationEnvelope struct {
	SchemaVersion int    `json:"schema_version"`
	Operation     string `json:"operation"`
	Method        string `json:"method,omitempty"`
	Path          string `json:"path,omitempty"`
	Permission    string `json:"permission,omitempty"`
	Body          []byte `json:"body,omitempty"`
}

func NewWASIRuntime(options WASIRuntimeOptions) (*WASIRuntime, error) {
	if strings.TrimSpace(options.Root) == "" {
		return nil, errors.New("module runtime root is required")
	}
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve module runtime root: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect module runtime root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, ErrUnsafeInstallRoot
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve module runtime root symlinks: %w", err)
	}
	maxIOBytes := options.MaxIOBytes
	if maxIOBytes <= 0 {
		maxIOBytes = defaultWASIIOBytes
	}
	return &WASIRuntime{
		root:       canonicalRoot,
		maxIOBytes: maxIOBytes,
		broker:     options.Broker,
		limiters:   make(map[string]*wasiLimiter),
	}, nil
}

func (r *WASIRuntime) Health(ctx context.Context, module InstalledModule) error {
	input, err := json.Marshal(wasiInvocationEnvelope{SchemaVersion: wasiInvocationSchema, Operation: "health"})
	if err != nil {
		return fmt.Errorf("encode WASI health envelope: %w", err)
	}
	_, err = r.execute(ctx, module, input, ResourceScope{}, false)
	return err
}

func (r *WASIRuntime) Invoke(ctx context.Context, module InstalledModule, invocation Invocation) (InvocationResult, error) {
	if len(invocation.Body) > r.maxIOBytes {
		return InvocationResult{}, ErrRuntimeInputTooLarge
	}
	input, err := json.Marshal(wasiInvocationEnvelope{
		SchemaVersion: wasiInvocationSchema,
		Operation:     "invoke",
		Method:        invocation.Method,
		Path:          invocation.Path,
		Permission:    invocation.Permission,
		Body:          invocation.Body,
	})
	if err != nil {
		return InvocationResult{}, fmt.Errorf("encode WASI invocation envelope: %w", err)
	}
	if len(input) > r.maxIOBytes*2 {
		return InvocationResult{}, ErrRuntimeInputTooLarge
	}
	output, err := r.execute(ctx, module, input, invocation.Scope, true)
	if err != nil {
		return InvocationResult{}, err
	}
	return InvocationResult{StatusCode: 200, Body: output}, nil
}

func (r *WASIRuntime) execute(ctx context.Context, module InstalledModule, input []byte, scope ResourceScope, brokerAllowed bool) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("module runtime context is required")
	}
	spec, version, wasmPath, err := r.resolveRuntime(module)
	if err != nil {
		return nil, err
	}
	if err := r.acquire(ctx, module.Manifest.ID, version, spec.MaxConcurrency); err != nil {
		return nil, err
	}
	defer r.release(module.Manifest.ID, version)

	wasm, err := readBoundedRegularFile(wasmPath, maxWASIBinaryBytes)
	if err != nil {
		return nil, err
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(spec.TimeoutSeconds)*time.Second)
	defer cancel()
	pages, err := memoryLimitPages(spec.MemoryMB)
	if err != nil {
		return nil, err
	}
	config := wazero.NewRuntimeConfig().WithMemoryLimitPages(pages).WithCloseOnContextDone(true)
	engine := wazero.NewRuntimeWithConfig(execCtx, config)
	defer func() { _ = engine.Close(context.Background()) }()

	if _, err := wasi_snapshot_preview1.Instantiate(execCtx, engine); err != nil {
		return nil, fmt.Errorf("instantiate WASI host: %w", err)
	}
	if spec.Network == RuntimeNetworkBrokered {
		if r.broker == nil {
			return nil, ErrRuntimeBrokerUnavailable
		}
		if err := r.instantiateBrokerHost(execCtx, engine, module, scope, brokerAllowed); err != nil {
			return nil, err
		}
	}
	compiled, err := engine.CompileModule(execCtx, wasm)
	if err != nil {
		return nil, fmt.Errorf("compile WASI module: %s", boundedRuntimeError(err))
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
			return nil, execCtx.Err()
		}
		return nil, fmt.Errorf("execute WASI module: %s", boundedRuntimeError(err))
	}
	if stdout.overflow || stderr.overflow {
		return nil, ErrRuntimeOutputTooLarge
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

func memoryLimitPages(memoryMB int) (uint32, error) {
	if memoryMB < 16 || memoryMB > 512 {
		return 0, ErrRuntimeMemoryLimit
	}

	// Avoid narrowing an int into wazero's uint32 page count. RuntimeSpec uses
	// int for JSON/YAML compatibility, so accumulate the bounded count directly
	// in the destination type after validating the manifest range.
	var pages uint32
	for megabyte := 0; megabyte < memoryMB; megabyte++ {
		pages += wasmPagesPerMiB
	}
	return pages, nil
}

func (r *WASIRuntime) resolveRuntime(module InstalledModule) (RuntimeSpec, string, string, error) {
	if module.Manifest.Runtime == nil {
		return RuntimeSpec{}, "", "", ErrRuntimeSpecRequired
	}
	spec := *module.Manifest.Runtime
	if err := spec.Validate(); err != nil {
		return RuntimeSpec{}, "", "", fmt.Errorf("revalidate module runtime: %w", err)
	}
	state, err := ReadActiveVersion(r.root, module.Manifest.ID)
	if err != nil {
		return RuntimeSpec{}, "", "", err
	}
	if state.ActiveVersion != module.Manifest.Version {
		return RuntimeSpec{}, "", "", ErrActiveVersionMismatch
	}
	versionRoot := filepath.Join(r.root, module.Manifest.ID, state.ActiveVersion)
	entrypoint := filepath.Join(versionRoot, filepath.FromSlash(spec.Entrypoint))
	if err := ensureContained(versionRoot, entrypoint); err != nil {
		return RuntimeSpec{}, "", "", ErrUnsafeRuntimeEntrypoint
	}
	if err := rejectSymlinkPath(versionRoot, entrypoint); err != nil {
		return RuntimeSpec{}, "", "", err
	}
	return spec, state.ActiveVersion, entrypoint, nil
}

func rejectSymlinkPath(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrUnsafeRuntimeEntrypoint
	}
	current := root
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect module runtime entrypoint: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafeRuntimeEntrypoint
		}
	}
	return nil
}

func readBoundedRegularFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open WASI entrypoint: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect WASI entrypoint: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return nil, ErrUnsafeRuntimeEntrypoint
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read WASI entrypoint: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, ErrUnsafeRuntimeEntrypoint
	}
	return data, nil
}

func (r *WASIRuntime) acquire(ctx context.Context, moduleID, version string, limit int) error {
	key := moduleID + "\x00" + version
	r.mu.Lock()
	limiter := r.limiters[key]
	if limiter == nil {
		limiter = &wasiLimiter{limit: limit, sem: make(chan struct{}, limit)}
		r.limiters[key] = limiter
	} else if limiter.limit != limit {
		r.mu.Unlock()
		return errors.New("module runtime concurrency declaration changed for active version")
	}
	r.mu.Unlock()

	select {
	case limiter.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrRuntimeConcurrencyLimited, ctx.Err())
	}
}

func (r *WASIRuntime) release(moduleID, version string) {
	key := moduleID + "\x00" + version
	r.mu.Lock()
	limiter := r.limiters[key]
	r.mu.Unlock()
	if limiter != nil {
		<-limiter.sem
	}
}

type boundedBuffer struct {
	buf      bytes.Buffer
	limit    int
	overflow bool
}

func newBoundedBuffer(limit int) *boundedBuffer { return &boundedBuffer{limit: limit} }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.overflow {
		return 0, ErrRuntimeOutputTooLarge
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.overflow = true
		return 0, ErrRuntimeOutputTooLarge
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.overflow = true
		return remaining, ErrRuntimeOutputTooLarge
	}
	return b.buf.Write(p)
}

func (b *boundedBuffer) Bytes() []byte { return b.buf.Bytes() }

func boundedRuntimeError(err error) string {
	const max = 512
	message := strings.ReplaceAll(err.Error(), "\x00", "")
	if len(message) > max {
		message = message[:max] + "..."
	}
	return message
}
