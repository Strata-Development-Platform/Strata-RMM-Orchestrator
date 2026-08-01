//go:build dbintegration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

const providerTestUserID = "67000000-0000-0000-0000-000000000001"

func newProviderIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	base := os.Getenv("TEST_POSTGRES_DSN")
	if base == "" {
		t.Fatal("TEST_POSTGRES_DSN is required")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("provider_%d", time.Now().UnixNano())
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
		_, _ = control.Exec("DROP DATABASE IF EXISTS " + databaseName + " WITH (FORCE)")
		_ = control.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = control.Exec("DROP DATABASE IF EXISTS " + databaseName + " WITH (FORCE)")
		_ = control.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := NewSchemaManager(db).Apply(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func migrationByID(t *testing.T, id int) Migration {
	t.Helper()
	for _, migration := range Migrations() {
		if migration.ID == id {
			return migration
		}
	}
	t.Fatalf("migration %d not found", id)
	return Migration{}
}

func TestMigration67BackfillsBootstrapOwnerIdempotently(t *testing.T) {
	db := newProviderIntegrationDB(t)
	migration := migrationByID(t, 67)
	if _, err := db.Exec(migration.Down); err != nil {
		t.Fatalf("unapply migration 67 SQL: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE id = 67`); err != nil {
		t.Fatal(err)
	}

	const tenantID = "67000000-0000-0000-0000-000000000002"
	if _, err := db.Exec(`INSERT INTO tenants (id, name, slug, plan) VALUES ($1, 'Existing Platform', 'existing-platform', 'enterprise')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, tenant_id, email, password_hash, role)
		VALUES ($2, $1, 'existing-owner@example.test', 'redacted-hash', 'admin')
	`, tenantID, providerTestUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO audit_log (tenant_id, user_id, action, resource, details)
		VALUES ($1, $2, 'platform.bootstrap_admin', 'user', '{"source":"local-installer"}')
	`, tenantID, providerTestUserID); err != nil {
		t.Fatal(err)
	}

	if err := NewSchemaManager(db).Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := NewSchemaManager(db).Apply(context.Background()); err != nil {
		t.Fatalf("second migration apply: %v", err)
	}
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM memberships
		WHERE user_id = $1 AND role = 'platform_owner'
		  AND scope_type = 'platform' AND scope_id = $2 AND status = 'active'
	`, providerTestUserID, SingletonPlatformID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("backfilled owner memberships = %d, want 1", count)
	}

	var legalName, displayName string
	var updatedAt time.Time
	if err := db.QueryRow(`SELECT legal_name, display_name, updated_at FROM platforms WHERE id = $1`, SingletonPlatformID).
		Scan(&legalName, &displayName, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if legalName != "" || displayName != "" || updatedAt.IsZero() {
		t.Fatalf("incompatible migrated defaults: legal=%q display=%q updated=%v", legalName, displayName, updatedAt)
	}
}

func TestProviderSetupAtomicConcurrentRetrySafeAndAudited(t *testing.T) {
	db := newProviderIntegrationDB(t)
	seedProviderActor(t, db, providerTestUserID, "platform_owner", "platform", SingletonPlatformID)
	values := providerIntegrationValues()

	// A caller-controlled rollback proves profile and audit share the transaction.
	tx := beginProviderTransaction(t, db, providerTestUserID, true)
	if _, created, err := CompleteProviderSetup(context.Background(), tx, providerTestUserID, values); err != nil || !created {
		_ = tx.Rollback()
		t.Fatalf("staged completion: created=%v err=%v", created, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertProviderIncomplete(t, db)

	// Force the audit insert to fail and verify the surrounding transaction can
	// leave neither the profile nor a partial completion marker behind.
	if _, err := db.Exec(`
		CREATE OR REPLACE FUNCTION fail_provider_setup_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.action = 'provider.setup_completed' THEN
				RAISE EXCEPTION 'injected provider audit failure';
			END IF;
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER fail_provider_setup_audit
			BEFORE INSERT ON control_plane_audit
			FOR EACH ROW EXECUTE FUNCTION fail_provider_setup_audit();
	`); err != nil {
		t.Fatal(err)
	}
	tx = beginProviderTransaction(t, db, providerTestUserID, true)
	if _, _, err := CompleteProviderSetup(context.Background(), tx, providerTestUserID, values); err == nil {
		_ = tx.Rollback()
		t.Fatal("completion succeeded despite injected audit failure")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TRIGGER fail_provider_setup_audit ON control_plane_audit; DROP FUNCTION fail_provider_setup_audit()`); err != nil {
		t.Fatal(err)
	}
	assertProviderIncomplete(t, db)

	// Two simultaneous submissions serialize on SELECT ... FOR UPDATE. Exactly
	// one performs the write; the other observes the committed identical profile.
	start := make(chan struct{})
	results := make(chan struct {
		created bool
		err     error
	}, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			actorTx, err := beginProviderTransactionE(db, providerTestUserID, true)
			if err != nil {
				ready.Done()
				results <- struct {
					created bool
					err     error
				}{err: err}
				return
			}
			ready.Done()
			<-start
			_, created, err := CompleteProviderSetup(context.Background(), actorTx, providerTestUserID, values)
			if err == nil {
				err = actorTx.Commit()
			} else {
				_ = actorTx.Rollback()
			}
			results <- struct {
				created bool
				err     error
			}{created: created, err: err}
		}()
	}
	ready.Wait()
	close(start)
	createdCount := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent completion: %v", result.err)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent writers that created setup = %d, want 1", createdCount)
	}

	// A later identical retry is also a no-op with no duplicate audit row.
	tx = beginProviderTransaction(t, db, providerTestUserID, true)
	profile, created, err := CompleteProviderSetup(context.Background(), tx, providerTestUserID, values)
	if err != nil || created {
		_ = tx.Rollback()
		t.Fatalf("identical retry: created=%v err=%v", created, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if !profile.SetupComplete || profile.SetupCompletedBy != providerTestUserID || profile.SetupCompletedAt == nil {
		t.Fatalf("unexpected completed profile: %+v", profile)
	}
	assertProviderAuditCount(t, db, "provider.setup_completed", 1)

	different := values
	different.DisplayName = "Different Provider"
	tx = beginProviderTransaction(t, db, providerTestUserID, true)
	if _, _, err := CompleteProviderSetup(context.Background(), tx, providerTestUserID, different); !errors.Is(err, ErrProviderSetupAlreadyCompleted) {
		_ = tx.Rollback()
		t.Fatalf("different retry error = %v", err)
	}
	_ = tx.Rollback()

	completedAt := *profile.SetupCompletedAt
	completedBy := profile.SetupCompletedBy
	updatedValues := values
	updatedValues.DisplayName = "Updated Provider"
	tx = beginProviderTransaction(t, db, providerTestUserID, true)
	updated, changed, err := UpdateProviderBusinessProfile(context.Background(), tx, providerTestUserID, updatedValues)
	if err != nil || !changed {
		_ = tx.Rollback()
		t.Fatalf("later profile update: changed=%v err=%v", changed, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if updated.SetupCompletedAt == nil || !updated.SetupCompletedAt.Equal(completedAt) || updated.SetupCompletedBy != completedBy {
		t.Fatalf("update rewrote protected completion metadata: before=%v/%s after=%v/%s",
			completedAt, completedBy, updated.SetupCompletedAt, updated.SetupCompletedBy)
	}
	assertProviderAuditCount(t, db, "provider.profile_updated", 1)

	var auditID string
	if err := db.QueryRow(`SELECT id::text FROM control_plane_audit WHERE action = 'provider.setup_completed'`).Scan(&auditID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE control_plane_audit SET action = 'tampered' WHERE id = $1`, auditID); err == nil {
		t.Fatal("immutable control-plane audit row was updated")
	}
}

func TestProviderAuthorizationRequiresExactActivePlatformMembership(t *testing.T) {
	db := newProviderIntegrationDB(t)
	const (
		ownerID    = "67000000-0000-0000-0000-000000000011"
		adminID    = "67000000-0000-0000-0000-000000000012"
		mspAdminID = "67000000-0000-0000-0000-000000000013"
		viewerID   = "67000000-0000-0000-0000-000000000014"
		mspID      = "67000000-0000-0000-0000-000000000015"
	)
	seedProviderActor(t, db, ownerID, "platform_owner", "platform", SingletonPlatformID)
	seedProviderActor(t, db, adminID, "platform_admin", "platform", SingletonPlatformID)
	if _, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug) VALUES ($1, 'Auth MSP', 'provider-auth-msp')`, mspID); err != nil {
		t.Fatal(err)
	}
	seedProviderActor(t, db, mspAdminID, "msp_admin", "msp", mspID)
	seedProviderActor(t, db, viewerID, "platform_viewer", "platform", SingletonPlatformID)

	for _, test := range []struct {
		name   string
		userID string
		global bool
		want   bool
	}{
		{name: "owner", userID: ownerID, global: true, want: true},
		{name: "platform admin", userID: adminID, global: true, want: true},
		{name: "MSP admin", userID: mspAdminID, want: false},
		{name: "platform viewer", userID: viewerID, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := beginProviderTransaction(t, db, test.userID, test.global)
			got, err := UserCanManageProvider(context.Background(), tx, test.userID)
			_ = tx.Rollback()
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("allowed = %v, want %v", got, test.want)
			}
		})
	}
}

func seedProviderActor(t *testing.T, db *sql.DB, userID, role, scopeType, scopeID string) {
	t.Helper()
	tenantID := userID[:len(userID)-1] + "9"
	if _, err := db.Exec(`
		INSERT INTO tenants (id, name, slug, plan)
		VALUES ($1, 'Provider Actor', $2, 'enterprise') ON CONFLICT (id) DO NOTHING
	`, tenantID, "actor-"+userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, tenant_id, email, password_hash, role, email_verified_at)
		VALUES ($2, $1, $3, 'redacted-hash', 'admin', NOW()) ON CONFLICT (id) DO NOTHING
	`, tenantID, userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO memberships (user_id, role, scope_type, scope_id, status)
		VALUES ($1, $2, $3, $4, 'active')
	`, userID, role, scopeType, scopeID); err != nil {
		t.Fatal(err)
	}
}

func beginProviderTransaction(t *testing.T, db *sql.DB, userID string, platformGlobal bool) *sql.Tx {
	t.Helper()
	tx, err := beginProviderTransactionE(db, userID, platformGlobal)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func beginProviderTransactionE(db *sql.DB, userID string, platformGlobal bool) (*sql.Tx, error) {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	role := ""
	if platformGlobal {
		role = "platform_admin"
	}
	if _, err := tx.Exec(`
		SELECT set_config('app.user_id', $1, true),
		       set_config('app.role', $2, true),
		       set_config('app.permission', 'write', true)
	`, userID, role); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func providerIntegrationValues() ProviderBusinessProfileValues {
	return ProviderBusinessProfileValues{
		LegalName: "Provider LLC", DisplayName: "Provider", ContactName: "Ada Operator",
		SupportEmail: "support@example.test", BillingEmail: "billing@example.test",
		BusinessPhone: "+14155550123", WebsiteURL: "https://example.test",
		AddressLine1: "1 Main Street", City: "Test City", StateProvince: "CA",
		PostalCode: "94105", CountryCode: "US", DefaultTimezone: "UTC",
		DefaultLocale: "en-US", DefaultCurrency: "USD", TaxIdentifier: "tax-reference",
	}
}

func assertProviderIncomplete(t *testing.T, db *sql.DB) {
	t.Helper()
	profile, err := GetProviderBusinessProfile(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if profile.SetupComplete || profile.SetupCompletedAt != nil || profile.LegalName != "" {
		t.Fatalf("provider setup was not rolled back: %+v", profile)
	}
	assertProviderAuditCount(t, db, "provider.setup_completed", 0)
}

func assertProviderAuditCount(t *testing.T, db *sql.DB, action string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM control_plane_audit WHERE action = $1`, action).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%s audit count = %d, want %d", action, count, want)
	}
}
