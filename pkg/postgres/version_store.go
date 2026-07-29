package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

const versionStoreTable = "schema_version_store"

// PostgresVersionStore implements VersionStore with persistent storage in the
// database.  It keeps a single row (key=version) for the current schema version
// and an arbitrary key-value map for release checksums.
type PostgresVersionStore struct {
	db       *sql.DB
	logger   *zap.SugaredLogger
	initOnce sync.Once
	initErr  error
}

func NewPostgresVersionStore(db *sql.DB, logger *zap.SugaredLogger) *PostgresVersionStore {
	return &PostgresVersionStore{db: db, logger: logger}
}

// init ensures the backing tables exist before any read/write.
func (vs *PostgresVersionStore) init(ctx context.Context) error {
	vs.initOnce.Do(func() {
		_, err := vs.db.ExecContext(ctx, fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				key        TEXT PRIMARY KEY,
				value      TEXT NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)
		`, versionStoreTable))
		if err != nil {
			vs.initErr = fmt.Errorf("create version store table: %w", err)
			return
		}
		vs.logger.Info("version store table initialized")
	})
	return vs.initErr
}

func (vs *PostgresVersionStore) GetVersion() (int32, error) {
	if vs.db == nil {
		return 0, fmt.Errorf("version store: database connection is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := vs.init(ctx); err != nil {
		return 0, err
	}

	var value string
	err := vs.db.QueryRowContext(ctx,
		`SELECT value FROM `+versionStoreTable+` WHERE key = 'version'`,
	).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil // no version recorded yet
		}
		return 0, fmt.Errorf("query version: %w", err)
	}

	var v int32
	if _, err := fmt.Sscanf(value, "%d", &v); err != nil {
		return 0, fmt.Errorf("parse stored version %q: %w", value, err)
	}
	return v, nil
}

func (vs *PostgresVersionStore) SetVersion(v int32) error {
	if vs.db == nil {
		return fmt.Errorf("version store: database connection is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := vs.init(ctx); err != nil {
		return err
	}

	value := fmt.Sprintf("%d", v)
	_, err := vs.db.ExecContext(ctx,
		`INSERT INTO `+versionStoreTable+` (key, value, updated_at)
		 VALUES ('version', $1, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = NOW()`,
		value,
	)
	if err != nil {
		return fmt.Errorf("set version: %w", err)
	}
	return nil
}

func (vs *PostgresVersionStore) SetVersionAndChecksum(v int32, checksum string) error {
	if err := vs.SetVersion(v); err != nil {
		return err
	}
	return vs.AddChecksum(fmt.Sprintf("v%d", v), checksum)
}

func (vs *PostgresVersionStore) GetChecksum(key string) (string, error) {
	if vs.db == nil {
		return "", fmt.Errorf("version store: database connection is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := vs.init(ctx); err != nil {
		return "", err
	}

	var value string
	err := vs.db.QueryRowContext(ctx,
		`SELECT value FROM `+versionStoreTable+` WHERE key = $1`, key,
	).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("get checksum for key %q: %w", key, err)
	}
	return value, nil
}

func (vs *PostgresVersionStore) AddChecksum(key, value string) error {
	if vs.db == nil {
		return fmt.Errorf("version store: database connection is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := vs.init(ctx); err != nil {
		return err
	}

	_, err := vs.db.ExecContext(ctx,
		`INSERT INTO `+versionStoreTable+` (key, value, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("add checksum for key %q: %w", key, err)
	}
	return nil
}

func (vs *PostgresVersionStore) ExistsChecksum(key string) (bool, error) {
	if vs.db == nil {
		return false, fmt.Errorf("version store: database connection is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := vs.init(ctx); err != nil {
		return false, err
	}

	var exists bool
	err := vs.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM `+versionStoreTable+` WHERE key = $1)`, key,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check checksum existence for key %q: %w", key, err)
	}
	return exists, nil
}
