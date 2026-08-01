package postgres

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestMigration67ProviderProfileContract(t *testing.T) {
	migrations := Migrations()
	var migration Migration
	for _, candidate := range migrations {
		if candidate.ID == 67 {
			migration = candidate
			break
		}
	}
	if migration.ID != 67 {
		t.Fatal("migration 67 is missing")
	}
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS legal_name",
		"setup_completed_at TIMESTAMPTZ",
		"setup_completed_by UUID REFERENCES users(id)",
		"ON CONFLICT (user_id, scope_type, scope_id, role)",
		"platform.bootstrap_admin",
		"control_plane_audit_immutable",
	} {
		if !strings.Contains(migration.Up, fragment) {
			t.Errorf("migration 67 missing %q", fragment)
		}
	}
}

func TestChangedProviderFieldsAreSortedAndContainNoValues(t *testing.T) {
	before := ProviderBusinessProfileValues{DisplayName: "Old", TaxIdentifier: "sensitive-old"}
	after := before
	after.DisplayName = "New"
	after.TaxIdentifier = "sensitive-new"
	want := []string{"display_name", "tax_identifier"}
	if got := changedProviderFields(before, after); !reflect.DeepEqual(got, want) {
		t.Fatalf("changed fields = %v, want %v", got, want)
	}
}

func TestProviderSetupErrorsAreStable(t *testing.T) {
	if !errors.Is(ErrProviderSetupAlreadyCompleted, ErrProviderSetupAlreadyCompleted) {
		t.Fatal("completed sentinel error is not stable")
	}
	if !errors.Is(ErrProviderSetupIncomplete, ErrProviderSetupIncomplete) {
		t.Fatal("incomplete sentinel error is not stable")
	}
}
