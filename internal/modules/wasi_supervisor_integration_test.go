package modules

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestWASISupervisorQuarantinesRepeatedRealGuestTrap(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	manifest := alphaReferenceManifest("1.0.0")
	manifest.Routes = []Route{{
		Path:       "/api/modules/alpha.reference/trap",
		Methods:    []string{"GET"},
		Permission: "devices.read",
	}}
	pkg, payload := signedReferencePayload(t, manifest, wasmTrap)
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, manifest.ID, manifest.Version); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if _, err := registry.Install(pkg.Manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(pkg.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	runtimeEngine, err := NewWASIRuntime(WASIRuntimeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewSupervisor(registry, runtimeEngine, SupervisorOptions{InvocationTimeout: time.Second, FailureThreshold: 2})
	if err != nil {
		t.Fatal(err)
	}
	invocation := Invocation{Method: "GET", Path: manifest.Routes[0].Path, Permission: "devices.read"}
	for i := 0; i < 2; i++ {
		if _, err := supervisor.Invoke(context.Background(), manifest.ID, invocation); !errors.Is(err, ErrRuntimeUnavailable) {
			t.Fatalf("trap %d error = %v, want ErrRuntimeUnavailable", i+1, err)
		}
	}
	current, err := registry.Get(manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != StateQuarantined {
		t.Fatalf("state = %q, want quarantined", current.State)
	}
	if _, err := supervisor.Invoke(context.Background(), manifest.ID, invocation); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("post-quarantine error = %v, want ErrQuarantined", err)
	}
}

func TestWASISupervisorHonorsCallerCancellationWithRealGuest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	manifest := alphaReferenceManifest("1.0.0")
	manifest.Routes = []Route{{
		Path:       "/api/modules/alpha.reference/loop",
		Methods:    []string{"GET"},
		Permission: "devices.read",
	}}
	pkg, payload := signedReferencePayload(t, manifest, wasmInfiniteLoop)
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, manifest.ID, manifest.Version); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if _, err := registry.Install(pkg.Manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(pkg.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	runtimeEngine, err := NewWASIRuntime(WASIRuntimeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewSupervisor(registry, runtimeEngine, SupervisorOptions{InvocationTimeout: 5 * time.Second, FailureThreshold: 3})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = supervisor.Invoke(ctx, manifest.ID, Invocation{Method: "GET", Path: manifest.Routes[0].Path, Permission: "devices.read"})
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("error = %v, want ErrRuntimeUnavailable", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("caller cancellation took too long: %s", elapsed)
	}
	if current, getErr := registry.Get(manifest.ID); getErr != nil {
		t.Fatal(getErr)
	} else if current.State != StateEnabled {
		t.Fatalf("single cancellation changed state to %q", current.State)
	}
}

func TestWASIRuntimeRepeatedExecutionDoesNotGrowGoroutinesUnboundedly(t *testing.T) {
	runtimeEngine, module := newWASIRuntimeFixture(t, wasmEmpty, 16, 1, 2)
	baseline := runtime.NumGoroutine()
	for i := 0; i < 100; i++ {
		if err := runtimeEngine.Health(context.Background(), module); err != nil {
			t.Fatalf("health %d: %v", i, err)
		}
		if _, err := runtimeEngine.Invoke(context.Background(), module, Invocation{}); err != nil {
			t.Fatalf("invoke %d: %v", i, err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline+8 && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > baseline+8 {
		t.Fatalf("goroutines grew from %d to %d after repeated runtime creation/cleanup", baseline, got)
	}
}
