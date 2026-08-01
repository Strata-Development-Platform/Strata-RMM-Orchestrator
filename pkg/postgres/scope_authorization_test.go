package postgres

import (
	"strings"
	"testing"
)

func TestMigration69ScopeAuthorizationContract(t *testing.T) {
	migrations := Migrations()
	migration := migrations[len(migrations)-1]
	if migration.ID != 69 || migration.Name != "enforce_scope_bound_authorization" {
		t.Fatalf("last migration = %d/%q", migration.ID, migration.Name)
	}
	for _, fragment := range []string{
		"authorization_migration_issues",
		"legacy_role_without_membership",
		"legacy_tenant_access_without_membership",
		"memberships_role_scope_check",
		"app_scope_is_authorized",
		"app_actor_can_manage_scope",
		"app_may_manage_membership",
		"app_is_trusted_runtime",
		"app_is_initial_bootstrap",
		"safe_app_setting('app.scope_type')",
		"control_plane_audit_select",
		"SELECT 1;",
	} {
		if !strings.Contains(migration.Up+migration.Down, fragment) {
			t.Errorf("migration 69 missing %q", fragment)
		}
	}
}
