package backup

import (
	"bytes"
	"context"
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
)

type ObjectStorageBackup struct {
	ID        string             `json:"id"`
	Bucket    string             `json:"bucket"`
	Objects   []ObjectMetadata   `json:"objects"`
	Integrity string             `json:"integrity"`
	Timestamp time.Time          `json:"timestamp"`
	Version   string             `json:"version"`
}

type ObjectMetadata struct {
	Key         string `json:"key"`
	Length      int64  `json:"length"`
	ContentType string `json:"content_type"`
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
	startTime := time.Now()

	objects, err := s.listObjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}

	backup := &ObjectStorageBackup{
		Bucket:    "object-storage",
		Objects:   objects,
		Timestamp: startTime,
		Version:   "1.0.0",
	}

	data, err := json.Marshal(backup)
	if err != nil {
		return nil, fmt.Errorf("marshal backup: %w", err)
	}

	var encryptedData []byte
	if s.encryptor != nil {
		key, err := s.encryptor.GetActiveKey(ctx, "system")
		if err != nil {
			return nil, fmt.Errorf("get encryption key: %w", err)
		}
		encryptedData, err = s.encryptData(data, key)
		if err != nil {
			return nil, fmt.Errorf("encrypt backup: %w", err)
		}
	} else {
		encryptedData = data
	}

	digest := sha256.Sum256(encryptedData)
	backup.Integrity = base64.StdEncoding.EncodeToString(digest[:])

	err = s.storeBackup(ctx, backup, encryptedData)
	if err != nil {
		return nil, fmt.Errorf("store backup: %w", err)
	}

	return backup, nil
}

func (s *ObjectStorageStore) Restore(ctx context.Context, backupID string) error {
	backup, data, err := s.loadBackup(ctx, backupID)
	if err != nil {
		return fmt.Errorf("load backup: %w", err)
	}

	err = s.verifyIntegrity(data, backup.Integrity)
	if err != nil {
		return fmt.Errorf("verify integrity: %w", err)
	}

	var decryptedData []byte
	if backup.Integrity != "none" && s.encryptor != nil {
		key, err := s.encryptor.GetActiveKey(ctx, "system")
		if err != nil {
			return fmt.Errorf("get encryption key: %w", err)
		}
		decryptedData, err = s.decryptData(data, key)
		if err != nil {
			return fmt.Errorf("decrypt backup: %w", err)
		}
	} else {
		decryptedData = data
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
		SELECT id, bucket, integrity, timestamp, version, objects
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
		var objectsJSON []byte
		err := rows.Scan(&b.ID, &b.Bucket, &b.Integrity, &b.Timestamp, &b.Version, &objectsJSON)
		if err != nil {
			continue
		}
		json.Unmarshal(objectsJSON, &b.Objects)
		backups = append(backups, &b)
	}
	return backups, nil
}

func (s *ObjectStorageStore) DeleteBackup(ctx context.Context, backupID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM object_storage_backups WHERE id = $1`, backupID)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

func (s *ObjectStorageStore) listObjects(ctx context.Context) ([]ObjectMetadata, error) {
	var objects []ObjectMetadata
	iter := s.bucket.List(nil)
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list object: %w", err)
		}

		objects = append(objects, ObjectMetadata{
			Key:    obj.Key,
			Length: obj.Size,
		})
	}
	return objects, nil
}

func (s *ObjectStorageStore) restoreObjects(ctx context.Context, objects []ObjectMetadata) error {
	for _, obj := range objects {
		exists, err := s.bucket.Exists(ctx, obj.Key)
		if err != nil {
			return fmt.Errorf("check object %s: %w", obj.Key, err)
		}
		if exists {
			continue
		}

		reader, err := s.bucket.NewRangeReader(ctx, obj.Key, 0, -1, nil)
		if err != nil {
			return fmt.Errorf("read object %s: %w", obj.Key, err)
		}

		var buf bytes.Buffer
		_, err = io.Copy(&buf, reader)
		reader.Close()
		if err != nil {
			return fmt.Errorf("copy object %s: %w", obj.Key, err)
		}

	err = s.bucket.Upload(ctx, obj.Key, bytes.NewReader(buf.Bytes()), nil)
		if err != nil {
			return fmt.Errorf("write object %s: %w", obj.Key, err)
		}
	}
	return nil
}

func (s *ObjectStorageStore) encryptData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	ciphertext, err := encryptor.Encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	return []byte(ciphertext), nil
}

func (s *ObjectStorageStore) decryptData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	plaintext, err := encryptor.Decrypt(string(data))
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func (s *ObjectStorageStore) verifyIntegrity(data []byte, expectedDigest string) error {
	digest := sha256.Sum256(data)
	actualDigest := base64.StdEncoding.EncodeToString(digest[:])
	if actualDigest != expectedDigest {
		return fmt.Errorf("%w: expected %s, got %s", ErrBackupCorrupted, expectedDigest, actualDigest)
	}
	return nil
}

func (s *ObjectStorageStore) storeBackup(ctx context.Context, backup *ObjectStorageBackup, data []byte) error {
	objectsJSON, _ := json.Marshal(backup.Objects)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO object_storage_backups (id, bucket, integrity, timestamp, version, objects, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, backup.ID, backup.Bucket, backup.Integrity, backup.Timestamp, backup.Version, objectsJSON, data)
	if err != nil {
		return fmt.Errorf("store backup: %w", err)
	}
	return nil
}

func (s *ObjectStorageStore) loadBackup(ctx context.Context, backupID string) (*ObjectStorageBackup, []byte, error) {
	var backup ObjectStorageBackup
	var data []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, bucket, integrity, timestamp, version, objects, data
		FROM object_storage_backups WHERE id = $1
	`, backupID).Scan(
		&backup.ID, &backup.Bucket, &backup.Integrity, &backup.Timestamp, &backup.Version,
		&backup.Objects, &data,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load backup: %w", err)
	}
	return &backup, data, nil
}
