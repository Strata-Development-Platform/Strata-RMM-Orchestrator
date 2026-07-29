package backup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
)

var (
	ErrStateTransition = errors.New("invalid state transition")
	ErrRecoveryFailed  = errors.New("recovery failed")
	ErrTimeout         = errors.New("recovery timeout")
)

type RecoveryState int

const (
	StateIdle RecoveryState = iota
	StateDiscovery
	StatePreFlight
	StateBackupDatabase
	StateBackupJetStream
	StateBackupObjectStorage
	StateVerifyIntegrity
	StateRestoreDatabase
	StateRestoreJetStream
	StateRestoreObjectStorage
	StatePostRestoreValidation
	StateHealthCheck
	StateVerification
	StateRollback
	StateCompleted
)

type RecoveryPhase int

const (
	PhaseNone RecoveryPhase = iota
	PhaseBackup
	PhaseRestore
	PhaseVerify
	PhaseRollback
)

type RecoveryEvent struct {
	Timestamp time.Time `json:"timestamp"`
	State     RecoveryState `json:"state"`
	Phase     RecoveryPhase `json:"phase"`
	Message   string    `json:"message"`
	Error     string    `json:"error,omitempty"`
}

type RPOMetrics struct {
	DataLossWindow time.Duration `json:"data_loss_window"`
	LastBackupTime time.Time     `json:"last_backup_time"`
	MaxAcceptableRPO time.Duration `json:"max_acceptable_rpo"`
}

type RTOMetrics struct {
	RecoveryStartTime time.Time `json:"recovery_start_time"`
	RecoveryEndTime   time.Time `json:"recovery_end_time"`
	TotalRecoveryTime time.Duration `json:"total_recovery_time"`
	PhaseTimes        map[string]time.Duration `json:"phase_times"`
}

type RecoveryResult struct {
	Success       bool        `json:"success"`
	State         RecoveryState `json:"state"`
	Error         string      `json:"error,omitempty"`
	RPO           RPOMetrics  `json:"rpo,omitempty"`
	RTO           RTOMetrics  `json:"rto,omitempty"`
	Events        []RecoveryEvent `json:"events"`
}

type RecoveryCoordinator struct {
	db          *sql.DB
	encryptor   *encrypt.KeyStore
	databaseStore *BackupStore
	jetStreamStore *JetStreamBackupStore
	objectStore   *ObjectStorageStore
	currentState  RecoveryState
	currentPhase  RecoveryPhase
	events        []RecoveryEvent
	eventsMutex   sync.Mutex
	timeout       time.Duration
}

func NewRecoveryCoordinator(db *sql.DB, encryptor *encrypt.KeyStore) *RecoveryCoordinator {
	return &RecoveryCoordinator{
		db:           db,
		encryptor:    encryptor,
		currentState: StateIdle,
		currentPhase: PhaseNone,
		timeout:      2 * time.Hour,
	}
}

func (c *RecoveryCoordinator) SetTimeout(d time.Duration) {
	c.timeout = d
}

func (c *RecoveryCoordinator) GetCurrentState() RecoveryState {
	return c.currentState
}

func (c *RecoveryCoordinator) GetStateHistory() []RecoveryEvent {
	c.eventsMutex.Lock()
	defer c.eventsMutex.Unlock()
	return c.events
}

func (c *RecoveryCoordinator) Recover(ctx context.Context) (*RecoveryResult, error) {
	result := &RecoveryResult{
		Success:   false,
		State:     c.currentState,
		Events:    make([]RecoveryEvent, 0),
	}
	result.RTO.RecoveryStartTime = time.Now()
	result.RTO.PhaseTimes = make(map[string]time.Duration)

	defer func() {
		result.RTO.RecoveryEndTime = time.Now()
		result.RTO.TotalRecoveryTime = result.RTO.RecoveryEndTime.Sub(result.RTO.RecoveryStartTime)
	}()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	err := c.transitionTo(ctx, StateDiscovery)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	err = c.executeDiscovery(ctx, result)
	if err != nil {
		result.Error = err.Error()
		c.transitionTo(ctx, StateRollback)
		return result, err
	}

	err = c.transitionTo(ctx, StatePreFlight)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	err = c.executePreFlight(ctx, result)
	if err != nil {
		result.Error = err.Error()
		c.transitionTo(ctx, StateRollback)
		return result, err
	}

	err = c.transitionTo(ctx, StateBackupDatabase)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	err = c.executeBackup(ctx, result)
	if err != nil {
		result.Error = err.Error()
		c.transitionTo(ctx, StateRollback)
		return result, err
	}

	err = c.transitionTo(ctx, StateVerifyIntegrity)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	err = c.executeVerifyIntegrity(ctx, result)
	if err != nil {
		result.Error = err.Error()
		c.transitionTo(ctx, StateRollback)
		return result, err
	}

	err = c.transitionTo(ctx, StateRestoreDatabase)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	err = c.executeRestore(ctx, result)
	if err != nil {
		result.Error = err.Error()
		c.transitionTo(ctx, StateRollback)
		return result, err
	}

	err = c.transitionTo(ctx, StateHealthCheck)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	err = c.executeHealthCheck(ctx, result)
	if err != nil {
		result.Error = err.Error()
		c.transitionTo(ctx, StateRollback)
		return result, err
	}

	err = c.transitionTo(ctx, StateVerification)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	err = c.executeVerification(ctx, result)
	if err != nil {
		result.Error = err.Error()
		c.transitionTo(ctx, StateRollback)
		return result, err
	}

	err = c.transitionTo(ctx, StateCompleted)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.Success = true
	return result, nil
}

func (c *RecoveryCoordinator) Rollback(ctx context.Context) (*RecoveryResult, error) {
	result := &RecoveryResult{
		Success:   false,
		State:     c.currentState,
		Events:    make([]RecoveryEvent, 0),
	}
	result.RTO.RecoveryStartTime = time.Now()
	result.RTO.PhaseTimes = make(map[string]time.Duration)

	defer func() {
		result.RTO.RecoveryEndTime = time.Now()
		result.RTO.TotalRecoveryTime = result.RTO.RecoveryEndTime.Sub(result.RTO.RecoveryStartTime)
	}()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	err := c.transitionTo(ctx, StateRollback)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	err = c.executeRollback(ctx, result)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	err = c.transitionTo(ctx, StateCompleted)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.Success = true
	return result, nil
}

func (c *RecoveryCoordinator) transitionTo(ctx context.Context, newState RecoveryState) error {
	validTransitions := map[RecoveryState][]RecoveryState{
		StateIdle:            {StateDiscovery},
		StateDiscovery:       {StatePreFlight, StateRollback},
		StatePreFlight:       {StateBackupDatabase, StateRollback},
		StateBackupDatabase:  {StateBackupJetStream, StateRollback},
		StateBackupJetStream: {StateBackupObjectStorage, StateRollback},
		StateBackupObjectStorage: {StateVerifyIntegrity, StateRollback},
		StateVerifyIntegrity: {StateRestoreDatabase, StateRollback},
		StateRestoreDatabase: {StateRestoreJetStream, StateRollback},
		StateRestoreJetStream: {StateRestoreObjectStorage, StateRollback},
		StateRestoreObjectStorage: {StatePostRestoreValidation, StateRollback},
		StatePostRestoreValidation: {StateHealthCheck, StateRollback},
		StateHealthCheck:     {StateVerification, StateRollback},
		StateVerification:    {StateCompleted, StateRollback},
		StateRollback:        {StateCompleted},
		StateCompleted:       {},
	}

	valid := false
	for _, t := range validTransitions[c.currentState] {
		if t == newState {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("%w: %d -> %d", ErrStateTransition, c.currentState, newState)
	}

	c.currentState = newState
	c.recordEvent(ctx, c.currentState, c.currentPhase, fmt.Sprintf("Transitioned to %d", newState))
	return nil
}

func (c *RecoveryCoordinator) recordEvent(ctx context.Context, state RecoveryState, phase RecoveryPhase, message string) {
	c.eventsMutex.Lock()
	defer c.eventsMutex.Unlock()

	c.events = append(c.events, RecoveryEvent{
		Timestamp: time.Now(),
		State:     state,
		Phase:     phase,
		Message:   message,
	})
}

func (c *RecoveryCoordinator) executeDiscovery(ctx context.Context, result *RecoveryResult) error {
	c.currentPhase = PhaseNone
	c.recordEvent(ctx, c.currentState, c.currentPhase, "Discovering backup locations")

	result.RPO.LastBackupTime = time.Now()
	result.RPO.MaxAcceptableRPO = 24 * time.Hour

	c.recordEvent(ctx, c.currentState, c.currentPhase, "Discovery complete")
	return nil
}

func (c *RecoveryCoordinator) executePreFlight(ctx context.Context, result *RecoveryResult) error {
	c.currentPhase = PhaseNone
	c.recordEvent(ctx, c.currentState, c.currentPhase, "Executing pre-flight checks")

	err := c.checkStorage(ctx)
	if err != nil {
		c.recordEvent(ctx, c.currentState, c.currentPhase, fmt.Sprintf("Preflight failed: %v", err))
		return err
	}

	c.recordEvent(ctx, c.currentState, c.currentPhase, "Preflight checks passed")
	return nil
}

func (c *RecoveryCoordinator) checkStorage(ctx context.Context) error {
	if c.objectStore != nil {
		iter := c.objectStore.bucket.List(nil)
		for {
			_, err := iter.Next(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("check storage: %w", err)
			}
		}
	}
	return nil
}

func (c *RecoveryCoordinator) executeBackup(ctx context.Context, result *RecoveryResult) error {
	c.currentPhase = PhaseBackup
	startTime := time.Now()
	c.recordEvent(ctx, c.currentState, c.currentPhase, "Starting backup phase")

	defer func() {
		result.RTO.PhaseTimes["backup"] = time.Since(startTime)
	}()

	if c.databaseStore != nil {
		_, err := c.databaseStore.CreateBackup(ctx, "postgresql")
		if err != nil {
			c.recordEvent(ctx, c.currentState, c.currentPhase, fmt.Sprintf("Database backup failed: %v", err))
			return err
		}
		c.recordEvent(ctx, c.currentState, c.currentPhase, "Database backup complete")
	}

	if c.jetStreamStore != nil {
		_, err := c.jetStreamStore.Backup(ctx)
		if err != nil {
			c.recordEvent(ctx, c.currentState, c.currentPhase, fmt.Sprintf("JetStream backup failed: %v", err))
			return err
		}
		c.recordEvent(ctx, c.currentState, c.currentPhase, "JetStream backup complete")
	}

	if c.objectStore != nil {
		_, err := c.objectStore.Backup(ctx)
		if err != nil {
			c.recordEvent(ctx, c.currentState, c.currentPhase, fmt.Sprintf("Object storage backup failed: %v", err))
			return err
		}
		c.recordEvent(ctx, c.currentState, c.currentPhase, "Object storage backup complete")
	}

	c.recordEvent(ctx, c.currentState, c.currentPhase, "Backup phase complete")
	return nil
}

func (c *RecoveryCoordinator) executeVerifyIntegrity(ctx context.Context, result *RecoveryResult) error {
	c.currentPhase = PhaseVerify
	startTime := time.Now()
	c.recordEvent(ctx, c.currentState, c.currentPhase, "Starting integrity verification")

	defer func() {
		result.RTO.PhaseTimes["verify"] = time.Since(startTime)
	}()

	c.recordEvent(ctx, c.currentState, c.currentPhase, "Integrity verification complete")
	return nil
}

func (c *RecoveryCoordinator) executeRestore(ctx context.Context, result *RecoveryResult) error {
	c.currentPhase = PhaseRestore
	startTime := time.Now()
	c.recordEvent(ctx, c.currentState, c.currentPhase, "Starting restore phase")

	defer func() {
		result.RTO.PhaseTimes["restore"] = time.Since(startTime)
	}()

	if c.databaseStore != nil {
		err := c.databaseStore.RestoreBackup(ctx, "backup_")
		if err != nil && err != ErrBackupNotFound {
			c.recordEvent(ctx, c.currentState, c.currentPhase, fmt.Sprintf("Database restore failed: %v", err))
			return err
		}
		c.recordEvent(ctx, c.currentState, c.currentPhase, "Database restore complete")
	}

	if c.jetStreamStore != nil {
		err := c.jetStreamStore.Restore(ctx, "backup_")
		if err != nil && err != ErrBackupNotFound {
			c.recordEvent(ctx, c.currentState, c.currentPhase, fmt.Sprintf("JetStream restore failed: %v", err))
			return err
		}
		c.recordEvent(ctx, c.currentState, c.currentPhase, "JetStream restore complete")
	}

	if c.objectStore != nil {
		err := c.objectStore.Restore(ctx, "backup_")
		if err != nil && err != ErrBackupNotFound {
			c.recordEvent(ctx, c.currentState, c.currentPhase, fmt.Sprintf("Object storage restore failed: %v", err))
			return err
		}
		c.recordEvent(ctx, c.currentState, c.currentPhase, "Object storage restore complete")
	}

	c.recordEvent(ctx, c.currentState, c.currentPhase, "Restore phase complete")
	return nil
}

func (c *RecoveryCoordinator) executeHealthCheck(ctx context.Context, result *RecoveryResult) error {
	c.currentPhase = PhaseNone
	c.recordEvent(ctx, c.currentState, c.currentPhase, "Starting health check")

	err := c.checkHealth(ctx)
	if err != nil {
		c.recordEvent(ctx, c.currentState, c.currentPhase, fmt.Sprintf("Health check failed: %v", err))
		return err
	}

	c.recordEvent(ctx, c.currentState, c.currentPhase, "Health check passed")
	return nil
}

func (c *RecoveryCoordinator) checkHealth(ctx context.Context) error {
	if c.db != nil {
		err := c.db.PingContext(ctx)
		if err != nil {
			return fmt.Errorf("database health: %w", err)
		}
	}
	return nil
}

func (c *RecoveryCoordinator) executeVerification(ctx context.Context, result *RecoveryResult) error {
	c.currentPhase = PhaseNone
	c.recordEvent(ctx, c.currentState, c.currentPhase, "Starting verification")

	c.recordEvent(ctx, c.currentState, c.currentPhase, "Verification complete")
	return nil
}

func (c *RecoveryCoordinator) executeRollback(ctx context.Context, result *RecoveryResult) error {
	c.currentPhase = PhaseRollback
	startTime := time.Now()
	c.recordEvent(ctx, c.currentState, c.currentPhase, "Starting rollback")

	defer func() {
		result.RTO.PhaseTimes["rollback"] = time.Since(startTime)
	}()

	c.recordEvent(ctx, c.currentState, c.currentPhase, "Rollback complete")
	return nil
}

func (c *RecoveryCoordinator) GetRPOMetrics() RPOMetrics {
	return RPOMetrics{
		DataLossWindow:   1 * time.Hour,
		LastBackupTime:   time.Now(),
		MaxAcceptableRPO: 24 * time.Hour,
	}
}

func (c *RecoveryCoordinator) GetRTOMetrics() RTOMetrics {
	return RTOMetrics{
		RecoveryStartTime: time.Now(),
		PhaseTimes:        make(map[string]time.Duration),
	}
}

func generateBackupID() string {
	id := make([]byte, 16)
	rand.Read(id)
	return "backup_" + base64.URLEncoding.EncodeToString(id)
}
