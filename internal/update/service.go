package update

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PreflightCheck is one fail-closed prerequisite evaluated before an upgrade is staged.
type PreflightCheck struct {
	Name    string `json:"name"`
	Pass    bool   `json:"pass"`
	Message string `json:"message,omitempty"`
}

// PreflightResult is shared by CLI and HTTP update surfaces. SourceSchemaVersion
// is the live schema captured before staging; TargetSchemaVersion is the signed
// candidate schema. They are carried through the restart handoff so a failed
// candidate can roll the database back before the previous binary is restored.
type PreflightResult struct {
	Pass                bool             `json:"pass"`
	Checks              []PreflightCheck `json:"checks"`
	Timestamp           time.Time        `json:"timestamp"`
	SourceSchemaVersion int              `json:"source_schema_version"`
	TargetSchemaVersion int              `json:"target_schema_version"`
}

// PreflightFunc evaluates runtime prerequisites such as live schema compatibility,
// temporary disk capacity, and the presence of a usable pre-upgrade backup.
type PreflightFunc func(context.Context, *OrchestratorRelease) (PreflightResult, error)

// Plan is the single signed upgrade decision exposed to every entrypoint.
type Plan struct {
	CurrentVersion string               `json:"current_version"`
	Mode           string               `json:"mode"`
	Available      bool                 `json:"update_available"`
	Release        *OrchestratorRelease `json:"release,omitempty"`
	Preflight      *PreflightResult     `json:"preflight,omitempty"`
}

// Service owns candidate discovery and preflight policy so CLI and UI cannot
// independently decide whether a release is safe to apply.
type Service struct {
	updater   *OrchestratorUpdater
	preflight PreflightFunc
	mu        sync.Mutex
}

func NewService(updater *OrchestratorUpdater, preflight PreflightFunc) *Service {
	if updater == nil {
		panic("update service requires an updater")
	}
	return &Service{updater: updater, preflight: preflight}
}

func NewDefaultService(currentVersion, owner, repo string) *Service {
	return NewService(NewOrchestratorUpdater(currentVersion, owner, repo), nil)
}

func (s *Service) Updater() *OrchestratorUpdater { return s.updater }

func (s *Service) Plan(ctx context.Context, includePreflight bool) (*Plan, error) {
	release, err := s.updater.Check(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover verified release: %w", err)
	}
	plan := &Plan{
		CurrentVersion: s.updater.CurrentVersion(),
		Mode:           s.updater.DetectMode(),
		Available:      release != nil,
		Release:        release,
	}
	if release == nil || !includePreflight {
		return plan, nil
	}
	result, err := s.RunPreflight(ctx, release)
	if err != nil {
		return nil, err
	}
	plan.Preflight = &result
	return plan, nil
}

func (s *Service) RunPreflight(ctx context.Context, release *OrchestratorRelease) (PreflightResult, error) {
	if release == nil {
		return PreflightResult{}, fmt.Errorf("verified release is required for preflight")
	}
	if s.preflight == nil {
		return PreflightResult{}, fmt.Errorf("upgrade preflight is not configured")
	}
	result, err := s.preflight(ctx, release)
	if result.Timestamp.IsZero() {
		result.Timestamp = time.Now().UTC()
	}
	if err != nil {
		return result, fmt.Errorf("upgrade preflight failed: %w", err)
	}
	if !result.Pass {
		return result, fmt.Errorf("upgrade preflight did not pass")
	}
	return result, nil
}

// Stage downloads and cryptographically verifies the selected artifact only
// after the shared preflight has passed. It deliberately does not restart the
// service or claim post-upgrade health; those are separate lifecycle states.
func (s *Service) Stage(ctx context.Context, release *OrchestratorRelease) (string, PreflightResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.RunPreflight(ctx, release)
	if err != nil {
		return "", result, err
	}
	path, err := s.updater.Download(ctx, release)
	if err != nil {
		return "", result, fmt.Errorf("stage verified release artifact: %w", err)
	}
	return path, result, nil
}
