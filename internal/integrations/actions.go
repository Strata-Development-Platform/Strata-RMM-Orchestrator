package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// JetStreamPublisher abstracts NATS JetStream publishing.
type JetStreamPublisher interface {
	Publish(subject string, data []byte) (*PublishAck, error)
}

// PublishAck is a simplified publish acknowledgment.
type PublishAck struct {
	Subject    string
	Sequence   uint64
	Domain     string
	Stream     string
}

// IsolationAction triggers automated security isolation for a device.
type IsolationAction struct {
	DeviceID  string `json:"device_id"`
	TenantID  string `json:"tenant_id"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
	AlertID   string `json:"alert_id"`
	Provider  string `json:"provider"`
}

// IsolationCommand represents the NATS message sent to isolate a device.
type IsolationCommand struct {
	EventID   string `json:"event_id"`
	DeviceID  string `json:"device_id"`
	TenantID  string `json:"tenant_id"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
	Provider  string `json:"provider"`
	AlertID   string `json:"alert_id"`
	Isolate   bool   `json:"isolate"`
	Timestamp string `json:"timestamp"`
}

// IsolationHandler processes EDR alerts and dispatches isolation commands.
type IsolationHandler struct {
	js     JetStreamPublisher
	logger *zap.Logger
}

// NewIsolationHandler creates a new isolation handler.
func NewIsolationHandler(js JetStreamPublisher, logger *zap.Logger) *IsolationHandler {
	return &IsolationHandler{js: js, logger: logger}
}

// HandleIsolation processes an EDR high-severity alert and dispatches isolation.
func (h *IsolationHandler) HandleIsolation(w http.ResponseWriter, r *http.Request) {
	var alert IsolationAction
	if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if alert.DeviceID == "" || alert.TenantID == "" {
		http.Error(w, `{"error":"missing required fields: device_id, tenant_id"}`, http.StatusBadRequest)
		return
	}

	if alert.Severity == "" {
		alert.Severity = "high"
	}

	if h.js == nil {
		http.Error(w, `{"error":"nats not configured"}`, http.StatusServiceUnavailable)
		return
	}

	cmd := IsolationCommand{
		EventID:   fmt.Sprintf("iso-%s", alert.AlertID),
		DeviceID:  alert.DeviceID,
		TenantID:  alert.TenantID,
		Reason:    alert.Reason,
		Severity:  alert.Severity,
		Provider:  alert.Provider,
		AlertID:   alert.AlertID,
		Isolate:   true,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	subject := fmt.Sprintf("tenant.%s.cmd.isolate", alert.TenantID)
	payload, err := json.Marshal(cmd)
	if err != nil {
		http.Error(w, `{"error":"failed to marshal command"}`, http.StatusInternalServerError)
		return
	}

	_, err = h.js.Publish(subject, payload)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("failed to publish isolation command",
				zap.String("subject", subject),
				zap.String("device_id", alert.DeviceID),
				zap.Error(err),
			)
		}
		http.Error(w, `{"error":"failed to dispatch isolation"}`, http.StatusServiceUnavailable)
		return
	}

	if h.logger != nil {
		h.logger.Info("dispatched isolation command",
			zap.String("tenant_id", alert.TenantID),
			zap.String("device_id", alert.DeviceID),
			zap.String("severity", alert.Severity),
			zap.String("reason", alert.Reason),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "isolated",
		"event_id":  cmd.EventID,
		"device_id": cmd.DeviceID,
	})
}

// contextWithIntegrationID adds the integration ID to the context.
func contextWithIntegrationID(ctx context.Context, id, integration string) context.Context {
	return context.WithValue(ctx, IntegrationContextKey, IntegrationContext{
		ID:          id,
		Integration: integration,
	})
}
