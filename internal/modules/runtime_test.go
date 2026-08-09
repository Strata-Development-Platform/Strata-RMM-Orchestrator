package modules

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRuntime struct {
	healthErr error
	invokeErr error
	calls     int
	block     bool
}

func (f *fakeRuntime) Health(context.Context, InstalledModule) error { return f.healthErr }

func (f *fakeRuntime) Invoke(ctx context.Context, _ InstalledModule, _ Invocation) (InvocationResult, error) {
	f.calls++
	if f.block {
		<-ctx.Done()
		return InvocationResult{}, ctx.Err()
	}
	if f.invokeErr != nil {
		return InvocationResult{}, f.invokeErr
	}
	return InvocationResult{StatusCode: 200, Body: []byte(`{"ok":true}`)}, nil
}

func enabledModuleSupervisor(t *testing.T, runtime Runtime, threshold int) (*Registry, *Supervisor) {
	t.Helper()
	registry := NewRegistry()
	if _, err := registry.Install(validManifest()); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(validManifest().ID); err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewSupervisor(registry, runtime, SupervisorOptions{
		InvocationTimeout: time.Second,
		FailureThreshold:  threshold,
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry, supervisor
}

func TestSupervisorRejectsDisabledModuleBeforeRuntime(t *testing.T) {
	registry := NewRegistry()
	manifest := validManifest()
	if _, err := registry.Install(manifest); err != nil {
		t.Fatal(err)
	}
	runtime := &fakeRuntime{}
	supervisor, err := NewSupervisor(registry, runtime, SupervisorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = supervisor.Invoke(context.Background(), manifest.ID, Invocation{
		Method: "GET", Path: manifest.Routes[0].Path, Permission: "devices.read",
	})
	if !errors.Is(err, ErrModuleDisabled) {
		t.Fatalf("Invoke() error = %v, want ErrModuleDisabled", err)
	}
	if runtime.calls != 0 {
		t.Fatalf("runtime called %d times for disabled module", runtime.calls)
	}
}

func TestSupervisorRequiresDeclaredRouteMethodAndPermission(t *testing.T) {
	_, supervisor := enabledModuleSupervisor(t, &fakeRuntime{}, 3)
	manifest := validManifest()

	tests := []struct {
		name string
		inv  Invocation
		want error
	}{
		{"route escape", Invocation{Method: "GET", Path: "/api/v1/devices", Permission: "devices.read"}, ErrRouteNotDeclared},
		{"method escalation", Invocation{Method: "POST", Path: manifest.Routes[0].Path, Permission: "devices.read"}, ErrMethodNotDeclared},
		{"permission mismatch", Invocation{Method: "GET", Path: manifest.Routes[0].Path, Permission: "alerts.write"}, ErrPermissionMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := supervisor.Invoke(context.Background(), manifest.ID, tt.inv)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Invoke() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSupervisorInvokesEnabledModuleWithinDeclaredCapability(t *testing.T) {
	runtime := &fakeRuntime{}
	_, supervisor := enabledModuleSupervisor(t, runtime, 3)
	manifest := validManifest()
	result, err := supervisor.Invoke(context.Background(), manifest.ID, Invocation{
		Method: "GET", Path: manifest.Routes[0].Path, Permission: "devices.read",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != 200 || runtime.calls != 1 {
		t.Fatalf("result=%+v calls=%d", result, runtime.calls)
	}
}

func TestSupervisorTimesOutBlockedRuntime(t *testing.T) {
	runtime := &fakeRuntime{block: true}
	registry := NewRegistry()
	manifest := validManifest()
	if _, err := registry.Install(manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(manifest.ID); err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewSupervisor(registry, runtime, SupervisorOptions{
		InvocationTimeout: 20 * time.Millisecond,
		FailureThreshold:  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = supervisor.Invoke(context.Background(), manifest.ID, Invocation{
		Method: "GET", Path: manifest.Routes[0].Path, Permission: "devices.read",
	})
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("Invoke() error = %v, want ErrRuntimeUnavailable", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("runtime timeout took too long: %v", elapsed)
	}
}

func TestSupervisorQuarantinesRepeatedRuntimeFailure(t *testing.T) {
	runtime := &fakeRuntime{invokeErr: errors.New("module crashed")}
	registry, supervisor := enabledModuleSupervisor(t, runtime, 2)
	manifest := validManifest()
	invocation := Invocation{Method: "GET", Path: manifest.Routes[0].Path, Permission: "devices.read"}

	for i := 0; i < 2; i++ {
		_, err := supervisor.Invoke(context.Background(), manifest.ID, invocation)
		if !errors.Is(err, ErrRuntimeUnavailable) {
			t.Fatalf("failure %d error = %v", i+1, err)
		}
	}
	module, err := registry.Get(manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if module.State != StateQuarantined {
		t.Fatalf("state=%q, want %q", module.State, StateQuarantined)
	}
	if _, err := supervisor.Invoke(context.Background(), manifest.ID, invocation); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("quarantined invoke error = %v", err)
	}
	if runtime.calls != 2 {
		t.Fatalf("runtime called %d times after quarantine", runtime.calls)
	}
}

func TestSupervisorSuccessResetsConsecutiveFailureCounter(t *testing.T) {
	runtime := &fakeRuntime{invokeErr: errors.New("temporary")}
	registry, supervisor := enabledModuleSupervisor(t, runtime, 2)
	manifest := validManifest()
	invocation := Invocation{Method: "GET", Path: manifest.Routes[0].Path, Permission: "devices.read"}

	_, _ = supervisor.Invoke(context.Background(), manifest.ID, invocation)
	runtime.invokeErr = nil
	if _, err := supervisor.Invoke(context.Background(), manifest.ID, invocation); err != nil {
		t.Fatal(err)
	}
	runtime.invokeErr = errors.New("temporary again")
	_, _ = supervisor.Invoke(context.Background(), manifest.ID, invocation)

	module, err := registry.Get(manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if module.State != StateEnabled {
		t.Fatalf("state=%q after non-consecutive failures, want enabled", module.State)
	}
}
