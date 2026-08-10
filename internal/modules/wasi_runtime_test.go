package modules

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var (
	wasmEmpty = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	wasmTrap = []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		0x03, 0x02, 0x01, 0x00,
		0x07, 0x0a, 0x01, 0x06, 0x5f, 0x73, 0x74, 0x61, 0x72, 0x74, 0x00, 0x00,
		0x0a, 0x05, 0x01, 0x03, 0x00, 0x00, 0x0b,
	}
	wasmInfiniteLoop = []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		0x03, 0x02, 0x01, 0x00,
		0x07, 0x0a, 0x01, 0x06, 0x5f, 0x73, 0x74, 0x61, 0x72, 0x74, 0x00, 0x00,
		0x0a, 0x09, 0x01, 0x07, 0x00, 0x03, 0x40, 0x0c, 0x00, 0x0b, 0x0b,
	}
	wasmMemory257Pages = []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x05, 0x04, 0x01, 0x00, 0x81, 0x02,
	}
)

func TestWASIRuntimeExecutesKnownGoodModule(t *testing.T) {
	runtime, module := newWASIRuntimeFixture(t, wasmEmpty, 16, 1, 1)
	if err := runtime.Health(context.Background(), module); err != nil {
		t.Fatalf("health execute: %v", err)
	}
	result, err := runtime.Invoke(context.Background(), module, Invocation{Method: "GET", Path: "/api/modules/test.module/ping"})
	if err != nil {
		t.Fatalf("invoke execute: %v", err)
	}
	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", result.StatusCode)
	}
}

func TestWASIRuntimeRejectsMalformedWasm(t *testing.T) {
	runtime, module := newWASIRuntimeFixture(t, []byte("not-wasm"), 16, 1, 1)
	if err := runtime.Health(context.Background(), module); err == nil {
		t.Fatal("expected malformed wasm to fail")
	}
}

func TestWASIRuntimeSurfacesGuestTrap(t *testing.T) {
	runtime, module := newWASIRuntimeFixture(t, wasmTrap, 16, 1, 1)
	if err := runtime.Health(context.Background(), module); err == nil {
		t.Fatal("expected guest trap to fail")
	}
}

func TestWASIRuntimeTerminatesOnContextDeadline(t *testing.T) {
	runtime, module := newWASIRuntimeFixture(t, wasmInfiniteLoop, 16, 1, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := runtime.Health(ctx, module)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("deadline termination took too long: %s", elapsed)
	}
}

func TestWASIRuntimeEnforcesManifestMemoryLimit(t *testing.T) {
	runtime, module := newWASIRuntimeFixture(t, wasmMemory257Pages, 16, 1, 1)
	if err := runtime.Health(context.Background(), module); err == nil {
		t.Fatal("expected wasm minimum memory above manifest limit to fail")
	}
}

func TestMemoryLimitPagesBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		memoryMB int
		want     uint32
		wantErr  bool
	}{
		{name: "below minimum", memoryMB: 15, wantErr: true},
		{name: "minimum", memoryMB: 16, want: 256},
		{name: "maximum", memoryMB: 512, want: 8192},
		{name: "above maximum", memoryMB: 513, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := memoryLimitPages(tc.memoryMB)
			if tc.wantErr {
				if !errors.Is(err, ErrRuntimeMemoryLimit) {
					t.Fatalf("error = %v, want ErrRuntimeMemoryLimit", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("memoryLimitPages: %v", err)
			}
			if got != tc.want {
				t.Fatalf("pages = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestWASIRuntimeFailsClosedOnActiveVersionMismatch(t *testing.T) {
	runtime, module := newWASIRuntimeFixture(t, wasmEmpty, 16, 1, 1)
	module.Manifest.Version = "2.0.0"
	if err := runtime.Health(context.Background(), module); !errors.Is(err, ErrActiveVersionMismatch) {
		t.Fatalf("error = %v, want ErrActiveVersionMismatch", err)
	}
}

func TestWASIRuntimeRejectsSymlinkEntrypoint(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "test.module")
	versionDir := filepath.Join(moduleDir, "1.0.0")
	if err := os.MkdirAll(versionDir, 0o750); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(versionDir, "target.wasm")
	if err := os.WriteFile(target, wasmEmpty, 0o440); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(versionDir, "main.wasm")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeWASIActiveState(t, moduleDir, "1.0.0")
	runtime, err := NewWASIRuntime(WASIRuntimeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	module := wasiTestModule("1.0.0", 16, 1, 1)
	if err := runtime.Health(context.Background(), module); !errors.Is(err, ErrUnsafeRuntimeEntrypoint) {
		t.Fatalf("error = %v, want ErrUnsafeRuntimeEntrypoint", err)
	}
}

func TestWASIRuntimeConcurrencyLimiterHonorsCancellation(t *testing.T) {
	runtime, _ := newWASIRuntimeFixture(t, wasmEmpty, 16, 1, 1)
	if err := runtime.acquire(context.Background(), "test.module", "1.0.0", 1); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer runtime.release("test.module", "1.0.0")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := runtime.acquire(ctx, "test.module", "1.0.0", 1); !errors.Is(err, ErrRuntimeConcurrencyLimited) {
		t.Fatalf("error = %v, want ErrRuntimeConcurrencyLimited", err)
	}
}

func TestWASIRuntimeRejectsOversizedInvocationBody(t *testing.T) {
	runtime, module := newWASIRuntimeFixture(t, wasmEmpty, 16, 1, 1)
	runtime.maxIOBytes = 8
	_, err := runtime.Invoke(context.Background(), module, Invocation{Body: make([]byte, 9)})
	if !errors.Is(err, ErrRuntimeInputTooLarge) {
		t.Fatalf("error = %v, want ErrRuntimeInputTooLarge", err)
	}
}

func newWASIRuntimeFixture(t *testing.T, wasm []byte, memoryMB, timeoutSeconds, maxConcurrency int) (*WASIRuntime, InstalledModule) {
	t.Helper()
	root := t.TempDir()
	moduleDir := filepath.Join(root, "test.module")
	versionDir := filepath.Join(moduleDir, "1.0.0")
	if err := os.MkdirAll(versionDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "main.wasm"), wasm, 0o440); err != nil {
		t.Fatal(err)
	}
	writeWASIActiveState(t, moduleDir, "1.0.0")
	runtime, err := NewWASIRuntime(WASIRuntimeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	return runtime, wasiTestModule("1.0.0", memoryMB, timeoutSeconds, maxConcurrency)
}

func wasiTestModule(version string, memoryMB, timeoutSeconds, maxConcurrency int) InstalledModule {
	return InstalledModule{
		Manifest: Manifest{
			ID:         "test.module",
			Name:       "Test Module",
			Version:    version,
			APIVersion: CurrentAPIVersion,
			Publisher:  "Strata",
			Runtime: &RuntimeSpec{
				Kind:           RuntimeKindWASI,
				Entrypoint:     "main.wasm",
				MemoryMB:       memoryMB,
				TimeoutSeconds: timeoutSeconds,
				MaxConcurrency: maxConcurrency,
				Network:        RuntimeNetworkNone,
			},
		},
		State: StateEnabled,
	}
}

func writeWASIActiveState(t *testing.T, moduleDir, version string) {
	t.Helper()
	data := []byte(`{"schema_version":1,"active_version":"` + version + `"}` + "\n")
	if err := os.WriteFile(filepath.Join(moduleDir, ".active.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
