package platform

import (
	"sync"
	"testing"
	"time"
)

func TestDeploymentController_RecordEvent(t *testing.T) {
	t.Parallel()

	ctrl := NewDeploymentController()

	if ctrl.GetState() != DeploymentStatePending {
		t.Fatalf("expected initial state pending, got %v", ctrl.GetState())
	}

	ctrl.RecordEvent(DeploymentEvent{
		ID:      "evt-1",
		Version: "v1.0.0",
		State:   DeploymentStateInProgress,
	})

	if ctrl.GetState() != DeploymentStatePending {
		t.Fatalf("expected state still pending (RecordEvent doesn't change current state), got %v", ctrl.GetState())
	}

	lastEvent := ctrl.GetLastEvent()
	if lastEvent == nil {
		t.Fatal("expected last event to be set")
	}
	if lastEvent.ID != "evt-1" {
		t.Errorf("expected event ID evt-1, got %s", lastEvent.ID)
	}
	if lastEvent.Version != "v1.0.0" {
		t.Errorf("expected version v1.0.0, got %s", lastEvent.Version)
	}

	history := ctrl.GetHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 event in history, got %d", len(history))
	}
	if history[0].ID != "evt-1" {
		t.Errorf("expected history[0] ID evt-1, got %s", history[0].ID)
	}

	ctrl.RecordEvent(DeploymentEvent{ID: "evt-2", Version: "v2.0.0", State: DeploymentStateCompleted})
	history = ctrl.GetHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 events in history, got %d", len(history))
	}
}

func TestDeploymentController_TransitionTo(t *testing.T) {
	t.Parallel()

	ctrl := NewDeploymentController()

	if ctrl.GetState() != DeploymentStatePending {
		t.Fatalf("expected initial state pending, got %v", ctrl.GetState())
	}

	ctrl.TransitionTo(DeploymentStateInProgress, "")
	if ctrl.GetState() != DeploymentStateInProgress {
		t.Errorf("expected state in_progress, got %v", ctrl.GetState())
	}

	ctrl.TransitionTo(DeploymentStateFailed, "build failed")
	if ctrl.GetState() != DeploymentStateFailed {
		t.Errorf("expected state failed, got %v", ctrl.GetState())
	}

	lastEvent := ctrl.GetLastEvent()
	if lastEvent == nil {
		t.Fatal("expected last event to be set")
	}
	if lastEvent.Error != "build failed" {
		t.Errorf("expected error 'build failed', got '%s'", lastEvent.Error)
	}

	ctrl.TransitionTo(DeploymentStateRollingBack, "")
	if ctrl.GetState() != DeploymentStateRollingBack {
		t.Errorf("expected state rolling_back, got %v", ctrl.GetState())
	}

	ctrl.TransitionTo(DeploymentStateCompleted, "")
	if ctrl.GetState() != DeploymentStateCompleted {
		t.Errorf("expected state completed, got %v", ctrl.GetState())
	}

	history := ctrl.GetHistory()
	if len(history) != 4 {
		t.Fatalf("expected 4 events in history, got %d", len(history))
	}
}

func TestDeploymentController_GetHistory_Concurrent(t *testing.T) {
	t.Parallel()

	ctrl := NewDeploymentController()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(id int) {
			defer wg.Done()
			ctrl.RecordEvent(DeploymentEvent{
				ID:      "evt-concurrent",
				Version: "v0.0.1",
				State:   DeploymentStateInProgress,
			})
		}(i)
		go func(id int) {
			defer wg.Done()
			_ = ctrl.GetHistory()
		}(i)
		go func(id int) {
			defer wg.Done()
			_ = ctrl.GetLastEvent()
		}(i)
	}

	wg.Wait()

	history := ctrl.GetHistory()
	if len(history) != 50 {
		t.Fatalf("expected 50 events in history, got %d", len(history))
	}
}

func TestDeploymentController_Reset(t *testing.T) {
	t.Parallel()

	ctrl := NewDeploymentController()

	ctrl.RecordEvent(DeploymentEvent{ID: "evt-1", Version: "v1.0.0", State: DeploymentStateInProgress})
	ctrl.TransitionTo(DeploymentStateCompleted, "")

	if ctrl.GetState() != DeploymentStateCompleted {
		t.Fatalf("expected state completed before reset, got %v", ctrl.GetState())
	}

	history := ctrl.GetHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 events before reset, got %d", len(history))
	}

	ctrl.Reset()

	if ctrl.GetState() != DeploymentStatePending {
		t.Errorf("expected state pending after reset, got %v", ctrl.GetState())
	}

	history = ctrl.GetHistory()
	if len(history) != 0 {
		t.Errorf("expected empty history after reset, got %d events", len(history))
	}

	if ctrl.GetLastEvent() != nil {
		t.Error("expected nil last event after reset")
	}
}

func TestDeploymentController_ThreadSafety(t *testing.T) {
	t.Parallel()

	ctrl := NewDeploymentController()
	numWriters := 100
	var wg sync.WaitGroup

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctrl.RecordEvent(DeploymentEvent{
				ID:      "evt-thread",
				Version: "v0.0.1",
				State:   DeploymentStateInProgress,
			})
			ctrl.TransitionTo(DeploymentStateInProgress, "")
			_ = ctrl.GetHistory()
			_ = ctrl.GetState()
			_ = ctrl.GetLastEvent()
		}(i)
	}

	wg.Wait()

	history := ctrl.GetHistory()
	if len(history) != numWriters*2 {
		t.Errorf("expected %d events, got %d", numWriters*2, len(history))
	}
}

func TestDeploymentState_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state  DeploymentState
		expect string
	}{
		{DeploymentStatePending, "pending"},
		{DeploymentStateInProgress, "in_progress"},
		{DeploymentStateCompleted, "completed"},
		{DeploymentStateFailed, "failed"},
		{DeploymentStateRollingBack, "rolling_back"},
		{DeploymentState(99), "unknown"},
	}

	for _, tc := range tests {
		if tc.state.String() != tc.expect {
			t.Errorf("DeploymentState(%d).String() = %q, want %q", tc.state, tc.state.String(), tc.expect)
		}
	}
}

func TestDeploymentController_GetHistory_ReturnsCopy(t *testing.T) {
	t.Parallel()

	ctrl := NewDeploymentController()
	ctrl.RecordEvent(DeploymentEvent{ID: "evt-1", Version: "v1.0.0", State: DeploymentStateInProgress})

	history := ctrl.GetHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 event, got %d", len(history))
	}

	history[0].ID = "modified"

	history2 := ctrl.GetHistory()
	if history2[0].ID != "evt-1" {
		t.Error("GetHistory should return a copy, not a reference")
	}
}

func TestDeploymentController_LastEventTimestampAutoSet(t *testing.T) {
	t.Parallel()

	ctrl := NewDeploymentController()
	before := time.Now()
	ctrl.RecordEvent(DeploymentEvent{ID: "evt-ts", Version: "v1.0.0", State: DeploymentStateInProgress})
	after := time.Now()

	lastEvent := ctrl.GetLastEvent()
	if lastEvent == nil {
		t.Fatal("expected last event")
	}
	if lastEvent.Timestamp.Before(before) || lastEvent.Timestamp.After(after) {
		t.Errorf("timestamp %v not within expected range [%v, %v]", lastEvent.Timestamp, before, after)
	}
}
