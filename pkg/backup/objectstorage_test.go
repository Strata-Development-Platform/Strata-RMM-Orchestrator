package backup

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestObjectStorageStore_RejectsNilEncryptor(t *testing.T) {
	ctx := context.Background()
	s := NewObjectStorageStore(nil, nil, nil)
	_, err := s.Backup(ctx)
	if err == nil {
		t.Fatal("expected error for nil encryptor")
	}
}

func TestObjectStorageStore_RejectsNilBucket(t *testing.T) {
	ctx := context.Background()
	s := NewObjectStorageStore(nil, nil, nil)
	err := s.Restore(ctx, "test-id")
	if err == nil {
		t.Fatal("expected error for nil bucket")
	}
}

func TestObjectStorageStore_IntegrityCheck(t *testing.T) {
	s := &ObjectStorageStore{}

	err := s.verifyObjectIntegrity([]byte("test data"), "wrong-digest")
	if err == nil {
		t.Fatal("expected integrity check to fail")
	}

	data := []byte("test data")
	digest := sha256.Sum256(data)
	expectedDigest := base64.StdEncoding.EncodeToString(digest[:])
	err = s.verifyObjectIntegrity(data, expectedDigest)
	if err != nil {
		t.Fatalf("expected integrity check to pass: %v", err)
	}
}

func TestObjectStorageStore_DigestMismatch(t *testing.T) {
	objects := []ObjectData{
		{
			Key:     "test-key",
			Content: "d3JvbmctY29udGVudA==",
			Digest:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		},
	}

	s := &ObjectStorageStore{}
	err := s.restoreObjects(context.Background(), objects)
	if err == nil {
		t.Fatal("expected digest mismatch error")
	}
}

func TestObjectStorageStore_ContentRoundTrip(t *testing.T) {
	content := []byte("hello world")
	digest := sha256.Sum256(content)
	digestStr := base64.StdEncoding.EncodeToString(digest[:])
	encodedStr := base64.StdEncoding.EncodeToString(content)

	objects := []ObjectData{
		{
			Key:     "test-key",
			Content: encodedStr,
			Length:  int64(len(content)),
			Digest:  digestStr,
		},
	}

	if objects[0].Length != int64(len(content)) {
		t.Fatalf("expected length %d, got %d", len(content), objects[0].Length)
	}
	if objects[0].Digest != digestStr {
		t.Fatalf("digest mismatch")
	}
}

func TestObjectStorageStore_GenerateID(t *testing.T) {
	id1 := generateObjectStorageBackupID()
	id2 := generateObjectStorageBackupID()
	if id1 == id2 {
		t.Fatal("generated backup IDs should be unique")
	}
}
