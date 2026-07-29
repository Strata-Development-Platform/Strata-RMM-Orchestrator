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
	updater *update.OrchestratorUpdater
	status  string
	version string
	apiAddr string
	logger  *zap.Logger
	dc      *DeploymentController
}

func NewUpdateManager(currentVersion, owner, repo, apiAddr string, logger *zap.Logger) *UpdateManager {
	return &UpdateManager{
		updater: update.NewOrchestratorUpdater(currentVersion, owner, repo),
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

func (s *APIServer) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.updateMgr == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update manager not available"})
		return
	}

	release, err := s.updateMgr.updater.Check(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]interface{}{
		"current_version": s.updateMgr.version,
		"status":          s.updateMgr.status,
		"mode":            s.updateMgr.updater.DetectMode(),
	}
	if release != nil {
		resp["update_available"] = true
		resp["latest_version"] = release.Version
		resp["changelog"] = release.Changelog
	} else {
		resp["update_available"] = false
	}

	writeJSON(w, http.StatusOK, resp)
}

var updateMu sync.Mutex

func (s *APIServer) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if s.updateMgr == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update manager not available"})
		return
	}

	if s.updateMgr.status == "applying" {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "update already in progress"})
		return
	}

	mode := s.updateMgr.updater.DetectMode()
	if mode != "baremetal" {
		var instructions string
		switch mode {
		case "docker":
			instructions = "docker compose pull && docker compose up -d"
		case "kubernetes":
			instructions = "helm upgrade strata-rmm ..."
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"mode":         mode,
			"instructions": instructions,
			"message":      fmt.Sprintf("Detected %s deployment. See instructions.", mode),
		})
		return
	}

	updateMu.Lock()
	s.updateMgr.status = "applying"
	updateMu.Unlock()

	go func() {
		defer func() {
			updateMu.Lock()
			s.updateMgr.status = "idle"
			updateMu.Unlock()
		}()
		s.applyUpdate(r.Context())
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "update_started",
		"message": "Update in progress. Service will restart briefly.",
	})
}

func (s *APIServer) applyUpdate(ctx context.Context) {
	s.updateMgr.status = "applying"

	if s.updateMgr.dc != nil {
		s.updateMgr.dc.TransitionTo(DeploymentStateInProgress, "")
		s.updateMgr.dc.RecordEvent(DeploymentEvent{
			ID:      fmt.Sprintf("deploy-%d", time.Now().Unix()),
			Version: s.updateMgr.version,
			State:   DeploymentStateInProgress,
		})
	}

	release, err := s.updateMgr.updater.Check(ctx)
	if err != nil || release == nil {
		if s.updateMgr.dc != nil {
			s.updateMgr.dc.TransitionTo(DeploymentStateFailed, fmt.Sprintf("check failed: %v", err))
		}
		s.updateMgr.status = "idle"
		return
	}

	if s.updateMgr.dc != nil {
		prev := s.updateMgr.version
		s.updateMgr.dc.RecordEvent(DeploymentEvent{
			ID:              fmt.Sprintf("deploy-%d", time.Now().Unix()),
			Version:         release.Version,
			PreviousVersion: prev,
			State:           DeploymentStateInProgress,
		})
	}

	binaryPath, err := s.updateMgr.updater.Download(ctx, release)
	if err != nil {
		if s.updateMgr.dc != nil {
			s.updateMgr.dc.TransitionTo(DeploymentStateFailed, fmt.Sprintf("download failed: %v", err))
		}
		s.updateMgr.logger.Error("update download failed", zap.Error(err))
		s.updateMgr.status = "failed"
		return
	}

	if err := s.updateMgr.updater.Apply(binaryPath); err != nil {
		if s.updateMgr.dc != nil {
			s.updateMgr.dc.TransitionTo(DeploymentStateRollingBack, "")
		}
		s.updateMgr.logger.Error("update apply failed", zap.Error(err))
		s.updateMgr.updater.Rollback()
		if s.updateMgr.dc != nil {
			s.updateMgr.dc.TransitionTo(DeploymentStateFailed, fmt.Sprintf("apply failed: %v", err))
		}
		s.updateMgr.status = "failed"
		return
	}

	healthURL := fmt.Sprintf("http://localhost%s/health", s.updateMgr.apiAddr)
	if err := s.updateMgr.updater.Verify(ctx, healthURL); err != nil {
		if s.updateMgr.dc != nil {
			s.updateMgr.dc.TransitionTo(DeploymentStateRollingBack, "")
		}
		s.updateMgr.logger.Error("update verification failed, rolling back", zap.Error(err))
		s.updateMgr.updater.Rollback()
		if s.updateMgr.dc != nil {
			s.updateMgr.dc.TransitionTo(DeploymentStateFailed, fmt.Sprintf("verification failed: %v", err))
		}
		s.updateMgr.status = "rolled_back"
		return
	}

	if s.updateMgr.dc != nil {
		s.updateMgr.dc.TransitionTo(DeploymentStateCompleted, "")
		s.updateMgr.dc.RecordEvent(DeploymentEvent{
			ID:              fmt.Sprintf("deploy-%d", time.Now().Unix()),
			Version:         release.Version,
			PreviousVersion: s.updateMgr.version,
			State:           DeploymentStateCompleted,
		})
	}

	s.updateMgr.logger.Info("update successful, restarting", zap.String("version", release.Version))
	s.updateMgr.updater.Cleanup()
	s.updateMgr.updater.TriggerRestart()
}
