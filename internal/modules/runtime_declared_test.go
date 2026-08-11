package modules

import (
	"context"
	"testing"
)

type declaredInvocationRuntime struct {
	got Invocation
}

func (r *declaredInvocationRuntime) Health(context.Context, InstalledModule) error { return nil }
func (r *declaredInvocationRuntime) Invoke(_ context.Context, _ InstalledModule, invocation Invocation) (InvocationResult, error) {
	r.got = invocation
	return InvocationResult{StatusCode: 204}, nil
}

func TestSupervisorInvokeDeclaredDerivesPermissionFromManifest(t *testing.T) {
	registry := NewRegistry()
	manifest := validManifest()
	manifest.Permissions = []string{"devices.read"}
	manifest.Routes = []Route{{
		Path:       "/api/modules/com.example.backup/device-action",
		Methods:    []string{"POST"},
		Permission: "devices.read",
	}}
	installed, err := registry.Install(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(installed.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	runtime := &declaredInvocationRuntime{}
	supervisor, err := NewSupervisor(registry, runtime, SupervisorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	scope := ResourceScope{MSPID: "msp-a", ClientID: "client-a", SiteID: "site-a"}
	result, err := supervisor.InvokeDeclared(
		context.Background(), installed.Manifest.ID, "POST",
		"/api/modules/com.example.backup/device-action", []byte(`{"action":"inspect"}`), scope,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != 204 {
		t.Fatalf("status=%d, want 204", result.StatusCode)
	}
	if runtime.got.Permission != "devices.read" {
		t.Fatalf("permission=%q, want devices.read", runtime.got.Permission)
	}
	if runtime.got.Scope != scope {
		t.Fatalf("scope=%+v, want %+v", runtime.got.Scope, scope)
	}
}

func TestSupervisorInvokeDeclaredRejectsUndeclaredRouteBeforeRuntime(t *testing.T) {
	registry := NewRegistry()
	manifest := validManifest()
	manifest.Permissions = []string{"devices.read"}
	manifest.Routes = []Route{{
		Path:       "/api/modules/com.example.backup/declared",
		Methods:    []string{"POST"},
		Permission: "devices.read",
	}}
	installed, err := registry.Install(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(installed.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	runtime := &declaredInvocationRuntime{}
	supervisor, err := NewSupervisor(registry, runtime, SupervisorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := supervisor.InvokeDeclared(context.Background(), installed.Manifest.ID, "POST", "/api/modules/com.example.backup/not-declared", nil, ResourceScope{MSPID: "msp-a"}); err != ErrRouteNotDeclared {
		t.Fatalf("error=%v, want ErrRouteNotDeclared", err)
	}
	if runtime.got.Path != "" {
		t.Fatal("runtime was reached for an undeclared route")
	}
}
