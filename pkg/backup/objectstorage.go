package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"gocloud.dev/blob"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
)

var (
	ErrBucketNotFound = errors.New("bucket not found")
	ErrObjectNotFound = errors.New("object not found")
	ErrDigestMismatch = errors.New("object content digest mismatch")
	ErrKeyMissing     = errors.New("encryption key is required for backup")
)

type ObjectStorageBackup struct {
	ID        string       `json:"id"`
	Bucket    string       `json:"bucket"`
	Objects   []ObjectData `json:"objects"`
	Integrity string       `json:"integrity"`
	Timestamp time.Time    `json:"timestamp"`
	Version   string       `json:"version"`
}

type ObjectData struct {
	Key     string `json:"key"`
	Content string `json:"content"`
	Length  int64  `json:"length"`
	Digest  string `json:"digest"`
}

type ObjectStorageStore struct {
	bucket    *blob.Bucket
	encryptor *encrypt.KeyStore
	db        *sql.DB
}

func NewObjectStorageStore(bucket *blob.Bucket, db *sql.DB, encryptor *encrypt.KeyStore) *ObjectStorageStore {
	return &ObjectStorageStore{bucket: bucket, db: db, encryptor: encryptor}
}

func (s *ObjectStorageStore) Backup(ctx context.Context) (*ObjectStorageBackup, error) {
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: encryptor cannot be nil", ErrKeyMissing)
	}
	if s.bucket == nil {
		return nil, fmt.Errorf("%w: bucket cannot be nil", ErrBucketNotFound)
	}

	startTime := time.Now()

	key, err := s.encryptor.GetActiveKey(ctx, "system")
	if err != nil {
		return nil, fmt.Errorf("get encryption key: %w", err)
	}

	objects, err := s.listObjectsWithContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("list objects with content: %w", err)
	}

	backup := &ObjectStorageBackup{
		ID:        generateObjectStorageBackupID(),
		Bucket:    "object-storage",
		Objects:   objects,
		Timestamp: startTime,
		Version:   "1.0.0",
	}

	data, err := json.Marshal(backup)
	if err != nil {
		return nil, fmt.Errorf("marshal backup: %w", err)
	}

	encryptedData, err := s.encryptObjectData(data, key)
	if err != nil {
		return nil, fmt.Errorf("encrypt backup: %w", err)
	}

	digest := sha256.Sum256(encryptedData)
	backup.Integrity = base64.StdEncoding.EncodeToString(digest[:])

	err = s.storeObjectBackup(ctx, backup, encryptedData)
	if err != nil {
		return nil, fmt.Errorf("store backup: %w", err)
	}

	return backup, nil
}

func (s *ObjectStorageStore) Restore(ctx context.Context, backupID string) error {
	if s.encryptor == nil {
		return fmt.Errorf("%w: encryptor cannot be nil", ErrKeyMissing)
	}

	backup, data, err := s.loadObjectBackup(ctx, backupID)
	if err != nil {
		return fmt.Errorf("load backup: %w", err)
	}

	err = s.verifyObjectIntegrity(data, backup.Integrity)
	if err != nil {
		return fmt.Errorf("verify integrity: %w", err)
	}

	key, err := s.encryptor.GetActiveKey(ctx, "system")
	if err != nil {
		return fmt.Errorf("get encryption key: %w", err)
	}

	decryptedData, err := s.decryptObjectData(data, key)
	if err != nil {
		return fmt.Errorf("decrypt backup: %w", err)
	}

	var restoredBackup ObjectStorageBackup
	err = json.Unmarshal(decryptedData, &restoredBackup)
	if err != nil {
		return fmt.Errorf("unmarshal backup: %w", err)
	}

	err = s.restoreObjects(ctx, restoredBackup.Objects)
	if err != nil {
		return fmt.Errorf("restore objects: %w", err)
	}

	return nil
}

func (s *ObjectStorageStore) ListBackups(ctx context.Context) ([]*ObjectStorageBackup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, bucket, integrity, timestamp, version, data
		FROM object_storage_backups
		ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	var backups []*ObjectStorageBackup
	for rows.Next() {
		var b ObjectStorageBackup
		var data []byte
		err := rows.Scan(&b.ID, &b.Bucket, &b.Integrity, &b.Timestamp, &b.Version, &data)
		if err != nil {
			return nil, fmt.Errorf("scan backup row: %w", err)
		}
		json.Unmarshal(data, &b)
		backups = append(backups, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return backups, nil
}

func (s *ObjectStorageStore) DeleteBackup(ctx context.Context, backupID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM object_storage_backups WHERE id = $1`, backupID)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("backup not found: %s", backupID)
	}
	return nil
}

func (s *ObjectStorageStore) listObjectsWithContent(ctx context.Context) ([]ObjectData, error) {
	var objects []ObjectData
	iter := s.bucket.List(nil)
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list object: %w", err)
		}

		reader, err := s.bucket.NewReader(ctx, obj.Key, nil)
		if err != nil {
			return nil, fmt.Errorf("read object %s: %w", obj.Key, err)
		}

		var buf bytes.Buffer
		_, err = io.Copy(&buf, reader)
		reader.Close()
		if err != nil {
			return nil, fmt.Errorf("copy object %s: %w", obj.Key, err)
		}

		content := buf.Bytes()
		digest := sha256.Sum256(content)

		objects = append(objects, ObjectData{
			Key:     obj.Key,
			Content: base64.StdEncoding.EncodeToString(content),
			Length:  int64(len(content)),
			Digest:  base64.StdEncoding.EncodeToString(digest[:]),
		})
	}
	return objects, nil
}

func (s *ObjectStorageStore) restoreObjects(ctx context.Context, objects []ObjectData) error {
	for _, obj := range objects {
		content, err := base64.StdEncoding.DecodeString(obj.Content)
		if err != nil {
			return fmt.Errorf("decode object %s content: %w", obj.Key, err)
		}

		digest := sha256.Sum256(content)
		actualDigest := base64.StdEncoding.EncodeToString(digest[:])
		if actualDigest != obj.Digest {
			return fmt.Errorf("%w: object %s digest mismatch", ErrDigestMismatch, obj.Key)
		}

		err = s.bucket.Upload(ctx, obj.Key, bytes.NewReader(content), nil)
		if err != nil {
			return fmt.Errorf("write object %s: %w", obj.Key, err)
		}
	}
	return nil
}

func (s *ObjectStorageStore) encryptObjectData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	ciphertext, err := encryptor.Encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	return []byte(ciphertext), nil
}

func (s *ObjectStorageStore) decryptObjectData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	plaintext, err := encryptor.Decrypt(string(data))
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func (s *ObjectStorageStore) verifyObjectIntegrity(data []byte, expectedDigest string) error {
	digest := sha256.Sum256(data)
	actualDigest := base64.StdEncoding.EncodeToString(digest[:])
	if actualDigest != expectedDigest {
		return fmt.Errorf("%w: expected %s, got %s", ErrBackupCorrupted, expectedDigest, actualDigest)
	}
	return nil
}

func (s *ObjectStorageStore) storeObjectBackup(ctx context.Context, backup *ObjectStorageBackup, data []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO object_storage_backups (id, bucket, integrity, timestamp, version, data)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, backup.ID, backup.Bucket, backup.Integrity, backup.Timestamp, backup.Version, data)
	return err
}

func generateObjectStorageBackupID() string {
	id := make([]byte, 16)
	rand.Read(id)
	return "osbackup_" + base64.StdEncoding.EncodeToString(id)
}

func (s *ObjectStorageStore) loadObjectBackup(ctx context.Context, backupID string) (*ObjectStorageBackup, []byte, error) {
	var backup ObjectStorageBackup
	var data []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, bucket, integrity, timestamp, version, data
		FROM object_storage_backups WHERE id = $1
	`, backupID).Scan(
		&backup.ID, &backup.Bucket, &backup.Integrity, &backup.Timestamp, &backup.Version, &data,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load backup: %w", err)
	}
	return &backup, data, nil
}
