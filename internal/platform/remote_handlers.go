package platform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/remote"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
)

type RemoteSessionRequest struct {
	DeviceID string `json:"device_id"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Quality  int    `json:"quality,omitempty"`
	FPS      int    `json:"fps,omitempty"`
}

type remoteSessionBinding struct {
	TenantID  string
	DeviceID  string
	AgentID   string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (s *APIServer) handleRemoteSessionStart(w http.ResponseWriter, r *http.Request) {
	var req RemoteSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id required"})
		return
	}

	tenantID := r.PathValue("tenantID")
	agentID, err := s.resolveRemoteAgent(r, tenantID, req.DeviceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active device not found"})
		return
	}
	sessionID := uuid.New().String()
	s.bindRemoteSession(sessionID, remoteSessionBinding{TenantID: tenantID, DeviceID: req.DeviceID, AgentID: agentID})

	cmdPayload, _ := json.Marshal(map[string]interface{}{
		"type":       "remote_start",
		"session_id": sessionID,
		"width":      req.Width,
		"height":     req.Height,
		"quality":    req.Quality,
		"fps":        req.FPS,
	})

	subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, agentID)
	if err := s.nats.Publish(subject, cmdPayload); err != nil {
		s.deleteRemoteSession(sessionID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"session_id":  sessionID,
		"device_id":   req.DeviceID,
		"tenant_id":   tenantID,
		"frame_topic": fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.frame", tenantID, agentID, sessionID),
		"input_topic": fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.input", tenantID, agentID, sessionID),
		"ctrl_topic":  fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.ctrl", tenantID, agentID, sessionID),
		"created_at":  time.Now().UTC().Format(time.RFC3339),
		"expires_at":  time.Now().Add(s.remoteSessionLifetime()).UTC().Format(time.RFC3339),
		"status":      "pending",
	})
}

func (s *APIServer) handleRemoteSessionInput(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	sessionID := r.PathValue("sessionID")

	var input struct {
		DeviceID string  `json:"device_id"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		Type     string  `json:"type"`
		Button   string  `json:"button,omitempty"`
		Key      string  `json:"key,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid input"})
		return
	}
	binding, ok := s.remoteSessionFor(sessionID, tenantID, input.DeviceID, "")
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "remote session not found for device"})
		return
	}

	payload, _ := json.Marshal(input)
	subject := fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.input", tenantID, binding.AgentID, sessionID)
	if err := s.nats.Publish(subject, payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "publish failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *APIServer) handleRemoteSessionStop(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	deviceID := r.URL.Query().Get("device_id")
	tenantID := r.PathValue("tenantID")
	if deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id required"})
		return
	}
	binding, ok := s.remoteSessionFor(sessionID, tenantID, deviceID, "")
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "remote session not found for device"})
		return
	}

	cmdPayload, _ := json.Marshal(map[string]string{
		"type":       "remote_stop",
		"session_id": sessionID,
	})

	subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, binding.AgentID)
	if err := s.nats.Publish(subject, cmdPayload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "publish failed"})
		return
	}
	s.deleteRemoteSession(sessionID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

func (s *APIServer) bindRemoteSession(sessionID string, binding remoteSessionBinding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.remoteSessionTime()
	s.cleanupExpiredRemoteSessionsLocked(now)
	if s.remoteSessions == nil {
		s.remoteSessions = make(map[string]remoteSessionBinding)
	}
	binding.CreatedAt = now
	binding.ExpiresAt = now.Add(s.remoteSessionLifetime())
	s.remoteSessions[sessionID] = binding
}

func (s *APIServer) remoteSessionFor(sessionID, tenantID, deviceID, agentID string) (remoteSessionBinding, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredRemoteSessionsLocked(s.remoteSessionTime())
	binding, ok := s.remoteSessions[sessionID]
	if !ok || binding.TenantID != tenantID || binding.DeviceID != deviceID || agentID != "" && binding.AgentID != agentID {
		return remoteSessionBinding{}, false
	}
	return binding, ok
}

func (s *APIServer) deleteRemoteSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.remoteSessions, sessionID)
}

func (s *APIServer) clearRemoteSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.remoteSessions)
}

func (s *APIServer) cleanupExpiredRemoteSessionsLocked(now time.Time) {
	for sessionID, binding := range s.remoteSessions {
		if !binding.ExpiresAt.After(now) {
			delete(s.remoteSessions, sessionID)
		}
	}
}

func (s *APIServer) remoteSessionTime() time.Time {
	if s.remoteSessionNow != nil {
		return s.remoteSessionNow()
	}
	return time.Now()
}

func (s *APIServer) remoteSessionLifetime() time.Duration {
	if s.remoteSessionTTL > 0 {
		return s.remoteSessionTTL
	}
	return 30 * time.Minute
}

func (s *APIServer) resolveRemoteAgent(r *http.Request, tenantID, deviceID string) (string, error) {
	if tenantID == "" || deviceID == "" || s.db == nil || s.nats == nil {
		return "", fmt.Errorf("remote dependencies or identity unavailable")
	}
	var agentID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT agent_id::text FROM devices
		WHERE tenant_id = $1 AND id = $2 AND agent_id IS NOT NULL
		  AND status IN ('online', 'offline')
	`, tenantID, deviceID).Scan(&agentID)
	if err != nil {
		return "", fmt.Errorf("resolve remote agent: %w", err)
	}
	if agentID == "" {
		return "", fmt.Errorf("resolve remote agent: empty identity")
	}
	return agentID, nil
}

func (s *APIServer) handleStartInteractiveSession(w http.ResponseWriter, r *http.Request) {
	var req RemoteSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id required"})
		return
	}

	tenantID := r.PathValue("tenantID")
	agentID, err := s.resolveRemoteAgent(r, tenantID, req.DeviceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active device not found"})
		return
	}
	sessionID := uuid.New().String()

	cmdPayload, _ := json.Marshal(map[string]interface{}{
		"type":       "remote_start",
		"session_id": sessionID,
		"width":      req.Width,
		"height":     req.Height,
		"quality":    req.Quality,
		"fps":        req.FPS,
	})

	subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, agentID)
	if err := s.nats.Publish(subject, cmdPayload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"session_id":  sessionID,
		"device_id":   req.DeviceID,
		"tenant_id":   tenantID,
		"frame_topic": fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.frame", tenantID, agentID, sessionID),
		"input_topic": fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.input", tenantID, agentID, sessionID),
		"ctrl_topic":  fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.ctrl", tenantID, agentID, sessionID),
		"created_at":  time.Now().UTC().Format(time.RFC3339),
		"expires_at":  time.Now().Add(s.remoteSessionLifetime()).UTC().Format(time.RFC3339),
		"status":      "active",
	})
}

func (s *APIServer) handleInteractiveSessionInput(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	sessionID := r.PathValue("sessionID")

	var input struct {
		DeviceID string  `json:"device_id"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		Type     string  `json:"type"`
		Button   string  `json:"button,omitempty"`
		Key      string  `json:"key,omitempty"`
		Mod      int     `json:"mod,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid input"})
		return
	}

	binding, ok := s.remoteSessionFor(sessionID, tenantID, input.DeviceID, "")
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "remote session not found for device"})
		return
	}

	payload, _ := json.Marshal(input)
	subject := fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.input", tenantID, binding.AgentID, sessionID)
	if err := s.nats.Publish(subject, payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "publish failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *APIServer) handleStopInteractiveSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	deviceID := r.URL.Query().Get("device_id")
	tenantID := r.PathValue("tenantID")
	if deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id required"})
		return
	}

	binding, ok := s.remoteSessionFor(sessionID, tenantID, deviceID, "")
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "remote session not found for device"})
		return
	}

	cmdPayload, _ := json.Marshal(map[string]string{
		"type":       "remote_stop",
		"session_id": sessionID,
	})

	subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, binding.AgentID)
	if err := s.nats.Publish(subject, cmdPayload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "publish failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": sessionID,
		"status":     "stopped",
		"stopped_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleListInteractiveSessions(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	deviceID := r.URL.Query().Get("device_id")

	bindings := make([]map[string]interface{}, 0)
	s.mu.RLock()
	for sessionID, binding := range s.remoteSessions {
		if binding.TenantID == tenantID && (deviceID == "" || binding.DeviceID == deviceID) {
			bindings = append(bindings, map[string]interface{}{
				"session_id": sessionID,
				"device_id":  binding.DeviceID,
				"tenant_id":  binding.TenantID,
				"agent_id":   binding.AgentID,
				"created_at": binding.CreatedAt.Format(time.RFC3339),
				"expires_at": binding.ExpiresAt.Format(time.RFC3339),
			})
		}
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": bindings,
		"count":    len(bindings),
	})
}

func (s *APIServer) handleStartInteractiveRecording(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	sessionID := r.PathValue("sessionID")

	if s.recordingStore == nil || s.storageBackend == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recording service not available"})
		return
	}

	var req struct {
		Format        string `json:"format"`
		Compression   string `json:"compression"`
		RetentionDays int    `json:"retention_days"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	recordingID := uuid.New().String()
	storageKey := fmt.Sprintf("recordings/%s/%s/%s.webm", tenantID, sessionID, recordingID)

	now := time.Now()
	expiresAt := now.Add(time.Duration(req.RetentionDays) * 24 * time.Hour)

	var format string
	if req.Format != "" {
		format = req.Format
	} else {
		format = "webm"
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"recording_id":   recordingID,
		"session_id":     sessionID,
		"tenant_id":      tenantID,
		"storage_key":    storageKey,
		"format":         format,
		"started_at":     now.UTC().Format(time.RFC3339),
		"expires_at":     expiresAt.UTC().Format(time.RFC3339),
		"status":         "recording",
	})

	go func() {
		rec := &remote.Recording{
			ID:             recordingID,
			SessionID:      sessionID,
			TenantID:       tenantID,
			DeviceID:       "",
			SizeBytes:      0,
			DurationMs:     0,
			Format:         format,
			StorageKey:     storageKey,
			StorageBackend: "minio",
			ExpiresAt:      &expiresAt,
			CreatedAt:      now,
		}
		if s.recordingStore != nil {
			if err := s.recordingStore.Create(rec); err != nil {
				s.logger.Error("save recording metadata", zap.Error(err))
			}
		}
	}()
}

func (s *APIServer) handleStopInteractiveRecording(w http.ResponseWriter, r *http.Request) {
	recordingID := r.PathValue("recordingID")

	if s.recordingStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recording store not available"})
		return
	}

	rec, err := s.recordingStore.GetByID(recordingID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording not found"})
		return
	}

	rec.DurationMs = time.Since(rec.CreatedAt).Milliseconds()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"recording_id":   rec.ID,
		"session_id":     rec.SessionID,
		"status":         "completed",
		"duration_ms":    rec.DurationMs,
		"storage_key":    rec.StorageKey,
		"size_bytes":     rec.SizeBytes,
		"format":         rec.Format,
	})
}

func (s *APIServer) handleInteractiveRecordingPlayback(w http.ResponseWriter, r *http.Request) {
	recordingID := r.PathValue("recordingID")

	if s.recordingStore == nil || s.storageBackend == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recording service not available"})
		return
	}

	rec, err := s.recordingStore.GetByID(recordingID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recording not found"})
		return
	}

	url, err := s.storageBackend.PresignedURL(r.Context(), rec.StorageKey, storage.PresignedOptions{
		Method:      "GET",
		Expiry:      time.Hour,
		ContentType: "video/webm",
		Disposition: "inline",
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "generate playback URL"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"recording":    rec,
		"playback_url": url,
	})
}

func (s *APIServer) handleListInteractiveRecordings(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")

	if s.recordingStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recording store not available"})
		return
	}

	recordings, err := s.recordingStore.ListByTenant(tenantID, 50, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"recordings": recordings,
		"count":      len(recordings),
	})
}
