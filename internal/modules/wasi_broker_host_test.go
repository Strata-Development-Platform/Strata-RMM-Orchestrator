package modules

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWASIBrokerHostDispatchesWithTrustedInvocationScope(t *testing.T) {
	operation := "devices.echo"
	input := []byte("x")
	probe := wasiBrokerCallProbe(operation, input, true)

	module := wasiTestModule("1.0.0", 16, 1, 1)
	module.Manifest.Runtime.Network = RuntimeNetworkBrokered
	module.Manifest.Permissions = []string{"devices.read"}
	registry := NewRegistry()
	if _, err := registry.Install(module.Manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(module.Manifest.ID); err != nil {
		t.Fatal(err)
	}

	trustedScope := ResourceScope{MSPID: "msp-1", ClientID: "client-1", SiteID: "site-1"}
	var gotScope ResourceScope
	var gotInput []byte
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{{
		Name:       operation,
		Permission: "devices.read",
		Handler: func(_ context.Context, _ InstalledModule, scope ResourceScope, body []byte) ([]byte, error) {
			gotScope = scope
			gotInput = append([]byte(nil), body...)
			return []byte("ok"), nil
		},
	}}, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}

	runtime := newWASIRuntimeWithOptionsFixture(t, probe, module, WASIRuntimeOptions{Broker: broker})
	if _, err := runtime.Invoke(context.Background(), module, Invocation{Scope: trustedScope}); err != nil {
		t.Fatalf("invoke brokered guest: %v", err)
	}
	if gotScope != trustedScope {
		t.Fatalf("broker scope = %#v, want %#v", gotScope, trustedScope)
	}
	if string(gotInput) != string(input) {
		t.Fatalf("broker input = %q, want %q", gotInput, input)
	}
}

func TestWASIBrokerHostIsNotExposedWhenNetworkNone(t *testing.T) {
	operation := "devices.echo"
	module := wasiTestModule("1.0.0", 16, 1, 1)
	registry := NewRegistry()
	if _, err := registry.Install(module.Manifest); err != nil {
		t.Fatal(err)
	}
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{{
		Name:       operation,
		Permission: "devices.read",
		Handler: func(context.Context, InstalledModule, ResourceScope, []byte) ([]byte, error) {
			return nil, nil
		},
	}}, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newWASIRuntimeWithOptionsFixture(t, wasiBrokerCallProbe(operation, nil, false), module, WASIRuntimeOptions{Broker: broker})
	if _, err := runtime.Invoke(context.Background(), module, Invocation{}); err == nil {
		t.Fatal("expected network:none guest broker import to remain unavailable")
	}
}

func TestWASIBrokerHostRequiresConfiguredBroker(t *testing.T) {
	module := wasiTestModule("1.0.0", 16, 1, 1)
	module.Manifest.Runtime.Network = RuntimeNetworkBrokered
	runtime := newWASIRuntimeWithOptionsFixture(t, wasmEmpty, module, WASIRuntimeOptions{})
	if _, err := runtime.Invoke(context.Background(), module, Invocation{}); !errors.Is(err, ErrRuntimeBrokerUnavailable) {
		t.Fatalf("error = %v, want ErrRuntimeBrokerUnavailable", err)
	}
}

func TestWASIBrokerHostHealthCannotDispatchCapability(t *testing.T) {
	operation := "devices.echo"
	module := wasiTestModule("1.0.0", 16, 1, 1)
	module.Manifest.Runtime.Network = RuntimeNetworkBrokered
	module.Manifest.Permissions = []string{"devices.read"}
	registry := NewRegistry()
	if _, err := registry.Install(module.Manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(module.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	calls := 0
	broker, err := NewCapabilityBroker(registry, []BrokerOperation{{
		Name:       operation,
		Permission: "devices.read",
		Handler: func(context.Context, InstalledModule, ResourceScope, []byte) ([]byte, error) {
			calls++
			return nil, nil
		},
	}}, CapabilityBrokerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newWASIRuntimeWithOptionsFixture(t, wasiBrokerCallProbe(operation, nil, false), module, WASIRuntimeOptions{Broker: broker})
	if err := runtime.Health(context.Background(), module); err != nil {
		t.Fatalf("health execute: %v", err)
	}
	if calls != 0 {
		t.Fatalf("health broker calls = %d, want 0", calls)
	}
}

func TestBrokerABIStatusDoesNotLeakBackendErrors(t *testing.T) {
	if got := brokerABIStatusForError(ErrBrokerOperationUnknown); got != wasiBrokerStatusUnknownOperation {
		t.Fatalf("unknown status = %d", got)
	}
	if got := brokerABIStatusForError(ErrPermissionDenied); got != wasiBrokerStatusDenied {
		t.Fatalf("denied status = %d", got)
	}
	if got := brokerABIStatusForError(errors.New("sensitive backend detail")); got != wasiBrokerStatusBackendFailure {
		t.Fatalf("backend status = %d", got)
	}
}

func newWASIRuntimeWithOptionsFixture(t *testing.T, wasm []byte, module InstalledModule, options WASIRuntimeOptions) *WASIRuntime {
	t.Helper()
	root := t.TempDir()
	moduleDir := filepath.Join(root, module.Manifest.ID)
	versionDir := filepath.Join(moduleDir, module.Manifest.Version)
	if err := os.MkdirAll(versionDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "main.wasm"), wasm, 0o440); err != nil {
		t.Fatal(err)
	}
	writeWASIActiveState(t, moduleDir, module.Manifest.Version)
	options.Root = root
	runtime, err := NewWASIRuntime(options)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func wasiBrokerCallProbe(operation string, input []byte, requireSuccess bool) []byte {
	module := wasmHeader()
	module = append(module, wasmSection(1, []byte{
		0x02,
		0x60, 0x06, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x01, 0x7f,
		0x60, 0x00, 0x00,
	})...)
	module = append(module, wasmSection(2, wasmSingleFunctionImport(wasiBrokerModuleName, wasiBrokerCallName, 0))...)
	module = append(module, wasmSection(3, []byte{0x01, 0x01})...)
	module = append(module, wasmSection(5, []byte{0x01, 0x00, 0x01})...)

	exports := []byte{0x02}
	exports = append(exports, wasmName("_start")...)
	exports = append(exports, 0x00, 0x01)
	exports = append(exports, wasmName("memory")...)
	exports = append(exports, 0x02, 0x00)
	module = append(module, wasmSection(7, exports)...)

	body := []byte{0x00, 0x41, 0x00, 0x41}
	body = append(body, wasmU32Length(operation)...)
	body = append(body, 0x41, 0x10, 0x41)
	body = append(body, wasmU32ByteLength(input)...)
	body = append(body, 0x41, 0x20, 0x41, 0x10, 0x10, 0x00)
	if requireSuccess {
		body = append(body, 0x45, 0x04, 0x40, 0x05, 0x00, 0x0b)
	} else {
		body = append(body, 0x1a)
	}
	body = append(body, 0x0b)
	module = append(module, wasmSection(10, wasmSingleBody(body))...)

	data := []byte{0x02, 0x00, 0x41, 0x00, 0x0b}
	data = append(data, wasmU32Length(operation)...)
	data = append(data, []byte(operation)...)
	data = append(data, 0x00, 0x41, 0x10, 0x0b)
	data = append(data, wasmU32ByteLength(input)...)
	data = append(data, input...)
	module = append(module, wasmSection(11, data)...)
	return module
}

func wasmU32Length(value string) []byte {
	var size uint32
	for range []byte(value) {
		size++
	}
	return wasmU32(size)
}

func wasmU32ByteLength(value []byte) []byte {
	var size uint32
	for range value {
		size++
	}
	return wasmU32(size)
}
