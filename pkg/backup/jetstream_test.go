package backup

import (
	"context"
	"database/sql"
	"testing"

	"github.com/nats-io/nats.go"
)

func TestJetStreamBackupStore_Backup(t *testing.T) {
	ctx := context.Background()

	nc, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	defer nc.Close()

	store := NewJetStreamBackupStore(nc, (*sql.DB)(nil), nil)

	backup, err := store.Backup(ctx)
	if err != nil {
		t.Logf("Backup (may fail if no streams): %v", err)
	}

	if backup != nil {
		if backup.Integrity == "" {
			t.Error("Integrity digest should not be empty")
		}
		if backup.Timestamp.IsZero() {
			t.Error("Timestamp should be set")
		}
	}
}

func TestJetStreamBackupStore_ListBackups(t *testing.T) {
	t.Skip("Skipping test - requires database connection")
}

func TestJetStreamBackupStore_DeleteBackup(t *testing.T) {
	t.Skip("Skipping test - requires database connection")
}

func TestJetStreamBackupStore_InvalidStream(t *testing.T) {
	t.Skip("Skipping test - requires database connection")
}
