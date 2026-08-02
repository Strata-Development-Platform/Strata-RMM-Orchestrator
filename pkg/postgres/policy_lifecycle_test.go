package postgres

import (
	"strings"
	"testing"
)

func TestMigration71PolicyLifecycleContract(t *testing.T) {
	migrations := Migrations()
	var migration *Migration
	for i := range migrations {
		if migrations[i].ID == 71 {
			migration = &migrations[i]
			break
		}
	}
	if migration == nil || migration.Name != "policy_lifecycle_and_revisions" {
		t.Fatal("migration 71 policy lifecycle contract not found")
	}
	for _, fragment := range []string{
		"policies ADD COLUMN IF NOT EXISTS device_id",
		"policies ADD COLUMN IF NOT EXISTS validated_at",
		"policies ADD COLUMN IF NOT EXISTS previewed_at",
		"policies ADD COLUMN IF NOT EXISTS published_version",
		"policies ADD COLUMN IF NOT EXISTS published_config",
		"policies ADD COLUMN IF NOT EXISTS maintenance_start",
		"policies ADD COLUMN IF NOT EXISTS maintenance_end",
		"policies ADD COLUMN IF NOT EXISTS maintenance_days",
		"policies ADD COLUMN IF NOT EXISTS maintenance_timezone",
		"CREATE TABLE IF NOT EXISTS policy_revisions",
		"UNIQUE(policy_id, version)",
		"CREATE POLICY policy_scope ON policies",
		"CREATE POLICY policy_revision_scope ON policy_revisions",
		"ALTER TABLE policy_revisions FORCE ROW LEVEL SECURITY",
		"DROP TABLE IF EXISTS policy_revisions",
	} {
		if !strings.Contains(migration.Up+migration.Down, fragment) {
			t.Errorf("migration 71 missing %q", fragment)
		}
	}
}
