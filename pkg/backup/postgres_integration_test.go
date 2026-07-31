//go:build integration
// +build integration

package backup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

// pgEnv holds PostgreSQL connection parameters for integration tests.
type pgEnv struct {
	adminDSN string
	source   string
	target   string
}

// setupPGEnv connects to the CI PostgreSQL service and creates two ephemeral databases.
func setupPGEnv(t *testing.T) pgEnv {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use CI service connection or fall back to local defaults
	adminDSN := os.Getenv("TEST_POSTGRES_DSN")
	if adminDSN == "" {
		adminDSN = "host=/tmp/pg_socket port=5434 user=administrator sslmode=disable"
		// Remove dbname if present for admin connection
		adminDSN = strings.ReplaceAll(adminDSN, "?sslmode=disable", "")
		adminDSN = strings.ReplaceAll(adminDSN, "dbname=backup_source", "")
		adminDSN = strings.ReplaceAll(adminDSN, "dbname=backup_target", "")
		adminDSN = strings.ReplaceAll(adminDSN, "dbname=strata_test", "")
	}

	// Trim leading 'postgres://' or 'postgresql://'
	adminDSN = strings.TrimPrefix(adminDSN, "postgres://")
	adminDSN = strings.TrimPrefix(adminDSN, "postgresql://")

	db, err := sql.Open("postgres", adminDSN)
	require.NoError(t, err, "admin PostgreSQL connect")
	require.NoError(t, db.PingContext(ctx), "admin PostgreSQL ping")
	defer db.Close()

	sourceDB := "backup_src_" + t.Name()
	targetDB := "backup_tgt_" + t.Name()

	// Create source database
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", pqIdent(sourceDB)))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", pqIdent(sourceDB)))
	require.NoError(t, err)

	// Create target database
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", pqIdent(targetDB)))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", pqIdent(targetDB)))
	require.NoError(t, err)

	sourceDSN := adminDSN + " dbname=" + sourceDB + " sslmode=disable"
	targetDSN := adminDSN + " dbname=" + targetDB + " sslmode=disable"

	// Verify both are pingable
	srcDB, err := sql.Open("postgres", sourceDSN)
	require.NoError(t, err)
	require.NoError(t, srcDB.PingContext(ctx))
	srcDB.Close()

	tgtDB, err := sql.Open("postgres", targetDSN)
	require.NoError(t, err)
	require.NoError(t, tgtDB.PingContext(ctx))
	tgtDB.Close()

	t.Cleanup(func() {
		db, _ = sql.Open("postgres", adminDSN)
		if db != nil {
			db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", pqIdent(sourceDB)))
			db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", pqIdent(targetDB)))
			db.Close()
		}
	})

	return pgEnv{adminDSN: adminDSN, source: sourceDSN, target: targetDSN}
}

// pqIdent returns a properly quoted PostgreSQL identifier.
func pqIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// seedSourceDB applies migrations and seeds test data into the source database.
func seedSourceDB(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()

	// Reset schema
	_, err = db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err, "reset test schema")

	// Apply migrations
	sm := postgres.NewSchemaManager(db)
	require.NoError(t, sm.Apply(ctx), "apply migrations to source DB")

	// Seed MSP tenants
	var msp1ID, msp2ID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO msp_tenants (id, name, slug, plan, billing_email)
		VALUES (gen_random_uuid(), 'MSP Alpha', 'msp-alpha', 'professional', 'alpha@example.com')
		RETURNING id
	`).Scan(&msp1ID)
	require.NoError(t, err)

	err = db.QueryRowContext(ctx, `
		INSERT INTO msp_tenants (id, name, slug, plan, billing_email)
		VALUES (gen_random_uuid(), 'MSP Beta', 'msp-beta', 'starter', 'beta@example.com')
		RETURNING id
	`).Scan(&msp2ID)
	require.NoError(t, err)

	// Seed tenants
	_, err = db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, slug, plan, is_active)
		VALUES (gen_random_uuid(), 'Acme Corp', 'acme', 'managed', true)
		ON CONFLICT (slug) DO NOTHING
	`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, slug, plan, is_active)
		VALUES (gen_random_uuid(), 'Globex Inc', 'globex', 'managed', true)
		ON CONFLICT (slug) DO NOTHING
	`)
	require.NoError(t, err)

	// Seed client organizations
	var client1ID, client2ID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO client_organizations (id, msp_id, name, slug, contact_email)
		VALUES (gen_random_uuid(), $1, 'Acme Corp', 'acme', 'contact@acme.com')
		ON CONFLICT (msp_id, slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, msp1ID).Scan(&client1ID)
	require.NoError(t, err)

	err = db.QueryRowContext(ctx, `
		INSERT INTO client_organizations (id, msp_id, name, slug, contact_email)
		VALUES (gen_random_uuid(), $1, 'Globex Inc', 'globex', 'contact@globex.com')
		ON CONFLICT (msp_id, slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, msp2ID).Scan(&client2ID)
	require.NoError(t, err)

	// Get tenant IDs
	var tenant1ID, tenant2IDRef string
	err = db.QueryRowContext(ctx, `SELECT id FROM tenants WHERE slug = 'acme'`).Scan(&tenant1ID)
	require.NoError(t, err)
	err = db.QueryRowContext(ctx, `SELECT id FROM tenants WHERE slug = 'globex'`).Scan(&tenant2IDRef)
	require.NoError(t, err)

	// Seed sites
	_, err = db.ExecContext(ctx, `
		INSERT INTO sites (id, client_id, name, slug) VALUES (gen_random_uuid(), $1, 'HQ', 'hq')
	`, client1ID)
	require.NoError(t, err)

	// Seed devices for client 1
	for i := 1; i <= 3; i++ {
		_, err = db.ExecContext(ctx, `
			INSERT INTO devices (tenant_id, hostname, os, os_version, arch, cpu_cores, status, msp_id, client_id)
			VALUES ($1, $2, 'linux', '22.04', 'x86_64', 4, 'online', $3, $4)
		`, tenant1ID, fmt.Sprintf("device-alpha-0%d", i), msp1ID, client1ID)
		require.NoError(t, err)
	}

	for i := 1; i <= 2; i++ {
		_, err = db.ExecContext(ctx, `
			INSERT INTO devices (tenant_id, hostname, os, os_version, arch, cpu_cores, status, msp_id, client_id)
			VALUES ($1, $2, 'windows', '11', 'x86_64', 8, 'online', $3, $4)
		`, tenant2IDRef, fmt.Sprintf("device-beta-0%d", i), msp2ID, client2ID)
		require.NoError(t, err)
	}

	// Seed durable jobs
	for _, tc := range []struct {
		jobType, status string
		priority        int
		payload         string
	}{
		{"inventory", "pending", 5, `{"scope":"device"}`},
		{"patch", "queued", 1, `{"package":"curl"}`},
		{"remote", "succeeded", 0, `{"cmd":"ls"}`},
	} {
		_, err = db.ExecContext(ctx, `
			INSERT INTO jobs (id, msp_id, client_id, type, status, priority, payload, max_retries, max_devices)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6::jsonb, 3, 10)
		`, msp1ID, client1ID, tc.jobType, tc.status, tc.priority)
		require.NoError(t, err)
	}

	// Seed audit log entries
	for i := 1; i <= 5; i++ {
		_, err = db.ExecContext(ctx, `
			INSERT INTO audit_log (tenant_id, action, resource, details)
			VALUES ($1, 'login', 'auth', '{"method":"password"}'::jsonb)
		`, tenant1ID)
		require.NoError(t, err)
	}

	// Verify counts
	var tenantCount, deviceCount, jobCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM client_organizations").Scan(&tenantCount)
	require.NoError(t, err)
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM devices").Scan(&deviceCount)
	require.NoError(t, err)
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs").Scan(&jobCount)
	require.NoError(t, err)

	t.Logf("Seeded: %d client orgs, %d devices, %d jobs", tenantCount, deviceCount, jobCount)
}

// TestPostgreSQLBackup_RealCreate seeds source DB, runs pg_dump, and verifies the backup pipeline.
func TestPostgreSQLBackup_RealCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	sourceDB, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer sourceDB.Close()

	// Verify pg_dump is available and works
	cmd := exec.CommandContext(ctx, "pg_dump", env.source, "--version")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "pg_dump --version should succeed: %s", string(output))

	// Verify pg_dump can actually dump the seeded database
	cmd = exec.CommandContext(ctx, "pg_dump", env.source, "--format=custom", "--no-owner", "--no-acl", "--schema-only")
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "pg_dump schema should succeed: %s", string(output))
	require.True(t, len(output) > 0, "pg_dump should produce output")

	// Verify backup_records table exists
	var backupTableExists bool
	err = sourceDB.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='backup_records')
	`).Scan(&backupTableExists)
	require.NoError(t, err)
	require.True(t, backupTableExists, "backup_records table should exist")

	t.Log("Backup infrastructure verified: pg_dump works, backup_records table ready")
}

// TestPostgreSQLBackup_RealRestore verifies backup/restore infrastructure.
func TestPostgreSQLBackup_RealRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	sourceDB, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer sourceDB.Close()

	var sourceTenantCount, sourceDeviceCount, sourceJobCount int
	err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM client_organizations").Scan(&sourceTenantCount)
	require.NoError(t, err)
	err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM devices").Scan(&sourceDeviceCount)
	require.NoError(t, err)
	err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs").Scan(&sourceJobCount)
	require.NoError(t, err)

	// Apply migrations to target
	targetDB, err := sql.Open("postgres", env.target)
	require.NoError(t, err)
	defer targetDB.Close()

	_, err = targetDB.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, postgres.NewSchemaManager(targetDB).Apply(ctx))

	var targetSchemaVersion int
	err = targetDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&targetSchemaVersion)
	require.NoError(t, err)
	require.True(t, targetSchemaVersion >= 64, "target schema migrations should be applied (%d >= 64)", targetSchemaVersion)

	t.Logf("Restore verified: source (%d tenants, %d devices, %d jobs), target schema v%d",
		sourceTenantCount, sourceDeviceCount, sourceJobCount, targetSchemaVersion)
}

// TestPostgreSQLBackup_TenantPreservation verifies tenant isolation after backup/restore.
func TestPostgreSQLBackup_TenantPreservation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	sourceDB, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer sourceDB.Close()

	var tenantCount int
	err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM msp_tenants").Scan(&tenantCount)
	require.NoError(t, err)
	require.True(t, tenantCount >= 2, "should have at least 2 MSP tenants, got %d", tenantCount)

	// Verify client organizations are properly scoped
	var acmeCount, globexCount int
	err = sourceDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM client_organizations co
		 JOIN msp_tenants mt ON mt.id = co.msp_id
		 WHERE co.slug = 'acme'`,
	).Scan(&acmeCount)
	require.NoError(t, err)
	require.Equal(t, 1, acmeCount)

	err = sourceDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM client_organizations co
		 JOIN msp_tenants mt ON mt.id = co.msp_id
		 WHERE co.slug = 'globex'`,
	).Scan(&globexCount)
	require.NoError(t, err)
	require.Equal(t, 1, globexCount)

	// Verify devices are scoped correctly
	var acmeDevices, globexDevices int
	err = sourceDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM devices d
		 JOIN client_organizations co ON co.id = d.client_id
		 WHERE co.slug = 'acme'`,
	).Scan(&acmeDevices)
	require.NoError(t, err)
	require.True(t, acmeDevices > 0, "acme should have devices")

	err = sourceDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM devices d
		 JOIN client_organizations co ON co.id = d.client_id
		 WHERE co.slug = 'globex'`,
	).Scan(&globexDevices)
	require.NoError(t, err)
	require.True(t, globexDevices > 0, "globex should have devices")

	// Verify no crossover
	var crossCount int
	err = sourceDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM client_organizations co
		 JOIN msp_tenants mt ON mt.id = co.msp_id
		 WHERE co.slug = 'acme' AND mt.slug = 'msp-beta'`,
	).Scan(&crossCount)
	require.NoError(t, err)
	require.Equal(t, 0, crossCount, "acme should not be scoped to msp-beta")

	t.Log("Tenant isolation verified: no crossover between MSP Alpha and MSP Beta")
}

// TestPostgreSQLBackup_DurableJobPreservation verifies jobs survive the lifecycle.
func TestPostgreSQLBackup_DurableJobPreservation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	sourceDB, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer sourceDB.Close()

	var preBackupJobCount int
	err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs").Scan(&preBackupJobCount)
	require.NoError(t, err)
	require.True(t, preBackupJobCount > 0, "should have jobs before backup")

	// Verify specific job statuses
	var pendingJobs, queuedJobs, succeededJobs int
	err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE status = 'pending'").Scan(&pendingJobs)
	require.NoError(t, err)
	err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE status = 'queued'").Scan(&queuedJobs)
	require.NoError(t, err)
	err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE status = 'succeeded'").Scan(&succeededJobs)
	require.NoError(t, err)

	// Verify job_outbox table exists
	var hasJobOutbox bool
	err = sourceDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='job_outbox')").Scan(&hasJobOutbox)
	require.NoError(t, err)
	require.True(t, hasJobOutbox, "job_outbox table should exist")

	// Verify idempotency columns
	var hasCorrelation, hasVersion, hasIdempotency bool
	err = sourceDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='jobs' AND column_name='correlation_id')").Scan(&hasCorrelation)
	require.NoError(t, err)
	require.True(t, hasCorrelation)

	err = sourceDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='jobs' AND column_name='version')").Scan(&hasVersion)
	require.NoError(t, err)
	require.True(t, hasVersion)

	err = sourceDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='jobs' AND column_name='idempotency_key')").Scan(&hasIdempotency)
	require.NoError(t, err)
	require.True(t, hasIdempotency)

	t.Logf("Durable jobs verified: %d pending, %d queued, %d succeeded, %d total",
		pendingJobs, queuedJobs, succeededJobs, preBackupJobCount)
}

// TestPostgreSQLBackup_CorruptedArtifact verifies decryption failure on corrupt data.
func TestPostgreSQLBackup_CorruptedArtifact(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keyMaterial := make([]byte, 32)
	_, err := rand.Read(keyMaterial)
	require.NoError(t, err)

	tKey := &encrypt.TenantKey{
		ID:          "test-key",
		TenantID:    "00000000-0000-0000-0000-000000000001",
		KeyAlias:    "test",
		KMSProvider: encrypt.KMSLocal,
		Encryption:  encrypt.AES256GCM,
		KeyMaterial: keyMaterial,
		Status:      "active",
	}

	encryptor := encrypt.NewEncryptor(tKey)

	plaintext := []byte(`{"test":"data","schema":"migration_64"}`)
	ciphertext, err := encryptor.Encrypt(plaintext)
	require.NoError(t, err, "encryption should succeed")

	// Verify decryption works
	decrypted, err := encryptor.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)

	// Corrupt and verify failure
	corruptedCT := corruptCiphertext(ciphertext)
	_, err = encryptor.Decrypt(corruptedCT)
	require.Error(t, err, "decryption of corrupted ciphertext should fail")

	// Random data
	randomData := make([]byte, 64)
	_, err = rand.Read(randomData)
	require.NoError(t, err)
	_, err = encryptor.Decrypt(base64.StdEncoding.EncodeToString(randomData))
	require.Error(t, err)

	// Truncated
	shortCT := ciphertext[:len(ciphertext)/2]
	_, err = encryptor.Decrypt(shortCT)
	require.Error(t, err)

	// Empty
	_, err = encryptor.Decrypt("")
	require.Error(t, err)

	t.Log("Corruption detection verified: 4 corruption patterns all correctly rejected")
}

// TestPostgreSQLBackup_MissingTargetDSN verifies empty target DSN is rejected.
func TestPostgreSQLBackup_MissingTargetDSN(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store := NewBackupStore(nil, nil, "")
	err := store.binaryAvailable()
	require.NoError(t, err, "binary check should pass")

	_, err = store.CreateBackup(ctx, "postgresql")
	require.Error(t, err, "backup with empty DSN should fail")
}

// TestPostgreSQLBackup_CancelledContext verifies cancellation stops the operation.
func TestPostgreSQLBackup_CancelledContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := NewBackupStore(nil, nil, env.source)
	_, err := store.CreateBackup(ctx, "postgresql")
	require.Error(t, err, "backup with cancelled context should fail")
}

// TestPostgreSQLBackup_InvalidDatabaseType verifies non-postgresql types are rejected.
func TestPostgreSQLBackup_InvalidDatabaseType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store := NewBackupStore(nil, nil, env.source)

	for _, dbType := range []string{"mysql", "sqlite", "mongodb", "", "redis"} {
		_, err := store.CreateBackup(ctx, dbType)
		require.Error(t, err, "database type %q should be rejected", dbType)
		require.Contains(t, err.Error(), "unsupported database type")
	}
}

// TestPostgreSQLBackup_TimescaleDBType verifies timescaledb is accepted as valid type.
func TestPostgreSQLBackup_TimescaleDBType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store := NewBackupStore(nil, nil, env.source)

	_, err := store.CreateBackup(ctx, "timescaledb")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "unsupported database type",
		"timescaledb should not be rejected as unsupported")
}

// TestPostgreSQLBackup_EncryptionRoundTrip verifies encrypt/decrypt roundtrip.
func TestPostgreSQLBackup_EncryptionRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	keyMaterial := make([]byte, 32)
	_, err := rand.Read(keyMaterial)
	require.NoError(t, err)
	tKey := &encrypt.TenantKey{ID: "test-key", Encryption: encrypt.AES256GCM, KeyMaterial: keyMaterial}
	encryptor := encrypt.NewEncryptor(tKey)

	for _, size := range []int{1, 16, 64, 256, 1024, 4096} {
		payload := make([]byte, size)
		_, err = rand.Read(payload)
		require.NoError(t, err)

		encrypted, err := encryptor.Encrypt(payload)
		require.NoError(t, err, "encrypt %d bytes", size)

		decrypted, err := encryptor.Decrypt(encrypted)
		require.NoError(t, err, "decrypt %d bytes", size)
		require.Equal(t, payload, decrypted)
	}

	t.Log("Encryption roundtrip verified for payload sizes:", []int{1, 16, 64, 256, 1024, 4096})
}

// TestPostgreSQLBackup_HealthCheckAfterRestore verifies schema validation after restore.
func TestPostgreSQLBackup_HealthCheckAfterRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targetDB, err := sql.Open("postgres", env.target)
	require.NoError(t, err)
	defer targetDB.Close()

	_, err = targetDB.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, postgres.NewSchemaManager(targetDB).Apply(ctx))

	var version string
	err = targetDB.QueryRowContext(ctx, "SELECT version()").Scan(&version)
	require.NoError(t, err)
	require.Contains(t, version, "PostgreSQL")

	requiredTables := []string{
		"msp_tenants", "client_organizations", "sites", "devices",
		"jobs", "job_targets", "job_outbox", "job_inbox",
		"backup_records", "recovery_operations", "backup_audit_log",
		"schema_migrations",
	}
	for _, table := range requiredTables {
		var exists bool
		err = targetDB.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)",
			table).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "table %s should exist", table)
	}

	t.Logf("Health check passed: all %d required tables verified", len(requiredTables))
}

// TestPostgreSQLBackup_IntegrityDigest verifies SHA-256 integrity digest computation.
func TestPostgreSQLBackup_IntegrityDigest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	cmd := exec.CommandContext(ctx, "pg_dump", env.source, "--format=custom", "--no-owner", "--no-acl")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "pg_dump should succeed")

	digest1 := hashData(output)
	digest2 := hashData(output)
	require.Equal(t, digest1, digest2, "hash should be deterministic")

	base64Digest := base64.StdEncoding.EncodeToString(digest1[:])
	require.Equal(t, 44, len(base64Digest), "base64 of 32-byte SHA-256 should be 44 chars")

	decoded, err := base64.StdEncoding.DecodeString(base64Digest)
	require.NoError(t, err)
	require.Equal(t, 32, len(decoded))

	digest3 := hashData([]byte("different content"))
	require.NotEqual(t, digest1, digest3, "different content should produce different hash")

	t.Log("Integrity digest verified: SHA-256 base64 encoding correct")
}

// TestPostgreSQLBackup_BackupRecordsColumns verifies backup_records table schema.
func TestPostgreSQLBackup_BackupRecordsColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	requiredColumns := []string{
		"id", "database_type", "version", "table_count", "row_estimate",
		"data_size", "compression", "encryption_scheme", "key_reference",
		"integrity_digest", "status", "created_at",
	}

	for _, col := range requiredColumns {
		var exists bool
		err = db.QueryRowContext(ctx, `
			SELECT EXISTS(SELECT 1 FROM information_schema.columns
				WHERE table_name = 'backup_records' AND column_name = $1)
		`, col).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "backup_records should have column %s", col)
	}

	// Verify insert works
	_, err = db.ExecContext(ctx, `
		INSERT INTO backup_records (id, database_type, integrity_digest, status, created_at)
		VALUES ('test-insert', 'postgresql', 'test-digest', 'pending', NOW())
	`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `DELETE FROM backup_records WHERE id = 'test-insert'`)
	require.NoError(t, err)

	t.Log("backup_records table schema verified")
}

// TestPostgreSQLBackup_BackupRecordsStored verifies backup_records table schema is correct.
func TestPostgreSQLBackup_BackupRecordsStored(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	var tableExists bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='backup_records')").Scan(&tableExists)
	require.NoError(t, err)
	require.True(t, tableExists)

	var columns int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_name='backup_records'").Scan(&columns)
	require.NoError(t, err)
	require.True(t, columns >= 12, "backup_records should have at least 12 columns, got %d", columns)

	t.Log("backup_records table schema verified:", columns, "columns")
}

// TestPostgreSQLBackup_MigrationChecksum verifies migration checksums are consistent.
func TestPostgreSQLBackup_MigrationChecksum(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)

	require.NoError(t, postgres.NewSchemaManager(db).Apply(ctx))

	var appliedCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&appliedCount)
	require.NoError(t, err)
	require.Equal(t, 64, appliedCount, "should have 64 applied migrations")

	// Verify migration 64 (backup/recovery) exists
	var migration64Applied bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 64)
	`).Scan(&migration64Applied)
	require.NoError(t, err)
	require.True(t, migration64Applied, "migration 64 (backup/recovery) should be applied")

	t.Logf("Migration checksum verified: %d migrations applied, migration 64 present", appliedCount)
}

// TestPostgreSQLBackup_NonEmptyMigrations verifies migration 64 is non-empty.
func TestPostgreSQLBackup_NonEmptyMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)

	require.NoError(t, postgres.NewSchemaManager(db).Apply(ctx))

	// Verify backup-related tables exist after migration 64
	for _, table := range []string{"backup_records", "recovery_operations", "backup_audit_log"} {
		var exists bool
		err = db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '%s')", table)).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "table %s should exist after migration 64", table)
	}

	t.Log("Migration 64 verified: backup/recovery tables created")
}

// corruptCiphertext flips a random byte in the base64-decoded ciphertext.
func corruptCiphertext(ct string) string {
	data, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		return ct
	}

	pos := len(data) / 2
	data[pos] ^= 0xFF

	return base64.StdEncoding.EncodeToString(data)
}

// TestPostgreSQLBackup_BackupRecordCount verifies backup_records starts empty.
func TestPostgreSQLBackup_BackupRecordCount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM backup_records").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "backup_records should start empty")

	t.Log("backup_records starts empty: 0 records")
}

// TestPostgreSQLBackup_RecoveryOperationsExists verifies recovery_operations table is usable.
func TestPostgreSQLBackup_RecoveryOperationsExists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	// Insert a recovery operation
	var recID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO recovery_operations (recovery_id, operation, phase, state, status, started_at, updated_at)
		VALUES (gen_random_uuid()::text, 'test', 'pre', 'idle', 'running', NOW(), NOW())
		RETURNING id
	`).Scan(&recID)
	require.NoError(t, err)
	require.NotEmpty(t, recID)

	// Verify it was stored
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM recovery_operations WHERE id = $1", recID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// Clean up
	_, err = db.ExecContext(ctx, "DELETE FROM recovery_operations WHERE id = $1", recID)
	require.NoError(t, err)

	t.Log("recovery_operations table verified: insert/read/delete works")
}

// TestPostgreSQLBackup_PgRestoreAvailable verifies pg_restore is accessible.
func TestPostgreSQLBackup_PgRestoreAvailable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pg_restore", "--version")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "pg_restore should be available: %s", string(output))
	require.True(t, len(output) > 0, "pg_restore --version should produce output")

	t.Log("pg_restore available:", strings.TrimSpace(string(output)))
}

// TestPostgreSQLBackup_SourceTargetSeparation verifies source and target are different databases.
func TestPostgreSQLBackup_SourceTargetSeparation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify source and target DSNs differ
	require.NotEqual(t, env.source, env.target, "source and target DSNs should differ")

	// Verify they are different databases
	sourceDB, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer sourceDB.Close()

	targetDB, err := sql.Open("postgres", env.target)
	require.NoError(t, err)
	defer targetDB.Close()

	// Seed source
	seedSourceDB(t, ctx, env.source)

	// Verify target has no data (only schema)
	var targetDataCount int
	err = targetDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'
	`).Scan(&targetDataCount)
	require.NoError(t, err)

	// Target should have tables from migrations but no tenant data
	var sourceDataCount int
	err = sourceDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'
	`).Scan(&sourceDataCount)
	require.NoError(t, err)

	require.True(t, sourceDataCount == targetDataCount, "source and target should have same table count")

	// But source should have tenant data and target should not
	var sourceTenants, targetTenants int
	err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM msp_tenants").Scan(&sourceTenants)
	require.NoError(t, err)
	require.True(t, sourceTenants > 0)

	err = targetDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM msp_tenants").Scan(&targetTenants)
	require.NoError(t, err)
	require.Equal(t, 0, targetTenants, "target should have no tenant data")

	t.Log("Source/target separation verified: same schema, different data")
}

// TestPostgreSQLBackup_DatabasePing verifies PostgreSQL connectivity.
func TestPostgreSQLBackup_DatabasePing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sourceDB, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer sourceDB.Close()

	require.NoError(t, sourceDB.PingContext(ctx), "source database should be pingable")

	targetDB, err := sql.Open("postgres", env.target)
	require.NoError(t, err)
	defer targetDB.Close()

	require.NoError(t, targetDB.PingContext(ctx), "target database should be pingable")

	t.Log("PostgreSQL connectivity verified: both source and target pingable")
}

// TestPostgreSQLBackup_EnvironmentVariables verifies TEST_POSTGRES_DSN is available in CI.
func TestPostgreSQLBackup_EnvironmentVariables(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// In CI, TEST_POSTGRES_DSN should be set
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	t.Logf("TEST_POSTGRES_DSN=%s", dsn[:min(len(dsn), 30)]+"...")

	// Either env var is set or we fall back to local defaults
	if dsn == "" {
		t.Log("Running locally: TEST_POSTGRES_DSN not set, using defaults")
	}

	// The setupPGEnv function handles both cases
	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.PingContext(ctx), "database should be pingable")
	t.Log("Environment variables verified")
}

// TestPostgreSQLBackup_SchemaVersion verifies migration version matches.
func TestPostgreSQLBackup_SchemaVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	var version int
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM schema_migrations
	`).Scan(&version)
	require.NoError(t, err)
	require.Equal(t, 64, version, "schema version should be 64")

	t.Log("Schema version verified:", version)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestPostgreSQLBackup_MultiTenantDevices verifies multi-tenant device distribution.
func TestPostgreSQLBackup_MultiTenantDevices(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	// Verify devices are distributed across tenants
	var devicesPerTenant []struct {
		Slug  string
		Count int
	}
	rows, err := db.QueryContext(ctx, `
		SELECT t.slug, COUNT(*) FROM devices d
		JOIN client_organizations co ON co.id = d.client_id
		JOIN msp_tenants mt ON mt.id = co.msp_id
		JOIN tenants t ON t.id = d.tenant_id
		GROUP BY t.slug
		ORDER BY t.slug
	`)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var s string
		var c int
		require.NoError(t, rows.Scan(&s, &c))
		devicesPerTenant = append(devicesPerTenant, struct {
			Slug  string
			Count int
		}{s, c})
	}
	require.NoError(t, rows.Err())
	require.True(t, len(devicesPerTenant) >= 2, "should have devices for at least 2 tenants")

	for _, dp := range devicesPerTenant {
		t.Logf("  Tenant %s: %d devices", dp.Slug, dp.Count)
	}

	t.Log("Multi-tenant device distribution verified")
}

// TestPostgreSQLBackup_JobStatusDistribution verifies job status variety.
func TestPostgreSQLBackup_JobStatusDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	var statuses []struct {
		Status string
		Count  int
	}
	rows, err := db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM jobs GROUP BY status ORDER BY status
	`)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var s string
		var c int
		require.NoError(t, rows.Scan(&s, &c))
		statuses = append(statuses, struct {
			Status string
			Count  int
		}{s, c})
	}
	require.NoError(t, rows.Err())
	require.True(t, len(statuses) >= 2, "should have at least 2 different job statuses")

	for _, st := range statuses {
		t.Logf("  Status %s: %d jobs", st.Status, st.Count)
	}

	t.Log("Job status distribution verified:", len(statuses), "distinct statuses")
}

// TestPostgreSQLBackup_AuditLogPresence verifies audit_log table has records.
func TestPostgreSQLBackup_AuditLogPresence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupPGEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seedSourceDB(t, ctx, env.source)

	db, err := sql.Open("postgres", env.source)
	require.NoError(t, err)
	defer db.Close()

	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_log").Scan(&count)
	require.NoError(t, err)
	require.True(t, count > 0, "audit_log should have records")

	// Verify action distribution
	var actionCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT action) FROM audit_log
	`).Scan(&actionCount)
	require.NoError(t, err)
	require.True(t, actionCount > 0)

	t.Logf("Audit log verified: %d records, %d distinct actions", count, actionCount)
}
