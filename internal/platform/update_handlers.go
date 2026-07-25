package platform

import (
	"context"
	"fmt"
	"net/http"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/update"
	"go.uber.org/zap"
)

type UpdateManager struct {
	updater *update.OrchestratorUpdater
	status  string
	version string
	apiAddr string
	logger  *zap.Logger
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

	go s.applyUpdate(r.Context())

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "update_started",
		"message": "Update in progress. Service will restart briefly.",
	})
}

func (s *APIServer) applyUpdate(ctx context.Context) {
	s.updateMgr.status = "applying"

	release, err := s.updateMgr.updater.Check(ctx)
	if err != nil || release == nil {
		s.updateMgr.status = "idle"
		return
	}

	binaryPath, err := s.updateMgr.updater.Download(ctx, release)
	if err != nil {
		s.updateMgr.logger.Error("update download failed", zap.Error(err))
		s.updateMgr.status = "failed"
		return
	}

	if err := s.updateMgr.updater.Apply(binaryPath); err != nil {
		s.updateMgr.logger.Error("update apply failed", zap.Error(err))
		s.updateMgr.updater.Rollback()
		s.updateMgr.status = "failed"
		return
	}

	healthURL := fmt.Sprintf("http://localhost%s/health", s.updateMgr.apiAddr)
	if err := s.updateMgr.updater.Verify(ctx, healthURL); err != nil {
		s.updateMgr.logger.Error("update verification failed, rolling back", zap.Error(err))
		s.updateMgr.updater.Rollback()
		s.updateMgr.status = "rolled_back"
		return
	}

	s.updateMgr.logger.Info("update successful, restarting", zap.String("version", release.Version))
	s.updateMgr.updater.Cleanup()
	s.updateMgr.updater.TriggerRestart()
}
