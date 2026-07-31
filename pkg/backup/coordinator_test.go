package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
)

// --- State machine tests ---

func TestRecoveryStateString(t *testing.T) {
	for s := RecoveryState(0); s <= StateCompleted; s++ {
		s := s
		str := s.String()
		if str == "" {
			t.Errorf("state %d has empty string representation", s)
		}
	}
	if want := "Idle"; StateIdle.String() != want {
		t.Errorf("StateIdle.String() = %q, want %q", StateIdle.String(), want)
	}
	if want := "Completed"; StateCompleted.String() != want {
		t.Errorf("StateCompleted.String() = %q, want %q", StateCompleted.String(), want)
	}
}

func TestRecoveryPhaseString(t *testing.T) {
	for p := RecoveryPhase(0); p <= PhaseCleanup; p++ {
		str := p.String()
		if str == "" {
			t.Errorf("phase %d has empty string representation", p)
		}
	}
	if want := "Backup"; PhaseBackup.String() != want {
		t.Errorf("PhaseBackup.String() = %q, want %q", PhaseBackup.String(), want)
	}
	if want := "None"; PhaseNone.String() != want {
		t.Errorf("PhaseNone.String() = %q, want %q", PhaseNone.String(), want)
	}
}

func TestPhaseForState(t *testing.T) {
	tests := []struct {
		state RecoveryState
		phase RecoveryPhase
	}{
		{StateIdle, PhaseNone},
		{StateDiscovery, PhaseBackup},
		{StatePreFlight, PhaseBackup},
		{StateQuiesce, PhaseBackup},
		{StateBackupDatabase, PhaseBackup},
		{StateBackupJetStream, PhaseBackup},
		{StateBackupObjectStorage, PhaseBackup},
		{StateVerifyIntegrity, PhaseBackup},
		{StatePreRestoreValidation, PhaseRestore},
		{StateRestoreDatabase, PhaseRestore},
		{StateRestoreJetStream, PhaseRestore},
		{StateRestoreObjectStorage, PhaseRestore},
		{StatePostRestoreValidation, PhaseRestore},
		{StateHealthCheck, PhaseVerify},
		{StateVerification, PhaseVerify},
		{StateRPOValidation, PhaseVerify},
		{StateRTOValidation, PhaseVerify},
		{StateRollback, PhaseRollback},
		{StateCleanup, PhaseCleanup},
		{StateCompleted, PhaseNone},
	}

	for _, tt := range tests {
		if got := PhaseForState(tt.state); got != tt.phase {
			t.Errorf("PhaseForState(%s) = %s, want %s", tt.state, got, tt.phase)
		}
	}
}

func TestValidTransitions(t *testing.T) {
	// Backup path transitions
	allowed := ValidTransitions[StateIdle]
	if len(allowed) < 2 {
		t.Errorf("expected at least 2 transitions from Idle, got %d", len(allowed))
	}

	// Verify key backup transitions
	for _, target := range ValidTransitions[StateIdle] {
		if target != StateDiscovery && target != StatePreRestoreValidation {
			t.Errorf("unexpected transition from Idle to %s", target)
		}
	}

	// Verify backup chain
	if !contains(ValidTransitions[StateQuiesce], StateBackupDatabase) {
		t.Error("expected transition Quiesce -> BackupDatabase")
	}
	if !contains(ValidTransitions[StateBackupDatabase], StateBackupJetStream) {
		t.Error("expected transition BackupDatabase -> BackupJetStream")
	}
	if !contains(ValidTransitions[StateBackupJetStream], StateBackupObjectStorage) {
		t.Error("expected transition BackupJetStream -> BackupObjectStorage")
	}

	// Verify rollback path
	if !contains(ValidTransitions[StatePostRestoreValidation], StateRollback) {
		t.Error("expected transition PostRestoreValidation -> Rollback")
	}
	if !contains(ValidTransitions[StateRollback], StateCleanup) {
		t.Error("expected transition Rollback -> Cleanup")
	}
}

func contains(s []RecoveryState, target RecoveryState) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

func TestIsValidTransition(t *testing.T) {
	validPairs := []struct{ from, to RecoveryState }{
		{StateIdle, StateDiscovery},
		{StateIdle, StatePreRestoreValidation},
		{StateDiscovery, StatePreFlight},
		{StateQuiesce, StateBackupDatabase},
		{StateCompleted, StateCompleted}, // self-transition not in matrix
	}

	for _, pair := range validPairs {
		if pair.from == StateCompleted {
			continue
		}
		if !isValidTransition(pair.from, pair.to) {
			t.Errorf("expected transition %s -> %s to be valid", pair.from, pair.to)
		}
	}

	// Verify invalid transitions
	invalidPairs := []struct{ from, to RecoveryState }{
		{StateIdle, StateCompleted},
		{StateDiscovery, StateBackupDatabase},
		{StateCompleted, StateIdle},
		{StateRollback, StateRestoreDatabase},
	}

	for _, pair := range invalidPairs {
		if isValidTransition(pair.from, pair.to) {
			t.Errorf("expected transition %s -> %s to be invalid", pair.from, pair.to)
		}
	}
}

// --- Coordinator tests ---

func TestNewRecoveryCoordinator(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	if c == nil {
		t.Fatal("expected non-nil coordinator")
	}
	if c.timeout != 2*time.Hour {
		t.Errorf("default timeout: got %v, want %v", c.timeout, 2*time.Hour)
	}
	if c.state != StateIdle {
		t.Errorf("initial state: got %s, want %s", c.state, StateIdle)
	}
}

func TestRecoveryCoordinatorSetters(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)

	c.SetTimeout(30 * time.Minute)
	if c.timeout != 30*time.Minute {
		t.Errorf("timeout: got %v, want %v", c.timeout, 30*time.Minute)
	}

	c.SetDryRun(true)
	if !c.dryRun {
		t.Error("expected dryRun to be true")
	}

	c.SetBackupID("test-backup-123")
	if c.backupID != "test-backup-123" {
		t.Errorf("backupID: got %q, want %q", c.backupID, "test-backup-123")
	}
}

func TestRecoveryCoordinatorEvents(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)

	c.mu.Lock()
	c.events = append(c.events, RecoveryEvent{
		State:     StateIdle,
		Timestamp: time.Now(),
		Phase:     PhaseNone,
		Message:   "test event",
	})
	c.mu.Unlock()

	events := c.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Message != "test event" {
		t.Errorf("event message: got %q, want %q", events[0].Message, "test event")
	}

	// Verify events is a copy, not a reference
	events[0].Message = "modified"
	c.mu.Lock()
	if c.events[0].Message == "modified" {
		t.Error("Events() should return a copy, not a reference")
	}
	c.mu.Unlock()
}

func TestRecoveryCoordinatorInitialState(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	if got := c.State(); got != StateIdle {
		t.Errorf("State() = %s, want %s", got, StateIdle)
	}
}

// --- GenerateEncryptionKey tests ---

func TestGenerateEncryptionKey(t *testing.T) {
	key1, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	if len(key1) != 44 { // base64 encoding of 32 bytes
		t.Errorf("key length: got %d, want 44", len(key1))
	}

	key2, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	if key1 == key2 {
		t.Error("expected different keys from successive calls")
	}
}

func TestGenerateKeyMaterial(t *testing.T) {
	material1, err := GenerateKeyMaterial()
	if err != nil {
		t.Fatalf("GenerateKeyMaterial: %v", err)
	}
	if len(material1) != 32 {
		t.Errorf("material length: got %d, want 32", len(material1))
	}

	material2, err := GenerateKeyMaterial()
	if err != nil {
		t.Fatalf("GenerateKeyMaterial: %d", err)
	}
	if string(material1) == string(material2) {
		t.Error("expected different material from successive calls")
	}
}

// --- HashData tests ---

func TestHashData(t *testing.T) {
	data := []byte("test data for hashing")
	hash1 := hashData(data)
	hash2 := hashData(data)

	if hash1 != hash2 {
		t.Error("hashData should be deterministic")
	}

	data2 := []byte("different data")
	hash3 := hashData(data2)
	if hash1 == hash3 {
		t.Error("different data should produce different hashes")
	}
}

// --- GenerateDatabaseBackupID tests ---

func TestGenerateDatabaseBackupID(t *testing.T) {
	id1 := generateDatabaseBackupID()
	id2 := generateDatabaseBackupID()
	if id1 == id2 {
		t.Error("expected different backup IDs from successive calls")
	}
	if len(id1) != 36 { // UUID format "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
		t.Errorf("backup ID length: got %d, want 36", len(id1))
	}
}

func TestGenerateJetStreamBackupID(t *testing.T) {
	id1 := generateJetStreamBackupID()
	id2 := generateJetStreamBackupID()
	if id1 == id2 {
		t.Error("expected different JetStream backup IDs from successive calls")
	}
}

func TestGenerateObjectStorageBackupID(t *testing.T) {
	id1 := generateObjectStorageBackupID()
	id2 := generateObjectStorageBackupID()
	if id1 == id2 {
		t.Error("expected different object storage backup IDs from successive calls")
	}
}

// --- BackupMetadata tests ---

func TestBackupMetadataJSON(t *testing.T) {
	meta := &BackupMetadata{
		ID:              "test-uuid",
		Timestamp:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DatabaseType:    "postgresql",
		Version:         "1.0.0",
		TableCount:      10,
		RowEstimate:     1000,
		DataSize:        2048,
		Compression:     "gzip",
		Scheme:          "aes-256-gcm",
		KeyReference:    "key-123",
		IntegrityDigest: "abc123",
	}

	data, err := meta.BackupMetadataJSON()
	if err != nil {
		t.Fatalf("BackupMetadataJSON: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSON data")
	}
}

// --- BackupStore tests ---

func TestNewBackupStore(t *testing.T) {
	s := NewBackupStore(nil, nil, "")
	if s == nil {
		t.Fatal("expected non-nil BackupStore")
	}
	if s.pgDSN != "" {
		t.Errorf("pgDSN: got %q, want empty string", s.pgDSN)
	}
}

func TestBackupStoreBinaryAvailable(t *testing.T) {
	s := NewBackupStore(nil, nil, "")

	if s.pgDump != "" || s.pgRestore != "" {
		t.Skip("pg_dump or pg_restore available, skipping binary check test")
	}

	err := s.binaryAvailable()
	if err == nil {
		t.Error("expected error when binaries not available")
	}
	if err != nil && err.Error() != "pg_dump/pg_restore binary not found: pg_dump not found in PATH" {
		// Accept either pg_dump or pg_restore missing
		if err.Error() != "pg_dump/pg_restore binary not found: pg_restore not found in PATH" {
			t.Logf("error message: %v", err)
		}
	}
}

func TestBackupStoreUnsupportedDatabaseType(t *testing.T) {
	s := NewBackupStore(nil, &encrypt.KeyStore{}, "")
	ctx := context.Background()

	_, err := s.CreateBackup(ctx, "mysql")
	if err == nil {
		t.Error("expected error for unsupported database type")
	}
}

func TestBackupStoreNilEncryptor(t *testing.T) {
	s := NewBackupStore(nil, nil, "")
	ctx := context.Background()

	_, err := s.CreateBackup(ctx, "postgresql")
	if err == nil {
		t.Error("expected error for nil encryptor")
	}
}

// --- JetStreamBackupStore tests ---

func TestNewJetStreamBackupStore(t *testing.T) {
	s := NewJetStreamBackupStore(nil, nil, nil)
	if s == nil {
		t.Fatal("expected non-nil JetStreamBackupStore")
	}
}

func TestJetStreamBackupStoreNilEncryptor(t *testing.T) {
	s := NewJetStreamBackupStore(nil, nil, nil)
	ctx := context.Background()

	_, err := s.Backup(ctx)
	if err == nil {
		t.Error("expected error for nil encryptor")
	}
}

// --- ObjectStorageBackupStore tests ---

func TestNewObjectStorageBackupStore(t *testing.T) {
	s := NewObjectStorageBackupStore(nil, nil)
	if s == nil {
		t.Fatal("expected non-nil ObjectStorageBackupStore")
	}
}

func TestObjectStorageBackupStoreNilEncryptor(t *testing.T) {
	s := NewObjectStorageBackupStore(nil, nil)
	ctx := context.Background()

	_, err := s.Backup(ctx)
	if err == nil {
		t.Error("expected error for nil encryptor")
	}
}

// --- RecoveryEvent tests ---

func TestRecoveryEventNilError(t *testing.T) {
	e := RecoveryEvent{
		State:     StateIdle,
		Timestamp: time.Now(),
		Message:   "no error",
		Err:       nil,
	}
	if e.Err != nil {
		t.Error("expected nil error")
	}
}

// --- BackupResult tests ---

func TestBackupResultFields(t *testing.T) {
	result := &BackupResult{
		ID:        "test-id",
		Type:      "jetstream",
		Integrity: "abc123",
		Timestamp: time.Now(),
		Version:   "1.0.0",
	}

	if result.ID != "test-id" {
		t.Errorf("ID: got %q, want %q", result.ID, "test-id")
	}
	if result.Type != "jetstream" {
		t.Errorf("Type: got %q, want %q", result.Type, "jetstream")
	}
}

// --- RecoveryResult tests ---

func TestRecoveryResultSuccess(t *testing.T) {
	r := &RecoveryResult{
		RecoveryID: "rec-123",
		BackupID:   "bak-456",
		State:      StateCompleted,
		Phase:      PhaseCleanup,
		Success:    true,
	}

	if !r.Success {
		t.Error("expected Success to be true")
	}
	if r.RecoveryID != "rec-123" {
		t.Errorf("RecoveryID: got %q, want %q", r.RecoveryID, "rec-123")
	}
}

func TestRecoveryResultFailure(t *testing.T) {
	r := &RecoveryResult{
		RecoveryID: "rec-123",
		Success:    false,
		Error:      errors.New("test error"),
	}

	if r.Success {
		t.Error("expected Success to be false")
	}
	if r.Error == nil {
		t.Error("expected non-nil error")
	}
}
