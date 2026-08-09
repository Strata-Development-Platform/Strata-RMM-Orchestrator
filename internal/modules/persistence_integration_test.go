//go:build moduleintegration

package modules

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestAddonPersistenceRLSAuditAndRestart(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("TEST_POSTGRES_DSN is required")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	applyAddonMigration(t, admin)
	const role = "strata_module_rls_test"
	const password = "module-test-only-password"
	_, _ = admin.ExecContext(ctx, `DROP OWNED BY `+role+` CASCADE`)
	_, _ = admin.ExecContext(ctx, `DROP ROLE IF EXISTS `+role)
	if _, err := admin.ExecContext(ctx, `CREATE ROLE `+role+` LOGIN PASSWORD '`+password+`' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = admin.ExecContext(ctx, `DROP OWNED BY `+role+` CASCADE`)
		_, _ = admin.ExecContext(ctx, `DROP ROLE IF EXISTS `+role)
	}()
	for _, grant := range []string{
		`GRANT USAGE ON SCHEMA public TO ` + role,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON addon_modules TO ` + role,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON addon_module_audit TO ` + role,
		`GRANT USAGE, SELECT ON SEQUENCE addon_module_audit_id_seq TO ` + role,
	} {
		if _, err := admin.ExecContext(ctx, grant); err != nil {
			t.Fatal(err)
		}
	}

	userDB, err := sql.Open("postgres", dsnForUser(t, dsn, role, password))
	if err != nil {
		t.Fatal(err)
	}
	defer userDB.Close()
	if err := userDB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	store := NewSQLStore()
	registry := NewRegistry()
	registry.now = func() time.Time { return time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC) }
	installed, err := registry.Install(validManifest())
	if err != nil {
		t.Fatal(err)
	}

	platformTx := scopedModuleTx(t, userDB, "platform_admin")
	if err := store.Save(ctx, platformTx, installed, "integration-admin", "install"); err != nil {
		t.Fatal(err)
	}
	if err := platformTx.Commit(); err != nil {
		t.Fatal(err)
	}

	quarantined, err := registry.Quarantine(installed.Manifest.ID, "integration quarantine")
	if err != nil {
		t.Fatal(err)
	}
	platformTx = scopedModuleTx(t, userDB, "platform_admin")
	if err := store.Save(ctx, platformTx, quarantined, "integration-admin", "quarantine"); err != nil {
		t.Fatal(err)
	}
	if err := platformTx.Commit(); err != nil {
		t.Fatal(err)
	}

	// A non-platform application scope receives no module rows and cannot write.
	mspTx := scopedModuleTx(t, userDB, "msp_admin")
	modules, err := store.List(ctx, mspTx)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 0 {
		t.Fatalf("MSP scope observed %d platform module rows", len(modules))
	}
	if err := mspTx.Rollback(); err != nil {
		t.Fatal(err)
	}

	mspTx = scopedModuleTx(t, userDB, "msp_admin")
	if err := store.Save(ctx, mspTx, installed, "msp-user", "install"); err == nil {
		t.Fatal("MSP scope unexpectedly persisted a platform module")
	}
	_ = mspTx.Rollback()

	// Simulate orchestrator restart: reconstruct the in-memory registry from the
	// durable database state. Quarantine must survive and remain fail closed.
	platformTx = scopedModuleTx(t, userDB, "platform_owner")
	restored, err := store.RestoreRegistry(ctx, platformTx)
	if err != nil {
		t.Fatal(err)
	}
	if err := platformTx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := restored.Get(installed.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateQuarantined || got.Reason != "integration quarantine" {
		t.Fatalf("restored module = %+v", got)
	}
	if _, err := restored.Enable(installed.Manifest.ID); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("restored quarantine was bypassed: %v", err)
	}

	platformTx = scopedModuleTx(t, userDB, "platform_admin")
	var auditRows int
	if err := platformTx.QueryRowContext(ctx, `SELECT count(*) FROM addon_module_audit WHERE module_id=$1`, installed.Manifest.ID).Scan(&auditRows); err != nil {
		t.Fatal(err)
	}
	if auditRows != 2 {
		t.Fatalf("audit rows=%d, want 2", auditRows)
	}
	if err := platformTx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Audit rows are immutable even to the platform role used by the app.
	platformTx = scopedModuleTx(t, userDB, "platform_owner")
	if _, err := platformTx.ExecContext(ctx, `UPDATE addon_module_audit SET reason='tampered' WHERE module_id=$1`, installed.Manifest.ID); err == nil {
		t.Fatal("append-only addon audit allowed update")
	}
	_ = platformTx.Rollback()
}

func scopedModuleTx(t *testing.T, db *sql.DB, role string) *sql.Tx {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`SELECT set_config('app.role', $1, true)`, role); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	return tx
}

func applyAddonMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	down, err := os.ReadFile(filepath.Join(root, "migrations", "00090_addon_modules.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(string(down))
	up, err := os.ReadFile(filepath.Join(root, "migrations", "00090_addon_modules.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatal(err)
	}
}

func dsnForUser(t *testing.T, raw, user, password string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(user, password)
	return u.String()
}
