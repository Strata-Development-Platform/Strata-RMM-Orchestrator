package platform

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/update"
	"go.uber.org/zap"
)

type UpdateManager struct {
	service *update.Service
	updater *update.OrchestratorUpdater
	status  string
	version string
	apiAddr string
	logger  *zap.Logger
	dc      *DeploymentController
}

func NewUpdateManager(currentVersion, owner, repo, apiAddr string, logger *zap.Logger) *UpdateManager {
	updater := update.NewOrchestratorUpdater(currentVersion, owner, repo)
	return &UpdateManager{
		service: update.NewService(updater, nil),
		updater: updater,
		status:  "idle",
		version: currentVersion,
		apiAddr: apiAddr,
		logger:  logger,
	}
}

func (m *UpdateManager) WithDeploymentController(dc *DeploymentController) *UpdateManager {
	m.dc = dc
	return m
}

func (m *UpdateManager) WithPreflight(preflight update.PreflightFunc) *UpdateManager {
	m.service = update.NewService(m.updater, preflight)
	return m
}

func (s *APIServer) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.updateMgr == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update manager not available"})
		return
	}

	plan, err := s.updateMgr.service.Plan(r.Context(), false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	updateMu.Lock()
	status := s.updateMgr.status
	updateMu.Unlock()
	resp := map[string]interface{}{
		"current_version":  plan.CurrentVersion,
		"status":           status,
		"mode":             plan.Mode,
		"update_available": plan.Available,
	}
	if plan.Release != nil {
		resp["latest_version"] = plan.Release.Version
		resp["source_sha"] = plan.Release.SourceSHA
		resp["schema_compatibility"] = plan.Release.SchemaCompatibility
		resp["changelog"] = plan.Release.Changelog
	}

	writeJSON(w, http.StatusOK, resp)
}

var updateMu sync.Mutex

func (s *APIServer) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if s.updateMgr == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update manager not available"})
		return
	}

	updateMu.Lock()
	if s.updateMgr.status == "applying" {
		updateMu.Unlock()
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "update already in progress"})
		return
	}
	updateMu.Unlock()

	plan, err := s.updateMgr.service.Plan(r.Context(), false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !plan.Available || plan.Release == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no verified update is available"})
		return
	}
	if plan.Mode != "baremetal" {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"mode":    plan.Mode,
			"message": "this deployment mode requires a digest-pinned promoted release update; mutable-tag instructions are intentionally disabled",
		})
		return
	}
	if s.db == nil || s.db.DB() == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "live database is unavailable for upgrade preflight"})
		return
	}

	// CLI and HTTP bind the exact same fail-closed runtime prerequisite policy.
	s.updateMgr.WithPreflight(update.NewRuntimePreflight(s.db.DB(), s.logger, update.DefaultUpgradeSnapshotDir))

	updateMu.Lock()
	s.updateMgr.status = "applying"
	updateMu.Unlock()

	// Preserve request values while intentionally detaching cancellation. The
	// handler returns immediately, but staging must be allowed to finish or fail
	// under its own bounded timeout rather than being canceled mid-download.
	upgradeParent := context.WithoutCancel(r.Context())
	go func(parent context.Context) {
		ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
		defer cancel()
		defer func() {
			updateMu.Lock()
			if s.updateMgr.status == "applying" {
				s.updateMgr.status = "idle"
			}
			updateMu.Unlock()
		}()
		s.applyUpdate(ctx, plan.Release)
	}(upgradeParent)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "update_started",
		"message": "Verified update staging started.",
	})
}

func (s *APIServer) applyUpdate(ctx context.Context, release *update.OrchestratorRelease) {
	if release == nil {
		updateMu.Lock()
		s.updateMgr.status = "failed"
		updateMu.Unlock()
		return
	}

	if s.updateMgr.dc != nil {
		s.updateMgr.dc.TransitionTo(DeploymentStateInProgress, "")
		s.updateMgr.dc.RecordEvent(DeploymentEvent{
			ID:              fmt.Sprintf("deploy-%d", time.Now().Unix()),
			Version:         release.Version,
			PreviousVersion: s.updateMgr.version,
			State:           DeploymentStateInProgress,
		})
	}

	binaryPath, preflight, err := s.updateMgr.service.Stage(ctx, release)
	if err != nil {
		if s.updateMgr.dc != nil {
			s.updateMgr.dc.TransitionTo(DeploymentStateFailed, fmt.Sprintf("preflight/stage failed: %v", err))
		}
		s.updateMgr.logger.Error("update preflight/stage failed", zap.Error(err))
		updateMu.Lock()
		s.updateMgr.status = "failed"
		updateMu.Unlock()
		return
	}

	// The running process deliberately does not replace its own executable.
	// Until systemd accepts the transient finalizer, the currently running and
	// on-disk binary remains the known-good version. The finalizer becomes the
	// sole owner of stop -> swap -> candidate start -> restore/rollback.
	if s.updateMgr.dc != nil {
		s.updateMgr.dc.RecordEvent(DeploymentEvent{
			ID:              fmt.Sprintf("deploy-%d", time.Now().Unix()),
			Version:         release.Version,
			PreviousVersion: s.updateMgr.version,
			State:           DeploymentStateInProgress,
		})
	}
	updateMu.Lock()
	s.updateMgr.status = "restart_pending"
	updateMu.Unlock()
	s.updateMgr.logger.Info("verified update and PostgreSQL recovery point staged; handing mutation ownership to external finalizer",
		zap.String("version", release.Version),
		zap.Int("source_schema", preflight.SourceSchemaVersion),
		zap.Int("target_schema", preflight.TargetSchemaVersion),
	)

	if err := s.updateMgr.updater.TriggerRestartWithSchema(binaryPath, preflight.SourceSchemaVersion, preflight.TargetSchemaVersion); err != nil {
		// No binary or database mutation has occurred if the finalizer could not
		// be launched. Keep the staged candidate and recovery handoff intact for
		// operator inspection; a later upgrade is intentionally blocked until
		// the unresolved recovery state is explicitly handled.
		s.updateMgr.logger.Error("external upgrade finalizer launch failed; staged recovery state retained", zap.Error(err))
		if s.updateMgr.dc != nil {
			s.updateMgr.dc.TransitionTo(DeploymentStateFailed, fmt.Sprintf("finalizer launch failed before mutation: %v", err))
		}
		updateMu.Lock()
		s.updateMgr.status = "handoff_failed"
		updateMu.Unlock()
	}
}
