package postgres

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type RollbackPhase string

const (
	RBPreCheck         RollbackPhase = "pre_check"
	RBVersionDowngrade RollbackPhase = "version_downgrade"
	RBDataRollback     RollbackPhase = "data_rollback"
	RBPostRollback     RollbackPhase = "post_rollback"
	RBFinalize         RollbackPhase = "finalize"
)

var rollbackPhases = []RollbackPhase{
	RBPreCheck,
	RBVersionDowngrade,
	RBDataRollback,
	RBPostRollback,
	RBFinalize,
}

var rollbackLockID = 0x524F4C4C4241434B // "ROLLBACK" as int64

type RollbackHook func(ctx context.Context, fromVersion int32, toVersion int32) error

type RollbackResult struct {
	Success        bool
	FromVersion    int32
	ToVersion      int32
	StepsCompleted []string
	Duration       time.Duration
	Error          error
	DryRun         bool
}

type RollbackEngine struct {
	db               dbConn
	versionStore     VersionStore
	logger           *zap.SugaredLogger
	rollbackHooks    map[RollbackPhase]RollbackHook
	lockTimeout      time.Duration
	maxRetryAttempts int
	rollbackMutex    sync.Mutex
	dryRunMode       bool
	currentVersion   int32
	versionCheckDone atomic.Bool
}

func NewRollbackEngine(db dbConn, logger *zap.SugaredLogger, versionStore VersionStore) *RollbackEngine {
	return &RollbackEngine{
		db:               db,
		versionStore:     versionStore,
		logger:           logger,
		rollbackHooks:    make(map[RollbackPhase]RollbackHook),
		lockTimeout:      30 * time.Second,
		maxRetryAttempts: 3,
	}
}

func (re *RollbackEngine) RegisterHook(phase RollbackPhase, hook RollbackHook) {
	re.rollbackHooks[phase] = hook
}

func (re *RollbackEngine) RunPhase(ctx context.Context, phase RollbackPhase, fromVersion, toVersion int32) error {
	re.logger.Infow("starting rollback phase", "phase", phase, "from_version", fromVersion, "to_version", toVersion)

	if phase == RBPreCheck {
		if err := re.preCheck(ctx, fromVersion, toVersion); err != nil {
			return fmt.Errorf("pre-check failed: %w", err)
		}
	}

	hook, ok := re.rollbackHooks[phase]
	if ok {
		err := re.runWithRetry(ctx, func(ctx context.Context) error {
			return hook(ctx, fromVersion, toVersion)
		})
		if err != nil {
			re.logger.Errorw("rollback phase hook failed", "phase", phase, "error", err)
			return fmt.Errorf("phase %s hook failed: %w", phase, err)
		}
		re.logger.Infow("rollback phase completed", "phase", phase)
		return nil
	}

	if phase == RBPreCheck {
		re.logger.Infow("pre-check completed", "phase", phase)
		return nil
	}
	if !ok {
		return re.runDefaultRollbackPhase(ctx, phase, fromVersion, toVersion)
	}

	err := re.runWithRetry(ctx, func(ctx context.Context) error {
		return hook(ctx, fromVersion, toVersion)
	})

	if err != nil {
		re.logger.Errorw("rollback phase hook failed", "phase", phase, "error", err)
		return fmt.Errorf("phase %s hook failed: %w", phase, err)
	}

	re.logger.Infow("rollback phase completed", "phase", phase)
	return nil
}

func (re *RollbackEngine) runDefaultRollbackPhase(ctx context.Context, phase RollbackPhase, fromVersion, toVersion int32) error {
	switch phase {
	case RBVersionDowngrade:
		return re.defaultVersionDowngrade(ctx, fromVersion, toVersion)
	case RBDataRollback:
		return re.defaultDataRollback(ctx, fromVersion, toVersion)
	case RBPostRollback:
		return re.defaultPostRollback(ctx, fromVersion, toVersion)
	case RBFinalize:
		return re.defaultFinalizeRollback(ctx, fromVersion, toVersion)
	default:
		re.logger.Warnw("unknown rollback phase, skipping", "phase", phase)
		return nil
	}
}

func (re *RollbackEngine) preCheck(ctx context.Context, fromVersion, toVersion int32) error {
	re.logger.Infow("running pre-check validations", "from_version", fromVersion, "to_version", toVersion)

	if err := re.ValidateRollbackTarget(ctx, toVersion); err != nil {
		return fmt.Errorf("pre-check validation failed: %w", err)
	}

	migrations := Migrations()
	if len(migrations) == 0 {
		return nil
	}

	maxID := int32(migrations[len(migrations)-1].ID)
	if fromVersion > maxID {
		return fmt.Errorf("current version %d exceeds maximum migration ID %d; cannot rollback", fromVersion, maxID)
	}

	return nil
}

func (re *RollbackEngine) defaultVersionDowngrade(ctx context.Context, fromVersion, toVersion int32) error {
	re.logger.Infow("downgrading schema version", "from", fromVersion, "to", toVersion)
	if err := re.versionStore.SetVersion(toVersion); err != nil {
		return fmt.Errorf("set version: %w", err)
	}
	return nil
}

func (re *RollbackEngine) defaultDataRollback(ctx context.Context, fromVersion, toVersion int32) error {
	re.logger.Infow("running data rollback", "from", fromVersion, "to", toVersion)
	return nil
}

func (re *RollbackEngine) defaultPostRollback(ctx context.Context, fromVersion, toVersion int32) error {
	re.logger.Infow("running post-rollback checks", "from", fromVersion, "to", toVersion)
	return nil
}

func (re *RollbackEngine) defaultFinalizeRollback(ctx context.Context, fromVersion, toVersion int32) error {
	re.logger.Infow("finalizing rollback", "from", fromVersion, "to", toVersion)
	re.versionCheckDone.Store(false)
	return nil
}

func (re *RollbackEngine) runWithRetry(ctx context.Context, fn func(ctx context.Context) error) error {
	var lastErr error
	for attempt := 1; attempt <= re.maxRetryAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		re.logger.Warnw("rollback retry attempt", "attempt", attempt, "max_retries", re.maxRetryAttempts, "error", lastErr)
		if attempt < re.maxRetryAttempts {
			retryDelay := time.Duration(attempt) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}
	return fmt.Errorf("failed after %d attempts: %w", re.maxRetryAttempts, lastErr)
}

func (re *RollbackEngine) RunRollback(ctx context.Context, targetVersion int32) (*RollbackResult, error) {
	startTime := time.Now()
	re.logger.Infow("starting rollback", "target_version", targetVersion)

	result := &RollbackResult{
		ToVersion: targetVersion,
		DryRun:    re.dryRunMode,
	}

	re.rollbackMutex.Lock()
	defer re.rollbackMutex.Unlock()

	fromVersion, err := re.GetSchemaVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("get current version: %w", err)
	}
	result.FromVersion = fromVersion

	if targetVersion >= fromVersion {
		err := fmt.Errorf("target version %d is not less than current version %d", targetVersion, fromVersion)
		result.Error = err
		result.Duration = time.Since(startTime)
		return result, err
	}

	if err := re.acquireRollbackLock(ctx); err != nil {
		result.Error = fmt.Errorf("acquire rollback lock: %w", err)
		result.Duration = time.Since(startTime)
		return result, result.Error
	}

	lockReleased := false
	defer func() {
		if !lockReleased {
			if releaseErr := re.releaseRollbackLock(ctx); releaseErr != nil {
				re.logger.Errorw("failed to release rollback lock", "error", releaseErr)
			}
		}
		lockReleased = true
	}()

	if err := re.ValidateRollbackTarget(ctx, targetVersion); err != nil {
		result.Error = fmt.Errorf("validate rollback path: %w", err)
		result.Duration = time.Since(startTime)
		return result, result.Error
	}

	for _, phase := range rollbackPhases {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.Duration = time.Since(startTime)
			return result, ctx.Err()
		default:
		}

		stepLabel := fmt.Sprintf("phase:%s", phase)

		if re.dryRunMode {
			dryErr := re.dryRunPhase(ctx, phase, fromVersion, targetVersion)
			if dryErr != nil {
				result.Error = fmt.Errorf("dry-run phase %s: %w", phase, dryErr)
				result.Duration = time.Since(startTime)
				re.logger.Errorw("dry-run rollback failed", "phase", phase, "error", dryErr)
				return result, result.Error
			}
			result.StepsCompleted = append(result.StepsCompleted, stepLabel)
			re.logger.Infow("dry-run phase passed", "phase", phase)
			continue
		}

		if err := re.RunPhase(ctx, phase, fromVersion, targetVersion); err != nil {
			result.Error = fmt.Errorf("phase %s: %w", phase, err)
			result.Duration = time.Since(startTime)
			re.logger.Errorw("rollback failed", "phase", phase, "error", err)
			return result, result.Error
		}

		result.StepsCompleted = append(result.StepsCompleted, stepLabel)
	}

	if !re.dryRunMode {
		lockReleased = true
		if releaseErr := re.releaseRollbackLock(ctx); releaseErr != nil {
			re.logger.Errorw("failed to release rollback lock", "error", releaseErr)
		}
	}

	result.Success = true
	result.Duration = time.Since(startTime)
	re.logger.Infow("rollback completed successfully", "from", fromVersion, "to", targetVersion, "duration", result.Duration)
	return result, nil
}

func (re *RollbackEngine) dryRunPhase(ctx context.Context, phase RollbackPhase, fromVersion, toVersion int32) error {
	switch phase {
	case RBPreCheck:
		return re.ValidateRollbackTarget(ctx, toVersion)
	case RBVersionDowngrade:
		return re.validateVersionDowngrade(ctx, fromVersion, toVersion)
	case RBDataRollback:
		return nil
	case RBPostRollback:
		return nil
	case RBFinalize:
		return nil
	default:
		return nil
	}
}

func (re *RollbackEngine) validateVersionDowngrade(ctx context.Context, fromVersion, toVersion int32) error {
	migrations := Migrations()
	if len(migrations) == 0 {
		return nil
	}

	maxID := int32(migrations[len(migrations)-1].ID)
	if toVersion > maxID {
		return fmt.Errorf("target version %d exceeds maximum migration ID %d", toVersion, maxID)
	}
	return nil
}

func (re *RollbackEngine) ValidateRollbackTarget(ctx context.Context, targetVersion int32) error {
	currentVersion, err := re.GetSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	if targetVersion >= currentVersion {
		return fmt.Errorf("target version %d is not less than current version %d", targetVersion, currentVersion)
	}

	if targetVersion < 0 {
		return fmt.Errorf("target version %d cannot be negative", targetVersion)
	}

	migrations := Migrations()
	if len(migrations) > 0 {
		maxID := int32(migrations[len(migrations)-1].ID)
		if targetVersion > maxID {
			return fmt.Errorf("target version %d exceeds maximum migration ID %d", targetVersion, maxID)
		}
	}

	return nil
}

func (re *RollbackEngine) SetDryRun(dryRun bool) {
	re.dryRunMode = dryRun
}

func (re *RollbackEngine) GetSchemaVersion(ctx context.Context) (int32, error) {
	if re.versionCheckDone.Load() {
		return re.currentVersion, nil
	}

	version, err := re.versionStore.GetVersion()
	if err != nil {
		return 0, fmt.Errorf("get version from store: %w", err)
	}

	re.currentVersion = version
	re.versionCheckDone.Store(true)
	re.logger.Debugw("schema version cached", "version", version)
	return version, nil
}

func (re *RollbackEngine) acquireRollbackLock(ctx context.Context) error {
	if re.db == nil {
		return nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, re.lockTimeout)
	defer cancel()

	conn, err := re.db.Conn(timeoutCtx)
	if err != nil {
		return fmt.Errorf("get database connection: %w", err)
	}
	defer conn.Close()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for i := 0; i < 5; i++ {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("rollback lock timed out: %w", context.DeadlineExceeded)
		default:
		}

		var acquired bool
		err := conn.QueryRowContext(timeoutCtx, "SELECT pg_try_advisory_lock($1)", rollbackLockID).Scan(&acquired)
		if err != nil {
			if i < 4 {
				<-ticker.C
				continue
			}
			return fmt.Errorf("lock attempt %d/%d: %w", i+1, 5, err)
		}
		if acquired {
			re.logger.Info("rollback advisory lock acquired")
			return nil
		}
		if i < 4 {
			<-ticker.C
		}
	}

	return fmt.Errorf("rollback lock timed out after 5 attempts: %w", ErrLockTimeout)
}

func (re *RollbackEngine) releaseRollbackLock(ctx context.Context) error {
	if re.db == nil {
		return nil
	}

	conn, err := re.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get connection for lock release: %w", err)
	}
	defer conn.Close()

	var unlocked bool
	err = conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", rollbackLockID).Scan(&unlocked)
	if err != nil {
		return fmt.Errorf("release rollback lock: %w", ErrLockReleaseFailed)
	}
	if !unlocked {
		return fmt.Errorf("rollback lock was not held: %w", ErrLockHeld)
	}

	re.logger.Info("rollback advisory lock released")
	return nil
}
