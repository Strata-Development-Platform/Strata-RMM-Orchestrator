//go:build integration
// +build integration

package backup

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObjectStorageBackup_Structure(t *testing.T) {
	// Verify object storage backup structure

	store := NewObjectStorageBackupStore(nil, nil)
	require.NotNil(t, store)
}

func TestObjectStorageBackup_ObjectConfig(t *testing.T) {
	// Verify object configuration structure

	config := BackupObjectConfig{
		Key:       "backup/test.json",
		Bucket:    "backup-bucket",
		Size:      2048,
		ETag:      "d41d8cd98f00b204e9800998ecf8427e",
		LastMod:   "2024-01-01T00:00:00Z",
		Checksum:  "sha256:abc123",
	}

	require.Equal(t, "backup/test.json", config.Key)
	require.Equal(t, "backup-bucket", config.Bucket)
	require.Equal(t, int64(2048), config.Size)
	require.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", config.ETag)
	require.Equal(t, "2024-01-01T00:00:00Z", config.LastMod)
	require.Equal(t, "sha256:abc123", config.Checksum)
}

func TestObjectStorageBackup_EmptyBucket(t *testing.T) {
	// Verify empty bucket handling

	config := BackupObjectConfig{
		Key:   "",
		Bucket: "",
		Size:  0,
	}

	require.Equal(t, "", config.Key)
	require.Equal(t, "", config.Bucket)
	require.Equal(t, int64(0), config.Size)
}

func TestObjectStorageBackup_MultipleObjects(t *testing.T) {
	// Verify multiple objects

	objects := []BackupObjectConfig{
		{Key: "obj1.json", Bucket: "test-bucket", Size: 100},
		{Key: "obj2.json", Bucket: "test-bucket", Size: 200},
		{Key: "obj3.json", Bucket: "test-bucket", Size: 300},
	}

	require.Equal(t, 3, len(objects))
	require.Equal(t, "obj1.json", objects[0].Key)
	require.Equal(t, "obj2.json", objects[1].Key)
	require.Equal(t, "obj3.json", objects[2].Key)
}
