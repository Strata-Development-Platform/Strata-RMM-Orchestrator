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
	PreCheck      UpgradePhase = "pre_check"
	VersionUpgrade UpgradePhase = "version_upgrade"
	DataMigration UpgradePhase = "data_migration"
	PostUpgrade   UpgradePhase = "post_upgrade"
	Finalize      UpgradePhase = "finalize"
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
	Conn(ctx context.Context) (dbConnConn, error)
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
	lockConn         *sql.Conn
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

	err := um.runWithRetry(ctx, func(ctx context.Context) error {
		return hook(ctx, version)
	})

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
	um.logger.Infow("running pre-check validations", "target_version", version)
	return nil
}

func (um *UpgradeManager) defaultVersionUpgrade(ctx context.Context, version int32) error {
	um.logger.Infow("updating schema version", "version", version)
	if err := um.versionStore.SetVersion(version); err != nil {
		return fmt.Errorf("set version: %w", err)
	}
	return nil
}

func (um *UpgradeManager) defaultDataMigration(ctx context.Context, version int32) error {
	um.logger.Infow("running default data migration", "version", version)
	return nil
}

func (um *UpgradeManager) defaultPostUpgrade(ctx context.Context, version int32) error {
	um.logger.Infow("running post-upgrade checks", "version", version)
	return nil
}

func (um *UpgradeManager) defaultFinalize(ctx context.Context, version int32) error {
	um.logger.Infow("finalizing upgrade", "version", version)
	um.versionCheckDone.Store(false)
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
		if releaseErr := um.releaseUpgradeLock(ctx); releaseErr != nil {
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
		maxID := int(migrations[len(migrations)-1].ID)
		if int(targetVersion) > maxID {
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

	c, err := um.db.Conn(timeoutCtx)
	if err != nil {
		return fmt.Errorf("get database connection: %w", err)
	}
	defer c.Close()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for i := 0; i < 5; i++ {
		select {
		case <-timeoutCtx.Done():
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
			return fmt.Errorf("lock attempt %d/%d: %w", i+1, 5, err)
		}
		if acquired {
			um.logger.Info("upgrade advisory lock acquired")
			return nil
		}
		if i < 4 {
			<-ticker.C
		}
	}

	return fmt.Errorf("upgrade lock timed out after 5 attempts: %w", ErrLockTimeout)
}

func (um *UpgradeManager) releaseUpgradeLock(ctx context.Context) error {
	c, err := um.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get connection for lock release: %w", err)
	}
	defer c.Close()

	_, err = c.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", upgradeLockID)
	if err != nil {
		return fmt.Errorf("release upgrade lock: %w", ErrLockReleaseFailed)
	}

	um.logger.Info("upgrade advisory lock released")
	return nil
}
