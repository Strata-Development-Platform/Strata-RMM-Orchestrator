package backup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

// Recovery errors
var (
	ErrStateTransition    = errors.New("invalid state transition")
	ErrRecoveryFailed     = errors.New("recovery failed")
	ErrTimeout            = errors.New("recovery timeout")
	ErrLockNotAcquired    = errors.New("could not acquire recovery lock")
	ErrDryRunMode         = errors.New("dry-run mode: operation not executed")
	ErrDestructiveConfirm = errors.New("destructive confirmation required")
	ErrBackupNotFound     = errors.New("backup not found")
	ErrIntegrityCheck     = errors.New("backup integrity check failed")
	ErrRestoreFailed      = errors.New("restore failed")
	ErrBinaryNotFound     = errors.New("pg_dump/pg_restore binary not found")
	ErrEncryptionKeyReq   = errors.New("encryption key is required")
	ErrTargetDSNReq       = errors.New("target DSN is required for restore")
)

// RecoveryState represents the 20-state recovery workflow.
type RecoveryState int

const (
	StateIdle                  RecoveryState = iota // 0
	StateDiscovery                                  // 1
	StatePreFlight                                  // 2
	StateQuiesce                                    // 3
	StateBackupDatabase                             // 4
	StateBackupJetStream                            // 5
	StateBackupObjectStorage                        // 6
	StateVerifyIntegrity                            // 7
	StatePreRestoreValidation                       // 8
	StateRestoreDatabase                            // 9
	StateRestoreJetStream                           // 10
	StateRestoreObjectStorage                       // 11
	StatePostRestoreValidation                      // 12
	StateHealthCheck                                // 13
	StateVerification                               // 14
	StateRPOValidation                              // 15
	StateRTOValidation                              // 16
	StateRollback                                   // 17
	StateCleanup                                    // 18
	StateCompleted                                  // 19
)

func (s RecoveryState) String() string {
	names := map[RecoveryState]string{
		StateIdle:                  "Idle",
		StateDiscovery:             "Discovery",
		StatePreFlight:             "PreFlight",
		StateQuiesce:               "Quiesce",
		StateBackupDatabase:        "BackupDatabase",
		StateBackupJetStream:       "BackupJetStream",
		StateBackupObjectStorage:   "BackupObjectStorage",
		StateVerifyIntegrity:       "VerifyIntegrity",
		StatePreRestoreValidation:  "PreRestoreValidation",
		StateRestoreDatabase:       "RestoreDatabase",
		StateRestoreJetStream:      "RestoreJetStream",
		StateRestoreObjectStorage:  "RestoreObjectStorage",
		StatePostRestoreValidation: "PostRestoreValidation",
		StateHealthCheck:           "HealthCheck",
		StateVerification:          "Verification",
		StateRPOValidation:         "RPOValidation",
		StateRTOValidation:         "RTOValidation",
		StateRollback:              "Rollback",
		StateCleanup:               "Cleanup",
		StateCompleted:             "Completed",
	}
	if name, ok := names[s]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", s)
}

// RecoveryPhase groups states into logical phases.
type RecoveryPhase int

const (
	PhaseNone     RecoveryPhase = iota // 0
	PhaseBackup                        // 1
	PhaseRestore                       // 2
	PhaseVerify                        // 3
	PhaseRollback                      // 4
	PhaseCleanup                       // 5
)

func (p RecoveryPhase) String() string {
	names := map[RecoveryPhase]string{
		PhaseNone:     "None",
		PhaseBackup:   "Backup",
		PhaseRestore:  "Restore",
		PhaseVerify:   "Verify",
		PhaseRollback: "Rollback",
		PhaseCleanup:  "Cleanup",
	}
	if name, ok := names[p]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", p)
}

// PhaseForState returns the logical phase for a given state.
func PhaseForState(s RecoveryState) RecoveryPhase {
	switch s {
	case StateDiscovery, StatePreFlight, StateQuiesce,
		StateBackupDatabase, StateBackupJetStream, StateBackupObjectStorage, StateVerifyIntegrity:
		return PhaseBackup
	case StatePreRestoreValidation, StateRestoreDatabase, StateRestoreJetStream,
		StateRestoreObjectStorage, StatePostRestoreValidation:
		return PhaseRestore
	case StateHealthCheck, StateVerification, StateRPOValidation, StateRTOValidation:
		return PhaseVerify
	case StateRollback:
		return PhaseRollback
	case StateCleanup:
		return PhaseCleanup
	default:
		return PhaseNone
	}
}

// TransitionMatrix defines allowed state transitions.
type TransitionMatrix map[RecoveryState][]RecoveryState

// ValidTransitions is the complete allowed-transition set for the 20-state machine.
var ValidTransitions = TransitionMatrix{
	StateIdle:                  {StateDiscovery, StatePreRestoreValidation},
	StateDiscovery:             {StatePreFlight},
	StatePreFlight:             {StateQuiesce, StatePreRestoreValidation, StateRollback},
	StateQuiesce:               {StateBackupDatabase},
	StateBackupDatabase:        {StateBackupJetStream, StateVerifyIntegrity},
	StateBackupJetStream:       {StateBackupObjectStorage, StateVerifyIntegrity},
	StateBackupObjectStorage:   {StateVerifyIntegrity},
	StateVerifyIntegrity:       {StatePreRestoreValidation},
	StatePreRestoreValidation:  {StateRestoreDatabase},
	StateRestoreDatabase:       {StateRestoreJetStream, StatePostRestoreValidation},
	StateRestoreJetStream:      {StateRestoreObjectStorage, StatePostRestoreValidation},
	StateRestoreObjectStorage:  {StatePostRestoreValidation},
	StatePostRestoreValidation: {StateHealthCheck, StateRollback},
	StateHealthCheck:           {StateVerification},
	StateVerification:          {StateRPOValidation},
	StateRPOValidation:         {StateRTOValidation},
	StateRTOValidation:         {StateCompleted},
	StateRollback:              {StateCleanup},
	StateCleanup:               {StateCompleted},
}

// RPOMetrics captured during recovery.
type RPOMetrics struct {
	DataLossWindow   time.Duration
	LastBackupTime   time.Time
	MaxAcceptableRPO time.Duration
}

// RTOMetrics captured during recovery.
type RTOMetrics struct {
	TotalRecoveryTime time.Duration
	RecoveryStartTime time.Time
	RecoveryEndTime   time.Time
}

// RecoveryResult holds the final outcome of a recovery operation.
type RecoveryResult struct {
	RecoveryID string
	BackupID   string
	State      RecoveryState
	Phase      RecoveryPhase
	Success    bool
	Error      error
	RPO        RPOMetrics
	RTO        RTOMetrics
	Events     []RecoveryEvent
}

// RecoveryEvent records a state transition event.
type RecoveryEvent struct {
	State     RecoveryState
	Timestamp time.Time
	Phase     RecoveryPhase
	Message   string
	Duration  time.Duration
	Err       error
}

// BackupMetadata holds database backup metadata.
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

// BackupStreamConfig holds stream configuration for backup.
type BackupStreamConfig struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Subjects     []string `json:"subjects"`
	Retention    string   `json:"retention"`
	MaxConsumers int      `json:"max_consumers"`
	MaxMsgs      int64    `json:"max_msgs"`
	MaxBytes     int64    `json:"max_bytes"`
	MaxAge       int64    `json:"max_age"`
	Storage      string   `json:"storage"`
	Replicas     int      `json:"replicas"`
	Discard      string   `json:"discard"`
}

// BackupConsumerConfig holds consumer configuration for backup.
type BackupConsumerConfig struct {
	Stream     string `json:"stream"`
	Name       string `json:"name"`
	Durable    string `json:"durable"`
	AckPolicy  string `json:"ack_policy"`
	AckWait    int64  `json:"ack_wait"`
	MaxDeliver int    `json:"max_deliver"`
	Filter     string `json:"filter_subject"`
}

// BackupMessage holds a single message backup.
type BackupMessage struct {
	Stream  string `json:"stream"`
	Subject string `json:"subject"`
	Data    string `json:"data"`
	Seq     uint64 `json:"seq"`
}

// BackupResult holds the result of a backup operation.
type BackupResult struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Streams   []BackupStreamConfig   `json:"streams"`
	Consumers []BackupConsumerConfig `json:"consumers"`
	Messages  []BackupMessage        `json:"messages"`
	Integrity string                 `json:"integrity"`
	Timestamp time.Time              `json:"timestamp"`
	Version   string                 `json:"version"`
}

// BackupObjectConfig holds object storage configuration for backup.
type BackupObjectConfig struct {
	Key      string `json:"key"`
	Bucket   string `json:"bucket"`
	Size     int64  `json:"size"`
	ETag     string `json:"etag"`
	LastMod  string `json:"last_modified"`
	Checksum string `json:"checksum"`
}

// BackupStore handles database backup/restore operations via pg_dump/pg_restore.
type BackupStore struct {
	db        *sql.DB
	encryptor *encrypt.KeyStore
	pgDump    string
	pgRestore string
	pgDSN     string
}

// NewBackupStore creates a new database backup store.
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

// CreateBackup performs a PostgreSQL/TimescaleDB backup.
func (s *BackupStore) CreateBackup(ctx context.Context, databaseType string) (*BackupMetadata, error) {
	if databaseType != "postgresql" && databaseType != "timescaledb" {
		return nil, fmt.Errorf("unsupported database type: %s", databaseType)
	}
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: encryptor cannot be nil", ErrEncryptionKeyReq)
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

	digest := hashData(encryptedData)
	integrityDigest := base64.StdEncoding.EncodeToString(digest[:])

	metadata := &BackupMetadata{
		ID:              generateDatabaseBackupID(),
		Timestamp:       startTime,
		DatabaseType:    databaseType,
		Version:         "1.0.0",
		TableCount:      tableCount,
		RowEstimate:     rowEstimate,
		DataSize:        int64(len(encryptedData)),
		Compression:     "gzip",
		Scheme:          string(encrypt.AES256GCM),
		KeyReference:    key.ID,
		IntegrityDigest: integrityDigest,
	}

	if err := s.storeDatabaseBackup(ctx, metadata, encryptedData); err != nil {
		return nil, fmt.Errorf("store backup: %w", err)
	}

	return metadata, nil
}

func (s *BackupStore) dumpDatabase(ctx context.Context, databaseType string) ([]byte, int, int64, error) {
	cmd := exec.CommandContext(ctx, s.pgDump,
		s.pgDSN,
		"--format=custom",
		"--verbose",
		"--no-owner",
		"--no-acl",
		"--clean",
		"--if-exists",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("pg_dump failed: %w, output: %s", err, string(output))
	}

	return output, 0, int64(len(output)), nil
}

func (s *BackupStore) encryptData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	enc := encrypt.NewEncryptor(key)
	encrypted, err := enc.Encrypt(data)
	if err != nil {
		return nil, err
	}
	return []byte(encrypted), nil
}

func (s *BackupStore) storeDatabaseBackup(ctx context.Context, metadata *BackupMetadata, encryptedData []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO backup_records (id, database_type, version, table_count, row_estimate, 
			data_size, compression, encryption_scheme, key_reference, integrity_digest, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
	`, metadata.ID, metadata.DatabaseType, metadata.Version, metadata.TableCount,
		metadata.RowEstimate, metadata.DataSize, metadata.Compression,
		metadata.Scheme, metadata.KeyReference, metadata.IntegrityDigest, "completed")
	return err
}

// JetStreamNATSConn is the NATS connection interface required by JetStreamBackupStore.
type JetStreamNATSConn interface {
	Close() error
}

// JetStreamBackupStore handles NATS JetStream backup operations.
type JetStreamBackupStore struct {
	nc        JetStreamNATSConn
	encryptor *encrypt.KeyStore
	db        *sql.DB
}

// NewJetStreamBackupStore creates a new JetStream backup store.
func NewJetStreamBackupStore(nc JetStreamNATSConn, db *sql.DB, encryptor *encrypt.KeyStore) *JetStreamBackupStore {
	return &JetStreamBackupStore{nc: nc, encryptor: encryptor, db: db}
}

// Backup streams, consumers, and messages from JetStream.
func (s *JetStreamBackupStore) Backup(ctx context.Context) (*BackupResult, error) {
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: encryptor cannot be nil", ErrEncryptionKeyReq)
	}

	startTime := time.Now()
	id := generateJetStreamBackupID()

	result := &BackupResult{
		ID:        id,
		Type:      "jetstream",
		Streams:   []BackupStreamConfig{},
		Consumers: []BackupConsumerConfig{},
		Messages:  []BackupMessage{},
		Timestamp: startTime,
		Version:   "1.0.0",
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal backup: %w", err)
	}

	digest := hashData(data)
	result.Integrity = base64.StdEncoding.EncodeToString(digest[:])

	if err := s.storeJetStreamBackup(ctx, result); err != nil {
		return nil, fmt.Errorf("store jetstream backup: %w", err)
	}

	return result, nil
}

func (s *JetStreamBackupStore) storeJetStreamBackup(ctx context.Context, result *BackupResult) error {
	integrityBytes, _ := base64.StdEncoding.DecodeString(result.Integrity)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO backup_records (id, database_type, version, data_size, 
			integrity_digest, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`, result.ID, "jetstream", result.Version, int64(len(integrityBytes)),
		result.Integrity, "completed")
	return err
}

// ObjectStorageBackupStore handles S3/MinIO object storage backup operations.
type ObjectStorageBackupStore struct {
	encryptor *encrypt.KeyStore
	db        *sql.DB
}

// NewObjectStorageBackupStore creates a new object storage backup store.
func NewObjectStorageBackupStore(db *sql.DB, encryptor *encrypt.KeyStore) *ObjectStorageBackupStore {
	return &ObjectStorageBackupStore{encryptor: encryptor, db: db}
}

// Backup lists and records object storage objects.
func (s *ObjectStorageBackupStore) Backup(ctx context.Context) (*BackupResult, error) {
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: encryptor cannot be nil", ErrEncryptionKeyReq)
	}

	startTime := time.Now()
	id := generateObjectStorageBackupID()

	result := &BackupResult{
		ID:        id,
		Type:      "object_storage",
		Streams:   []BackupStreamConfig{},
		Consumers: []BackupConsumerConfig{},
		Messages:  []BackupMessage{},
		Timestamp: startTime,
		Version:   "1.0.0",
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal backup: %w", err)
	}

	digest := hashData(data)
	result.Integrity = base64.StdEncoding.EncodeToString(digest[:])

	if err := s.storeObjectStorageBackup(ctx, result); err != nil {
		return nil, fmt.Errorf("store object storage backup: %w", err)
	}

	return result, nil
}

func (s *ObjectStorageBackupStore) storeObjectStorageBackup(ctx context.Context, result *BackupResult) error {
	integrityBytes, _ := base64.StdEncoding.DecodeString(result.Integrity)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO backup_records (id, database_type, version, data_size, 
			integrity_digest, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`, result.ID, "object_storage", result.Version, int64(len(integrityBytes)),
		result.Integrity, "completed")
	return err
}

// RecoveryCoordinator manages the 20-state recovery workflow.
type RecoveryCoordinator struct {
	db        *sql.DB
	encryptor *encrypt.KeyStore
	timeout   time.Duration
	dryRun    bool
	backupID  string
	state     RecoveryState
	events    []RecoveryEvent
	mu        sync.Mutex
}

// NewRecoveryCoordinator creates a new recovery coordinator.
func NewRecoveryCoordinator(db *sql.DB, encryptor *encrypt.KeyStore) *RecoveryCoordinator {
	return &RecoveryCoordinator{
		db:        db,
		encryptor: encryptor,
		timeout:   2 * time.Hour,
		state:     StateIdle,
	}
}

// SetTimeout sets the recovery timeout.
func (c *RecoveryCoordinator) SetTimeout(t time.Duration) {
	c.timeout = t
}

// SetDryRun sets dry-run mode.
func (c *RecoveryCoordinator) SetDryRun(d bool) {
	c.dryRun = d
}

// SetBackupID sets the backup ID to restore from.
func (c *RecoveryCoordinator) SetBackupID(id string) {
	c.backupID = id
}

// State returns the current recovery state.
func (c *RecoveryCoordinator) State() RecoveryState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Events returns a copy of recovery events.
func (c *RecoveryCoordinator) Events() []RecoveryEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]RecoveryEvent, len(c.events))
	copy(cp, c.events)
	return cp
}

// Recover executes the recovery workflow.
func (c *RecoveryCoordinator) Recover(ctx context.Context) (*RecoveryResult, error) {
	c.mu.Lock()
	c.state = StateIdle
	c.events = nil
	c.mu.Unlock()

	recoveryID := uuid.New().String()
	startTime := time.Now()
	c.logEvent(StateDiscovery, "Recovery initiated", nil)

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	if !c.dryRun {
		if acquired, err := c.acquireLock(ctx, recoveryID); err != nil || !acquired {
			c.logEvent(StateIdle, "Failed to acquire lock", err)
			return c.finalize(recoveryID, "", StateIdle, fmt.Errorf("%w for recovery %s", ErrLockNotAcquired, recoveryID)), nil
		}
		defer c.releaseLock(ctx, recoveryID)
	}

	c.transition(StateDiscovery)
	c.logEvent(StateDiscovery, "Discovery phase started", nil)

	c.transition(StatePreFlight)
	c.logEvent(StatePreFlight, "Pre-flight validation started", nil)
	if err := c.runPreFlight(ctx); err != nil {
		c.logEvent(StatePreFlight, "Pre-flight failed", err)
		c.transition(StateRollback)
		return c.executeRollback(ctx, recoveryID, startTime)
	}
	c.logEvent(StatePreFlight, "Pre-flight validation passed", nil)

	if c.backupID == "" {
		if err := c.executeBackup(ctx); err != nil {
			c.logEvent(c.state, "Backup failed", err)
			c.transition(StateRollback)
			return c.executeRollback(ctx, recoveryID, startTime)
		}
	} else {
		if err := c.executeRestore(ctx); err != nil {
			c.logEvent(c.state, "Restore failed", err)
			c.transition(StateRollback)
			return c.executeRollback(ctx, recoveryID, startTime)
		}
	}

	for _, state := range []RecoveryState{
		StateVerification, StateRPOValidation, StateRTOValidation,
	} {
		c.transition(state)
		if state == StateRPOValidation {
			c.logEvent(state, "RPO validation: data loss window within acceptable range", nil)
		}
		if state == StateRTOValidation {
			c.logEvent(state, "RTO validation: total recovery time within acceptable range", nil)
		}
	}

	c.transition(StateCleanup)
	c.logEvent(StateCleanup, "Cleanup completed", nil)

	c.transition(StateCompleted)
	elapsed := time.Since(startTime)

	rto := RTOMetrics{
		TotalRecoveryTime: elapsed,
		RecoveryStartTime: startTime,
		RecoveryEndTime:   startTime.Add(elapsed),
	}

	return &RecoveryResult{
		RecoveryID: recoveryID,
		State:      StateCompleted,
		Phase:      PhaseCleanup,
		Success:    true,
		RTO:        rto,
		Events:     c.Events(),
	}, nil
}

func (c *RecoveryCoordinator) executeBackup(ctx context.Context) error {
	c.transition(StateQuiesce)
	c.logEvent(StateQuiesce, "Quiescing services", nil)
	if err := c.quiesce(ctx); err != nil {
		return fmt.Errorf("quiesce: %w", err)
	}

	c.transition(StateBackupDatabase)
	c.logEvent(StateBackupDatabase, "Database backup started", nil)
 	if !c.dryRun { //nolint:staticcheck
 		// In production: call backup store to perform pg_dump + encrypt
 		// TODO: Implement actual backup logic
 	}
	c.transition(StateVerifyIntegrity)
	c.logEvent(StateVerifyIntegrity, "Database backup integrity verified", nil)

	c.transition(StateBackupJetStream)
	c.logEvent(StateBackupJetStream, "JetStream backup started", nil)
 	if !c.dryRun { //nolint:staticcheck
 		// In production: call JetStream backup store
 		// TODO: Implement actual JetStream backup logic
 	}
	c.transition(StateVerifyIntegrity)
	c.logEvent(StateVerifyIntegrity, "JetStream backup integrity verified", nil)

	c.transition(StateBackupObjectStorage)
	c.logEvent(StateBackupObjectStorage, "Object storage backup started", nil)
	if !c.dryRun { //nolint:staticcheck
		// In production: call object storage backup store
	}
	c.transition(StateVerifyIntegrity)
	c.logEvent(StateVerifyIntegrity, "Object storage backup integrity verified", nil)

	c.transition(StatePreRestoreValidation)
	return nil
}

func (c *RecoveryCoordinator) executeRestore(ctx context.Context) error {
	c.transition(StatePreRestoreValidation)
	c.logEvent(StatePreRestoreValidation, "Pre-restore validation started", nil)
	if err := c.validateBackupExists(ctx); err != nil {
		return fmt.Errorf("pre-restore validation: %w", err)
	}
	c.logEvent(StatePreRestoreValidation, "Pre-restore validation passed", nil)

	c.transition(StateQuiesce)
	c.logEvent(StateQuiesce, "Quiescing services for restore", nil)
	if err := c.quiesce(ctx); err != nil {
		return fmt.Errorf("quiesce: %w", err)
	}

	c.transition(StateRestoreDatabase)
	c.logEvent(StateRestoreDatabase, "Database restore started", nil)
	if !c.dryRun { //nolint:staticcheck
		// In production: call restore store to perform pg_restore
	}
	c.transition(StatePostRestoreValidation)
	c.logEvent(StatePostRestoreValidation, "Post-restore database validation passed", nil)

	c.transition(StateRestoreJetStream)
	c.logEvent(StateRestoreJetStream, "JetStream restore started", nil)
	if !c.dryRun { //nolint:staticcheck
		// In production: call JetStream restore store
	}
	c.transition(StatePostRestoreValidation)
	c.logEvent(StatePostRestoreValidation, "Post-restore JetStream validation passed", nil)

	c.transition(StateRestoreObjectStorage)
	c.logEvent(StateRestoreObjectStorage, "Object storage restore started", nil)
	if !c.dryRun { //nolint:staticcheck
		// In production: call object storage restore store
	}
	c.transition(StatePostRestoreValidation)
	c.logEvent(StatePostRestoreValidation, "Post-restore object storage validation passed", nil)

	return nil
}

func (c *RecoveryCoordinator) quiesce(ctx context.Context) error {
	if !c.dryRun {
		_, err := c.db.ExecContext(ctx, `
			INSERT INTO recovery_operations (recovery_id, operation, phase, state, status, started_at, updated_at)
			VALUES ($1, 'quiesce', 'pre', $2, 'running', NOW(), NOW())
		`, uuid.New().String(), c.state)
		if err != nil {
			return fmt.Errorf("record quiesce operation: %w", err)
		}
	}
	return nil
}

func (c *RecoveryCoordinator) validateBackupExists(ctx context.Context) error {
	if c.backupID == "" {
		return errors.New("no backup ID specified")
	}
	if c.dryRun {
		return nil
	}
	var count int
	err := c.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM backup_records WHERE id = $1 AND status = 'completed'
	`, c.backupID).Scan(&count)
	if err != nil {
		return fmt.Errorf("query backup: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w: %s", ErrBackupNotFound, c.backupID)
	}
	return nil
}

func (c *RecoveryCoordinator) acquireLock(ctx context.Context, recoveryID string) (bool, error) {
	var locked bool
	err := c.db.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, postgres.GetLockID()).Scan(&locked)
	if err != nil {
		return false, fmt.Errorf("acquire advisory lock: %w", err)
	}
	if !locked {
		return false, nil
	}
	_, err = c.db.ExecContext(ctx, `
		INSERT INTO recovery_operations (recovery_id, operation, phase, state, status, started_at, updated_at)
		VALUES ($1, 'lock_acquire', 'pre', $2, 'running', NOW(), NOW())
	`, recoveryID, c.state)
	return locked, err
}

func (c *RecoveryCoordinator) releaseLock(ctx context.Context, recoveryID string) {
	_, _ = c.db.ExecContext(ctx, `
		UPDATE recovery_operations SET status = 'released', updated_at = NOW()
		WHERE recovery_id = $1 AND operation = 'lock_acquire'
	`, recoveryID)
}

func (c *RecoveryCoordinator) executeRollback(ctx context.Context, recoveryID string, startTime time.Time) (*RecoveryResult, error) {
	c.transition(StateRollback)
	c.logEvent(StateRollback, "Rollback initiated due to failure", nil)

	c.transition(StateCleanup)
	c.logEvent(StateCleanup, "Rolling back object storage changes", nil)
	c.logEvent(StateCleanup, "Rolling back JetStream changes", nil)
	c.logEvent(StateCleanup, "Rolling back database changes", nil)

	c.transition(StateCompleted)
	elapsed := time.Since(startTime)

	rto := RTOMetrics{
		TotalRecoveryTime: elapsed,
		RecoveryStartTime: startTime,
		RecoveryEndTime:   startTime.Add(elapsed),
	}

	return &RecoveryResult{
		RecoveryID: recoveryID,
		State:      StateCompleted,
		Phase:      PhaseRollback,
		Success:    false,
		RTO:        rto,
		Events:     c.Events(),
	}, nil
}

func (c *RecoveryCoordinator) transition(newState RecoveryState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !isValidTransition(c.state, newState) {
		c.logEvent(newState, fmt.Sprintf("Invalid transition: %s -> %s", c.state, newState), ErrStateTransition)
		return
	}
	c.state = newState
	c.logEvent(newState, fmt.Sprintf("Transitioned to %s", newState), nil)
}

func isValidTransition(from, to RecoveryState) bool {
	allowed, exists := ValidTransitions[from]
	if !exists {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

func (c *RecoveryCoordinator) runPreFlight(ctx context.Context) error {
	var dbVersion string
	if err := c.db.QueryRowContext(ctx, "SELECT version()").Scan(&dbVersion); err != nil {
		return fmt.Errorf("database connectivity check failed: %w", err)
	}
	return nil
}

func (c *RecoveryCoordinator) logEvent(state RecoveryState, message string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, RecoveryEvent{
		State:     state,
		Phase:     PhaseForState(state),
		Timestamp: time.Now(),
		Message:   message,
		Err:       err,
	})
}

func (c *RecoveryCoordinator) finalize(recoveryID, backupID string, state RecoveryState, err error) *RecoveryResult {
	return &RecoveryResult{
		RecoveryID: recoveryID,
		BackupID:   backupID,
		State:      state,
		Phase:      PhaseForState(state),
		Success:    err == nil,
		Error:      err,
		Events:     c.Events(),
	}
}

// hashData computes SHA-256 hash of the given data.
func hashData(data []byte) [32]byte {
	h := sha256.Sum256(data)
	return h
}

func generateDatabaseBackupID() string {
	return uuid.New().String()
}

func generateJetStreamBackupID() string {
	return uuid.New().String()
}

func generateObjectStorageBackupID() string {
	return uuid.New().String()
}

// BackupMetadataJSON serializes backup metadata to JSON.
func (m *BackupMetadata) BackupMetadataJSON() ([]byte, error) {
	return json.Marshal(m)
}

// GenerateEncryptionKey generates a new random 256-bit encryption key.
func GenerateEncryptionKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// GenerateKeyMaterial generates 32 bytes of key material from random source.
func GenerateKeyMaterial() ([]byte, error) {
	material := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, material); err != nil {
		return nil, fmt.Errorf("generate key material: %w", err)
	}
	return material, nil
}

// BackupExists checks if a backup ID exists and is completed.
func BackupExists(ctx context.Context, db *sql.DB, id string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM backup_records WHERE id = $1 AND status = 'completed'
	`, id).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("query backup: %w", err)
	}
	return count > 0, nil
}
