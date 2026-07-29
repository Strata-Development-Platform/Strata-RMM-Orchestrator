package backup

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestRecoveryCoordinator_Transition(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://test:test@localhost:5432/test?sslmode=disable")
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer db.Close()

	coordinator := NewRecoveryCoordinator(db, nil)

	err = coordinator.transitionTo(context.Background(), StateDiscovery)
	if err != nil {
		t.Fatalf("Initial transition failed: %v", err)
	}

	if coordinator.GetCurrentState() != StateDiscovery {
		t.Errorf("Expected state StateDiscovery, got %v", coordinator.GetCurrentState())
	}
}

func TestRecoveryCoordinator_InvalidTransition(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://test:test@localhost:5432/test?sslmode=disable")
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer db.Close()

	coordinator := NewRecoveryCoordinator(db, nil)

	err = coordinator.transitionTo(context.Background(), StateCompleted)
	if err == nil {
		t.Error("Expected error for invalid transition")
	}
}

func TestRecoveryCoordinator_GetStateHistory(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://test:test@localhost:5432/test?sslmode=disable")
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer db.Close()

	coordinator := NewRecoveryCoordinator(db, nil)

	coordinator.transitionTo(context.Background(), StateDiscovery)
	coordinator.transitionTo(context.Background(), StatePreFlight)

	history := coordinator.GetStateHistory()
	if len(history) < 2 {
		t.Errorf("Expected at least 2 events, got %d", len(history))
	}
}

func TestRecoveryCoordinator_SetTimeout(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://test:test@localhost:5432/test?sslmode=disable")
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer db.Close()

	coordinator := NewRecoveryCoordinator(db, nil)

	coordinator.SetTimeout(30 * time.Minute)

	if coordinator.timeout != 30*time.Minute {
		t.Errorf("Expected timeout 30m, got %v", coordinator.timeout)
	}
}

func TestRecoveryCoordinator_GetRPOMetrics(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://test:test@localhost:5432/test?sslmode=disable")
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer db.Close()

	coordinator := NewRecoveryCoordinator(db, nil)

	metrics := coordinator.GetRPOMetrics()

	if metrics.DataLossWindow <= 0 {
		t.Error("Data loss window should be positive")
	}

	if metrics.MaxAcceptableRPO <= 0 {
		t.Error("Max RPO should be positive")
	}
}

func TestRecoveryCoordinator_GetRTOMetrics(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://test:test@localhost:5432/test?sslmode=disable")
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	defer db.Close()

	coordinator := NewRecoveryCoordinator(db, nil)

	metrics := coordinator.GetRTOMetrics()

	if metrics.RecoveryStartTime.IsZero() {
		t.Error("Recovery start time should be set")
	}
}
