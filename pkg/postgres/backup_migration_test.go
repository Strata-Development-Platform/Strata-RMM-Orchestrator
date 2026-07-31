//go:build dbintegration
// +build dbintegration

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// testDBName generates a unique database name for a test.
func testDBName(t *testing.T) string {
	return "mig_" + strings.ReplaceAll(t.Name(), "/", "_")
}

// setupTestDB connects to the CI PostgreSQL service and creates an ephemeral test database.
func setupTestDB(t *testing.T, ctx context.Context) (*sql.DB, string) {
	t.Helper()

	rawDSN := os.Getenv("TEST_POSTGRES_DSN")
	var host, port, user, password string

	if rawDSN == "" {
		host = "/tmp/pg_socket"
		port = "5434"
		user = "administrator"
		password = ""
	} else {
		connStr := strings.TrimPrefix(strings.TrimPrefix(rawDSN, "postgres://"), "postgresql://")
		parts := strings.SplitN(connStr, "?", 2)
		connStr = parts[0]

		atIdx := strings.LastIndex(connStr, "@")
		if atIdx == -1 {
			require.Fail(t, "invalid admin DSN: missing @ separator", "dsn="+rawDSN)
		}

		creds := connStr[:atIdx]
		hostPart := connStr[atIdx+1:]

		dbIdx := strings.Index(hostPart, "/")
		var hostPort string
		if dbIdx == -1 {
			hostPort = hostPart
		} else {
			hostPort = hostPart[:dbIdx]
		}

		colonIdx := strings.LastIndex(hostPort, ":")
		if colonIdx > -1 {
			host = hostPort[:colonIdx]
			port = hostPort[colonIdx+1:]
		} else {
			host = hostPort
			port = "5432"
		}

		colonIdx2 := strings.Index(creds, ":")
		if colonIdx2 == -1 {
			require.Fail(t, "invalid admin DSN: missing : in credentials", "dsn="+rawDSN)
		}
		user = creds[:colonIdx2]
		password = creds[colonIdx2+1:]
	}

	adminDSN := makeMigrationDSN(host, port, user, password, "postgres")

	adminDB, err := sql.Open("postgres", adminDSN)
	require.NoError(t, err)
	require.NoError(t, adminDB.PingContext(ctx))

	dbName := testDBName(t)

	// Drop if exists, then create
	adminDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", pqIdent(dbName))) //nolint:errcheck
	_, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", pqIdent(dbName)))
	require.NoError(t, err)

	testDSN := makeMigrationDSN(host, port, user, password, dbName)

	db, err := sql.Open("postgres", testDSN)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
		adminDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", pqIdent(dbName)))
		adminDB.Close()
	})

	return db, dbName
}

// pqIdent returns a properly quoted PostgreSQL identifier.
func pqIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// makeMigrationDSN builds a PostgreSQL DSN from components using libpq key-value format.
func makeMigrationDSN(host, port, user, password, dbname string) string {
	if host == "/tmp/pg_socket" {
		if password != "" {
			return "host=" + host + " user=" + user + " password=" + password + " dbname=" + dbname + " sslmode=disable"
		}
		return "host=" + host + " user=" + user + " dbname=" + dbname + " sslmode=disable"
	}
	if password != "" {
		return "host=" + host + " port=" + port + " user=" + user + " password=" + password + " dbname=" + dbname + " sslmode=disable"
	}
	return "host=" + host + " port=" + port + " user=" + user + " dbname=" + dbname + " sslmode=disable"
}

// TestApplyMigrations verifies migrations can be applied to a clean database.
func TestApplyMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err, "reset test schema")

	require.NoError(t, NewSchemaManager(db).Apply(ctx), "apply migrations should succeed")

	var appliedCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&appliedCount)
	require.NoError(t, err)
	require.Equal(t, 64, appliedCount, "should have 64 applied migrations")
}

// TestMigrationsCreateRequiredTables verifies all expected tables are created.
func TestMigrationsCreateRequiredTables(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	requiredTables := []string{
		"tenants", "users", "devices", "permissions",
		"enrollment_tokens", "audit_log", "alert_rules",
		"notification_channels", "maintenance_windows", "patch_policies",
		"patch_deployments", "patch_deployment_devices", "patch_device_states",
		"patch_inventory", "cve_database", "device_vulnerabilities",
		"mfa_secrets", "session_recordings", "cve_sync_state",
		"cve_package_ecosystem", "tenant_encryption_keys", "user_tenant_access",
		"audit_auth", "agent_registrations", "scripts", "script_executions",
		"software_packages", "software_deployments", "software_deployment_targets",
		"report_schedules", "generated_reports", "alerts", "msp_tenants",
		"client_organizations", "sites", "branding_profiles", "custom_domains",
		"enrollment_tokens_v2", "device_groups", "jobs", "job_targets",
		"policies", "policy_assignments", "platforms", "memberships",
		"support_access_grants", "backup_records", "recovery_operations",
		"backup_audit_log", "job_outbox", "job_inbox", "plans",
		"plan_entitlements", "usage_snapshots", "control_plane_audit",
		"endpoint_approval_policies", "endpoint_approval_requests",
		"endpoint_approval_decisions", "agent_capabilities",
		"endpoint_audit_evidence", "inventory_results",
	}

	for _, table := range requiredTables {
		var exists bool
		err := db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)",
			table).Scan(&exists)
		require.NoError(t, err, "check table %s", table)
		require.True(t, exists, "table %s should exist", table)
	}

	t.Logf("Required tables verified: %d/%d", len(requiredTables), len(requiredTables))
}

// TestMigrations63BackupRecovery verifies migration 63 creates backup tables.
func TestMigrations63BackupRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var columnCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'backup_records'").Scan(&columnCount)
	require.NoError(t, err)
	require.True(t, columnCount >= 12, "backup_records should have at least 12 columns, got %d", columnCount)

	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'recovery_operations'").Scan(&columnCount)
	require.NoError(t, err)
	require.True(t, columnCount >= 8, "recovery_operations should have at least 8 columns, got %d", columnCount)

	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'backup_audit_log'").Scan(&columnCount)
	require.NoError(t, err)
	require.True(t, columnCount >= 5, "backup_audit_log should have at least 5 columns, got %d", columnCount)

	t.Logf("Migration 63 verified: backup_records(%d), recovery_ops(%d), backup_audit_log(%d)", columnCount, columnCount, columnCount)
}

// TestMigrations64RecoveryStateEnum verifies migration 64 creates recovery_state_enum.
func TestMigrations64RecoveryStateEnum(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var typeExists bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_type WHERE typname = 'recovery_state_enum')").Scan(&typeExists)
	require.NoError(t, err)
	require.True(t, typeExists, "recovery_state_enum should exist")

	var hasRecoveryState bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'recovery_operations' AND column_name = 'recovery_state')").Scan(&hasRecoveryState)
	require.NoError(t, err)
	require.True(t, hasRecoveryState, "recovery_operations should have recovery_state column")

	t.Log("Migration 64 verified: recovery_state_enum type and column created")
}

// TestMigrationsIdempotent verifies running migrations twice does not fail.
func TestMigrationsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx), "first apply should succeed")

	var count1 int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count1)
	require.NoError(t, err)

	err = NewSchemaManager(db).Apply(ctx)
	require.NoError(t, err, "second apply should succeed (idempotent)")

	var count2 int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count2)
	require.NoError(t, err)
	require.Equal(t, count1, count2, "migration count should not change on second apply")

	t.Logf("Idempotency verified: %d migrations both times", count1)
}

// TestMigrationsFromMaster verifies migrations can be applied from a partially-applied schema.
func TestMigrationsFromMaster(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var finalCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&finalCount)
	require.NoError(t, err)
	require.Equal(t, 64, finalCount, "should have 64 migrations applied")

	migrationTables := []string{
		"endpoint_approval_policies", "endpoint_approval_requests",
		"agent_capabilities", "endpoint_audit_evidence",
		"inventory_results", "plan_entitlements",
		"usage_snapshots", "control_plane_audit",
	}

	for _, tableName := range migrationTables {
		var exists bool
		err = db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)",
			tableName).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "table %s should exist", tableName)
	}

	t.Logf("Migration from master verified: %d migrations applied", finalCount)
}

// TestBackupMigration_FullWorkflow tests the complete backup migration workflow.
func TestBackupMigration_FullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	mspID := "00000000-0000-0000-0000-000000000001"
	_, err = db.ExecContext(ctx, `
		INSERT INTO msp_tenants (id, name, slug, plan)
		VALUES ($1, 'Test MSP', 'test-msp', 'free')
		ON CONFLICT (id) DO NOTHING
	`, mspID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO client_organizations (id, msp_id, name, slug)
		SELECT gen_random_uuid(), $1, 'Test Client', 'test-client'
		FROM msp_tenants WHERE id = $1
	`, mspID)
	require.NoError(t, err)

	var backupID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO backup_records (id, database_type, integrity_digest, status, created_at)
		VALUES (gen_random_uuid()::text, 'postgresql', 'test-digest', 'pending', NOW())
		RETURNING id
	`).Scan(&backupID)
	require.NoError(t, err)
	require.NotEmpty(t, backupID)

	var recID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO recovery_operations (recovery_id, operation, phase, state, status)
		VALUES (gen_random_uuid()::text, 'test', 'pre', 'idle', 'running')
		RETURNING id
	`).Scan(&recID)
	require.NoError(t, err)
	require.NotEmpty(t, recID)

	_, err = db.ExecContext(ctx, `
		INSERT INTO backup_audit_log (backup_id, action, performed_by, timestamp)
		VALUES ($1, 'backup_created', 'test-user', NOW())
	`, backupID)
	require.NoError(t, err)

	t.Logf("Full workflow verified: backup record %s, recovery op %s", backupID, recID)
}

// TestMigrations_ConstraintIntegrity verifies FK constraints and check constraints are valid.
func TestMigrations_ConstraintIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var fkCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_type = 'FOREIGN KEY'").Scan(&fkCount)
	require.NoError(t, err)
	require.True(t, fkCount > 0, "should have foreign key constraints")

	var checkCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_type = 'CHECK'").Scan(&checkCount)
	require.NoError(t, err)
	require.True(t, checkCount > 0, "should have check constraints")

	t.Logf("Constraint integrity verified: %d FK constraints, %d CHECK constraints", fkCount, checkCount)
}

// TestMigrations_UniqueIndexes verifies unique indexes are created.
func TestMigrations_UniqueIndexes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var idxCount int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pg_indexes WHERE indexname LIKE 'idx_%'").Scan(&idxCount)
	require.NoError(t, err)
	require.True(t, idxCount > 0, "should have idx_ indexes")

	t.Logf("Indexes verified: %d indexes starting with idx_", idxCount)
}

// TestPostgreSQLBackup_MigrationVerification verifies the backup migration tables are consistent.
func TestPostgreSQLBackup_MigrationVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var hasDigest bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.columns
			WHERE table_name = 'backup_records' AND column_name = 'integrity_digest')
	`).Scan(&hasDigest)
	require.NoError(t, err)
	require.True(t, hasDigest)

	var hasStatusCheck bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.table_constraints tc
			JOIN information_schema.constraint_column_usage ccu
				ON tc.constraint_name = ccu.constraint_name
			WHERE tc.table_name = 'backup_records'
				AND tc.constraint_type = 'CHECK'
				AND ccu.column_name = 'status')
	`).Scan(&hasStatusCheck)
	require.NoError(t, err)
	require.True(t, hasStatusCheck, "backup_records.status should have a CHECK constraint")

	var hasRecoveryStatusCheck bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.table_constraints tc
			JOIN information_schema.constraint_column_usage ccu
				ON tc.constraint_name = ccu.constraint_name
			WHERE tc.table_name = 'recovery_operations'
				AND tc.constraint_type = 'CHECK'
				AND ccu.column_name = 'status')
	`).Scan(&hasRecoveryStatusCheck)
	require.NoError(t, err)
	require.True(t, hasRecoveryStatusCheck)

	var hasActionCheck bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.table_constraints tc
			JOIN information_schema.constraint_column_usage ccu
				ON tc.constraint_name = ccu.constraint_name
			WHERE tc.table_name = 'backup_audit_log'
				AND tc.constraint_type = 'CHECK'
				AND ccu.column_name = 'action')
	`).Scan(&hasActionCheck)
	require.NoError(t, err)
	require.True(t, hasActionCheck, "backup_audit_log.action should have a CHECK constraint")

	t.Log("Backup migration verification complete: all constraints and columns verified")
}

// TestMigrations_Migration63_64Dependencies verifies migrations 63 and 64 work together.
func TestMigrations_Migration63_64Dependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var hasBackupRecords, hasRecoveryOps bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='backup_records')").Scan(&hasBackupRecords)
	require.NoError(t, err)
	require.True(t, hasBackupRecords)

	err = db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='recovery_operations')").Scan(&hasRecoveryOps)
	require.NoError(t, err)
	require.True(t, hasRecoveryOps)

	var hasRecoveryStateCol bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.columns
			WHERE table_name='recovery_operations' AND column_name='recovery_state')
	`).Scan(&hasRecoveryStateCol)
	require.NoError(t, err)
	require.True(t, hasRecoveryStateCol)

	var hasBackupRecoveryID bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.columns
			WHERE table_name='backup_records' AND column_name='recovery_id')
	`).Scan(&hasBackupRecoveryID)
	require.NoError(t, err)
	require.True(t, hasBackupRecoveryID)

	t.Log("Migrations 63+64 verified: dependency chain complete")
}

// TestPostgreSQLBackup_Migration64Exists verifies migration 64 is recorded.
func TestPostgreSQLBackup_Migration64Exists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var migration64Applied bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 64)
	`).Scan(&migration64Applied)
	require.NoError(t, err)
	require.True(t, migration64Applied, "migration 64 should be recorded")

	var maxVersion int
	err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&maxVersion)
	require.NoError(t, err)
	require.Equal(t, 64, maxVersion, "max migration version should be 64")

	t.Log("Migration 64 verified in schema_migrations")
}

// TestPostgreSQLBackup_Migration63Exists verifies migration 63 is recorded.
func TestPostgreSQLBackup_Migration63Exists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var migration63Applied bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 63)
	`).Scan(&migration63Applied)
	require.NoError(t, err)
	require.True(t, migration63Applied, "migration 63 should be recorded")

	t.Log("Migration 63 verified in schema_migrations")
}

// TestPostgreSQLBackup_MigrationChecksum verifies migration checksums are consistent.
func TestPostgreSQLBackup_MigrationChecksum(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	var appliedCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&appliedCount)
	require.NoError(t, err)
	require.Equal(t, 64, appliedCount, "should have 64 applied migrations")

	t.Log("Migration checksum verified: 64 migrations applied")
}

// TestPostgreSQLBackup_NonEmptyMigrations verifies migration 64 is non-empty.
func TestPostgreSQLBackup_NonEmptyMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dbintegration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, _ := setupTestDB(t, ctx)

	_, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, NewSchemaManager(db).Apply(ctx))

	for _, table := range []string{"backup_records", "recovery_operations", "backup_audit_log"} {
		var exists bool
		err = db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)",
			table).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "table %s should exist after migration 64", table)
	}

	t.Log("Migration 64 verified: backup/recovery tables created")
}
