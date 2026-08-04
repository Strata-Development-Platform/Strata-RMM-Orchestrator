package integrations

import (
	"encoding/json"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// EDRAlert represents a normalized alert from any EDR integration.
type EDRAlert struct {
	Provider    string    `json:"provider"`
	AlertID     string    `json:"alert_id"`
	DeviceID    string    `json:"device_id"`
	TenantID    string    `json:"tenant_id"`
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Timestamp   string    `json:"timestamp"`
	Actions     []Action  `json:"actions,omitempty"`
	NetworkInfo []Network `json:"network_info,omitempty"`
}

// Action represents a remediation action available for an alert.
type Action struct {
	Name        string `json:"name"`
	Command     string `json:"command,omitempty"`
	RequiresApp string `json:"requires_app,omitempty"`
}

// Network represents network connection information.
type Network struct {
	Direction  string `json:"direction"`
	Protocol   string `json:"protocol"`
	LocalAddr  string `json:"local_addr"`
	RemoteAddr string `json:"remote_addr"`
}

// BackupSync represents a backup status event.
type BackupSync struct {
	Provider  string `json:"provider"`
	TenantID  string `json:"tenant_id"`
	DeviceID  string `json:"device_id"`
	Status    string `json:"status"` // "success", "failed", "in_progress"
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// PSAWebhook represents a PSA ticket event.
type PSAWebhook struct {
	Provider  string `json:"provider"`
	TenantID  string `json:"tenant_id"`
	Action    string `json:"action"` // "created", "updated", "closed"
	TicketID  string `json:"ticket_id"`
	Subject   string `json:"subject"`
	DeviceID  string `json:"device_id"`
	Owner     string `json:"owner"`
	Severity  string `json:"severity"`
	Timestamp string `json:"timestamp"`
}

// WebhookHandler dispatches webhook payloads to the appropriate handlers.
type WebhookHandler struct {
	logger *zap.Logger
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{logger: logger}
}

// HandleEDRAlert processes an EDR alert and routes it to the NATS command bus.
func (h *WebhookHandler) HandleEDRAlert(w http.ResponseWriter, r *http.Request) {
	var alert EDRAlert
	if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Validate required fields
	if alert.DeviceID == "" || alert.AlertID == "" {
		http.Error(w, `{"error":"missing required fields: device_id, alert_id"}`, http.StatusBadRequest)
		return
	}

	// Normalize severity
	alert.Severity = normalizeSeverity(alert.Severity)

	// Log and forward to NATS
	if h.logger != nil {
		h.logger.Info("received EDR alert",
			zap.String("provider", alert.Provider),
			zap.String("tenant_id", alert.TenantID),
			zap.String("device_id", alert.DeviceID),
			zap.String("severity", alert.Severity),
			zap.String("alert_id", alert.AlertID),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "received",
		"alert_id": alert.AlertID,
		"provider": alert.Provider,
	})
}

// HandleBackupSync processes backup sync status events.
func (h *WebhookHandler) HandleBackupSync(w http.ResponseWriter, r *http.Request) {
	var sync BackupSync
	if err := json.NewDecoder(r.Body).Decode(&sync); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if sync.DeviceID == "" {
		http.Error(w, `{"error":"missing required field: device_id"}`, http.StatusBadRequest)
		return
	}

	if sync.Status == "" {
		sync.Status = "unknown"
	}

	if h.logger != nil {
		h.logger.Info("received backup sync event",
			zap.String("provider", sync.Provider),
			zap.String("tenant_id", sync.TenantID),
			zap.String("device_id", sync.DeviceID),
			zap.String("status", sync.Status),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "received",
		"device_id":     sync.DeviceID,
		"backup_status": sync.Status,
	})
}

// HandlePSAWebhook processes PSA ticket events.
func (h *WebhookHandler) HandlePSAWebhook(w http.ResponseWriter, r *http.Request) {
	var psa PSAWebhook
	if err := json.NewDecoder(r.Body).Decode(&psa); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if psa.TenantID == "" {
		http.Error(w, `{"error":"missing required field: tenant_id"}`, http.StatusBadRequest)
		return
	}

	if psa.Action == "" {
		psa.Action = "unknown"
	}

	if h.logger != nil {
		h.logger.Info("received PSA webhook event",
			zap.String("provider", psa.Provider),
			zap.String("tenant_id", psa.TenantID),
			zap.String("action", psa.Action),
			zap.String("ticket_id", psa.TicketID),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "received",
		"ticket_id": psa.TicketID,
		"action":    psa.Action,
	})
}

// normalizeSeverity normalizes severity strings to standard values.
func normalizeSeverity(sev string) string {
	s := strings.ToLower(strings.TrimSpace(sev))
	switch s {
	case "critical", "critical_severity", "severe":
		return "critical"
	case "high", "high_severity", "serious":
		return "high"
	case "medium", "medium_severity", "moderate":
		return "medium"
	case "low", "low_severity":
		return "low"
	case "informational", "info":
		return "informational"
	default:
		return s
	}
}
