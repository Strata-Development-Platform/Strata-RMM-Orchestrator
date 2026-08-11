package modules

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWASINetworkNoneDoesNotExposeBrokerImport(t *testing.T) {
	manifest := alphaReferenceManifest("1.0.0")
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

func TestWASINoPreopenedFilesystemIsGuestVisible(t *testing.T) {
	runtime, module := alphaIsolatedRuntimeFixture(t, alphaPreopenProbeWASM(), 1, 0)
	if err := runtime.Health(context.Background(), module); err != nil {
		t.Fatalf("guest observed an unexpected preopened filesystem: %v", err)
	}
}

func TestWASIRawSocketImportIsUnavailable(t *testing.T) {
	runtime, module := alphaIsolatedRuntimeFixture(t, alphaUnknownSocketImportWASM(), 1, 0)
	if err := runtime.Health(context.Background(), module); err == nil {
		t.Fatal("raw socket-style host import unexpectedly resolved")
	}
}

func TestWASIRealConcurrencyLimitBlocksSecondGuest(t *testing.T) {
	runtime, module := alphaIsolatedRuntimeFixture(t, wasmInfiniteLoop, 1, 0)
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	done := make(chan error, 1)
	go func() { done <- runtime.Health(ctx1, module) }()
	time.Sleep(20 * time.Millisecond)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()
	err := runtime.Health(ctx2, module)
	if !errors.Is(err, ErrRuntimeConcurrencyLimited) {
		t.Fatalf("second guest error = %v, want ErrRuntimeConcurrencyLimited", err)
	}
	cancel1()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first guest did not terminate after cancellation")
	}
}

func TestWASIOutputBoundAppliesToStderr(t *testing.T) {
	runtime, module := alphaIsolatedRuntimeFixture(t, alphaStderrFloodWASM(), 1, 8)
	if err := runtime.Health(context.Background(), module); !errors.Is(err, ErrRuntimeOutputTooLarge) {
		t.Fatalf("stderr overflow error = %v, want ErrRuntimeOutputTooLarge", err)
	}
}

func TestWASIParallelSuccessStaysWithinDeclaredConcurrency(t *testing.T) {
	runtime, module := alphaIsolatedRuntimeFixture(t, wasmEmpty, 4, 0)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- runtime.Health(context.Background(), module)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("parallel health failed: %v", err)
		}
	}
}

func alphaIsolatedRuntimeFixture(t *testing.T, wasm []byte, maxConcurrency, maxIO int) (*WASIRuntime, InstalledModule) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "modules")
	manifest := alphaReferenceManifest("1.0.0")
	manifest.Runtime.MaxConcurrency = maxConcurrency
	pkg, payload := signedReferencePayload(t, manifest, wasm)
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, manifest.ID, manifest.Version); err != nil {
		t.Fatal(err)
	}
	opts := WASIRuntimeOptions{Root: root}
	if maxIO > 0 {
		opts.MaxIOBytes = maxIO
	}
	runtime, err := NewWASIRuntime(opts)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, InstalledModule{Manifest: manifest, State: StateEnabled}
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

func alphaPreopenProbeWASM() []byte {
	return alphaSingleWASIImportProbe("fd_prestat_get", []byte{0x41, 0x03, 0x41, 0x00, 0x10, 0x00, 0x45, 0x04, 0x40, 0x00, 0x0b})
}

func alphaUnknownSocketImportWASM() []byte {
	wasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	types := []byte{0x02, 0x60, 0x00, 0x01, 0x7f, 0x60, 0x00, 0x00}
	wasm = alphaWASMSection(wasm, 1, types)
	imports := []byte{0x01}
	imports = append(imports, alphaWASMName("strata_socket")...)
	imports = append(imports, alphaWASMName("connect")...)
	imports = append(imports, 0x00, 0x00)
	wasm = alphaWASMSection(wasm, 2, imports)
	wasm = alphaWASMSection(wasm, 3, []byte{0x01, 0x01})
	exports := []byte{0x01}
	exports = append(exports, alphaWASMName("_start")...)
	exports = append(exports, 0x00, 0x01)
	wasm = alphaWASMSection(wasm, 7, exports)
	body := []byte{0x00, 0x10, 0x00, 0x1a, 0x0b}
	code := append([]byte{0x01}, alphaULEB(uint32(len(body)))...)
	code = append(code, body...)
	return alphaWASMSection(wasm, 10, code)
}

func alphaStderrFloodWASM() []byte {
	wasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	types := []byte{0x02, 0x60, 0x04, 0x7f, 0x7f, 0x7f, 0x7f, 0x01, 0x7f, 0x60, 0x00, 0x00}
	wasm = alphaWASMSection(wasm, 1, types)
	imports := []byte{0x01}
	imports = append(imports, alphaWASMName("wasi_snapshot_preview1")...)
	imports = append(imports, alphaWASMName("fd_write")...)
	imports = append(imports, 0x00, 0x00)
	wasm = alphaWASMSection(wasm, 2, imports)
	wasm = alphaWASMSection(wasm, 3, []byte{0x01, 0x01})
	wasm = alphaWASMSection(wasm, 5, []byte{0x01, 0x00, 0x01})
	exports := []byte{0x02}
	exports = append(exports, alphaWASMName("_start")...)
	exports = append(exports, 0x00, 0x01)
	exports = append(exports, alphaWASMName("memory")...)
	exports = append(exports, 0x02, 0x00)
	wasm = alphaWASMSection(wasm, 7, exports)
	body := []byte{0x00,
		0x41, 0x00, 0x41, 0xc0, 0x00, 0x36, 0x02, 0x00,
		0x41, 0x04, 0x41, 0x20, 0x36, 0x02, 0x00,
		0x41, 0x02, 0x41, 0x00, 0x41, 0x01, 0x41, 0x08, 0x10, 0x00, 0x1a,
		0x0b}
	code := append([]byte{0x01}, alphaULEB(uint32(len(body)))...)
	code = append(code, body...)
	wasm = alphaWASMSection(wasm, 10, code)
	data := []byte{0x01}
	data = append(data, alphaActiveDataSegment(64, []byte("0123456789abcdef0123456789abcdef"))...)
	return alphaWASMSection(wasm, 11, data)
}

func alphaSingleWASIImportProbe(name string, instructions []byte) []byte {
	wasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	types := []byte{0x02, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f, 0x60, 0x00, 0x00}
	wasm = alphaWASMSection(wasm, 1, types)
	imports := []byte{0x01}
	imports = append(imports, alphaWASMName("wasi_snapshot_preview1")...)
	imports = append(imports, alphaWASMName(name)...)
	imports = append(imports, 0x00, 0x00)
	wasm = alphaWASMSection(wasm, 2, imports)
	wasm = alphaWASMSection(wasm, 3, []byte{0x01, 0x01})
	wasm = alphaWASMSection(wasm, 5, []byte{0x01, 0x00, 0x01})
	exports := []byte{0x02}
	exports = append(exports, alphaWASMName("_start")...)
	exports = append(exports, 0x00, 0x01)
	exports = append(exports, alphaWASMName("memory")...)
	exports = append(exports, 0x02, 0x00)
	wasm = alphaWASMSection(wasm, 7, exports)
	body := append([]byte{0x00}, instructions...)
	body = append(body, 0x0b)
	code := append([]byte{0x01}, alphaULEB(uint32(len(body)))...)
	code = append(code, body...)
	return alphaWASMSection(wasm, 10, code)
}
