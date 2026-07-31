//go:build integration

package backup

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

func TestConcurrentRecoveryOperationLock(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is required")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, postgres.NewSchemaManager(db).Apply(context.Background()))

	first, err := NewPostgresOperationLock(db, postgres.DefaultRecoveryLockID)
	require.NoError(t, err)
	second, err := NewPostgresOperationLock(db, postgres.DefaultRecoveryLockID)
	require.NoError(t, err)
	require.NoError(t, first.Acquire(context.Background()))
	t.Cleanup(func() { _ = first.Release(context.Background()) })

	blockedCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, second.Acquire(blockedCtx), context.DeadlineExceeded)
	require.NoError(t, first.Release(context.Background()))

	acquireCtx, acquireCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer acquireCancel()
	require.NoError(t, second.Acquire(acquireCtx))
	require.NoError(t, second.Release(context.Background()))
}

func TestQuiescingPersistsAndOwnershipIsEnforced(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is required")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, postgres.NewSchemaManager(db).Apply(context.Background()))
	_, err = db.Exec(`UPDATE recovery_mutation_gate SET quiesced = FALSE, operation_id = NULL WHERE singleton = TRUE`)
	require.NoError(t, err)

	first, err := NewPostgresQuiescer(db, "operation-a")
	require.NoError(t, err)
	second, err := NewPostgresQuiescer(db, "operation-b")
	require.NoError(t, err)
	require.NoError(t, first.Quiesce(context.Background()))
	t.Cleanup(func() { _ = first.Resume(context.Background()) })
	status, err := first.Status(context.Background())
	require.NoError(t, err)
	require.True(t, status.Quiesced)
	_, err = db.Exec(`
		INSERT INTO backup_records (id, integrity_digest)
		VALUES ('blocked-while-quiesced', 'digest')
	`)
	require.Error(t, err, "database trigger must reject mutations outside API and dispatcher paths")
	require.Error(t, second.Quiesce(context.Background()))
	require.Error(t, second.Resume(context.Background()))
	require.NoError(t, first.Resume(context.Background()))
	status, err = first.Status(context.Background())
	require.NoError(t, err)
	require.False(t, status.Quiesced)
	_, err = db.Exec(`
		INSERT INTO backup_records (id, integrity_digest)
		VALUES ('allowed-after-resume', 'digest')
		ON CONFLICT (id) DO NOTHING
	`)
	require.NoError(t, err)
	_, _ = db.Exec(`DELETE FROM backup_records WHERE id = 'allowed-after-resume'`)
}
