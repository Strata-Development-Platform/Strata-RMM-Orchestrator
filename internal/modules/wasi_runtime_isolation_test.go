package modules

import (
	"context"
	"testing"
)

func TestWASIRuntimeDoesNotExposeHostEnvironment(t *testing.T) {
	t.Setenv("STRATA_WASI_SECRET", "must-not-reach-guest")
	probe := wasiCountMustBeZeroProbe("environ_sizes_get")
	runtime, module := newWASIRuntimeFixture(t, probe, 16, 1, 1)
	if err := runtime.Health(context.Background(), module); err != nil {
		t.Fatalf("guest observed ambient host environment: %v", err)
	}
}

func TestWASIRuntimeDoesNotExposeHostArgs(t *testing.T) {
	probe := wasiCountMustBeZeroProbe("args_sizes_get")
	runtime, module := newWASIRuntimeFixture(t, probe, 16, 1, 1)
	if err := runtime.Health(context.Background(), module); err != nil {
		t.Fatalf("guest observed ambient host argv: %v", err)
	}
}

func TestWASIRuntimeHasNoPreopenedFilesystem(t *testing.T) {
	probe := wasiErrnoMustNotSucceedProbe("fd_prestat_get", 3, 0)
	runtime, module := newWASIRuntimeFixture(t, probe, 16, 1, 1)
	if err := runtime.Health(context.Background(), module); err != nil {
		t.Fatalf("guest unexpectedly obtained preopened filesystem fd: %v", err)
	}
}

func TestWASIRuntimeDoesNotExposeRawSocketCreationImport(t *testing.T) {
	probe := missingImportProbe("wasi_snapshot_preview1", "sock_open")
	runtime, module := newWASIRuntimeFixture(t, probe, 16, 1, 1)
	if err := runtime.Health(context.Background(), module); err == nil {
		t.Fatal("expected raw socket creation import to be unavailable")
	}
}

func TestWASIRuntimeRejectsUndeclaredStrataHostImport(t *testing.T) {
	probe := missingImportProbe("strata_host", "secrets_get")
	runtime, module := newWASIRuntimeFixture(t, probe, 16, 1, 1)
	if err := runtime.Health(context.Background(), module); err == nil {
		t.Fatal("expected undeclared Strata host import to fail instantiation")
	}
}

func wasiCountMustBeZeroProbe(importName string) []byte {
	module := wasmHeader()
	module = append(module, wasmSection(1, []byte{
		0x02,
		0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f,
		0x60, 0x00, 0x00,
	})...)
	module = append(module, wasmSection(2, wasmSingleFunctionImport("wasi_snapshot_preview1", importName, 0))...)
	module = append(module, wasmSection(3, []byte{0x01, 0x01})...)
	module = append(module, wasmSection(5, []byte{0x01, 0x00, 0x01})...)
	module = append(module, wasmSection(7, wasmStartExport(1))...)
	body := []byte{
		0x00,
		0x41, 0x00,
		0x41, 0x04,
		0x10, 0x00,
		0x1a,
		0x41, 0x00,
		0x28, 0x02, 0x00,
		0x04, 0x40,
		0x00,
		0x0b,
		0x0b,
	}
	module = append(module, wasmSection(10, wasmSingleBody(body))...)
	return module
}

func wasiErrnoMustNotSucceedProbe(importName string, arg0, arg1 byte) []byte {
	module := wasmHeader()
	module = append(module, wasmSection(1, []byte{
		0x02,
		0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f,
		0x60, 0x00, 0x00,
	})...)
	module = append(module, wasmSection(2, wasmSingleFunctionImport("wasi_snapshot_preview1", importName, 0))...)
	module = append(module, wasmSection(3, []byte{0x01, 0x01})...)
	module = append(module, wasmSection(5, []byte{0x01, 0x00, 0x01})...)
	module = append(module, wasmSection(7, wasmStartExport(1))...)
	body := []byte{
		0x00,
		0x41, arg0,
		0x41, arg1,
		0x10, 0x00,
		0x45,
		0x04, 0x40,
		0x00,
		0x0b,
		0x0b,
	}
	module = append(module, wasmSection(10, wasmSingleBody(body))...)
	return module
}

func missingImportProbe(moduleName, importName string) []byte {
	module := wasmHeader()
	module = append(module, wasmSection(1, []byte{
		0x02,
		0x60, 0x00, 0x01, 0x7f,
		0x60, 0x00, 0x00,
	})...)
	module = append(module, wasmSection(2, wasmSingleFunctionImport(moduleName, importName, 0))...)
	module = append(module, wasmSection(3, []byte{0x01, 0x01})...)
	module = append(module, wasmSection(7, wasmStartExport(1))...)
	body := []byte{0x00, 0x10, 0x00, 0x1a, 0x0b}
	module = append(module, wasmSection(10, wasmSingleBody(body))...)
	return module
}

func wasmHeader() []byte {
	return []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
}

func wasmSection(id byte, payload []byte) []byte {
	section := []byte{id}
	section = append(section, wasmU32(uint32(len(payload)))...)
	section = append(section, payload...)
	return section
}

func wasmSingleFunctionImport(moduleName, importName string, typeIndex uint32) []byte {
	payload := []byte{0x01}
	payload = append(payload, wasmName(moduleName)...)
	payload = append(payload, wasmName(importName)...)
	payload = append(payload, 0x00)
	payload = append(payload, wasmU32(typeIndex)...)
	return payload
}

func wasmStartExport(functionIndex uint32) []byte {
	payload := []byte{0x01}
	payload = append(payload, wasmName("_start")...)
	payload = append(payload, 0x00)
	payload = append(payload, wasmU32(functionIndex)...)
	return payload
}

func wasmSingleBody(body []byte) []byte {
	payload := []byte{0x01}
	payload = append(payload, wasmU32(uint32(len(body)))...)
	payload = append(payload, body...)
	return payload
}

func wasmName(value string) []byte {
	encoded := wasmU32(uint32(len(value)))
	return append(encoded, []byte(value)...)
}

func wasmU32(value uint32) []byte {
	var encoded []byte
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		encoded = append(encoded, b)
		if value == 0 {
			return encoded
		}
	}
}
