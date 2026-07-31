package platform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type RemoteSessionRequest struct {
	DeviceID string `json:"device_id"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Quality  int    `json:"quality,omitempty"`
	FPS      int    `json:"fps,omitempty"`
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
	agentID, err := s.resolveRemoteAgent(r, tenantID, input.DeviceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active device not found"})
		return
	}

	payload, _ := json.Marshal(input)
	subject := fmt.Sprintf("tenant.%s.agent.%s.tunnel.%s.input", tenantID, agentID, sessionID)
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
	agentID, err := s.resolveRemoteAgent(r, tenantID, deviceID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active device not found"})
		return
	}

	cmdPayload, _ := json.Marshal(map[string]string{
		"type":       "remote_stop",
		"session_id": sessionID,
	})

	subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, agentID)
	if err := s.nats.Publish(subject, cmdPayload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "publish failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
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
