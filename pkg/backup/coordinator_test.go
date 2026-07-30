package backup

import (
	"context"
	"testing"
	"time"
)

func TestRecoveryCoordinator_InitialState(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	if c.GetCurrentState() != StateIdle {
		t.Fatalf("expected initial state Idle, got %s", c.GetCurrentState().String())
	}
}

func TestRecoveryCoordinator_ValidTransition(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	err := c.transitionTo(context.Background(), StateDiscovery)
	if err != nil {
		t.Fatalf("expected valid transition Idle->Discovery: %v", err)
	}
	if c.GetCurrentState() != StateDiscovery {
		t.Fatalf("expected state Discovery, got %s", c.GetCurrentState().String())
	}
}

func TestRecoveryCoordinator_InvalidTransition(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	err := c.transitionTo(context.Background(), StateCompleted)
	if err == nil {
		t.Fatal("expected error for invalid transition Idle->Completed")
	}
}

func TestRecoveryCoordinator_StateHistory(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	c.transitionTo(context.Background(), StateDiscovery)
	c.transitionTo(context.Background(), StatePreFlight)
	c.transitionTo(context.Background(), StateQuiesce)

	history := c.GetStateHistory()
	if len(history) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(history))
	}
}

func TestRecoveryCoordinator_SetTimeout(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	c.SetTimeout(30 * time.Minute)
	if c.timeout != 30*time.Minute {
		t.Fatalf("expected timeout 30m, got %v", c.timeout)
	}
}

func TestRecoveryCoordinator_SetDryRun(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	c.SetDryRun(true)
	if !c.dryRun {
		t.Fatal("expected dryRun to be true")
	}
}

func TestRecoveryCoordinator_DryRunRecovery(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	c.SetDryRun(true)
	c.SetBackupID("test-backup-id")

	result, err := c.Recover(context.Background())
	if err != nil {
		t.Fatalf("dry-run recovery should not error: %v", err)
	}
	if !result.Success {
		t.Fatalf("dry-run recovery should succeed")
	}
	if result.State != StateCompleted {
		t.Fatalf("expected final state Completed, got %s", result.State.String())
	}
}

func TestRecoveryCoordinator_Rollback(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	c.SetBackupID("test-backup-id")
	c.currentState = StatePreFlight

	result, err := c.Rollback(context.Background())
	if err != nil {
		t.Fatalf("rollback should not error: %v", err)
	}
	if !result.Success {
		t.Fatalf("rollback should succeed")
	}
}

func TestRecoveryCoordinator_GetRPOMetrics(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	metrics := c.GetRPOMetrics()
	if metrics.DataLossWindow <= 0 {
		t.Fatal("data loss window should be positive")
	}
	if metrics.MaxAcceptableRPO <= 0 {
		t.Fatal("max RPO should be positive")
	}
}

func TestRecoveryCoordinator_GetRTOMetrics(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	metrics := c.GetRTOMetrics()
	if metrics.RecoveryStartTime.IsZero() {
		t.Fatal("recovery start time should be set")
	}
}

func TestRecoveryCoordinator_GenerateRecoveryID(t *testing.T) {
	id1 := generateRecoveryID()
	id2 := generateRecoveryID()
	if id1 == id2 {
		t.Fatal("generated recovery IDs should be unique")
	}
}

func TestRecoveryCoordinator_StateMachineRollbackOnFailure(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	ctx := context.Background()

	c.transitionTo(ctx, StateDiscovery)

	err := c.transitionTo(ctx, StateCompleted)
	if err == nil {
		t.Fatal("expected invalid transition error")
	}

	c.transitionTo(ctx, StateRollback)
	if c.GetCurrentState() != StateRollback {
		t.Fatalf("expected state Rollback, got %s", c.GetCurrentState().String())
	}
}

func TestRecoveryCoordinator_LockAcquisitionWithNilDB(t *testing.T) {
	c := NewRecoveryCoordinator(nil, nil)
	err := c.acquireLock(context.Background())
	if err != nil {
		t.Fatalf("lock acquisition with nil DB should succeed: %v", err)
	}
}

func TestRecoveryCoordinator_20StateEnum(t *testing.T) {
	expected := 20
	count := 0
	for s := StateIdle; s <= StateCompleted; s++ {
		count++
		_ = s.String()
	}
	if count != expected {
		t.Fatalf("expected %d states, got %d", expected, count)
	}
}

func TestRecoveryCoordinator_AllTransitionsAreValid(t *testing.T) {
	validTransitions := map[RecoveryState][]RecoveryState{
		StateIdle:                  {StateDiscovery},
		StateDiscovery:             {StatePreFlight, StateRollback},
		StatePreFlight:             {StateQuiesce, StateRollback},
		StateQuiesce:               {StateBackupDatabase, StateRollback},
		StateBackupDatabase:        {StateBackupJetStream, StateRollback},
		StateBackupJetStream:       {StateBackupObjectStorage, StateRollback},
		StateBackupObjectStorage:   {StateVerifyIntegrity, StateRollback},
		StateVerifyIntegrity:       {StatePreRestoreValidation, StateRollback},
		StatePreRestoreValidation:  {StateRestoreDatabase, StateRollback},
		StateRestoreDatabase:       {StateRestoreJetStream, StateRollback},
		StateRestoreJetStream:      {StateRestoreObjectStorage, StateRollback},
		StateRestoreObjectStorage:  {StatePostRestoreValidation, StateRollback},
		StatePostRestoreValidation: {StateHealthCheck, StateRollback},
		StateHealthCheck:           {StateVerification, StateRollback},
		StateVerification:          {StateRPOValidation, StateRollback},
		StateRPOValidation:         {StateRTOValidation, StateRollback},
		StateRTOValidation:         {StateCleanup, StateRollback},
		StateCleanup:               {StateCompleted, StateRollback},
		StateRollback:              {StateCleanup},
		StateCompleted:             {},
	}

	coord := NewRecoveryCoordinator(nil, nil)
	for from, targets := range validTransitions {
		for _, to := range targets {
			coord.currentState = from
			err := coord.transitionTo(context.Background(), to)
			if err != nil {
				t.Fatalf("transition %s->%s should be valid but got: %v", from.String(), to.String(), err)
			}
		}
	}
}
