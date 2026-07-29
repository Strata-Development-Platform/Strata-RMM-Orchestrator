package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func newTestRollbackLogger() *zap.SugaredLogger {
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

// ---------------------------------------------------------------------------
// Test: basic rollback succeeds with all hooks registered
// ---------------------------------------------------------------------------
func TestRunRollback_Success(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	for _, phase := range rollbackPhases {
		eng.RegisterHook(phase, func(ctx context.Context, fromVersion, toVersion int32) error {
			return nil
		})
	}

	ctx := context.Background()
	result, err := eng.RunRollback(ctx, 5)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Fatal("expected rollback to succeed")
	}
	if result.FromVersion != 10 {
		t.Errorf("expected from version 10, got %d", result.FromVersion)
	}
	if result.ToVersion != 5 {
		t.Errorf("expected to version 5, got %d", result.ToVersion)
	}
	if len(result.StepsCompleted) != 5 {
		t.Errorf("expected 5 steps completed, got %d", len(result.StepsCompleted))
	}
	if result.DryRun {
		t.Error("expected DryRun to be false")
	}
}

// ---------------------------------------------------------------------------
// Test: dry-run mode does not execute but validates
// ---------------------------------------------------------------------------
func TestRunRollback_DryRunMode(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	var hookExecuted bool
	for _, phase := range rollbackPhases {
		p := phase
		eng.RegisterHook(p, func(ctx context.Context, fromVersion, toVersion int32) error {
			hookExecuted = true
			return nil
		})
	}

	eng.SetDryRun(true)
	ctx := context.Background()
	result, err := eng.RunRollback(ctx, 5)
	if err != nil {
		t.Fatalf("expected no error in dry-run, got: %v", err)
	}
	if !result.Success {
		t.Fatal("expected dry-run rollback to succeed")
	}
	if !result.DryRun {
		t.Error("expected DryRun to be true")
	}
	if len(result.StepsCompleted) != 5 {
		t.Errorf("expected 5 dry-run steps, got %d", len(result.StepsCompleted))
	}
	if hookExecuted {
		t.Error("expected dry-run to NOT execute hooks")
	}
	if result.FromVersion != 10 {
		t.Errorf("expected from version 10, got %d", result.FromVersion)
	}
	if result.ToVersion != 5 {
		t.Errorf("expected to version 5, got %d", result.ToVersion)
	}
}

// ---------------------------------------------------------------------------
// Test: missing hooks fall back to defaults
// ---------------------------------------------------------------------------
func TestRunRollback_MissingHooks_UsesDefaults(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	ctx := context.Background()
	result, err := eng.RunRollback(ctx, 5)
	if err != nil {
		t.Fatalf("expected no error with default hooks, got: %v", err)
	}
	if !result.Success {
		t.Fatal("expected rollback to succeed with default hooks")
	}
}

// ---------------------------------------------------------------------------
// Test: context cancellation during rollback
// ---------------------------------------------------------------------------
func TestRunRollback_ContextCancellation(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	eng.RegisterHook(RBVersionDowngrade, func(ctx context.Context, fromVersion, toVersion int32) error {
		<-ctx.Done()
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := eng.RunRollback(ctx, 5)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
}

// ---------------------------------------------------------------------------
// Test: concurrent rollback prevention
// ---------------------------------------------------------------------------
func TestRunRollback_ConcurrentPrevention(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(20)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	for _, phase := range rollbackPhases {
		p := phase
		eng.RegisterHook(p, func(ctx context.Context, fromVersion, toVersion int32) error {
			time.Sleep(20 * time.Millisecond)
			return nil
		})
	}

	ctx := context.Background()
	done := make(chan bool, 2)

	go func() {
		r, e := eng.RunRollback(ctx, 15)
		done <- r != nil && e == nil
	}()

	time.Sleep(10 * time.Millisecond)

	go func() {
		r, e := eng.RunRollback(ctx, 10)
		done <- r != nil && e == nil
	}()

	for i := 0; i < 2; i++ {
		select {
		case ok := <-done:
			if !ok {
				t.Errorf("rollback %d did not complete successfully", i+1)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("rollback timed out waiting for completion")
		}
	}
}

// ---------------------------------------------------------------------------
// Test: version validation - target >= current rejected
// ---------------------------------------------------------------------------
func TestValidateRollbackTarget_RejectsTargetEqualsCurrent(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	ctx := context.Background()
	err := eng.ValidateRollbackTarget(ctx, 10)
	if err == nil {
		t.Fatal("expected error for target version equal to current")
	}
}

func TestValidateRollbackTarget_RejectsTargetGreaterThanCurrent(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	ctx := context.Background()
	err := eng.ValidateRollbackTarget(ctx, 15)
	if err == nil {
		t.Fatal("expected error for target version greater than current")
	}
}

func TestValidateRollbackTarget_RejectsNegativeVersion(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	ctx := context.Background()
	err := eng.ValidateRollbackTarget(ctx, -1)
	if err == nil {
		t.Fatal("expected error for negative target version")
	}
}

// ---------------------------------------------------------------------------
// Test: nested rollback validation - downgrade path correctness
// ---------------------------------------------------------------------------
func TestRunRollback_NestedRollbackValidation(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	// First rollback to version 8
	result1, err := eng.RunRollback(context.Background(), 8)
	if err != nil {
		t.Fatalf("first rollback failed: %v", err)
	}
	if !result1.Success {
		t.Fatal("first rollback should succeed")
	}

	// Second rollback to version 5 (nested scenario)
	result2, err := eng.RunRollback(context.Background(), 5)
	if err != nil {
		t.Fatalf("second rollback failed: %v", err)
	}
	if !result2.Success {
		t.Fatal("second rollback should succeed")
	}
	if result2.FromVersion != 8 {
		t.Errorf("expected second rollback from version 8, got %d", result2.FromVersion)
	}
	if result2.ToVersion != 5 {
		t.Errorf("expected second rollback to version 5, got %d", result2.ToVersion)
	}

	// Verify dry-run during nested rollback
	eng.SetDryRun(true)
	result3, err := eng.RunRollback(context.Background(), 3)
	if err != nil {
		t.Fatalf("dry-run rollback failed: %v", err)
	}
	if !result3.Success {
		t.Fatal("dry-run nested rollback should succeed")
	}
	if !result3.DryRun {
		t.Error("expected DryRun to be true for dry-run nested rollback")
	}
}

// ---------------------------------------------------------------------------
// Test: RunPhase executes hook or default
// ---------------------------------------------------------------------------
func TestRunPhase_HookExecution(t *testing.T) {
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(nil, newTestRollbackLogger(), store)

	var called bool
	eng.RegisterHook(RBVersionDowngrade, func(ctx context.Context, fromVersion, toVersion int32) error {
		called = true
		return nil
	})

	ctx := context.Background()
	err := eng.RunPhase(ctx, RBVersionDowngrade, 10, 5)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !called {
		t.Fatal("expected hook to be called")
	}
}

// ---------------------------------------------------------------------------
// Test: RunPhase with missing hook uses default
// ---------------------------------------------------------------------------
func TestRunPhase_DefaultPhase(t *testing.T) {
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(nil, newTestRollbackLogger(), store)

	ctx := context.Background()
	err := eng.RunPhase(ctx, RBVersionDowngrade, 10, 5)
	if err != nil {
		t.Fatalf("expected no error with default, got: %v", err)
	}
}
// ---------------------------------------------------------------------------
// Test: version store error propagation
// ---------------------------------------------------------------------------
func TestRunRollback_VersionStoreError(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	store.getErr = errGetVersion
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	ctx := context.Background()
	_, err := eng.RunRollback(ctx, 5)
	if err == nil {
		t.Fatal("expected error when version store fails")
	}
}

// ---------------------------------------------------------------------------
// Test: RollbackResult contains audit trail
// ---------------------------------------------------------------------------
func TestRollbackResult_AuditTrail(t *testing.T) {
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(nil, newTestRollbackLogger(), store)

	for _, phase := range rollbackPhases {
		eng.RegisterHook(phase, func(ctx context.Context, fromVersion, toVersion int32) error {
			return nil
		})
	}

	ctx := context.Background()
	result, err := eng.RunRollback(ctx, 5)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(result.StepsCompleted) == 0 {
		t.Fatal("expected audit trail steps to be recorded")
	}

	for i, step := range result.StepsCompleted {
		expected := "phase:" + string(rollbackPhases[i])
		if step != expected {
			t.Errorf("expected step %d to be %s, got %s", i, expected, step)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: hook execution order matches rollbackPhases slice
// ---------------------------------------------------------------------------
func TestRollbackHookExecutionOrder(t *testing.T) {
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(nil, newTestRollbackLogger(), store)

	var executed []string
	var mu sync.Mutex

	for _, phase := range rollbackPhases {
		p := phase
		eng.RegisterHook(p, func(ctx context.Context, fromVersion, toVersion int32) error {
			mu.Lock()
			executed = append(executed, string(p))
			mu.Unlock()
			return nil
		})
	}

	ctx := context.Background()
	result, err := eng.RunRollback(ctx, 5)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Fatal("expected rollback to succeed")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(executed) != 5 {
		t.Fatalf("expected 5 phases executed, got %d: %v", len(executed), executed)
	}

	expectedOrder := []string{"pre_check", "version_downgrade", "data_rollback", "post_rollback", "finalize"}
	for i, exp := range expectedOrder {
		if executed[i] != exp {
			t.Errorf("expected phase %d to be %s, got %s", i, exp, executed[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Test: SetDryRun toggles correctly
// ---------------------------------------------------------------------------
func TestSetDryRun_Toggles(t *testing.T) {
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(nil, newTestRollbackLogger(), store)

	if eng.dryRunMode {
		t.Fatal("expected dryRunMode to be false by default")
	}

	eng.SetDryRun(true)
	if !eng.dryRunMode {
		t.Fatal("expected dryRunMode to be true after SetDryRun(true)")
	}

	eng.SetDryRun(false)
	if eng.dryRunMode {
		t.Fatal("expected dryRunMode to be false after SetDryRun(false)")
	}
}

// ---------------------------------------------------------------------------
// Test: rollback to version 0 is valid
// ---------------------------------------------------------------------------
func TestRunRollback_ToVersionZero(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(3)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	for _, phase := range rollbackPhases {
		eng.RegisterHook(phase, func(ctx context.Context, fromVersion, toVersion int32) error {
			return nil
		})
	}

	ctx := context.Background()
	result, err := eng.RunRollback(ctx, 0)
	if err != nil {
		t.Fatalf("expected no error rolling back to version 0, got: %v", err)
	}
	if !result.Success {
		t.Fatal("expected rollback to version 0 to succeed")
	}
	if result.FromVersion != 3 {
		t.Errorf("expected from version 3, got %d", result.FromVersion)
	}
	if result.ToVersion != 0 {
		t.Errorf("expected to version 0, got %d", result.ToVersion)
	}
}

// ---------------------------------------------------------------------------
// Test: pre-check fails when current version exceeds max migration ID
// ---------------------------------------------------------------------------
func TestRunRollback_PreCheckFailsOnVersionMismatch(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(999)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	for _, phase := range rollbackPhases {
		eng.RegisterHook(phase, func(ctx context.Context, fromVersion, toVersion int32) error {
			return nil
		})
	}

	ctx := context.Background()
	_, err := eng.RunRollback(ctx, 5)
	if err == nil {
		t.Fatal("expected error when current version exceeds max migration")
	}
}

// ---------------------------------------------------------------------------
// Test: pre-check phase always runs even without hook
// ---------------------------------------------------------------------------
func TestRunPhase_PreCheckAlwaysRuns(t *testing.T) {
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(nil, newTestRollbackLogger(), store)

	// Register only non-pre-check hooks to verify preCheck runs by default
	eng.RegisterHook(RBVersionDowngrade, func(ctx context.Context, fromVersion, toVersion int32) error {
		return nil
	})

	ctx := context.Background()
	err := eng.RunPhase(ctx, RBPreCheck, 10, 5)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: rollback result has Error set on failure
// ---------------------------------------------------------------------------
func TestRollbackResult_ErrorOnFailure(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	eng.RegisterHook(RBPreCheck, func(ctx context.Context, fromVersion, toVersion int32) error {
		return errors.New("simulated pre-check failure")
	})

	ctx := context.Background()
	result, err := eng.RunRollback(ctx, 10) // target >= current, will fail
	if err == nil {
		t.Fatal("expected error when target >= current")
	}
	if result == nil {
		t.Fatal("expected non-nil result on failure")
	}
	if result.Error == nil {
		t.Fatal("expected Error to be set in RollbackResult")
	}
	if result.Success {
		t.Fatal("expected Success to be false on failure")
	}
}

// ---------------------------------------------------------------------------
// Test: new RollbackEngine has correct defaults
// ---------------------------------------------------------------------------
func TestNewRollbackEngine_Defaults(t *testing.T) {
	store := newMockVersionStore(1)
	eng := NewRollbackEngine(nil, newTestRollbackLogger(), store)

	if eng.lockTimeout != 30*time.Second {
		t.Errorf("expected default lock timeout 30s, got %v", eng.lockTimeout)
	}
	if eng.maxRetryAttempts != 3 {
		t.Errorf("expected default maxRetryAttempts 3, got %d", eng.maxRetryAttempts)
	}
	if eng.dryRunMode {
		t.Fatal("expected dryRunMode to be false by default")
	}
	if len(eng.rollbackHooks) != 0 {
		t.Fatal("expected empty hooks map by default")
	}
}

// ---------------------------------------------------------------------------
// Test: dry-run validates target exceeds max migration
// ---------------------------------------------------------------------------
func TestRunRollback_DryRunValidationFailure(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	eng := NewRollbackEngine(db, newTestRollbackLogger(), store)

	eng.SetDryRun(true)
	migs := Migrations()
	exceedTarget := int32(len(migs) + 1)

	ctx := context.Background()
	result, err := eng.RunRollback(ctx, exceedTarget)
	if err == nil {
		t.Fatalf("expected error for target exceeding max migration in dry-run mode")
	}
	if result == nil {
		t.Fatal("expected non-nil result on dry-run validation failure")
	}
	if !result.DryRun {
		t.Fatal("expected result to indicate dry-run mode")
	}
	if result.Error == nil {
		t.Fatal("expected Error to be set in RollbackResult")
	}
}
