package modules

import (
	"context"
	"errors"
	"testing"
)

func TestCapabilityBrokerAllowsDeclaredPermissionWithTrustedScope(t *testing.T) {
	registry, module := enabledBrokerModule(t, []string{"devices.read"})
	called := false
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{{
		Name:       "devices.get",
		Permission: "devices.read",
		Handler: func(_ context.Context, got InstalledModule, scope ResourceScope, input []byte) ([]byte, error) {
			called = true
			if got.Manifest.ID != module.Manifest.ID {
				t.Fatalf("module id = %q", got.Manifest.ID)
			}
			if scope.MSPID != "msp-1" || scope.ClientID != "client-1" || scope.SiteID != "site-1" {
				t.Fatalf("unexpected scope: %+v", scope)
			}
			input[0] = 'X'
			return []byte("ok"), nil
		},
	}}, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	requestInput := []byte("request")
	output, err := broker.Call(context.Background(), module, BrokerRequest{
		Operation: "devices.get",
		Scope:     ResourceScope{MSPID: "msp-1", ClientID: "client-1", SiteID: "site-1"},
		Input:     requestInput,
	})
	if err != nil {
		t.Fatalf("broker call: %v", err)
	}
	if !called || string(output) != "ok" {
		t.Fatalf("called=%v output=%q", called, output)
	}
	if string(requestInput) != "request" {
		t.Fatalf("handler mutated caller input: %q", requestInput)
	}
}

func TestCapabilityBrokerDeniesUndeclaredPermission(t *testing.T) {
	registry, module := enabledBrokerModule(t, nil)
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{{
		Name:       "devices.get",
		Permission: "devices.read",
		Handler:    func(context.Context, InstalledModule, ResourceScope, []byte) ([]byte, error) { return nil, nil },
	}}, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = broker.Call(context.Background(), module, BrokerRequest{Operation: "devices.get"})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("error = %v, want ErrPermissionDenied", err)
	}
}

func TestCapabilityBrokerDeniesDisabledAndQuarantinedModules(t *testing.T) {
	registry, module := enabledBrokerModule(t, []string{"devices.read"})
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{{
		Name:       "devices.get",
		Permission: "devices.read",
		Handler:    func(context.Context, InstalledModule, ResourceScope, []byte) ([]byte, error) { return nil, nil },
	}}, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Disable(module.Manifest.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Call(context.Background(), module, BrokerRequest{Operation: "devices.get"}); !errors.Is(err, ErrModuleDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
	if _, err := registry.Quarantine(module.Manifest.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Call(context.Background(), module, BrokerRequest{Operation: "devices.get"}); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("quarantine error = %v", err)
	}
}

func TestCapabilityBrokerRejectsVersionMismatch(t *testing.T) {
	registry, module := enabledBrokerModule(t, []string{"devices.read"})
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{{
		Name:       "devices.get",
		Permission: "devices.read",
		Handler:    func(context.Context, InstalledModule, ResourceScope, []byte) ([]byte, error) { return nil, nil },
	}}, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	module.Manifest.Version = "2.0.0"
	if _, err := broker.Call(context.Background(), module, BrokerRequest{Operation: "devices.get"}); !errors.Is(err, ErrBrokerVersionMismatch) {
		t.Fatalf("error = %v, want ErrBrokerVersionMismatch", err)
	}
}

func TestCapabilityBrokerRejectsInvalidScopeAndBoundsIO(t *testing.T) {
	registry, module := enabledBrokerModule(t, []string{"devices.read"})
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{{
		Name:       "devices.get",
		Permission: "devices.read",
		Handler: func(context.Context, InstalledModule, ResourceScope, []byte) ([]byte, error) {
			return []byte("12345"), nil
		},
	}}, CapabilityBrokerOptions{MaxIOBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Call(context.Background(), module, BrokerRequest{
		Operation: "devices.get",
		Scope:     ResourceScope{ClientID: "client-without-msp"},
	}); !errors.Is(err, ErrBrokerScopeInvalid) {
		t.Fatalf("scope error = %v", err)
	}
	if _, err := broker.Call(context.Background(), module, BrokerRequest{Operation: "devices.get", Input: []byte("12345")}); !errors.Is(err, ErrBrokerInputTooLarge) {
		t.Fatalf("input error = %v", err)
	}
	if _, err := broker.Call(context.Background(), module, BrokerRequest{Operation: "devices.get", Input: []byte("1234")}); !errors.Is(err, ErrBrokerOutputTooLarge) {
		t.Fatalf("output error = %v", err)
	}
}

func TestCapabilityBrokerRejectsUnknownOrUnsafeRegistration(t *testing.T) {
	registry, module := enabledBrokerModule(t, []string{"devices.read"})
	broker, err := NewCapabilityBroker(registry, nil, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Call(context.Background(), module, BrokerRequest{Operation: "missing"}); !errors.Is(err, ErrBrokerOperationUnknown) {
		t.Fatalf("error = %v, want ErrBrokerOperationUnknown", err)
	}
	if _, err := NewCapabilityBroker(registry, []BrokerOperation{{Name: "bad", Permission: "root.all", Handler: func(context.Context, InstalledModule, ResourceScope, []byte) ([]byte, error) { return nil, nil }}}, CapabilityBrokerOptions{}); err == nil {
		t.Fatal("expected unknown permission registration to fail")
	}
	if _, err := NewCapabilityBroker(registry, []BrokerOperation{{Name: "devices.get", Permission: "devices.read"}, {Name: "devices.get", Permission: "devices.read", Handler: func(context.Context, InstalledModule, ResourceScope, []byte) ([]byte, error) { return nil, nil }}}, CapabilityBrokerOptions{}); err == nil {
		t.Fatal("expected duplicate/invalid registration to fail")
	}
}

func enabledBrokerModule(t *testing.T, permissions []string) (*Registry, InstalledModule) {
	t.Helper()
	registry := NewRegistry()
	installed, err := registry.Install(Manifest{
		ID:          "test.module",
		Name:        "Test Module",
		Version:     "1.0.0",
		APIVersion:  CurrentAPIVersion,
		Publisher:   "Strata",
		Permissions: permissions,
	})
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := registry.Enable(installed.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	return registry, enabled
}
