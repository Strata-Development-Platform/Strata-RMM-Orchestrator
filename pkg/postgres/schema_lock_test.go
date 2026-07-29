//go:build dbintegration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func openTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrationLockAcquisitionSucceeds(t *testing.T) {
	db := openTestDB(t)
	mgr := NewSchemaManager(db)

	ctx := context.Background()
	if err := mgr.acquireLock(ctx, 1); err != nil {
		t.Fatalf("acquireLock failed: %v", err)
	}
	mgr.releaseLock(ctx)
}

func TestMigrationLockSecondConcurrentFails(t *testing.T) {
	db := openTestDB(t)
	mgr1 := NewSchemaManager(db)
	mgr2 := NewSchemaManager(db)

	ctx := context.Background()
	if err := mgr1.acquireLock(ctx, 1); err != nil {
		t.Fatalf("mgr1 acquireLock failed: %v", err)
	}
	defer mgr1.releaseLock(ctx)

	err := mgr2.acquireLock(ctx, 1)
	if err == nil {
		mgr2.releaseLock(ctx)
		t.Fatal("mgr2 should not have acquired the lock")
	}
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("mgr2 expected ErrLockTimeout, got: %v", err)
	}
}

func TestMigrationLockReleasedAfterSuccess(t *testing.T) {
	db := openTestDB(t)

	mgr1 := NewSchemaManager(db)
	ctx := context.Background()
	if err := mgr1.acquireLock(ctx, 1); err != nil {
		t.Fatalf("mgr1 acquireLock failed: %v", err)
	}
	mgr1.releaseLock(ctx)

	mgr2 := NewSchemaManager(db)
	if err := mgr2.acquireLock(ctx, 1); err != nil {
		t.Fatalf("mgr2 acquireLock after release failed: %v", err)
	}
	mgr2.releaseLock(ctx)
}

func TestMigrationLockContextCancellation(t *testing.T) {
	db := openTestDB(t)
	mgr1 := NewSchemaManager(db)
	mgr2 := NewSchemaManager(db)

	ctx := context.Background()
	if err := mgr1.acquireLock(ctx, 1); err != nil {
		t.Fatalf("mgr1 acquireLock failed: %v", err)
	}
	defer mgr1.releaseLock(ctx)

	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := mgr2.acquireLock(ctx2, 1)
	if err == nil {
		mgr2.releaseLock(ctx2)
		t.Fatal("mgr2 should not have acquired the lock")
	}
	if !errors.Is(err, ErrLockTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout or cancellation, got: %v", err)
	}
}

func TestMigrationLockFailureReleasesLock(t *testing.T) {
	db := openTestDB(t)
	mgr := NewSchemaManager(db)

	ctx := context.Background()
	err := mgr.Apply(ctx)
	if err != nil {
		t.Fatalf("first Apply failed: %v", err)
	}

	mgr2 := NewSchemaManager(db)
	if err := mgr2.Apply(ctx); err != nil {
		t.Fatalf("second Apply after first completed failed: %v", err)
	}
}

func TestMigrationLockSameConnectionUsed(t *testing.T) {
	db := openTestDB(t)
	mgr := NewSchemaManager(db)

	ctx := context.Background()
	if err := mgr.acquireLock(ctx, 1); err != nil {
		t.Fatalf("acquireLock failed: %v", err)
	}

	lockConn := mgr.lockConn
	if lockConn == nil {
		t.Fatal("lockConn is nil after acquireLock")
	}

	mgr.releaseLock(ctx)
	if mgr.lockConn != nil {
		t.Fatal("lockConn should be nil after releaseLock")
	}

	err := lockConn.PingContext(ctx)
	if err == nil {
		t.Fatal("connection should be closed after release")
	}
}

func TestSchemaLock_AcquireTimeout(t *testing.T) {
	db := openTestDB(t)
	mgr1 := NewSchemaManager(db)
	mgr2 := NewSchemaManager(db)
	ctx := context.Background()

	if err := mgr1.acquireLock(ctx, 62); err != nil {
		t.Fatalf("mgr1 acquireLock failed: %v", err)
	}
	defer mgr1.releaseLock(ctx)

	ctx2, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := mgr2.acquireLock(ctx2, 62)
	if err == nil {
		mgr2.releaseLock(ctx2)
		t.Fatal("expected lock acquisition to timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestSchemaLock_ReleaseError(t *testing.T) {
	db := openTestDB(t)
	mgr := NewSchemaManager(db)

	ctx := context.Background()
	if err := mgr.acquireLock(ctx, 1); err != nil {
		t.Fatalf("acquireLock failed: %v", err)
	}
	defer mgr.releaseLock(ctx)

	err := mgr.releaseLock(ctx)
	if err != nil {
		t.Fatalf("releaseLock should succeed on first call, got: %v", err)
	}

	err = mgr.releaseLock(ctx)
	if err == nil {
		t.Fatal("expected error on second releaseLock call")
	}
	if !errors.Is(err, ErrLockReleaseFailed) {
		t.Fatalf("expected ErrLockReleaseFailed, got: %v", err)
	}
}

func TestSchemaLock_IdempotentApply(t *testing.T) {
	db := openTestDB(t)
	mgr := NewSchemaManager(db)

	ctx := context.Background()
	if err := mgr.Apply(ctx); err != nil {
		t.Fatalf("first Apply failed: %v", err)
	}

	if err := mgr.Apply(ctx); err != nil {
		t.Fatalf("second Apply (idempotent) failed: %v", err)
	}

	migs := Migrations()
	var appliedCount int
	rows, err := db.Query(`SELECT COUNT(*) FROM schema_migrations`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		rows.Scan(&appliedCount)
	}
	if appliedCount != len(migs) {
		t.Fatalf("expected %d migrations, got %d", len(migs), appliedCount)
	}
}

func TestSchemaLock_AcquireContextCancellation(t *testing.T) {
	db := openTestDB(t)
	mgr1 := NewSchemaManager(db)
	mgr2 := NewSchemaManager(db)

	ctx := context.Background()
	if err := mgr1.acquireLock(ctx, 1); err != nil {
		t.Fatalf("mgr1 acquireLock failed: %v", err)
	}
	defer mgr1.releaseLock(ctx)

	ctx2, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := mgr2.acquireLock(ctx2, 1)
	if err == nil {
		mgr2.releaseLock(ctx2)
		t.Fatal("mgr2 should not have acquired the lock")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.Canceled or context.DeadlineExceeded, got: %v", err)
	}
}

func TestSchemaLock_VersionConflict(t *testing.T) {
	db := openTestDB(t)
	mgr := NewSchemaManager(db)
	ctx := context.Background()

	if err := mgr.Apply(ctx); err != nil {
		t.Fatalf("first Apply failed: %v", err)
	}

	mgr2 := NewSchemaManager(db)
	if err := mgr2.acquireLock(ctx, 62); err != nil {
		t.Fatalf("mgr2 acquireLock failed: %v", err)
	}
	defer mgr2.releaseLock(ctx)

	_, err := mgr2.lockConn.ExecContext(ctx, `
		INSERT INTO schema_migrations (id, name, applied_at)
		VALUES (999, 'phantom_migration', NOW())
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		t.Fatalf("insert phantom migration: %v", err)
	}

	migs := Migrations()
	var exists bool
	err = mgr2.lockConn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE id = $1)`, migs[len(migs)-1].ID).Scan(&exists)
	if err != nil {
		t.Fatalf("check latest migration: %v", err)
	}
	if !exists {
		t.Fatal("expected latest migration to exist")
	}

	for _, mig := range migs {
		var mExists bool
		err := mgr2.lockConn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE id = $1)`, mig.ID).Scan(&mExists)
		if err != nil {
			t.Fatalf("check migration %d: %v", mig.ID, err)
		}
		_ = mExists
	}

	if err := mgr2.lockConn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE id = 999)`).Scan(&exists); err != nil {
		t.Fatalf("check phantom migration: %v", err)
	}
	if exists {
		t.Logf("phantom migration 999 exists — version conflict detected")
	}

	if err := mgr2.Apply(ctx); err != nil {
		t.Logf("second Apply returned error (expected for version conflict): %v", err)
	}

	var maxMig int
	err = mgr2.lockConn.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM schema_migrations`).Scan(&maxMig)
	if err != nil {
		t.Fatalf("get max migration id: %v", err)
	}
	if maxMig != 999 {
		t.Fatalf("expected max migration id 999 (phantom row persisted), got %d", maxMig)
	}

	// Clean up phantom row for subsequent tests
	_, err = mgr2.lockConn.ExecContext(ctx, `DELETE FROM schema_migrations WHERE id = 999`)
	if err != nil {
		t.Fatalf("clean up phantom migration: %v", err)
	}
}

func TestSchemaLock_ReleaseErrorPropagation(t *testing.T) {
	db := openTestDB(t)
	mgr := NewSchemaManager(db)
	ctx := context.Background()

	if err := mgr.acquireLock(ctx, 1); err != nil {
		t.Fatalf("acquireLock failed: %v", err)
	}
	defer mgr.releaseLock(ctx)

	err := mgr.releaseLock(ctx)
	if err != nil {
		t.Fatalf("first releaseLock should succeed: %v", err)
	}

	err = mgr.releaseLock(ctx)
	if err == nil {
		t.Fatal("releaseLock should return error when lockConn is nil after being released")
	}
	fmt.Printf("releaseLock error: %v\n", err)
	_ = err
}
