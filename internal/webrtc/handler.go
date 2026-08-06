package webrtc

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/remote"
)

// Handler handles WebRTC remote support API requests.
type Handler struct {
	peerManager          *PeerManager
	recordingManager     *RecordingManager
	transcriptionManager *TranscriptionManager
	logger               *zap.Logger
}

// NewHandler creates a new WebRTC handler.
func NewHandler(nc *nats.Conn, recorder *remote.Recorder, logger *zap.Logger) *Handler {
	return &Handler{
		peerManager:          NewPeerManager(nc, logger),
		recordingManager:     NewRecordingManager(nc, recorder, logger),
		transcriptionManager: NewTranscriptionManager(nc, logger),
		logger:               logger,
	}
}

// HandleCreateSession handles POST /api/v1/webrtc/sessions.
func (h *Handler) HandleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	session, err := h.peerManager.CreateSession(&req)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"id":              session.ID,
		"tenant_id":       session.TenantID,
		"device_id":       session.DeviceID,
		"support_user_id": session.SupportUserID,
		"direction":       string(session.Direction),
		"state":           string(session.State),
		"relay_enabled":   session.RelayEnabled,
		"relay_provider":  string(session.RelayProvider),
		"started_at":      session.StartedAt.Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		h.logger.Error("failed to encode session response", zap.Error(err))
	}
}

// HandleGetSession handles GET /api/v1/webrtc/sessions/{sessionID}.
func (h *Handler) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	session, err := h.peerManager.GetSession(sessionID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"id":              session.ID,
		"tenant_id":       session.TenantID,
		"device_id":       session.DeviceID,
		"support_user_id": session.SupportUserID,
		"device_user_id":  session.DeviceUserID,
		"direction":       string(session.Direction),
		"state":           string(session.State),
		"relay_enabled":   session.RelayEnabled,
		"relay_provider":  string(session.RelayProvider),
		"ice_candidates":  session.ICECandidates,
		"started_at":      session.StartedAt.Format("2006-01-02T15:04:05Z"),
		"ended_at":        session.EndedAt,
		"error":           session.Error,
	}); err != nil {
		h.logger.Error("failed to encode session response", zap.Error(err))
	}
}

// HandleCreateOffer handles POST /api/v1/webrtc/sessions/{sessionID}/offer.
func (h *Handler) HandleCreateOffer(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		SenderID string `json:"sender_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.SenderID == "" {
		http.Error(w, `{"error":"missing sender_id"}`, http.StatusBadRequest)
		return
	}

	offer, err := h.peerManager.CreateOffer(sessionID, req.SenderID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"type":       string(offer.Type),
		"sdp":        offer.SDP,
		"session_id": offer.SessionID,
		"sender_id":  offer.SenderID,
		"timestamp":  offer.Timestamp.Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		h.logger.Error("failed to encode offer response", zap.Error(err))
	}
}

// HandleHandleAnswer handles POST /api/v1/webrtc/sessions/{sessionID}/answer.
func (h *Handler) HandleHandleAnswer(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		SDP        string `json:"sdp"`
		ReceiverID string `json:"receiver_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.SDP == "" || req.ReceiverID == "" {
		http.Error(w, `{"error":"missing sdp or receiver_id"}`, http.StatusBadRequest)
		return
	}

	answer := &SDPMessage{
		Type:       SDPAnswer,
		SDP:        req.SDP,
		SessionID:  sessionID,
		ReceiverID: req.ReceiverID,
		Timestamp:  time.Now(),
	}

	if err := h.peerManager.HandleAnswer(sessionID, answer); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "connected",
		"session_id":  sessionID,
		"receiver_id": req.ReceiverID,
		"timestamp":   time.Now().Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		h.logger.Error("failed to encode answer response", zap.Error(err))
	}
}

// HandleAddICECandidate handles POST /api/v1/webrtc/sessions/{sessionID}/ice-candidate.
func (h *Handler) HandleAddICECandidate(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	var req ICECandidate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Candidate == "" {
		http.Error(w, `{"error":"missing candidate"}`, http.StatusBadRequest)
		return
	}

	if err := h.peerManager.AddICECandidate(sessionID, &req); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "candidate_added",
		"session_id": sessionID,
		"candidate":  req.Candidate,
		"timestamp":  time.Now().Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		h.logger.Error("failed to encode ICE candidate response", zap.Error(err))
	}
}

// HandleEndSession handles POST /api/v1/webrtc/sessions/{sessionID}/end.
func (h *Handler) HandleEndSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Error string `json:"error,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	session, err := h.peerManager.EndSession(sessionID, req.Error)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "ended",
		"session_id": sessionID,
		"state":      string(session.State),
		"ended_at":   session.EndedAt.Format("2006-01-02T15:04:05Z"),
		"error":      session.Error,
	}); err != nil {
		h.logger.Error("failed to encode end session response", zap.Error(err))
	}
}

// HandleListSessions handles GET /api/v1/webrtc/sessions?tenant_id=xxx.
func (h *Handler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		http.Error(w, `{"error":"missing tenant_id"}`, http.StatusBadRequest)
		return
	}

	sessions := h.peerManager.ListSessions(tenantID)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": sessions,
		"total":    len(sessions),
	}); err != nil {
		h.logger.Error("failed to encode sessions list response", zap.Error(err))
	}
}

// HandleGetRelayConfig handles GET /api/v1/webrtc/sessions/{sessionID}/relay-config.
func (h *Handler) HandleGetRelayConfig(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	relayConfig, err := h.peerManager.GetRelayConfig(sessionID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	if relayConfig == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"relay_enabled": false,
		}); err != nil {
			h.logger.Error("failed to encode relay config response", zap.Error(err))
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"relay_enabled": true,
		"provider":      string(relayConfig.Provider),
		"stun_servers":  relayConfig.STUNServers,
		"turn_servers":  relayConfig.TURNServers,
	}); err != nil {
		h.logger.Error("failed to encode relay config response", zap.Error(err))
	}
}

// HandleStartRecording handles POST /api/v1/webrtc/sessions/{sessionID}/record.
func (h *Handler) HandleStartRecording(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	var req StartRecordingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if err := req.Validate(); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	rs, err := h.recordingManager.StartRecording(r.Context(), sessionID, &req)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"id":             rs.ID,
		"webrtc_session": rs.WebRTCSession,
		"format":         rs.Format,
		"bitrate":        rs.Bitrate,
		"started_at":     rs.StartedAt.Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		h.logger.Error("failed to encode recording response", zap.Error(err))
	}
}

// HandleStopRecording handles POST /api/v1/webrtc/recordings/{recordingID}/stop.
func (h *Handler) HandleStopRecording(w http.ResponseWriter, r *http.Request) {
	recordingID := r.PathValue("recordingID")
	if recordingID == "" {
		http.Error(w, `{"error":"missing recording ID"}`, http.StatusBadRequest)
		return
	}

	rs, err := h.recordingManager.StopRecording(recordingID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "stopped",
		"recording_id":   rs.ID,
		"webrtc_session": rs.WebRTCSession,
		"duration_ms":    rs.DurationMs,
		"file_path":      rs.FilePath,
		"size_bytes":     rs.SizeBytes,
		"ended_at":       rs.EndedAt.Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		h.logger.Error("failed to encode stop recording response", zap.Error(err))
	}
}

// HandleListRecordings handles GET /api/v1/webrtc/sessions/{sessionID}/recordings.
func (h *Handler) HandleListRecordings(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	recordings := h.recordingManager.ListRecordings(sessionID)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"recordings": recordings,
		"total":      len(recordings),
	}); err != nil {
		h.logger.Error("failed to encode recordings list response", zap.Error(err))
	}
}

// HandleStartTranscription handles POST /api/v1/webrtc/sessions/{sessionID}/transcribe.
func (h *Handler) HandleStartTranscription(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	var req StartTranscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if err := req.Validate(); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	if err := ValidateProvider(req.Provider); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	tr, err := h.transcriptionManager.StartTranscription(sessionID, req.Language, req.Provider)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         tr.ID,
		"session_id": tr.SessionID,
		"language":   tr.Language,
		"provider":   string(tr.Provider),
		"status":     string(tr.Status),
		"started_at": tr.StartedAt.Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		h.logger.Error("failed to encode transcription response", zap.Error(err))
	}
}

// HandleStopTranscription handles POST /api/v1/webrtc/transcriptions/{transcriptionID}/stop.
func (h *Handler) HandleStopTranscription(w http.ResponseWriter, r *http.Request) {
	transcriptionID := r.PathValue("transcriptionID")
	if transcriptionID == "" {
		http.Error(w, `{"error":"missing transcription ID"}`, http.StatusBadRequest)
		return
	}

	tr, err := h.transcriptionManager.StopTranscription(transcriptionID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "completed",
		"transcription": tr,
		"segments":      len(tr.Segments),
		"duration_ms":   tr.DurationMs,
		"transcript":    tr.TranscriptText,
		"completed_at":  tr.CompletedAt.Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		h.logger.Error("failed to encode stop transcription response", zap.Error(err))
	}
}

// HandleListTranscriptions handles GET /api/v1/webrtc/sessions/{sessionID}/transcriptions.
func (h *Handler) HandleListTranscriptions(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session ID"}`, http.StatusBadRequest)
		return
	}

	transcriptions := h.transcriptionManager.ListTranscriptions(sessionID)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"transcriptions": transcriptions,
		"total":          len(transcriptions),
	}); err != nil {
		h.logger.Error("failed to encode transcriptions list response", zap.Error(err))
	}
}
