package modules

import (
	"errors"
	"testing"
)

func TestRegistryUpgradeRequiresInactiveModule(t *testing.T) {
	registry := NewRegistry()
	manifest := validManifest()
	installed, err := registry.Install(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(installed.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	upgrade := manifest
	upgrade.Version = "2.0.0"
	if _, err := registry.Upgrade(manifest.ID, upgrade); !errors.Is(err, ErrVersionTransition) {
		t.Fatalf("enabled upgrade error=%v, want ErrVersionTransition", err)
	}

	if _, err := registry.Disable(manifest.ID, "upgrade"); err != nil {
		t.Fatal(err)
	}
	updated, err := registry.Upgrade(manifest.ID, upgrade)
	if err != nil {
		t.Fatalf("disabled upgrade: %v", err)
	}
	if updated.Manifest.Version != "2.0.0" || updated.State != StateDisabled {
		t.Fatalf("unexpected upgraded module: %+v", updated)
	}
}

func TestRegistryRollbackRestoresVerifiedManifestAndPreservesState(t *testing.T) {
	registry := NewRegistry()
	original := validManifest()
	installed, err := registry.Install(original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Disable(installed.Manifest.ID, "prepare version change"); err != nil {
		t.Fatal(err)
	}
	upgrade := original
	upgrade.Version = "2.0.0"
	upgrade.Permissions = []string{"devices.read", "alerts.read"}
	if _, err := registry.Upgrade(original.ID, upgrade); err != nil {
		t.Fatal(err)
	}

	rolledBack, err := registry.Rollback(original.ID, original)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rolledBack.Manifest.Version != original.Version {
		t.Fatalf("rollback version=%q want=%q", rolledBack.Manifest.Version, original.Version)
	}
	if rolledBack.State != StateDisabled {
		t.Fatalf("rollback state=%q want disabled", rolledBack.State)
	}
}

func TestRegistryVersionTransitionRejectsWrongIDSameVersionAndQuarantine(t *testing.T) {
	registry := NewRegistry()
	manifest := validManifest()
	if _, err := registry.Install(manifest); err != nil {
		t.Fatal(err)
	}

	wrongID := manifest
	wrongID.ID = "com.example.other"
	wrongID.Routes[0].Path = "/api/modules/com.example.other/status"
	if _, err := registry.Upgrade(manifest.ID, wrongID); !errors.Is(err, ErrVersionTransition) {
		t.Fatalf("wrong-id error=%v, want ErrVersionTransition", err)
	}
	if _, err := registry.Upgrade(manifest.ID, manifest); !errors.Is(err, ErrVersionTransition) {
		t.Fatalf("same-version error=%v, want ErrVersionTransition", err)
	}
	if _, err := registry.Quarantine(manifest.ID, "security review"); err != nil {
		t.Fatal(err)
	}
	upgrade := manifest
	upgrade.Version = "2.0.0"
	if _, err := registry.Upgrade(manifest.ID, upgrade); !errors.Is(err, ErrVersionTransition) {
		t.Fatalf("quarantined upgrade error=%v, want ErrVersionTransition", err)
	}
}
