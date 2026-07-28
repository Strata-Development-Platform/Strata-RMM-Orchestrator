//go:build dbintegration

package postgres

import (
	"context"
	"database/sql"
	"errors"
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
	if err := mgr.acquireLock(ctx); err != nil {
		t.Fatalf("acquireLock failed: %v", err)
	}
	mgr.releaseLock()
}

func TestMigrationLockSecondConcurrentFails(t *testing.T) {
	db := openTestDB(t)
	mgr1 := NewSchemaManager(db)
	mgr2 := NewSchemaManager(db)

	ctx := context.Background()
	if err := mgr1.acquireLock(ctx); err != nil {
		t.Fatalf("mgr1 acquireLock failed: %v", err)
	}
	defer mgr1.releaseLock()

	err := mgr2.acquireLock(ctx)
	if err == nil {
		mgr2.releaseLock()
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
	if err := mgr1.acquireLock(ctx); err != nil {
		t.Fatalf("mgr1 acquireLock failed: %v", err)
	}
	mgr1.releaseLock()

	mgr2 := NewSchemaManager(db)
	if err := mgr2.acquireLock(ctx); err != nil {
		t.Fatalf("mgr2 acquireLock after release failed: %v", err)
	}
	mgr2.releaseLock()
}

func TestMigrationLockContextCancellation(t *testing.T) {
	db := openTestDB(t)
	mgr1 := NewSchemaManager(db)
	mgr2 := NewSchemaManager(db)

	ctx := context.Background()
	if err := mgr1.acquireLock(ctx); err != nil {
		t.Fatalf("mgr1 acquireLock failed: %v", err)
	}
	defer mgr1.releaseLock()

	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := mgr2.acquireLock(ctx2)
	if err == nil {
		mgr2.releaseLock()
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
	if err := mgr.acquireLock(ctx); err != nil {
		t.Fatalf("acquireLock failed: %v", err)
	}

	lockConn := mgr.lockConn
	if lockConn == nil {
		t.Fatal("lockConn is nil after acquireLock")
	}

	mgr.releaseLock()
	if mgr.lockConn != nil {
		t.Fatal("lockConn should be nil after releaseLock")
	}

	err := lockConn.PingContext(ctx)
	if err == nil {
		t.Fatal("connection should be closed after release")
	}
}
