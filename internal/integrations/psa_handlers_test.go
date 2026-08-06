package integrations

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func newTestPSATicketHandler(t *testing.T) *PSATicketHandler {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return NewPSATicketHandler(logger, nil)
}

func TestHandleCreatePSATicket(t *testing.T) {
	h := newTestPSATicketHandler(t)

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		wantField  string
	}{
		{name: "valid autotask", body: map[string]interface{}{"provider": "autotask", "tenant_id": "t1", "device_id": "d1", "subject": "Alert"}, wantStatus: http.StatusCreated, wantField: "ticket_id"},
		{name: "valid zendesk", body: map[string]interface{}{"provider": "zendesk", "tenant_id": "t1", "device_id": "d1", "subject": "Alert"}, wantStatus: http.StatusCreated, wantField: "ticket_id"},
		{name: "valid freshservice", body: map[string]interface{}{"provider": "freshservice", "tenant_id": "t1", "device_id": "d1", "subject": "Alert"}, wantStatus: http.StatusCreated, wantField: "ticket_id"},
		{name: "valid connectwise", body: map[string]interface{}{"provider": "connectwise", "tenant_id": "t1", "device_id": "d1", "subject": "Alert"}, wantStatus: http.StatusCreated, wantField: "ticket_id"},
		{name: "valid jira", body: map[string]interface{}{"provider": "jira", "tenant_id": "t1", "device_id": "d1", "subject": "Alert"}, wantStatus: http.StatusCreated, wantField: "ticket_id"},
		{name: "missing tenant_id", body: map[string]interface{}{"provider": "autotask", "device_id": "d1", "subject": "Alert"}, wantStatus: http.StatusBadRequest},
		{name: "missing provider", body: map[string]interface{}{"tenant_id": "t1", "device_id": "d1", "subject": "Alert"}, wantStatus: http.StatusBadRequest},
		{name: "missing subject", body: map[string]interface{}{"provider": "autotask", "tenant_id": "t1", "device_id": "d1"}, wantStatus: http.StatusBadRequest},
		{name: "unsupported provider", body: map[string]interface{}{"provider": "invalid", "tenant_id": "t1", "device_id": "d1", "subject": "Alert"}, wantStatus: http.StatusBadRequest},
		{name: "invalid priority", body: map[string]interface{}{"provider": "autotask", "tenant_id": "t1", "device_id": "d1", "subject": "Alert", "priority": "invalid"}, wantStatus: http.StatusBadRequest},
		{name: "default priority medium", body: map[string]interface{}{"provider": "autotask", "tenant_id": "t1", "device_id": "d1", "subject": "Alert"}, wantStatus: http.StatusCreated},
		{name: "valid critical priority", body: map[string]interface{}{"provider": "autotask", "tenant_id": "t1", "device_id": "d1", "subject": "Alert", "priority": "critical"}, wantStatus: http.StatusCreated},
		{name: "invalid JSON", body: nil, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.body != nil {
				b, err := json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("failed to marshal body: %v", err)
				}
				body = b
			}
			req := httptest.NewRequest(http.MethodPost, "/integrations/psa/tickets", bytes.NewReader(body))
			w := httptest.NewRecorder()
			h.HandleCreatePSATicket(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
			if tt.wantStatus == http.StatusCreated && tt.wantField != "" {
				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if _, ok := resp[tt.wantField]; !ok {
					t.Fatalf("expected response field %s, not found in %v", tt.wantField, resp)
				}
			}
		})
	}
}

func TestHandleGetPSATicket(t *testing.T) {
	h := newTestPSATicketHandler(t)
	tests := []struct {
		name       string
		ticketID   string
		wantStatus int
	}{
		{name: "valid ticket", ticketID: "psa-t1-d1", wantStatus: http.StatusOK},
		{name: "missing ticket ID", ticketID: "", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/integrations/psa/tickets/"+tt.ticketID, nil)
			w := httptest.NewRecorder()
			h.HandleGetPSATicket(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestHandleUpdatePSATicket(t *testing.T) {
	h := newTestPSATicketHandler(t)
	tests := []struct {
		name       string
		ticketID   string
		body       map[string]interface{}
		wantStatus int
	}{
		{name: "valid update", ticketID: "psa-t1-d1", body: map[string]interface{}{"status": "resolved"}, wantStatus: http.StatusOK},
		{name: "valid close", ticketID: "psa-t1-d1", body: map[string]interface{}{"status": "closed"}, wantStatus: http.StatusOK},
		{name: "invalid status", ticketID: "psa-t1-d1", body: map[string]interface{}{"status": "invalid"}, wantStatus: http.StatusBadRequest},
		{name: "missing ticket ID", ticketID: "", body: map[string]interface{}{"status": "resolved"}, wantStatus: http.StatusBadRequest},
		{name: "invalid JSON", ticketID: "psa-t1-d1", body: nil, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.body != nil {
				b, err := json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("failed to marshal body: %v", err)
				}
				body = b
			}
			req := httptest.NewRequest(http.MethodPut, "/integrations/psa/tickets/"+tt.ticketID, bytes.NewReader(body))
			w := httptest.NewRecorder()
			h.HandleUpdatePSATicket(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleDeletePSATicket(t *testing.T) {
	h := newTestPSATicketHandler(t)
	tests := []struct {
		name       string
		ticketID   string
		wantStatus int
	}{
		{name: "valid delete", ticketID: "psa-t1-d1", wantStatus: http.StatusOK},
		{name: "missing ticket ID", ticketID: "", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/integrations/psa/tickets/"+tt.ticketID, nil)
			w := httptest.NewRecorder()
			h.HandleDeletePSATicket(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleListPSATicketsByDevice(t *testing.T) {
	h := newTestPSATicketHandler(t)
	tests := []struct {
		name       string
		deviceID   string
		wantStatus int
	}{
		{name: "valid device ID", deviceID: "device-123", wantStatus: http.StatusOK},
		{name: "missing device ID", deviceID: "", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/integrations/psa/tickets/device/"+tt.deviceID, nil)
			w := httptest.NewRecorder()
			h.HandleListPSATicketsByDevice(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
			var resp PSATicketListResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if resp.Total != 0 {
				t.Fatalf("expected total 0, got %d", resp.Total)
			}
		})
	}
}

func TestHandlePSAAlertFeedback(t *testing.T) {
	h := newTestPSATicketHandler(t)
	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{name: "valid feedback", body: map[string]interface{}{"device_id": "d1", "tenant_id": "t1", "alert_id": "a1", "resolved": true, "action": "resolve"}, wantStatus: http.StatusOK},
		{name: "valid close action", body: map[string]interface{}{"device_id": "d1", "tenant_id": "t1", "alert_id": "a1", "resolved": true, "action": "close"}, wantStatus: http.StatusOK},
		{name: "valid escalate action", body: map[string]interface{}{"device_id": "d1", "tenant_id": "t1", "alert_id": "a1", "resolved": false, "action": "escalate"}, wantStatus: http.StatusOK},
		{name: "missing device_id", body: map[string]interface{}{"tenant_id": "t1", "alert_id": "a1"}, wantStatus: http.StatusBadRequest},
		{name: "missing tenant_id", body: map[string]interface{}{"device_id": "d1", "alert_id": "a1"}, wantStatus: http.StatusBadRequest},
		{name: "missing action defaults to resolve", body: map[string]interface{}{"device_id": "d1", "tenant_id": "t1", "alert_id": "a1", "resolved": true}, wantStatus: http.StatusOK},
		{name: "invalid JSON", body: nil, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.body != nil {
				b, err := json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("failed to marshal body: %v", err)
				}
				body = b
			}
			req := httptest.NewRequest(http.MethodPost, "/integrations/psa/feedback", bytes.NewReader(body))
			w := httptest.NewRecorder()
			h.HandlePSAAlertFeedback(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestAutoRemediatePSATicket(t *testing.T) {
	h := newTestPSATicketHandler(t)
	tickets, err := h.AutoRemediatePSATicket("device-1", "alert-1", "tenant-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("expected 0 tickets, got %d", len(tickets))
	}
}

func TestCreatePSATicketFromAlert(t *testing.T) {
	h := newTestPSATicketHandler(t)
	ticketID, err := h.CreatePSATicketFromAlert("alert-1", "device-1", "tenant-1", "critical", "Test Alert")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticketID == "" {
		t.Fatal("expected non-empty ticket ID")
	}
	if ticketID != "psa-alert-1-device-1" {
		t.Fatalf("expected ticket ID psa-alert-1-device-1, got %s", ticketID)
	}
}

func TestPSATicketStructFields(t *testing.T) {
	ticket := &PSATicket{
		ID: "psa-t1", PSAID: "autotask-123", Provider: PSAAutotask,
		TenantID: "tenant-1", DeviceID: "device-1", Subject: "Critical Alert",
		Description: "Device needs attention", Status: "open", Priority: "critical",
		Owner: "admin", CVEs: []string{"CVE-2024-1234"}, Severities: []string{"critical"},
	}
	if ticket.ID != "psa-t1" || ticket.Provider != PSAAutotask || ticket.Status != "open" || ticket.Priority != "critical" {
		t.Fatalf("unexpected struct fields: %+v", ticket)
	}
	if len(ticket.CVEs) != 1 || ticket.CVEs[0] != "CVE-2024-1234" {
		t.Fatalf("unexpected CVEs: %v", ticket.CVEs)
	}
}

func TestPSAProviderConstants(t *testing.T) {
	providers := []PSAProvider{PSAAutotask, PSAConnectWise, PSAFreshservice, PSAZendesk, PSAJira}
	for _, p := range providers {
		p := p
		t.Run(string(p), func(t *testing.T) {
			if string(p) == "" {
				t.Fatal("expected non-empty provider")
			}
		})
	}
}

func TestPSATicketListResponseFields(t *testing.T) {
	resp := PSATicketListResponse{
		Tickets: []PSATicket{{ID: "psa-1", Subject: "Alert 1"}, {ID: "psa-2", Subject: "Alert 2"}},
		Total:   2,
	}
	if resp.Total != 2 || len(resp.Tickets) != 2 || resp.Tickets[0].ID != "psa-1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestExtractPathParam(t *testing.T) {
	tests := []struct {
		path     string
		prefix   string
		expected string
	}{
		{"/integrations/psa/tickets/t123", "/integrations/psa/tickets/", "t123"},
		{"/integrations/psa/tickets/device/d456", "/integrations/psa/tickets/device/", "d456"},
		{"/other/path", "/integrations/psa/tickets/", ""},
		{"", "/integrations/psa/tickets/", ""},
	}
	for _, tt := range tests {
		result := extractPathParam(tt.path, tt.prefix)
		if result != tt.expected {
			t.Fatalf("extractPathParam(%q, %q) = %q, want %q", tt.path, tt.prefix, result, tt.expected)
		}
	}
}
