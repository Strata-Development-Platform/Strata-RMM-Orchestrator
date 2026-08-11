package modules

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestSignedWASIReferenceCallsDevicesGetThroughBrokerABI(t *testing.T) {
	const deviceID = "device-alpha-1"
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

	result, err := runtime.Invoke(context.Background(), enabled, Invocation{
		Method:     "GET",
		Path:       "/api/modules/alpha.reference/device",
		Permission: "devices.read",
		Scope:      scope,
	})
	if err != nil {
		t.Fatalf("brokered WASI invocation: %v", err)
	}
	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", result.StatusCode)
	}
	if resolver.seen != deviceID {
		t.Fatalf("resolver saw device %q, want %q", resolver.seen, deviceID)
	}
}

func TestSignedWASIReferenceBrokerDeniesCrossScopeDevice(t *testing.T) {
	const deviceID = "device-alpha-2"
	resolvedScope := ResourceScope{MSPID: "msp-1", ClientID: "client-1", SiteID: "site-1"}
	wrongScope := ResourceScope{MSPID: "msp-1", ClientID: "client-1", SiteID: "site-2"}
	resolver := &alphaBrokerResolver{device: BrokerDevice{ID: deviceID, Hostname: "alpha-host", Status: "online", Scope: resolvedScope}}
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
	if _, err := runtime.Invoke(context.Background(), enabled, Invocation{
		Method: "GET", Path: "/api/modules/alpha.reference/device", Permission: "devices.read", Scope: wrongScope,
	}); err == nil {
		t.Fatal("cross-scope broker access unexpectedly succeeded")
	}
	if resolver.seen != deviceID {
		t.Fatalf("resolver did not authoritatively resolve requested device; saw %q", resolver.seen)
	}
}

type alphaBrokerResolver struct {
	device BrokerDevice
	seen   string
}

func (r *alphaBrokerResolver) ResolveBrokerDevice(_ context.Context, id string) (BrokerDevice, error) {
	r.seen = id
	if id != r.device.ID {
		return BrokerDevice{}, fmt.Errorf("unknown device")
	}
	return r.device, nil
}

// alphaDevicesGetWASM builds a deterministic minimal wasm module with exactly
// one non-WASI host import: strata_broker.call. _start writes a devices.get
// request from guest memory, invokes the broker, and traps on any non-zero ABI
// status. Empty stdout intentionally exercises the Alpha response-ABI legacy
// success mapping after a successful broker call.
func alphaDevicesGetWASM(deviceID string) []byte {
	operation := []byte(BrokerOperationDevicesGet)
	input := []byte(`{"device_id":"` + deviceID + `"}`)
	wasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

	// Types: broker call (6 x i32 -> i32), then _start (() -> ()).
	types := []byte{0x02, 0x60, 0x06, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x01, 0x7f, 0x60, 0x00, 0x00}
	wasm = alphaWASMSection(wasm, 1, types)

	imports := []byte{0x01}
	imports = append(imports, alphaWASMName(wasiBrokerModuleName)...)
	imports = append(imports, alphaWASMName(wasiBrokerCallName)...)
	imports = append(imports, 0x00, 0x00) // function import, type 0
	wasm = alphaWASMSection(wasm, 2, imports)

	wasm = alphaWASMSection(wasm, 3, []byte{0x01, 0x01}) // one function, type 1
	wasm = alphaWASMSection(wasm, 5, []byte{0x01, 0x00, 0x01}) // one memory, min 1 page

	exports := []byte{0x02}
	exports = append(exports, alphaWASMName("_start")...)
	exports = append(exports, 0x00, 0x01) // function index 1 (import is index 0)
	exports = append(exports, alphaWASMName("memory")...)
	exports = append(exports, 0x02, 0x00) // memory index 0
	wasm = alphaWASMSection(wasm, 7, exports)

	body := []byte{0x00} // local decl count
	for _, value := range []uint32{0, uint32(len(operation)), 64, uint32(len(input)), 256, 512} {
		body = append(body, 0x41)
		body = append(body, alphaULEB(value)...)
	}
	body = append(body, 0x10, 0x00) // call imported broker function
	body = append(body, 0x04, 0x40, 0x00, 0x0b) // if status != 0: unreachable
	body = append(body, 0x0b)
	code := append([]byte{0x01}, alphaULEB(uint32(len(body)))...)
	code = append(code, body...)
	wasm = alphaWASMSection(wasm, 10, code)

	data := []byte{0x02}
	data = append(data, alphaActiveDataSegment(0, operation)...)
	data = append(data, alphaActiveDataSegment(64, input)...)
	wasm = alphaWASMSection(wasm, 11, data)
	return wasm
}

func alphaActiveDataSegment(offset uint32, data []byte) []byte {
	segment := []byte{0x00, 0x41}
	segment = append(segment, alphaULEB(offset)...)
	segment = append(segment, 0x0b)
	segment = append(segment, alphaULEB(uint32(len(data)))...)
	return append(segment, data...)
}

func alphaWASMSection(dst []byte, id byte, payload []byte) []byte {
	dst = append(dst, id)
	dst = append(dst, alphaULEB(uint32(len(payload)))...)
	return append(dst, payload...)
}

func alphaWASMName(value string) []byte {
	encoded := alphaULEB(uint32(len(value)))
	return append(encoded, []byte(value)...)
}

func alphaULEB(value uint32) []byte {
	var out []byte
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if value == 0 {
			return out
		}
	}
}
