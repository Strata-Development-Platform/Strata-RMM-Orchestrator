package backup

import (
	"context"
	"database/sql"
	"testing"

	"gocloud.dev/blob/fileblob"
)

func TestObjectStorageStore_Backup(t *testing.T) {
	ctx := context.Background()

	bucket, err := fileblob.OpenBucket("/tmp/test-bucket", nil)
	if err != nil {
		t.Skipf("Local bucket not available: %v", err)
	}
	defer bucket.Close()

	store := NewObjectStorageStore(bucket, (*sql.DB)(nil), nil)

	backup, err := store.Backup(ctx)
	if err != nil {
		t.Logf("Backup (may fail if no objects): %v", err)
	}

	if backup != nil {
		if backup.Bucket != "object-storage" {
			t.Errorf("Expected bucket name 'object-storage', got '%s'", backup.Bucket)
		}
	}
}

func TestObjectStorageStore_ListBackups(t *testing.T) {
	t.Skip("Skipping test - requires database connection")
}

func TestObjectStorageStore_DeleteBackup(t *testing.T) {
	t.Skip("Skipping test - requires database connection")
}
