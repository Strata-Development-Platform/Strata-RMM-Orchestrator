package backup

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestJetStreamBackupStore_NilConnection(t *testing.T) {
	_, err := NewJetStreamBackupStore(nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil NATS connection")
	}
}

func TestJetStreamBackupStore_Integrity(t *testing.T) {
	err := verifyJetStreamIntegrity([]byte("test data"), "wrong-digest")
	if err == nil {
		t.Fatal("expected integrity check to fail")
	}

	data := []byte("test data")
	digest := sha256.Sum256(data)
	expectedDigest := base64.StdEncoding.EncodeToString(digest[:])
	err = verifyJetStreamIntegrity(data, expectedDigest)
	if err != nil {
		t.Fatalf("expected integrity check to pass: %v", err)
	}
}

func TestJetStreamBackupStore_GenerateID(t *testing.T) {
	id1 := generateJetStreamBackupID()
	id2 := generateJetStreamBackupID()
	if id1 == id2 {
		t.Fatal("generated backup IDs should be unique")
	}
}
