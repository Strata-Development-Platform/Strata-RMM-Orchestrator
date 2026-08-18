package update

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceRunPreflightRequiresConfiguredChecker(t *testing.T) {
	service := NewDefaultService("1.0.0", "owner", "repo")
	_, err := service.RunPreflight(context.Background(), &OrchestratorRelease{Version: "1.1.0"})
	if err == nil {
		t.Fatal("RunPreflight accepted missing checker")
	}
}

func TestServiceRunPreflightFailsClosed(t *testing.T) {
	service := NewService(NewOrchestratorUpdater("1.0.0", "owner", "repo"), func(context.Context, *OrchestratorRelease) (PreflightResult, error) {
		return PreflightResult{Checks: []PreflightCheck{{Name: "backup", Pass: false}}}, nil
	})
	result, err := service.RunPreflight(context.Background(), &OrchestratorRelease{Version: "1.1.0"})
	if err == nil {
		t.Fatal("RunPreflight accepted failing prerequisite")
	}
	if result.Pass {
		t.Fatal("RunPreflight reported pass")
	}
	if result.Timestamp.IsZero() {
		t.Fatal("RunPreflight did not stamp result")
	}
}

func TestServiceRunPreflightPropagatesCheckerError(t *testing.T) {
	want := errors.New("database unavailable")
	service := NewService(NewOrchestratorUpdater("1.0.0", "owner", "repo"), func(context.Context, *OrchestratorRelease) (PreflightResult, error) {
		return PreflightResult{}, want
	})
	_, err := service.RunPreflight(context.Background(), &OrchestratorRelease{Version: "1.1.0"})
	if !errors.Is(err, want) {
		t.Fatalf("RunPreflight() error = %v, want %v", err, want)
	}
}

func TestServiceRunPreflightAcceptsPassingPrerequisites(t *testing.T) {
	stamp := time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC)
	service := NewService(NewOrchestratorUpdater("1.0.0", "owner", "repo"), func(context.Context, *OrchestratorRelease) (PreflightResult, error) {
		return PreflightResult{
			Pass: true,
			Checks: []PreflightCheck{
				{Name: "schema", Pass: true},
				{Name: "disk", Pass: true},
				{Name: "backup", Pass: true},
			},
			Timestamp:           stamp,
			SourceSchemaVersion: 92,
			TargetSchemaVersion: 93,
		}, nil
	})
	result, err := service.RunPreflight(context.Background(), &OrchestratorRelease{Version: "1.1.0"})
	if err != nil {
		t.Fatalf("RunPreflight() error = %v", err)
	}
	if !result.Pass || !result.Timestamp.Equal(stamp) {
		t.Fatalf("RunPreflight() = %+v", result)
	}
	if result.SourceSchemaVersion != 92 || result.TargetSchemaVersion != 93 {
		t.Fatalf("schema rollback boundary was not preserved: %+v", result)
	}
}
