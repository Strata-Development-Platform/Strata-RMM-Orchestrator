package backup

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	_ "github.com/lib/pq"
)

func TestBackupStore_BinaryCheck(t *testing.T) {
	s := NewBackupStore(nil, nil, "")
	err := s.binaryAvailable()
	if err != nil {
		t.Fatalf("pg_dump/pg_restore should be in PATH: %v", err)
	}
}

func TestBackupStore_RejectsNilEncryptor(t *testing.T) {
	ctx := context.Background()
	s := NewBackupStore(nil, nil, "")
	_, err := s.CreateBackup(ctx, "postgresql")
	if err == nil {
		t.Fatal("expected error for nil encryptor")
	}
}

func TestBackupStore_RejectsUnsupportedType(t *testing.T) {
	ctx := context.Background()
	s := NewBackupStore(nil, nil, "")
	_, err := s.CreateBackup(ctx, "mysql")
	if err == nil {
		t.Fatal("expected error for unsupported database type")
	}
}

func TestBackupStore_RejectsMissingTargetDSN(t *testing.T) {
	ctx := context.Background()
	s := NewBackupStore(nil, nil, "")
	err := s.RestoreBackup(ctx, "backup_test", "")
	if err == nil {
		t.Fatal("expected error for missing target DSN")
	}
}

func TestBackupStore_IntegrityCheck(t *testing.T) {
	s := NewBackupStore(nil, nil, "")

	err := s.verifyIntegrity([]byte("test data"), "wrong-digest")
	if err == nil {
		t.Fatal("expected integrity check to fail")
	}

	data := []byte("test data")
	expectedDigest := sha256Of(data)
	err = s.verifyIntegrity(data, expectedDigest)
	if err != nil {
		t.Fatalf("expected integrity check to pass: %v", err)
	}
}

func sha256Of(data []byte) string {
	h := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(h[:])
}

func TestBackupStore_GenerateID(t *testing.T) {
	id1 := generateDatabaseBackupID()
	id2 := generateDatabaseBackupID()
	if id1 == id2 {
		t.Fatal("generated backup IDs should be unique")
	}
	if len(id1) < 20 {
		t.Fatalf("generated backup ID too short: %s", id1)
	}
}
