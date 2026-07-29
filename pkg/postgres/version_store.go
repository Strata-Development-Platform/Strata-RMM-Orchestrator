package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

const versionStoreTable = "schema_version_store"

// MigrationChecksum returns a stable SHA-256 digest of the exact migration
// definitions through version. It is evidence of schema content, not a
// placeholder derived from the version number.
func MigrationChecksum(version int32) (string, error) {
	hash := sha256.New()
	found := version == 0
	for _, migration := range Migrations() {
		if migration.ID > int(version) {
			break
		}
		found = found || migration.ID == int(version)
		_, _ = fmt.Fprintf(hash, "%d\x00%s\x00%s\x00%s\x00", migration.ID, migration.Name, migration.Up, migration.Down)
	}
	if !found {
		return "", fmt.Errorf("migration version %d does not exist", version)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

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
		if vs.logger != nil {
			vs.logger.Info("version store table initialized")
		}
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
			// Existing installations predate schema_version_store. Bootstrap
			// their current version from immutable migration history without
			// rewriting that history.
			var migrationVersion int64
			if err := vs.db.QueryRowContext(ctx,
				`SELECT COALESCE(MAX(id), 0) FROM schema_migrations`,
			).Scan(&migrationVersion); err != nil {
				return 0, fmt.Errorf("bootstrap version from migration history: %w", err)
			}
			if migrationVersion > 2147483647 || migrationVersion < 0 {
				return 0, fmt.Errorf("migration version %d out of range", migrationVersion)
			}
			return int32(migrationVersion), nil
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
	expected, err := MigrationChecksum(v)
	if err != nil {
		return err
	}
	if checksum != expected {
		return fmt.Errorf("checksum for version %d does not match migration content", v)
	}
	if vs.db == nil {
		return fmt.Errorf("version store: database connection is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := vs.init(ctx); err != nil {
		return err
	}
	tx, err := vs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin version update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO `+versionStoreTable+` (key, value, updated_at)
		 VALUES ('version', $1, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = NOW()`,
		fmt.Sprintf("%d", v)); err != nil {
		return fmt.Errorf("set version: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO `+versionStoreTable+` (key, value, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()`,
		fmt.Sprintf("v%d", v), checksum); err != nil {
		return fmt.Errorf("set checksum: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit version update: %w", err)
	}
	return nil
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
