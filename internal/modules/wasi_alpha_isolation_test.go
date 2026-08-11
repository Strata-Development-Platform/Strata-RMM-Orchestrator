package modules

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWASINetworkNoneDoesNotExposeBrokerImport(t *testing.T) {
	manifest := alphaReferenceManifest("1.0.0")
	// The guest requires strata_broker.call, but network:none must not instantiate
	// that host module even if a broker object exists on the runtime.
	registry := NewRegistry()
	installed, err := registry.Install(manifest)
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := registry.Enable(installed.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &alphaBrokerResolver{device: BrokerDevice{ID: "device-alpha-none", Scope: ResourceScope{MSPID: "msp-1"}}}
	operation, err := NewDeviceGetBrokerOperation(resolver)
	if err != nil {
		t.Fatal(err)
	}
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{operation}, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "modules")
	pkg, payload := signedReferencePayload(t, manifest, alphaDevicesGetWASM("device-alpha-none"))
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, manifest.ID, manifest.Version); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewWASIRuntime(WASIRuntimeOptions{Root: root, Broker: broker})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(context.Background(), enabled, Invocation{Scope: ResourceScope{MSPID: "msp-1"}}); err == nil {
		t.Fatal("network:none module unexpectedly resolved strata_broker.call")
	}
	if resolver.seen != "" {
		t.Fatalf("resolver was reached despite network:none: %q", resolver.seen)
	}
}

func TestWASIBrokerReauthorizesDisabledModuleAtCallTime(t *testing.T) {
	runtime, registry, module, resolver, scope := alphaBrokerRuntimeFixture(t, "device-disabled")
	if _, err := registry.Disable(module.Manifest.ID, "alpha test"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(context.Background(), module, Invocation{Scope: scope}); err == nil {
		t.Fatal("disabled module broker invocation unexpectedly succeeded")
	}
	if resolver.seen != "" {
		t.Fatalf("disabled module reached resource resolver: %q", resolver.seen)
	}
}

func TestWASIBrokerReauthorizesQuarantinedModuleAtCallTime(t *testing.T) {
	runtime, registry, module, resolver, scope := alphaBrokerRuntimeFixture(t, "device-quarantined")
	if _, err := registry.Quarantine(module.Manifest.ID, "alpha test"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(context.Background(), module, Invocation{Scope: scope}); err == nil {
		t.Fatal("quarantined module broker invocation unexpectedly succeeded")
	}
	if resolver.seen != "" {
		t.Fatalf("quarantined module reached resource resolver: %q", resolver.seen)
	}
}

func alphaBrokerRuntimeFixture(t *testing.T, deviceID string) (*WASIRuntime, *Registry, InstalledModule, *alphaBrokerResolver, ResourceScope) {
	t.Helper()
	scope := ResourceScope{MSPID: "msp-1", ClientID: "client-1", SiteID: "site-1"}
	resolver := &alphaBrokerResolver{device: BrokerDevice{ID: deviceID, Hostname: "alpha-host", Status: "online", Scope: scope}}
	operation, err := NewDeviceGetBrokerOperation(resolver)
	if err != nil {
		t.Fatal(err)
	}
	manifest := alphaReferenceManifest("1.0.0")
	manifest.Runtime.Network = RuntimeNetworkBrokered
	registry := NewRegistry()
	installed, err := registry.Install(manifest)
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := registry.Enable(installed.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{operation}, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "modules")
	pkg, payload := signedReferencePayload(t, manifest, alphaDevicesGetWASM(deviceID))
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, manifest.ID, manifest.Version); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewWASIRuntime(WASIRuntimeOptions{Root: root, Broker: broker})
	if err != nil {
		t.Fatal(err)
	}
	return runtime, registry, enabled, resolver, scope
}
