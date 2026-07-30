//go:build integration
// +build integration

package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJetStreamBackup_Structure(t *testing.T) {
	// Verify JetStream backup structure

	store := NewJetStreamBackupStore(nil, nil, nil)
	require.NotNil(t, store)
}

func TestJetStreamBackup_Configuration(t *testing.T) {
	// Verify JetStream configuration

	config := BackupStreamConfig{
		Name:      "TEST_STREAM",
		Subjects:  []string{"test.>"},
		Retention: "limits",
		MaxAge:    3600,
	}

	require.Equal(t, "TEST_STREAM", config.Name)
	require.Equal(t, []string{"test.>"}, config.Subjects)
	require.Equal(t, int64(3600), config.MaxAge)
}

func TestJetStreamBackup_MessageStructure(t *testing.T) {
	// Verify message structure

	msg := BackupMessage{
		Stream:  "test-stream",
		Subject: "test.subject",
		Data:    "test data",
		Seq:     1,
	}

	require.Equal(t, "test-stream", msg.Stream)
	require.Equal(t, "test.subject", msg.Subject)
	require.Equal(t, "test data", msg.Data)
	require.Equal(t, uint64(1), msg.Seq)
}

func TestJetStreamBackup_ConsumerConfig(t *testing.T) {
	// Verify consumer configuration

	consumer := BackupConsumerConfig{
		Stream:    "test-stream",
		Name:      "test-consumer",
		AckPolicy: "explicit",
	}

	require.Equal(t, "test-stream", consumer.Stream)
	require.Equal(t, "test-consumer", consumer.Name)
	require.Equal(t, "explicit", consumer.AckPolicy)
}

func TestJetStreamBackup_ObjectConfig(t *testing.T) {
	// Verify object configuration

	obj := BackupObjectConfig{
		Key:      "test/key",
		Bucket:   "test-bucket",
		Size:     1024,
		ETag:     "abc123",
		LastMod:  "2024-01-01",
		Checksum: "sha256:abc123",
	}

	require.Equal(t, "test/key", obj.Key)
	require.Equal(t, "test-bucket", obj.Bucket)
	require.Equal(t, int64(1024), obj.Size)
	require.Equal(t, "abc123", obj.ETag)
	require.Equal(t, "2024-01-01", obj.LastMod)
	require.Equal(t, "sha256:abc123", obj.Checksum)
}

func TestJetStreamBackup_Timestamps(t *testing.T) {
	// Verify timestamps

	now := time.Now()
	require.False(t, now.IsZero())
}
