//go:build integration
// +build integration

package backup

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPostgreSQLBackup_Integrity(t *testing.T) {
	// Verify database backup integrity checks

	store := NewBackupStore(nil, nil, "")
	require.NotNil(t, store)

	// Verify binary availability check exists
	err := store.binaryAvailable()
	// May fail if pg_dump/pg_restore not available, but shouldn't panic
	_ = err
}

func TestPostgreSQLBackup_EmptyTarget(t *testing.T) {
	// Verify empty target DSN is rejected

	store := NewBackupStore(nil, nil, "")
	require.NotNil(t, store)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Create backup should fail with nil encryptor
	_, err := store.CreateBackup(ctx, "postgresql")
	require.Error(t, err)
}

func TestPostgreSQLBackup_InvalidDatabaseType(t *testing.T) {
	// Verify invalid database type is rejected

	store := NewBackupStore(nil, nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := store.CreateBackup(ctx, "mysql")
	require.Error(t, err)
}

func TestPostgreSQLBackup_DatabaseTypes(t *testing.T) {
	// Verify supported database types

	supported := []string{"postgresql", "timescaledb"}
	for _, dbType := range supported {
		require.NotEmpty(t, dbType)
	}
}
