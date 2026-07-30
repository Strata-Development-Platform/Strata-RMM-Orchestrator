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
	sessionID := uuid.New().String()

	cmdPayload, _ := json.Marshal(map[string]interface{}{
		"type":       "remote_start",
		"session_id": sessionID,
		"width":      req.Width,
		"height":     req.Height,
		"quality":    req.Quality,
		"fps":        req.FPS,
	})

	subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, req.DeviceID)
	if err := s.nats.Publish(subject, cmdPayload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"session_id":  sessionID,
		"device_id":   req.DeviceID,
		"tenant_id":   tenantID,
		"frame_topic": fmt.Sprintf("tenant.%s.tunnel.%s.frame", tenantID, sessionID),
		"input_topic": fmt.Sprintf("tenant.%s.tunnel.%s.input", tenantID, sessionID),
		"ctrl_topic":  fmt.Sprintf("tenant.%s.tunnel.%s.ctrl", tenantID, sessionID),
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleRemoteSessionInput(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	sessionID := r.PathValue("sessionID")

	var input struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Type   string  `json:"type"`
		Button string  `json:"button,omitempty"`
		Key    string  `json:"key,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid input"})
		return
	}

	payload, _ := json.Marshal(input)
	subject := fmt.Sprintf("tenant.%s.tunnel.%s.input", tenantID, sessionID)
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

	cmdPayload, _ := json.Marshal(map[string]string{
		"type":       "remote_stop",
		"session_id": sessionID,
	})

	subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, deviceID)
	s.nats.Publish(subject, cmdPayload)

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}
