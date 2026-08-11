package modules

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWASIRuntimeDoesNotInheritHostArgvOrEnvironment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	manifest := alphaReferenceManifest("1.0.0")
	pkg, payload := signedReferencePayload(t, manifest, alphaNoAmbientProcessStateWASM())
	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, manifest.ID, manifest.Version); err != nil {
		t.Fatal(err)
	}
	runtimeEngine, err := NewWASIRuntime(WASIRuntimeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	module := InstalledModule{Manifest: pkg.Manifest, State: StateEnabled}
	if err := runtimeEngine.Health(context.Background(), module); err != nil {
		t.Fatalf("ambient process state probe failed: %v", err)
	}
}

// alphaNoAmbientProcessStateWASM imports only standard WASI argv/environment
// sizing functions. It traps unless both calls succeed and report zero host
// arguments and zero host environment entries. This tests runtime behavior,
// rather than merely inspecting NewModuleConfig construction.
func alphaNoAmbientProcessStateWASM() []byte {
	wasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

	// type 0: (i32, i32) -> i32; type 1: () -> ()
	types := []byte{0x02, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f, 0x60, 0x00, 0x00}
	wasm = alphaWASMSection(wasm, 1, types)

	imports := []byte{0x02}
	for _, name := range []string{"environ_sizes_get", "args_sizes_get"} {
		imports = append(imports, alphaWASMName("wasi_snapshot_preview1")...)
		imports = append(imports, alphaWASMName(name)...)
		imports = append(imports, 0x00, 0x00) // function import using type 0
	}
	wasm = alphaWASMSection(wasm, 2, imports)
	wasm = alphaWASMSection(wasm, 3, []byte{0x01, 0x01})
	wasm = alphaWASMSection(wasm, 5, []byte{0x01, 0x00, 0x01})

	exports := []byte{0x02}
	exports = append(exports, alphaWASMName("_start")...)
	exports = append(exports, 0x00, 0x02) // two imported functions precede _start
	exports = append(exports, alphaWASMName("memory")...)
	exports = append(exports, 0x02, 0x00)
	wasm = alphaWASMSection(wasm, 7, exports)

	body := []byte{0x00}
	body = appendZeroCountProbe(body, 0, 0, 4)  // environ_sizes_get
	body = appendZeroCountProbe(body, 1, 8, 12) // args_sizes_get
	body = append(body, 0x0b)
	code := append([]byte{0x01}, alphaULEB(uint32(len(body)))...)
	code = append(code, body...)
	wasm = alphaWASMSection(wasm, 10, code)
	return wasm
}

func appendZeroCountProbe(body []byte, functionIndex, countPtr, bytesPtr uint32) []byte {
	// Call the WASI sizing function and trap on non-zero errno.
	body = append(body, 0x41)
	body = append(body, alphaULEB(countPtr)...)
	body = append(body, 0x41)
	body = append(body, alphaULEB(bytesPtr)...)
	body = append(body, 0x10)
	body = append(body, alphaULEB(functionIndex)...)
	body = append(body, 0x04, 0x40, 0x00, 0x0b)

	// Trap if count != 0.
	body = append(body, 0x41)
	body = append(body, alphaULEB(countPtr)...)
	body = append(body, 0x28, 0x02, 0x00) // i32.load align=4 offset=0
	body = append(body, 0x04, 0x40, 0x00, 0x0b)

	// Trap if aggregate byte size != 0.
	body = append(body, 0x41)
	body = append(body, alphaULEB(bytesPtr)...)
	body = append(body, 0x28, 0x02, 0x00)
	body = append(body, 0x04, 0x40, 0x00, 0x0b)
	return body
}
