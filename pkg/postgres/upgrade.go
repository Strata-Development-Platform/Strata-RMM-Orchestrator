package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type UpgradePhase string

const (
	PreCheck       UpgradePhase = "pre_check"
	VersionUpgrade UpgradePhase = "version_upgrade"
	DataMigration  UpgradePhase = "data_migration"
	PostUpgrade    UpgradePhase = "post_upgrade"
	Finalize       UpgradePhase = "finalize"
)

func (p UpgradePhase) String() string {
	return string(p)
}

var allPhases = []UpgradePhase{
	PreCheck,
	VersionUpgrade,
	DataMigration,
	PostUpgrade,
	Finalize,
}

var upgradeLockID = 0x5550475241444501

type UpgradeHook func(ctx context.Context, version int32) error

type VersionStore interface {
	GetVersion() (int32, error)
	SetVersion(int32) error
	SetVersionAndChecksum(int32, string) error
	GetChecksum(string) (string, error)
	AddChecksum(string, string) error
	ExistsChecksum(string) (bool, error)
}

// NewVersionStore creates a VersionStore backed by the given database connection.
func NewVersionStore(db *sql.DB, logger *zap.SugaredLogger) *PostgresVersionStore {
	return NewPostgresVersionStore(db, logger)
}

func int32FromInt(v int) int32 {
	if v > 2147483647 || v < -2147483648 {
		panic("value out of int32 range")
	}
	return int32(v)
}

type UpgradeResult struct {
	Success        bool
	FromVersion    int32
	ToVersion      int32
	StepsCompleted []string
	Duration       time.Duration
	Error          error
	RollbackSteps  []string
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

type queryRow interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) rowScanner
}

type dbConnConn interface {
	queryRow
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	Close() error
}

type dbConn interface {
	Conn(ctx context.Context) (any, error)
}

// SQLDB wraps *sql.DB to implement the dbConn interface.
type SQLDB struct {
	*sql.DB
}

func (s *SQLDB) Conn(ctx context.Context) (any, error) {
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

type UpgradeManager struct {
	db               dbConn
	versionStore     VersionStore
	logger           *zap.SugaredLogger
	upgradeHooks     map[UpgradePhase]UpgradeHook
	lockTimeout      time.Duration
	maxRetryAttempts int
	upgradeMutex     sync.Mutex
	currentVersion   int32
	versionCheckDone atomic.Bool
	lockConn         dbConnConn
	lockAcquired     bool
}

func NewUpgradeManager(db dbConn, logger *zap.SugaredLogger, versionStore VersionStore) *UpgradeManager {
	return &UpgradeManager{
		db:               db,
		versionStore:     versionStore,
		logger:           logger,
		upgradeHooks:     make(map[UpgradePhase]UpgradeHook),
		lockTimeout:      30 * time.Second,
		maxRetryAttempts: 3,
	}
}

func (um *UpgradeManager) RegisterHook(phase UpgradePhase, hook UpgradeHook) {
	um.upgradeHooks[phase] = hook
}

func (um *UpgradeManager) RunPhase(ctx context.Context, phase UpgradePhase, version int32) error {
	um.logger.Infow("starting phase", "phase", phase, "version", version)

	hook, ok := um.upgradeHooks[phase]
	if !ok {
		return um.runDefaultPhase(ctx, phase, version)
	}

	var err error
	if phase == DataMigration {
		// Migration hooks can contain non-idempotent mutations. They must never
		// be replayed automatically after an ambiguous failure.
		err = hook(ctx, version)
	} else {
		err = um.runWithRetry(ctx, func(ctx context.Context) error {
			return hook(ctx, version)
		})
	}

	if err != nil {
		um.logger.Errorw("phase hook failed", "phase", phase, "error", err)
		return fmt.Errorf("phase %s hook failed: %w", phase, err)
	}

	um.logger.Infow("phase completed", "phase", phase)
	return nil
}

func (um *UpgradeManager) runDefaultPhase(ctx context.Context, phase UpgradePhase, version int32) error {
	switch phase {
	case PreCheck:
		return um.defaultPreCheck(ctx, version)
	case VersionUpgrade:
		return um.defaultVersionUpgrade(ctx, version)
	case DataMigration:
		return um.defaultDataMigration(ctx, version)
	case PostUpgrade:
		return um.defaultPostUpgrade(ctx, version)
	case Finalize:
		return um.defaultFinalize(ctx, version)
	default:
		um.logger.Warnw("unknown phase, skipping", "phase", phase)
		return nil
	}
}

func (um *UpgradeManager) defaultPreCheck(ctx context.Context, version int32) error {
	currentVersion, err := um.GetSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	migrations := Migrations()
	if len(migrations) == 0 {
		return fmt.Errorf("no migrations available; cannot upgrade")
	}

	maxID := migrations[len(migrations)-1].ID
	if int(version) > maxID {
		return fmt.Errorf("target version %d exceeds maximum migration ID %d", version, maxID)
	}

	found := false
	for _, m := range migrations {
		if m.ID == int(version) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("target version %d is not a valid migration ID", version)
	}

	if currentVersion >= version {
		return fmt.Errorf("current version %d is not less than target %d", currentVersion, version)
	}

	um.logger.Infow("pre-check passed", "current_version", currentVersion, "target_version", version, "migration_count", len(migrations))
	return nil
}

func (um *UpgradeManager) defaultVersionUpgrade(ctx context.Context, version int32) error {
	um.logger.Infow("validating version upgrade path", "target_version", version)
	if err := um.ValidateTarget(ctx, version); err != nil {
		return fmt.Errorf("validate version upgrade: %w", err)
	}
	return nil
}

func (um *UpgradeManager) defaultDataMigration(ctx context.Context, version int32) error {
	currentVersion, err := um.GetSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	migrations := Migrations()
	if len(migrations) == 0 {
		return fmt.Errorf("no migrations available for data migration")
	}

	for _, m := range migrations {
		if m.ID <= int(currentVersion) || m.ID > int(version) {
			continue
		}

		um.logger.Infow("applying migration", "id", m.ID, "name", m.Name)

		if m.Up != "" {
			if _, err := um.lockConn.ExecContext(ctx, m.Up); err != nil {
				return fmt.Errorf("apply migration %d (%s): %w", m.ID, m.Name, err)
			}
		}

		if _, err := um.lockConn.ExecContext(ctx,
			`INSERT INTO schema_migrations (id, name) VALUES ($1, $2)
			 ON CONFLICT (id) DO NOTHING`,
			m.ID, m.Name,
		); err != nil {
			return fmt.Errorf("record migration %d: %w", m.ID, err)
		}

		if err := um.versionStore.SetVersion(int32FromInt(m.ID)); err != nil {
			return fmt.Errorf("update version store for migration %d: %w", m.ID, err)
		}
	}

	um.logger.Infow("data migration completed", "from_version", currentVersion, "to_version", version)
	return nil
}

func (um *UpgradeManager) defaultPostUpgrade(ctx context.Context, version int32) error {
	storedVersion, err := um.versionStore.GetVersion()
	if err != nil {
		return fmt.Errorf("query stored version: %w", err)
	}

	if storedVersion != version {
		return fmt.Errorf("stored version %d does not match target %d", storedVersion, version)
	}

	um.logger.Infow("post-upgrade verification passed", "verified_version", version)
	return nil
}

func (um *UpgradeManager) defaultFinalize(ctx context.Context, version int32) error {
	checksum, err := MigrationChecksum(version)
	if err != nil {
		return fmt.Errorf("calculate migration checksum: %w", err)
	}
	if err := um.versionStore.SetVersionAndChecksum(version, checksum); err != nil {
		return fmt.Errorf("commit final version: %w", err)
	}
	um.versionCheckDone.Store(false)
	um.logger.Infow("upgrade finalized", "version", version, "checksum", checksum)
	return nil
}

func (um *UpgradeManager) runWithRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	var lastErr error
	for attempt := 1; attempt <= um.maxRetryAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		um.logger.Warnw("retry attempt", "attempt", attempt, "max_retries", um.maxRetryAttempts, "error", lastErr)
		if attempt < um.maxRetryAttempts {
			retryDelay := time.Duration(attempt) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}
	return fmt.Errorf("failed after %d attempts: %w", um.maxRetryAttempts, lastErr)
}

func (um *UpgradeManager) RunUpgrade(ctx context.Context, targetVersion int32) (*UpgradeResult, error) {
	startTime := time.Now()
	um.logger.Infow("starting upgrade", "target_version", targetVersion)

	result := &UpgradeResult{
		ToVersion: targetVersion,
	}

	um.upgradeMutex.Lock()
	defer um.upgradeMutex.Unlock()

	fromVersion, err := um.GetSchemaVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("get current version: %w", err)
	}
	result.FromVersion = fromVersion

	if targetVersion <= fromVersion {
		return nil, fmt.Errorf("target version %d is not greater than current version %d", targetVersion, fromVersion)
	}

	if err := um.acquireUpgradeLock(ctx); err != nil {
		return nil, fmt.Errorf("acquire upgrade lock: %w", err)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if releaseErr := um.releaseUpgradeLock(releaseCtx); releaseErr != nil {
			um.logger.Errorw("failed to release upgrade lock", "error", releaseErr)
		}
	}()

	if err := um.ValidateTarget(ctx, targetVersion); err != nil {
		result.Error = fmt.Errorf("validate upgrade path: %w", err)
		result.Duration = time.Since(startTime)
		return result, result.Error
	}

	for _, phase := range allPhases {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.Duration = time.Since(startTime)
			return result, ctx.Err()
		default:
		}

		stepLabel := fmt.Sprintf("phase:%s", phase)
		if err := um.RunPhase(ctx, phase, targetVersion); err != nil {
			result.Error = fmt.Errorf("phase %s: %w", phase, err)
			result.Duration = time.Since(startTime)
			um.logger.Errorw("upgrade failed", "phase", phase, "error", err)
			return result, result.Error
		}

		result.StepsCompleted = append(result.StepsCompleted, stepLabel)
	}

	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := um.releaseUpgradeLock(releaseCtx); err != nil {
		result.Error = err
		result.Duration = time.Since(startTime)
		return result, err
	}
	result.Success = true
	result.Duration = time.Since(startTime)
	um.logger.Infow("upgrade completed successfully", "from", fromVersion, "to", targetVersion, "duration", result.Duration)
	return result, nil
}

func (um *UpgradeManager) ValidateTarget(ctx context.Context, targetVersion int32) error {
	currentVersion, err := um.GetSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	if targetVersion <= currentVersion {
		return fmt.Errorf("target version %d is not greater than current version %d", targetVersion, currentVersion)
	}

	migrations := Migrations()
	if len(migrations) > 0 {
		maxID := migrations[len(migrations)-1].ID
		if targetVersion > int32FromInt(maxID) {
			return fmt.Errorf("target version %d exceeds maximum migration ID %d", targetVersion, maxID)
		}
		return nil
	}

	return nil
}

func (um *UpgradeManager) GetSchemaVersion(ctx context.Context) (int32, error) {
	if um.versionCheckDone.Load() {
		return um.currentVersion, nil
	}

	version, err := um.versionStore.GetVersion()
	if err != nil {
		return 0, fmt.Errorf("get version from store: %w", err)
	}

	um.currentVersion = version
	um.versionCheckDone.Store(true)
	um.logger.Debugw("schema version cached", "version", version)
	return version, nil
}

func (um *UpgradeManager) acquireUpgradeLock(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, um.lockTimeout)
	defer cancel()

	conn, err := um.db.Conn(timeoutCtx)
	if err != nil {
		return fmt.Errorf("get database connection: %w", err)
	}
	c, ok := conn.(dbConnConn)
	if !ok {
		return fmt.Errorf("database connection does not implement dbConnConn")
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for i := 0; i < 5; i++ {
		select {
		case <-timeoutCtx.Done():
			_ = c.Close() //nolint:errcheck // best-effort cleanup on timeout
			return fmt.Errorf("upgrade lock timed out: %w", context.DeadlineExceeded)
		default:
		}

		var acquired bool
		err := c.QueryRowContext(timeoutCtx, "SELECT pg_try_advisory_lock($1)", upgradeLockID).Scan(&acquired)
		if err != nil {
			if i < 4 {
				<-ticker.C
				continue
			}
			_ = c.Close() //nolint:errcheck // best-effort cleanup on failure
			return fmt.Errorf("lock attempt %d/%d: %w", i+1, 5, err)
		}
		if acquired {
			um.lockConn = c
			um.lockAcquired = true
			um.logger.Info("upgrade advisory lock acquired")
			return nil
		}
		if i < 4 {
			<-ticker.C
		}
	}

	_ = c.Close() //nolint:errcheck // best-effort cleanup after exhausted retries
	return fmt.Errorf("upgrade lock timed out after 5 attempts: %w", ErrLockTimeout)
}

func (um *UpgradeManager) releaseUpgradeLock(ctx context.Context) error {
	if !um.lockAcquired {
		return nil
	}

	if um.lockConn == nil {
		um.logger.Errorw("upgrade lock acquired but connection is nil")
		um.lockAcquired = false
		return fmt.Errorf("upgrade lock connection unexpectedly nil")
	}

	var unlocked bool
	err := um.lockConn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", upgradeLockID).Scan(&unlocked)
	if err != nil {
		_ = um.lockConn.Close() //nolint:errcheck // best-effort cleanup on release failure
		um.lockConn = nil
		um.lockAcquired = false
		return fmt.Errorf("release upgrade lock: %w", ErrLockReleaseFailed)
	}
	if !unlocked {
		_ = um.lockConn.Close() //nolint:errcheck // best-effort cleanup when lock was not held
		um.lockConn = nil
		um.lockAcquired = false
		return fmt.Errorf("upgrade lock was not held: %w", ErrLockHeld)
	}

	err = um.lockConn.Close()
	um.lockConn = nil
	um.lockAcquired = false
	if err != nil {
		return fmt.Errorf("close upgrade lock connection: %w", ErrLockReleaseFailed)
	}

	um.logger.Info("upgrade advisory lock released")
	return nil
}
