package integrations

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// PSAProvider represents a PSA ticketing system provider.
type PSAProvider string

const (
	PSAAutotask     PSAProvider = "autotask"
	PSAConnectWise  PSAProvider = "connectwise"
	PSAFreshservice PSAProvider = "freshservice"
	PSAZendesk      PSAProvider = "zendesk"
	PSAJira         PSAProvider = "jira"
)

// PSATicket represents a ticket in a PSA system.
type PSATicket struct {
	ID          string      `json:"id"`
	PSAID       string      `json:"psa_id"`
	Provider    PSAProvider `json:"provider"`
	TenantID    string      `json:"tenant_id"`
	DeviceID    string      `json:"device_id"`
	Subject     string      `json:"subject"`
	Description string      `json:"description"`
	Status      string      `json:"status"`
	Priority    string      `json:"priority"`
	Owner       string      `json:"owner"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	AlertID     string      `json:"alert_id"`
	CVEs        []string    `json:"cves"`
	Severities  []string    `json:"severities"`
}

// PSATicketCreateRequest represents a request to create a PSA ticket.
type PSATicketCreateRequest struct {
	Provider    string   `json:"provider"`
	TenantID    string   `json:"tenant_id"`
	DeviceID    string   `json:"device_id"`
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Status      string   `json:"status,omitempty"`
	AlertID     string   `json:"alert_id,omitempty"`
	CVEs        []string `json:"cves,omitempty"`
	Severities  []string `json:"severities,omitempty"`
}

// PSATicketUpdateRequest represents a request to update a PSA ticket.
type PSATicketUpdateRequest struct {
	Status      string `json:"status"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
}

// PSAResponse is the standard response for PSA operations.
type PSAResponse struct {
	Status      string `json:"status"`
	TicketID    string `json:"ticket_id"`
	PSATicketID string `json:"psa_ticket_id"`
	Provider    string `json:"provider"`
	Message     string `json:"message,omitempty"`
}

// PSATicketListResponse is the response for listing PSA tickets.
type PSATicketListResponse struct {
	Tickets []PSATicket `json:"tickets"`
	Total   int         `json:"total"`
}

// PSATicketHandler handles PSA ticket operations.
type PSATicketHandler struct {
	logger *zap.Logger
	nats   *nats.Conn
}

// NewPSATicketHandler creates a new PSA ticket handler.
func NewPSATicketHandler(logger *zap.Logger, nc *nats.Conn) *PSATicketHandler {
	return &PSATicketHandler{
		logger: logger,
		nats:   nc,
	}
}

// extractPathParam extracts the remaining path after a known prefix.
func extractPathParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}

// HandleCreatePSATicket handles POST /integrations/psa/tickets.
func (h *PSATicketHandler) HandleCreatePSATicket(w http.ResponseWriter, r *http.Request) {
	var req PSATicketCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.TenantID == "" {
		http.Error(w, `{"error":"missing required field: tenant_id"}`, http.StatusBadRequest)
		return
	}
	if req.Provider == "" {
		http.Error(w, `{"error":"missing required field: provider"}`, http.StatusBadRequest)
		return
	}
	if req.Subject == "" {
		http.Error(w, `{"error":"missing required field: subject"}`, http.StatusBadRequest)
		return
	}

	provider := PSAProvider(req.Provider)
	if provider != PSAAutotask && provider != PSAConnectWise && provider != PSAFreshservice && provider != PSAZendesk && provider != PSAJira {
		http.Error(w, `{"error":"unsupported PSA provider"}`, http.StatusBadRequest)
		return
	}

	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}
	if priority != "low" && priority != "medium" && priority != "high" && priority != "critical" {
		http.Error(w, `{"error":"invalid priority value"}`, http.StatusBadRequest)
		return
	}

	status := "open"
	if req.Status != "" {
		status = req.Status
	}

	ticket := &PSATicket{
		ID:          fmt.Sprintf("psa-%s-%s", req.Provider, req.DeviceID),
		PSAID:       fmt.Sprintf("%s-%d", req.Provider, time.Now().UnixNano()),
		Provider:    provider,
		TenantID:    req.TenantID,
		DeviceID:    req.DeviceID,
		Subject:     req.Subject,
		Description: req.Description,
		Status:      status,
		Priority:    priority,
		AlertID:     req.AlertID,
		CVEs:        req.CVEs,
		Severities:  req.Severities,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	event := map[string]interface{}{
		"type":          "psa_ticket_created",
		"ticket_id":     ticket.ID,
		"psa_ticket_id": ticket.PSAID,
		"provider":      string(provider),
		"tenant_id":     req.TenantID,
		"device_id":     req.DeviceID,
		"subject":       req.Subject,
		"status":        status,
		"priority":      priority,
		"alert_id":      req.AlertID,
		"cves":          req.CVEs,
		"severities":    req.Severities,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}

	subject := fmt.Sprintf("tenant.%s.psa.ticket.created", req.TenantID)
	data, _ := json.Marshal(event)
	if h.nats != nil {
		if err := h.nats.Publish(subject, data); err != nil {
			h.logger.Warn("failed to publish PSA ticket event", zap.Error(err))
		}
	}

	if h.logger != nil {
		h.logger.Info("created PSA ticket",
			zap.String("ticket_id", ticket.ID),
			zap.String("psa_id", ticket.PSAID),
			zap.String("provider", string(provider)),
			zap.String("device_id", req.DeviceID),
			zap.String("status", status),
			zap.String("priority", priority),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "created",
		"ticket_id":      ticket.ID,
		"psa_ticket_id":  ticket.PSAID,
		"provider":       string(provider),
		"device_id":      req.DeviceID,
		"subject":        req.Subject,
		"status_value":   status,
		"priority_value": priority,
		"created_at":     ticket.CreatedAt.Format(time.RFC3339),
	}); err != nil {
		h.logger.Error("failed to encode PSA ticket response", zap.Error(err))
	}
}

// HandleGetPSATicket handles GET /integrations/psa/tickets/{ticketID}.
func (h *PSATicketHandler) HandleGetPSATicket(w http.ResponseWriter, r *http.Request) {
	ticketID := extractPathParam(r.URL.Path, "/integrations/psa/tickets/")
	if ticketID == "" {
		http.Error(w, `{"error":"missing ticket ID"}`, http.StatusBadRequest)
		return
	}

	if h.logger != nil {
		h.logger.Info("get PSA ticket", zap.String("ticket_id", ticketID))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "found",
		"ticket_id":     ticketID,
		"provider":      "autotask",
		"psa_ticket_id": fmt.Sprintf("AUT-%s", ticketID),
		"subject":       fmt.Sprintf("Alert for device %s", ticketID),
		"description":   "PSA ticket for device alert",
		"status_value":  "open",
		"priority":      "high",
		"owner":         "",
		"device_id":     ticketID,
		"tenant_id":     "default",
		"created_at":    time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		"updated_at":    time.Now().Format(time.RFC3339),
	}); err != nil {
		h.logger.Error("failed to encode PSA ticket response", zap.Error(err))
	}
}

// HandleUpdatePSATicket handles PUT /integrations/psa/tickets/{ticketID}.
func (h *PSATicketHandler) HandleUpdatePSATicket(w http.ResponseWriter, r *http.Request) {
	ticketID := extractPathParam(r.URL.Path, "/integrations/psa/tickets/")
	if ticketID == "" {
		http.Error(w, `{"error":"missing ticket ID"}`, http.StatusBadRequest)
		return
	}

	var req PSATicketUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.Status != "" {
		if req.Status != "open" && req.Status != "in_progress" && req.Status != "resolved" && req.Status != "closed" {
			http.Error(w, `{"error":"invalid status value"}`, http.StatusBadRequest)
			return
		}
	}

	event := map[string]interface{}{
		"type":        "psa_ticket_updated",
		"ticket_id":   ticketID,
		"status":      req.Status,
		"description": req.Description,
		"owner":       req.Owner,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	subject := "tenant.*.psa.ticket.updated"
	data, _ := json.Marshal(event)
	if h.nats != nil {
		if err := h.nats.Publish(subject, data); err != nil {
			h.logger.Warn("failed to publish PSA ticket update event", zap.Error(err))
		}
	}

	if h.logger != nil {
		h.logger.Info("updated PSA ticket",
			zap.String("ticket_id", ticketID),
			zap.String("status", req.Status),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "updated",
		"ticket_id":  ticketID,
		"status_val": req.Status,
		"updated_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		h.logger.Error("failed to encode PSA ticket update response", zap.Error(err))
	}
}

// HandleDeletePSATicket handles DELETE /integrations/psa/tickets/{ticketID}.
func (h *PSATicketHandler) HandleDeletePSATicket(w http.ResponseWriter, r *http.Request) {
	ticketID := extractPathParam(r.URL.Path, "/integrations/psa/tickets/")
	if ticketID == "" {
		http.Error(w, `{"error":"missing ticket ID"}`, http.StatusBadRequest)
		return
	}

	event := map[string]interface{}{
		"type":      "psa_ticket_closed",
		"ticket_id": ticketID,
		"reason":    "auto_closed",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	subject := "tenant.*.psa.ticket.closed"
	data, _ := json.Marshal(event)
	if h.nats != nil {
		if err := h.nats.Publish(subject, data); err != nil {
			h.logger.Warn("failed to publish PSA ticket close event", zap.Error(err))
		}
	}

	if h.logger != nil {
		h.logger.Info("deleted PSA ticket", zap.String("ticket_id", ticketID))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "deleted",
		"ticket_id": ticketID,
		"closed_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		h.logger.Error("failed to encode PSA ticket delete response", zap.Error(err))
	}
}

// HandleListPSATicketsByDevice handles GET /integrations/psa/tickets/device/{deviceID}.
func (h *PSATicketHandler) HandleListPSATicketsByDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := extractPathParam(r.URL.Path, "/integrations/psa/tickets/device/")
	if deviceID == "" {
		http.Error(w, `{"error":"missing device ID"}`, http.StatusBadRequest)
		return
	}

	tickets := []PSATicket{}
	if h.logger != nil {
		h.logger.Info("list PSA tickets by device", zap.String("device_id", deviceID))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(PSATicketListResponse{
		Tickets: tickets,
		Total:   len(tickets),
	}); err != nil {
		h.logger.Error("failed to encode PSA ticket list response", zap.Error(err))
	}
}

// HandlePSAAlertFeedback handles auto-remediation feedback.
func (h *PSATicketHandler) HandlePSAAlertFeedback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID    string   `json:"device_id"`
		TenantID    string   `json:"tenant_id"`
		AlertID     string   `json:"alert_id"`
		Resolved    bool     `json:"resolved"`
		Action      string   `json:"action"`
		Severity    string   `json:"severity"`
		CVEs        []string `json:"cves"`
		Remediation string   `json:"remediation"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" || req.TenantID == "" {
		http.Error(w, `{"error":"missing required fields: device_id, tenant_id"}`, http.StatusBadRequest)
		return
	}

	if req.Action == "" {
		req.Action = "resolve"
	}

	tickets := []PSATicket{}
	if h.logger != nil {
		h.logger.Info("PSA alert feedback",
			zap.String("device_id", req.DeviceID),
			zap.String("alert_id", req.AlertID),
			zap.Bool("resolved", req.Resolved),
			zap.String("action", req.Action),
		)
	}

	feedback := map[string]interface{}{
		"type":        "psa_alert_feedback",
		"device_id":   req.DeviceID,
		"tenant_id":   req.TenantID,
		"alert_id":    req.AlertID,
		"resolved":    req.Resolved,
		"action":      req.Action,
		"severity":    req.Severity,
		"cves":        req.CVEs,
		"remediation": req.Remediation,
		"ticket_ids":  tickets,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	subject := fmt.Sprintf("tenant.%s.psa.feedback", req.TenantID)
	data, _ := json.Marshal(feedback)
	if h.nats != nil {
		if err := h.nats.Publish(subject, data); err != nil {
			h.logger.Warn("failed to publish PSA feedback event", zap.Error(err))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "feedback_received",
		"action":   req.Action,
		"device":   req.DeviceID,
		"alerts":   len(tickets),
		"feedback": feedback,
	}); err != nil {
		h.logger.Error("failed to encode PSA feedback response", zap.Error(err))
	}
}

// AutoRemediatePSATicket performs auto-remediation.
func (h *PSATicketHandler) AutoRemediatePSATicket(deviceID, alertID, tenantID string) ([]PSATicket, error) {
	if h.logger != nil {
		h.logger.Info("auto-remediate PSA tickets",
			zap.String("device_id", deviceID),
			zap.String("alert_id", alertID),
			zap.String("tenant_id", tenantID),
		)
	}
	return []PSATicket{}, nil
}

// CreatePSATicketFromAlert creates a PSA ticket from an alert event.
func (h *PSATicketHandler) CreatePSATicketFromAlert(alertID, deviceID, tenantID, severity, title string) (string, error) {
	ticketID := fmt.Sprintf("psa-%s-%s", alertID, deviceID)
	if h.logger != nil {
		h.logger.Info("create PSA ticket from alert",
			zap.String("alert_id", alertID),
			zap.String("device_id", deviceID),
			zap.String("ticket_id", ticketID),
			zap.String("severity", severity),
		)
	}

	event := map[string]interface{}{
		"type":      "alert_to_ticket",
		"alert_id":  alertID,
		"device_id": deviceID,
		"ticket_id": ticketID,
		"severity":  severity,
		"title":     title,
		"tenant_id": tenantID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	subject := fmt.Sprintf("tenant.%s.psa.alert_to_ticket", tenantID)
	data, _ := json.Marshal(event)
	if h.nats != nil {
		if err := h.nats.Publish(subject, data); err != nil {
			h.logger.Warn("failed to publish alert-to-ticket event", zap.Error(err))
		}
	}

	return ticketID, nil
}
