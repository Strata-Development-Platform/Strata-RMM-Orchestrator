//go:build dbintegration

package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func TestTenantRLSMigration(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("TEST_POSTGRES_DSN is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset test schema: %v", err)
	}
	if err := NewSchemaManager(db).Apply(context.Background()); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if _, err := db.Exec(`
		DROP ROLE IF EXISTS strata_runtime;
		CREATE ROLE strata_runtime NOLOGIN NOSUPERUSER NOBYPASSRLS;
		GRANT USAGE ON SCHEMA public TO strata_runtime;
		GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO strata_runtime;
		GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO strata_runtime;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO strata_runtime
	`); err != nil {
		t.Fatalf("create runtime database role: %v", err)
	}

	const (
		mspA    = "10000000-0000-0000-0000-000000000001"
		mspB    = "20000000-0000-0000-0000-000000000001"
		clientA = "10000000-0000-0000-0000-000000000002"
		clientB = "20000000-0000-0000-0000-000000000002"
		userA   = "30000000-0000-0000-0000-000000000001"
		userB   = "30000000-0000-0000-0000-000000000003"
		grantID = "30000000-0000-0000-0000-000000000002"
	)

	seed, err := db.Begin()
	if err != nil {
		t.Fatalf("begin seed: %v", err)
	}
	if _, err := seed.Exec(`SELECT set_config('app.role', 'platform_admin', true)`); err != nil {
		t.Fatalf("set seed role: %v", err)
	}
	if _, err := seed.Exec(`
		INSERT INTO msp_tenants (id, name, slug) VALUES
			($1, 'MSP A', 'msp-a'),
			($2, 'MSP B', 'msp-b')
	`, mspA, mspB); err != nil {
		_ = seed.Rollback()
		t.Fatalf("seed MSPs: %v", err)
	}
	if _, err := seed.Exec(`
		INSERT INTO client_organizations (id, msp_id, name, slug) VALUES
			($1, $2, 'Client A', 'client-a'),
			($3, $4, 'Client B', 'client-b')
	`, clientA, mspA, clientB, mspB); err != nil {
		_ = seed.Rollback()
		t.Fatalf("seed clients: %v", err)
	}
	if _, err := seed.Exec(`
		INSERT INTO plan_entitlements (msp_id, plan_id) VALUES
			($1, '00000000-0000-0000-0000-000000000001'),
			($2, '00000000-0000-0000-0000-000000000001')
	`, mspA, mspB); err != nil {
		_ = seed.Rollback()
		t.Fatalf("seed entitlements: %v", err)
	}
	if _, err := seed.Exec(`
		INSERT INTO usage_snapshots (msp_id, device_count) VALUES ($1, 1), ($2, 2)
	`, mspA, mspB); err != nil {
		_ = seed.Rollback()
		t.Fatalf("seed usage snapshots: %v", err)
	}
	if _, err := seed.Exec(`
		INSERT INTO control_plane_audit (msp_id, actor_user_id, action, resource_type)
		VALUES ($1, 'ci', 'test.a', 'test'), ($2, 'ci', 'test.b', 'test')
	`, mspA, mspB); err != nil {
		_ = seed.Rollback()
		t.Fatalf("seed control-plane audit: %v", err)
	}
	if _, err := seed.Exec(`
		INSERT INTO support_access_grants (
			id, platform_user_id, msp_id, reason, ticket_ref, approved_by,
			expires_at, status, permissions
		) VALUES (
			$1, $2, $3, 'CI isolation test', 'CI-1', 'ci',
			NOW() + INTERVAL '15 minutes', 'active', ARRAY['read']::text[]
		)
	`, grantID, userA, mspB); err != nil {
		_ = seed.Rollback()
		t.Fatalf("seed support grant: %v", err)
	}
	if _, err := seed.Exec(`
		INSERT INTO users (id, email, password_hash, role, is_active, email_verified_at)
		VALUES
			($1, 'user-a@example.test', 'x', 'viewer', true, NOW()),
			($2, 'user-b@example.test', 'x', 'viewer', true, NOW())
	`, userA, userB); err != nil {
		_ = seed.Rollback()
		t.Fatalf("seed users: %v", err)
	}
	if _, err := seed.Exec(`
		INSERT INTO memberships (user_id, role, scope_type, scope_id, status)
		VALUES
			($1, 'msp_owner', 'msp', $2, 'active'),
			($3, 'msp_owner', 'msp', $4, 'active')
	`, userA, mspA, userB, mspB); err != nil {
		_ = seed.Rollback()
		t.Fatalf("seed memberships: %v", err)
	}
	if err := seed.Commit(); err != nil {
		t.Fatalf("commit seed: %v", err)
	}

	assertVisibleClients(t, db, "", "", "", 0)
	assertVisibleClients(t, db, mspA, userA, "", 1)
	assertVisibleClients(t, db, mspB, userB, "", 1)
	assertVisibleClients(t, db, "", userA, grantID, 1)
	assertVisibleEntitlements(t, db, "", "", "", 0)
	assertVisibleEntitlements(t, db, mspA, userA, "", 1)
	assertVisibleEntitlements(t, db, mspB, userB, "", 1)
	assertVisibleEntitlements(t, db, "", userA, grantID, 1)
	assertTenantTableCount(t, db, "usage_snapshots", mspA, userA, "", 1)
	assertTenantTableCount(t, db, "usage_snapshots", mspB, userB, "", 1)
	assertTenantTableCount(t, db, "control_plane_audit", mspA, userA, "", 1)
	assertTenantTableCount(t, db, "control_plane_audit", mspB, userB, "", 1)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin cross-tenant update: %v", err)
	}
	if _, err := tx.Exec(`SET LOCAL ROLE strata_runtime`); err != nil {
		t.Fatalf("set runtime role: %v", err)
	}
	if _, err := tx.Exec(`SELECT set_config('app.msp_id', $1, true)`, mspA); err != nil {
		t.Fatalf("set MSP context: %v", err)
	}
	result, err := tx.Exec(`UPDATE client_organizations SET name = 'forbidden' WHERE id = $1`, clientB)
	if err != nil {
		t.Fatalf("cross-tenant update returned unexpected database error: %v", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("cross-tenant rows affected: %v", err)
	}
	if affected != 0 {
		t.Fatalf("cross-tenant update affected %d rows, want 0", affected)
	}
	result, err = tx.Exec(`UPDATE plan_entitlements SET device_count = 99 WHERE msp_id = $1`, mspB)
	if err != nil {
		t.Fatalf("cross-tenant entitlement update returned unexpected database error: %v", err)
	}
	affected, err = result.RowsAffected()
	if err != nil {
		t.Fatalf("cross-tenant entitlement rows affected: %v", err)
	}
	if affected != 0 {
		t.Fatalf("cross-tenant entitlement update affected %d rows, want 0", affected)
	}
	_ = tx.Rollback()

	var forceRLS bool
	if err := db.QueryRow(`
		SELECT relforcerowsecurity
		FROM pg_class
		WHERE oid = 'plan_entitlements'::regclass
	`).Scan(&forceRLS); err != nil {
		t.Fatalf("inspect entitlement RLS: %v", err)
	}
	if !forceRLS {
		t.Fatal("plan_entitlements must force row-level security")
	}
	for _, table := range []string{"usage_snapshots", "control_plane_audit"} {
		if err := db.QueryRow(`
			SELECT relforcerowsecurity FROM pg_class WHERE oid = $1::regclass
		`, table).Scan(&forceRLS); err != nil {
			t.Fatalf("inspect %s RLS: %v", table, err)
		}
		if !forceRLS {
			t.Fatalf("%s must force row-level security", table)
		}
	}
}

func assertTenantTableCount(t *testing.T, db *sql.DB, table, mspID, userID, grantID string, want int) {
	t.Helper()
	if table != "usage_snapshots" && table != "control_plane_audit" {
		t.Fatalf("unsupported test table %q", table)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin %s visibility transaction: %v", table, err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL ROLE strata_runtime`); err != nil {
		t.Fatalf("set runtime role: %v", err)
	}
	if _, err := tx.Exec(`
		SELECT set_config('app.msp_id', $1, true),
		       set_config('app.user_id', $2, true),
		       set_config('app.support_grant_id', $3, true),
		       set_config('app.scope_type', CASE WHEN $1 = '' THEN '' ELSE 'msp' END, true),
		       set_config('app.permission', 'read', true)
	`, mspID, userID, grantID); err != nil {
		t.Fatalf("set %s visibility context: %v", table, err)
	}
	query := `SELECT COUNT(*) FROM usage_snapshots`
	if table == "control_plane_audit" {
		query = `SELECT COUNT(*) FROM control_plane_audit`
	}
	var got int
	if err := tx.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("count visible %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("visible %s for MSP %q = %d, want %d", table, mspID, got, want)
	}
}

func assertVisibleClients(t *testing.T, db *sql.DB, mspID, userID, grantID string, want int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin visibility transaction: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL ROLE strata_runtime`); err != nil {
		t.Fatalf("set runtime role: %v", err)
	}
	if _, err := tx.Exec(`
		SELECT
			set_config('app.msp_id', $1, true),
			set_config('app.user_id', $2, true),
			set_config('app.support_grant_id', $3, true),
			set_config('app.scope_type', CASE WHEN $1 = '' THEN '' ELSE 'msp' END, true),
			set_config('app.permission', 'read', true)
	`, mspID, userID, grantID); err != nil {
		t.Fatalf("set visibility context: %v", err)
	}
	var got int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM client_organizations`).Scan(&got); err != nil {
		t.Fatalf("count visible clients: %v", err)
	}
	if got != want {
		t.Fatalf("visible clients for MSP %q and grant %q = %d, want %d", mspID, grantID, got, want)
	}
}

func assertVisibleEntitlements(t *testing.T, db *sql.DB, mspID, userID, grantID string, want int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin entitlement visibility transaction: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL ROLE strata_runtime`); err != nil {
		t.Fatalf("set runtime role: %v", err)
	}
	if _, err := tx.Exec(`
		SELECT
			set_config('app.msp_id', $1, true),
			set_config('app.user_id', $2, true),
			set_config('app.support_grant_id', $3, true),
			set_config('app.scope_type', CASE WHEN $1 = '' THEN '' ELSE 'msp' END, true),
			set_config('app.permission', 'read', true)
	`, mspID, userID, grantID); err != nil {
		t.Fatalf("set entitlement visibility context: %v", err)
	}
	var got int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM plan_entitlements`).Scan(&got); err != nil {
		t.Fatalf("count visible entitlements: %v", err)
	}
	if got != want {
		t.Fatalf("visible entitlements for MSP %q and grant %q = %d, want %d", mspID, grantID, got, want)
	}
}
