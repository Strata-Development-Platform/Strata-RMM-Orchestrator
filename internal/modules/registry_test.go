package modules

import (
	"errors"
	"testing"
	"time"
)

func TestRegistryLifecycleIsFailClosed(t *testing.T) {
	registry := NewRegistry()
	fixed := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return fixed }

	installed, err := registry.Install(validManifest())
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if installed.State != StateInstalled {
		t.Fatalf("initial state = %q, want %q", installed.State, StateInstalled)
	}
	if !errors.Is(registry.RequirePermission(installed.Manifest.ID, "devices.read"), ErrPermissionDenied) {
		t.Fatal("installed-but-not-enabled module unexpectedly received permission")
	}

	enabled, err := registry.Enable(installed.Manifest.ID)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if enabled.State != StateEnabled {
		t.Fatalf("enabled state = %q", enabled.State)
	}
	if err := registry.RequirePermission(installed.Manifest.ID, "devices.read"); err != nil {
		t.Fatalf("declared permission denied: %v", err)
	}
	if !errors.Is(registry.RequirePermission(installed.Manifest.ID, "reports.write"), ErrPermissionDenied) {
		t.Fatal("undeclared permission unexpectedly granted")
	}
	if err := registry.Uninstall(installed.Manifest.ID); err == nil {
		t.Fatal("enabled module uninstalled without disable")
	}

	disabled, err := registry.Disable(installed.Manifest.ID, "operator maintenance")
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if disabled.State != StateDisabled || disabled.Reason == "" {
		t.Fatalf("unexpected disabled state: %+v", disabled)
	}
	if err := registry.Uninstall(installed.Manifest.ID); err != nil {
		t.Fatalf("uninstall disabled module: %v", err)
	}
	if _, err := registry.Get(installed.Manifest.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("uninstalled module still present: %v", err)
	}
}

func TestRegistryQuarantineCannotBeBypassedByEnable(t *testing.T) {
	registry := NewRegistry()
	installed, err := registry.Install(validManifest())
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := registry.Enable(installed.Manifest.ID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	quarantined, err := registry.Quarantine(installed.Manifest.ID, "health check failed")
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if quarantined.State != StateQuarantined {
		t.Fatalf("state = %q, want quarantined", quarantined.State)
	}
	if _, err := registry.Enable(installed.Manifest.ID); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("quarantine bypassed: %v", err)
	}
	if !errors.Is(registry.RequirePermission(installed.Manifest.ID, "devices.read"), ErrPermissionDenied) {
		t.Fatal("quarantined module retained permission")
	}
}

func TestRegistryRejectsInvalidAndDuplicateInstall(t *testing.T) {
	registry := NewRegistry()
	invalid := validManifest()
	invalid.Permissions = append(invalid.Permissions, "database.superuser")
	if _, err := registry.Install(invalid); err == nil {
		t.Fatal("invalid manifest installed")
	}

	manifest := validManifest()
	if _, err := registry.Install(manifest); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if _, err := registry.Install(manifest); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate install error = %v", err)
	}
}

func TestRegistryListIsDeterministic(t *testing.T) {
	registry := NewRegistry()
	for _, id := range []string{"z.example", "a.example", "m.example"} {
		manifest := validManifest()
		manifest.ID = id
		if _, err := registry.Install(manifest); err != nil {
			t.Fatalf("install %s: %v", id, err)
		}
	}
	modules := registry.List()
	if len(modules) != 3 {
		t.Fatalf("list length = %d", len(modules))
	}
	if modules[0].Manifest.ID != "a.example" || modules[1].Manifest.ID != "m.example" || modules[2].Manifest.ID != "z.example" {
		t.Fatalf("list order = %v", []string{modules[0].Manifest.ID, modules[1].Manifest.ID, modules[2].Manifest.ID})
	}
}
