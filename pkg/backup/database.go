package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
)

var (
	ErrBackupNotFound        = errors.New("backup not found")
	ErrIntegrityCheck        = errors.New("backup integrity check failed")
	ErrRestoreFailed         = errors.New("restore failed")
	ErrBackupCorrupted       = errors.New("backup data corrupted")
	ErrBinaryNotFound        = errors.New("pg_dump/pg_restore binary not found")
	ErrEncryptionKeyRequired = errors.New("encryption key is required")
	ErrTargetDSNRequired     = errors.New("target DSN is required for restore")
)

type BackupMetadata struct {
	ID              string    `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	DatabaseType    string    `json:"database_type"`
	Version         string    `json:"version"`
	TableCount      int       `json:"table_count"`
	RowEstimate     int64     `json:"row_estimate"`
	DataSize        int64     `json:"data_size"`
	Compression     string    `json:"compression"`
	Scheme          string    `json:"scheme"`
	KeyReference    string    `json:"key_reference"`
	IntegrityDigest string    `json:"integrity_digest"`
}

type BackupStore struct {
	db        *sql.DB
	encryptor *encrypt.KeyStore
	pgDump    string
	pgRestore string
	pgDSN     string
}

func NewBackupStore(db *sql.DB, encryptor *encrypt.KeyStore, pgDSN string) *BackupStore {
	s := &BackupStore{db: db, encryptor: encryptor, pgDSN: pgDSN}
	s.pgDump, _ = exec.LookPath("pg_dump")
	s.pgRestore, _ = exec.LookPath("pg_restore")
	return s
}

func (s *BackupStore) binaryAvailable() error {
	if s.pgDump == "" {
		return fmt.Errorf("%w: pg_dump not found in PATH", ErrBinaryNotFound)
	}
	if s.pgRestore == "" {
		return fmt.Errorf("%w: pg_restore not found in PATH", ErrBinaryNotFound)
	}
	return nil
}

func (s *BackupStore) CreateBackup(ctx context.Context, databaseType string) (*BackupMetadata, error) {
	if databaseType != "postgresql" && databaseType != "timescaledb" {
		return nil, fmt.Errorf("unsupported database type: %s", databaseType)
	}
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: encryptor cannot be nil", ErrEncryptionKeyRequired)
	}
	if err := s.binaryAvailable(); err != nil {
		return nil, err
	}

	startTime := time.Now()

	key, err := s.encryptor.GetActiveKey(ctx, "system")
	if err != nil {
		return nil, fmt.Errorf("get encryption key: %w", err)
	}

	data, tableCount, rowEstimate, err := s.dumpDatabase(ctx, databaseType)
	if err != nil {
		return nil, fmt.Errorf("dump database: %w", err)
	}

	encryptedData, err := s.encryptData(data, key)
	if err != nil {
		return nil, fmt.Errorf("encrypt backup: %w", err)
	}

	digest := sha256.Sum256(encryptedData)
	integrityDigest := base64.StdEncoding.EncodeToString(digest[:])

	metadata := &BackupMetadata{
		ID:              generateDatabaseBackupID(),
		Timestamp:       startTime,
		DatabaseType:    databaseType,
		Version:         "1.0.0",
		TableCount:      tableCount,
		RowEstimate:     rowEstimate,
		DataSize:        int64(len(encryptedData)),
		Compression:     "none",
		Scheme:          string(key.Encryption),
		KeyReference:    key.ID,
		IntegrityDigest: integrityDigest,
	}

	err = s.storeBackupMetadata(ctx, metadata, encryptedData)
	if err != nil {
		return nil, fmt.Errorf("store backup: %w", err)
	}

	return metadata, nil
}

func (s *BackupStore) RestoreBackup(ctx context.Context, backupID string, targetDSN string) error {
	if targetDSN == "" {
		return fmt.Errorf("%w", ErrTargetDSNRequired)
	}

	metadata, data, err := s.loadBackupData(ctx, backupID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBackupNotFound, err)
	}

	err = s.verifyIntegrity(data, metadata.IntegrityDigest)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrIntegrityCheck, err)
	}

	if metadata.Scheme == "none" || metadata.KeyReference == "" || s.encryptor == nil {
		return fmt.Errorf("%w: backup %s has no encryption scheme", ErrEncryptionKeyRequired, backupID)
	}

	key, err := s.getKeyByID(ctx, metadata.KeyReference)
	if err != nil {
		return fmt.Errorf("get encryption key: %w", err)
	}

	decryptedData, err := s.decryptData(data, key)
	if err != nil {
		return fmt.Errorf("decrypt backup: %w", err)
	}

	err = s.restoreDatabase(ctx, decryptedData, targetDSN)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRestoreFailed, err)
	}

	return nil
}

func (s *BackupStore) ListBackups(ctx context.Context) ([]*BackupMetadata, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, database_type, version, table_count, row_estimate, data_size, compression, scheme, key_reference, integrity_digest
		FROM backups
		ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	var backups []*BackupMetadata
	for rows.Next() {
		var b BackupMetadata
		err := rows.Scan(
			&b.ID, &b.Timestamp, &b.DatabaseType, &b.Version, &b.TableCount,
			&b.RowEstimate, &b.DataSize, &b.Compression, &b.Scheme, &b.KeyReference, &b.IntegrityDigest,
		)
		if err != nil {
			return nil, fmt.Errorf("scan backup row: %w", err)
		}
		backups = append(backups, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return backups, nil
}

func (s *BackupStore) DeleteBackup(ctx context.Context, backupID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = $1`, backupID)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrBackupNotFound, backupID)
	}
	return nil
}

func (s *BackupStore) dumpDatabase(ctx context.Context, databaseType string) ([]byte, int, int64, error) {
	var tableCount int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tableCount)
	if err != nil {
		tableCount = 0
	}
	var rowEstimate int64
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(n_live_tup), 0) FROM pg_stat_user_tables`).Scan(&rowEstimate)
	if err != nil {
		rowEstimate = 0
	}

	var dsn string
	if s.pgDSN != "" {
		dsn = s.pgDSN
	} else {
		dsn = s.buildDSN()
	}

	args := []string{
		"--no-owner",
		"--no-privileges",
		"--format=custom",
		"-d", dsn,
	}

	var stdout, stderr bytes.Buffer
	dumpCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(dumpCtx, s.pgDump, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, 0, 0, fmt.Errorf("pg_dump failed: %w\nstderr: %s", err, stderr.String())
	}
	return stdout.Bytes(), tableCount, rowEstimate, nil
}

func (s *BackupStore) restoreDatabase(ctx context.Context, data []byte, targetDSN string) error {
	args := []string{
		"--no-owner",
		"--no-privileges",
		"--clean",
		"--if-exists",
		"-d", targetDSN,
	}

	var stderr bytes.Buffer
	restoreCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(restoreCtx, s.pgRestore, args...)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_restore failed: %w\nstderr: %s", err, stderr.String())
	}
	return nil
}

func (s *BackupStore) buildDSN() string {
	if s.pgDSN != "" {
		return s.pgDSN
	}
	return "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
}

func (s *BackupStore) encryptData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	ciphertext, err := encryptor.Encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	return []byte(ciphertext), nil
}

func (s *BackupStore) decryptData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	plaintext, err := encryptor.Decrypt(string(data))
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func (s *BackupStore) verifyIntegrity(data []byte, expectedDigest string) error {
	digest := sha256.Sum256(data)
	actualDigest := base64.StdEncoding.EncodeToString(digest[:])
	if actualDigest != expectedDigest {
		return fmt.Errorf("%w: expected %s, got %s", ErrBackupCorrupted, expectedDigest, actualDigest)
	}
	return nil
}

func (s *BackupStore) storeBackupMetadata(ctx context.Context, metadata *BackupMetadata, data []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO backups (id, timestamp, database_type, version, table_count, row_estimate, data_size, compression, scheme, key_reference, integrity_digest, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, metadata.ID, metadata.Timestamp, metadata.DatabaseType, metadata.Version, metadata.TableCount,
		metadata.RowEstimate, metadata.DataSize, metadata.Compression, metadata.Scheme, metadata.KeyReference, metadata.IntegrityDigest, data)
	return err
}

func (s *BackupStore) loadBackupData(ctx context.Context, backupID string) (*BackupMetadata, []byte, error) {
	var metadata BackupMetadata
	var data []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, timestamp, database_type, version, table_count, row_estimate, data_size, compression, scheme, key_reference, integrity_digest, data
		FROM backups WHERE id = $1
	`, backupID).Scan(
		&metadata.ID, &metadata.Timestamp, &metadata.DatabaseType, &metadata.Version, &metadata.TableCount,
		&metadata.RowEstimate, &metadata.DataSize, &metadata.Compression, &metadata.Scheme, &metadata.KeyReference, &metadata.IntegrityDigest, &data,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load backup: %w", err)
	}
	return &metadata, data, nil
}

func (s *BackupStore) getKeyByID(ctx context.Context, keyID string) (*encrypt.TenantKey, error) {
	var key encrypt.TenantKey
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, key_alias, kms_type, encryption, kms_key_id, key_material, region, endpoint, status, created_at, rotated_at, expires_at
		FROM tenant_encryption_keys WHERE id = $1
	`, keyID).Scan(
		&key.ID, &key.TenantID, &key.KeyAlias, &key.KMSProvider, &key.Encryption,
		&key.KMSKeyID, &key.KeyMaterial, &key.Region, &key.Endpoint, &key.Status,
		&key.CreatedAt, &key.RotatedAt, &key.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("key not found: %s: %w", keyID, err)
	}
	return &key, nil
}

func generateDatabaseBackupID() string {
	id := make([]byte, 16)
	rand.Read(id)
	return fmt.Sprintf("backup_%s", base64.URLEncoding.EncodeToString(id))
}
