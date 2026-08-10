package modules

import (
	"reflect"
	"strings"
	"testing"
)

func validManifest() Manifest {
	return Manifest{
		ID:         "com.example.backup",
		Name:       "Example Backup",
		Version:    "1.0.0",
		APIVersion: CurrentAPIVersion,
		Publisher:  "Example Inc.",
		Permissions: []string{
			"devices.read",
			"alerts.write",
		},
		Routes: []Route{{
			Path:       "/api/modules/com.example.backup/status",
			Methods:    []string{"GET"},
			Permission: "devices.read",
		}},
	}
}

func validRuntime() *RuntimeSpec {
	return &RuntimeSpec{
		Kind:           RuntimeKindWASI,
		Entrypoint:     "module/main.wasm",
		MemoryMB:       64,
		TimeoutSeconds: 30,
		MaxConcurrency: 4,
		Network:        RuntimeNetworkBrokered,
	}
}

func TestManifestValidateAcceptsNamespacedLeastPrivilegeModule(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestManifestValidateAcceptsBoundedWASIRuntime(t *testing.T) {
	m := validManifest()
	m.Runtime = validRuntime()
	if err := m.Validate(); err != nil {
		t.Fatalf("valid WASI runtime rejected: %v", err)
	}
}

func TestManifestValidateKeepsRuntimeOptional(t *testing.T) {
	m := validManifest()
	m.Runtime = nil
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest without runtime rejected: %v", err)
	}
}

func TestManifestValidateRejectsInvalidIdentityAndCompatibility(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"invalid id", func(m *Manifest) { m.ID = "Bad Module" }, "module id"},
		{"missing name", func(m *Manifest) { m.Name = "" }, "name is required"},
		{"missing version", func(m *Manifest) { m.Version = "" }, "version is required"},
		{"unsupported api", func(m *Manifest) { m.APIVersion = "v999" }, "unsupported module API version"},
		{"missing publisher", func(m *Manifest) { m.Publisher = "" }, "publisher is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest()
			tt.mutate(&m)
			err := m.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestManifestValidateRejectsPermissionEscalation(t *testing.T) {
	m := validManifest()
	m.Permissions = append(m.Permissions, "database.superuser")
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "unknown module permission") {
		t.Fatalf("expected unknown permission rejection, got %v", err)
	}

	m = validManifest()
	m.Permissions = append(m.Permissions, "devices.read")
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate module permission") {
		t.Fatalf("expected duplicate permission rejection, got %v", err)
	}
}

func TestManifestValidateRejectsRouteEscapeAndUndeclaredPermission(t *testing.T) {
	m := validManifest()
	m.Routes[0].Path = "/api/v1/devices"
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "must be namespaced") {
		t.Fatalf("expected route namespace rejection, got %v", err)
	}

	m = validManifest()
	m.Routes[0].Permission = "reports.write"
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "undeclared permission") {
		t.Fatalf("expected undeclared permission rejection, got %v", err)
	}
}

func TestRuntimeSpecRejectsUnsafeOrUnboundedDeclarations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RuntimeSpec)
		want   string
	}{
		{"unsupported kind", func(r *RuntimeSpec) { r.Kind = "process" }, "unsupported runtime kind"},
		{"missing entrypoint", func(r *RuntimeSpec) { r.Entrypoint = "" }, "entrypoint is required"},
		{"absolute entrypoint", func(r *RuntimeSpec) { r.Entrypoint = "/module.wasm" }, "relative slash-separated"},
		{"backslash entrypoint", func(r *RuntimeSpec) { r.Entrypoint = `module\\main.wasm` }, "relative slash-separated"},
		{"traversal entrypoint", func(r *RuntimeSpec) { r.Entrypoint = "../main.wasm" }, "normalized and contained"},
		{"non-normalized entrypoint", func(r *RuntimeSpec) { r.Entrypoint = "module/../main.wasm" }, "normalized and contained"},
		{"wrong extension", func(r *RuntimeSpec) { r.Entrypoint = "module/main.exe" }, "must end in .wasm"},
		{"memory too small", func(r *RuntimeSpec) { r.MemoryMB = 15 }, "memory_mb"},
		{"memory too large", func(r *RuntimeSpec) { r.MemoryMB = 513 }, "memory_mb"},
		{"timeout too small", func(r *RuntimeSpec) { r.TimeoutSeconds = 0 }, "timeout_seconds"},
		{"timeout too large", func(r *RuntimeSpec) { r.TimeoutSeconds = 121 }, "timeout_seconds"},
		{"concurrency too small", func(r *RuntimeSpec) { r.MaxConcurrency = 0 }, "max_concurrency"},
		{"concurrency too large", func(r *RuntimeSpec) { r.MaxConcurrency = 33 }, "max_concurrency"},
		{"unsupported network", func(r *RuntimeSpec) { r.Network = "host" }, "network policy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := validRuntime()
			tt.mutate(runtime)
			m := validManifest()
			m.Runtime = runtime
			err := m.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestKnownPermissionsStableAndSorted(t *testing.T) {
	got := KnownPermissions()
	if len(got) == 0 {
		t.Fatal("KnownPermissions returned no permissions")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("permissions are not sorted: %v", got)
		}
	}
	copyOfGot := append([]string(nil), got...)
	if !reflect.DeepEqual(got, copyOfGot) {
		t.Fatal("permissions changed unexpectedly")
	}
}
