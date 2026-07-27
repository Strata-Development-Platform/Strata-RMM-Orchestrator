//go:build dbintegration

package postgres

import (
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
	if err := NewSchemaManager(db).Apply(); err != nil {
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
		userID  = "30000000-0000-0000-0000-000000000001"
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
		INSERT INTO support_access_grants (
			id, platform_user_id, msp_id, reason, ticket_ref, approved_by,
			expires_at, status, permissions
		) VALUES (
			$1, $2, $3, 'CI isolation test', 'CI-1', 'ci',
			NOW() + INTERVAL '15 minutes', 'active', ARRAY['read']::text[]
		)
	`, grantID, userID, mspB); err != nil {
		_ = seed.Rollback()
		t.Fatalf("seed support grant: %v", err)
	}
	if err := seed.Commit(); err != nil {
		t.Fatalf("commit seed: %v", err)
	}

	assertVisibleClients(t, db, "", "", "", 0)
	assertVisibleClients(t, db, mspA, "", "", 1)
	assertVisibleClients(t, db, mspB, "", "", 1)
	assertVisibleClients(t, db, "", userID, grantID, 1)

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
	_ = tx.Rollback()
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
