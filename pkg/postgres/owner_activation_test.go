package postgres

import (
	"strings"
	"testing"
)

func TestMigration68OwnerActivationContract(t *testing.T) {
	migrations := Migrations()
	var migration Migration
	for _, candidate := range migrations {
		if candidate.ID == 68 {
			migration = candidate
			break
		}
	}
	if migration.ID != 68 || migration.Name != "add_msp_owner_activation" {
		t.Fatalf("last migration = %d/%q", migration.ID, migration.Name)
	}
	for _, fragment := range []string{
		"GENERATED ALWAYS AS (lower(btrim(email))) STORED",
		"idx_users_normalized_email_unique",
		"email_verified_at",
		"onboarding_status IN ('pending_owner', 'active')",
		"CREATE TABLE IF NOT EXISTS account_invitations",
		"token_hash       CHAR(64)",
		"idx_account_invitations_one_unconsumed_msp",
		"msp_owner_activation",
		"ALTER TABLE account_invitations FORCE ROW LEVEL SECURITY",
		"ALTER TABLE users FORCE ROW LEVEL SECURITY",
	} {
		if !strings.Contains(migration.Up, fragment) {
			t.Errorf("migration 68 Up missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS account_invitations",
		"tenant-neutral user identities exist",
		"DROP COLUMN IF EXISTS normalized_email",
		"DROP COLUMN IF EXISTS onboarding_status",
	} {
		if !strings.Contains(migration.Down, fragment) {
			t.Errorf("migration 68 Down missing %q", fragment)
		}
	}
}
