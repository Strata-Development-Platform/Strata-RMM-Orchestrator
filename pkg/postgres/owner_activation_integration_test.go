//go:build dbintegration

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func newOwnerMigrationDB(t *testing.T, through int) *sql.DB {
	t.Helper()
	base := os.Getenv("TEST_POSTGRES_DSN")
	if base == "" {
		t.Fatal("TEST_POSTGRES_DSN is required")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("owner_migration_%d", time.Now().UnixNano())
	controlURL := *parsed
	controlURL.Path = "/postgres"
	control, err := sql.Open("postgres", controlURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.Exec("CREATE DATABASE " + databaseName); err != nil {
		_ = control.Close()
		t.Fatal(err)
	}
	testURL := *parsed
	testURL.Path = "/" + databaseName
	db, err := sql.Open("postgres", testURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = control.Exec("DROP DATABASE IF EXISTS " + databaseName + " WITH (FORCE)")
		_ = control.Close()
	})
	if _, err := db.Exec(`
		CREATE TABLE schema_migrations (
			id INT PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range Migrations() {
		if migration.ID > through {
			continue
		}
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(migration.Up); err != nil {
			_ = tx.Rollback()
			t.Fatalf("apply migration %d: %v", migration.ID, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (id, name) VALUES ($1, $2)`, migration.ID, migration.Name); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestMigration68UpgradeFrom67BackfillAndRollback(t *testing.T) {
	db := newOwnerMigrationDB(t, 67)
	const tenantID = "16800000-0000-0000-0000-000000000001"
	if _, err := db.Exec(`INSERT INTO tenants (id, name, slug) VALUES ($1, 'Existing', 'migration-68-existing')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (tenant_id, email, password_hash, role, is_active)
		VALUES ($1, 'Existing@Example.Test', 'redacted', 'viewer', TRUE)
	`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO msp_tenants (name, slug, is_active)
		VALUES ('Existing suspended MSP', 'migration-68-msp', FALSE)
	`); err != nil {
		t.Fatal(err)
	}
	migration := migrationByID(t, 68)
	if _, err := db.Exec(migration.Up); err != nil {
		t.Fatalf("upgrade from 67: %v", err)
	}
	var onboardingStatus, normalizedEmail string
	var mspActive bool
	if err := db.QueryRow(`SELECT onboarding_status, is_active FROM msp_tenants WHERE slug = 'migration-68-msp'`).Scan(&onboardingStatus, &mspActive); err != nil {
		t.Fatal(err)
	}
	if onboardingStatus != "active" || mspActive {
		t.Fatalf("backfill status=%q is_active=%v, want active/false", onboardingStatus, mspActive)
	}
	var verified bool
	if err := db.QueryRow(`
		SELECT normalized_email, email_verified_at IS NOT NULL
		FROM users WHERE email = 'Existing@Example.Test'
	`).Scan(&normalizedEmail, &verified); err != nil {
		t.Fatal(err)
	}
	if normalizedEmail != "existing@example.test" || !verified {
		t.Fatalf("normalized=%q verified=%v", normalizedEmail, verified)
	}
	if _, err := db.Exec(migration.Down); err != nil {
		t.Fatalf("rollback migration 68: %v", err)
	}
	var invitationTable, normalizedColumn bool
	if err := db.QueryRow(`SELECT to_regclass('public.account_invitations') IS NOT NULL`).Scan(&invitationTable); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'users' AND column_name = 'normalized_email'
		)
	`).Scan(&normalizedColumn); err != nil {
		t.Fatal(err)
	}
	if invitationTable || normalizedColumn {
		t.Fatalf("rollback retained migration 68 objects: table=%v column=%v", invitationTable, normalizedColumn)
	}
}

func TestMigration68ReportsDuplicateNormalizedEmails(t *testing.T) {
	db := newOwnerMigrationDB(t, 67)
	if _, err := db.Exec(`
		INSERT INTO tenants (id, name, slug) VALUES
			('26800000-0000-0000-0000-000000000001', 'A', 'migration-68-a'),
			('26800000-0000-0000-0000-000000000002', 'B', 'migration-68-b');
		INSERT INTO users (tenant_id, email, password_hash, role) VALUES
			('26800000-0000-0000-0000-000000000001', 'Owner@Example.Test', 'redacted', 'viewer'),
			('26800000-0000-0000-0000-000000000002', ' owner@example.test ', 'redacted', 'viewer');
	`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(migrationByID(t, 68).Up)
	if err == nil {
		t.Fatal("migration 68 accepted duplicate normalized emails")
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "duplicates") || !strings.Contains(message, "owner@example.test (2)") {
		t.Fatalf("duplicate report was not explicit: %v", err)
	}
}

func TestMigration68NullableIdentityAndInvitationRLSFailClosed(t *testing.T) {
	db := newOwnerMigrationDB(t, 68)
	const (
		userID     = "36800000-0000-0000-0000-000000000001"
		mspID      = "36800000-0000-0000-0000-000000000002"
		invitation = "36800000-0000-0000-0000-000000000003"
		tokenHash  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	seed, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`SELECT set_config('app.role', 'platform_admin', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`
		INSERT INTO users (id, tenant_id, email, password_hash, role, is_active, email_verified_at)
		VALUES ($1, NULL, 'nullable@example.test', 'redacted', 'viewer', TRUE, NOW())
	`, userID); err != nil {
		_ = seed.Rollback()
		t.Fatal(err)
	}
	if _, err := seed.Exec(`
		INSERT INTO msp_tenants (id, name, slug, is_active, onboarding_status)
		VALUES ($1, 'Nullable MSP', 'nullable-msp', FALSE, 'pending_owner')
	`, mspID); err != nil {
		_ = seed.Rollback()
		t.Fatal(err)
	}
	if _, err := seed.Exec(`
		INSERT INTO account_invitations (id, msp_id, email_normalized, token_hash, created_by, expires_at)
		VALUES ($1, $2, 'nullable@example.test', $3, $4, NOW() + INTERVAL '1 hour')
	`, invitation, mspID, tokenHash, userID); err != nil {
		_ = seed.Rollback()
		t.Fatal(err)
	}
	if err := seed.Commit(); err != nil {
		t.Fatal(err)
	}

	role := fmt.Sprintf("strata_activation_%d", time.Now().UnixNano())
	if _, err := db.Exec(`CREATE ROLE ` + role + ` NOLOGIN NOSUPERUSER NOBYPASSRLS`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP ROLE IF EXISTS ` + role) })
	if _, err := db.Exec(`
		GRANT USAGE ON SCHEMA public TO ` + role + `;
		GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ` + role + `;
		GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ` + role + `;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO ` + role + `;
	`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL ROLE ` + role); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"users", "account_invitations"} {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("missing identity settings caused %s error: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("missing identity settings exposed %d %s rows", count, table)
		}
	}
	if _, err := tx.Exec(`SELECT set_config('app.user_id', $1, true)`, userID); err != nil {
		t.Fatal(err)
	}
	var userCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil || userCount != 1 {
		t.Fatalf("self identity visibility count=%d err=%v", userCount, err)
	}
	if _, err := tx.Exec(`SELECT set_config('app.user_id', '', true), set_config('app.invitation_hash', $1, true)`, tokenHash); err != nil {
		t.Fatal(err)
	}
	var invitationCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM account_invitations`).Scan(&invitationCount); err != nil || invitationCount != 1 {
		t.Fatalf("invitation capability visibility count=%d err=%v", invitationCount, err)
	}
}
