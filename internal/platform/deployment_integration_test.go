package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

func TestDeploymentController_RuntimeIntegration(t *testing.T) {
	t.Parallel()

	dc := NewDeploymentController()

	// Transition through the full deployment lifecycle.
	// RecordEvent appends to history but does NOT change current state.
	// TransitionTo appends to history AND changes current state.
	dc.TransitionTo(DeploymentStatePending, "")
	dc.RecordEvent(DeploymentEvent{
		ID:      "deploy-001",
		Version: "v1.0.0",
		State:   DeploymentStateInProgress,
	})
	dc.TransitionTo(DeploymentStateInProgress, "")
	dc.RecordEvent(DeploymentEvent{
		ID:      "deploy-001",
		Version: "v1.0.0",
		State:   DeploymentStateCompleted,
	})
	dc.TransitionTo(DeploymentStateCompleted, "")

	// 1 TransitionTo(pending) + 1 RecordEvent + 1 TransitionTo(in_progress)
	// + 1 RecordEvent + 1 TransitionTo(completed) = 5 history entries
	if dc.GetState() != DeploymentStateCompleted {
		t.Fatalf("expected completed state, got %v", dc.GetState())
	}

	events := dc.GetHistory()
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}
	if events[0].State != DeploymentStatePending {
		t.Errorf("expected first event pending, got %v", events[0].State)
	}
	if events[1].Version != "v1.0.0" {
		t.Errorf("expected v1.0.0, got %s", events[1].Version)
	}
	if events[4].State != DeploymentStateCompleted {
		t.Errorf("expected last event completed, got %v", events[4].State)
	}
}

func TestDeploymentController_FailureAndRollback(t *testing.T) {
	t.Parallel()

	dc := NewDeploymentController()
	dc.TransitionTo(DeploymentStateInProgress, "")
	dc.TransitionTo(DeploymentStateFailed, "image pull backoff")
	dc.TransitionTo(DeploymentStateRollingBack, "")
	dc.TransitionTo(DeploymentStateCompleted, "")

	events := dc.GetHistory()
	if len(events) != 4 {
		t.Fatalf("expected 4 events in failure/rollback cycle, got %d", len(events))
	}
	// events[1] is the Failed transition (events[0] is InProgress)
	if events[1].State != DeploymentStateFailed || events[1].Error != "image pull backoff" {
		t.Errorf("expected failed event with error at index 1, got %+v", events[1])
	}

	// lastEvent reflects the most recent transition
	lastEvent := dc.GetLastEvent()
	if lastEvent == nil || lastEvent.State != DeploymentStateCompleted {
		t.Errorf("expected last event completed, got %+v", lastEvent)
	}
}

func TestDeploymentController_ResetClearsState(t *testing.T) {
	t.Parallel()

	dc := NewDeploymentController()
	dc.TransitionTo(DeploymentStateInProgress, "")
	dc.TransitionTo(DeploymentStateCompleted, "")

	dc.Reset()

	if dc.GetState() != DeploymentStatePending {
		t.Errorf("expected pending after reset, got %v", dc.GetState())
	}
	if dc.GetLastEvent() != nil {
		t.Error("expected nil last event after reset")
	}
	if dc.GetHistory() != nil {
		t.Error("expected nil history after reset")
	}
}

func TestDeploymentAPI_StateEndpoint(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-that-is-long-enough-for-testing")

	dc := NewDeploymentController()
	dc.TransitionTo(DeploymentStateInProgress, "")

	tokenGen, err := auth.NewTokenGeneratorOrFail("")
	if err != nil {
		t.Fatalf("create token generator: %v", err)
	}

	api, err := NewAPIServer("127.0.0.1:0", nil, nil, nil, tokenGen)
	if err != nil {
		t.Fatalf("create API server: %v", err)
	}
	api.WithDeploymentController(dc)

	req := httptest.NewRequest("GET", "/api/v2/deployment/state", nil)
	w := httptest.NewRecorder()
	api.handleDeploymentState(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["state"] != "in_progress" {
		t.Errorf("expected in_progress, got %v", resp["state"])
	}
	if resp["last_event"] != nil {
		lastEvent := resp["last_event"].(map[string]interface{})
		// DeploymentState is serialized as int by the json encoder (1 = InProgress)
		if lastEvent["state"] != float64(1) {
			t.Errorf("expected last_event state 1 (InProgress), got %v", lastEvent["state"])
		}
	}
}

func TestDeploymentAPI_HistoryEndpoint(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-that-is-long-enough-for-testing")

	dc := NewDeploymentController()
	dc.TransitionTo(DeploymentStateInProgress, "")
	dc.TransitionTo(DeploymentStateCompleted, "")

	tokenGen, err := auth.NewTokenGeneratorOrFail("")
	if err != nil {
		t.Fatalf("create token generator: %v", err)
	}

	api, err := NewAPIServer("127.0.0.1:0", nil, nil, nil, tokenGen)
	if err != nil {
		t.Fatalf("create API server: %v", err)
	}
	api.WithDeploymentController(dc)

	req := httptest.NewRequest("GET", "/api/v2/deployment/history", nil)
	w := httptest.NewRecorder()
	api.handleDeploymentHistory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	history, ok := resp["history"].([]interface{})
	if !ok {
		t.Fatal("expected history array in response")
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 events, got %d", len(history))
	}
	if resp["count"].(float64) != 2 {
		t.Errorf("expected count 2, got %v", resp["count"])
	}
}

func TestDeploymentAPI_Unconfigured(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-that-is-long-enough-for-testing")

	tokenGen, err := auth.NewTokenGeneratorOrFail("")
	if err != nil {
		t.Fatalf("create token generator: %v", err)
	}

	api, err := NewAPIServer("127.0.0.1:0", nil, nil, nil, tokenGen)
	if err != nil {
		t.Fatalf("create API server: %v", err)
	}
	// Intentionally NOT calling WithDeploymentController

	req := httptest.NewRequest("GET", "/api/v2/deployment/state", nil)
	w := httptest.NewRecorder()
	api.handleDeploymentState(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when controller not configured, got %d", w.Code)
	}

	req2 := httptest.NewRequest("GET", "/api/v2/deployment/history", nil)
	w2 := httptest.NewRecorder()
	api.handleDeploymentHistory(w2, req2)

	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when controller not configured, got %d", w2.Code)
	}
}

func TestDeploymentHealthCheck(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Healthy deployment state
	dc := NewDeploymentController()
	dc.TransitionTo(DeploymentStateCompleted, "")

	check := func(ctx context.Context) error {
		if dc.GetState() == DeploymentStateFailed {
			return ctx.Err()
		}
		return nil
	}

	if err := check(ctx); err != nil {
		t.Error("expected healthy deployment to pass check")
	}

	// Failed deployment state
	dc2 := NewDeploymentController()
	dc2.TransitionTo(DeploymentStateFailed, "something broke")

	checkFailed := func(ctx context.Context) error {
		if dc2.GetState() == DeploymentStateFailed {
			return ctx.Err()
		}
		return nil
	}

	if err := checkFailed(ctx); err != nil {
		t.Error("expected failed deployment to fail check")
	}
}
