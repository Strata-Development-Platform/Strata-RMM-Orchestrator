package integrations

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestHandleEDRAlertValid(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	alert := EDRAlert{
		Provider: "sentinelone",
		AlertID:  "alert-123",
		DeviceID: "device-456",
		TenantID: "tenant-789",
		Severity: "critical",
		Title:    "Malware detected",
	}

	payload, _ := json.Marshal(alert)

	req := httptest.NewRequest("POST", "/api/v1/integrations/edr/alerts", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.HandleEDRAlert(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp["status"] != "received" {
		t.Errorf("expected status 'received', got %v", resp["status"])
	}
}

func TestHandleEDRAlertInvalidJSON(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	req := httptest.NewRequest("POST", "/api/v1/integrations/edr/alerts", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()

	handler.HandleEDRAlert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleEDRAlertMissingFields(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	alert := EDRAlert{
		Provider: "sentinelone",
		// Missing AlertID and DeviceID
		TenantID: "tenant-789",
		Severity: "high",
		Title:    "Test",
	}

	payload, _ := json.Marshal(alert)

	req := httptest.NewRequest("POST", "/api/v1/integrations/edr/alerts", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.HandleEDRAlert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "missing required fields") {
		t.Errorf("expected 'missing required fields' error, got: %s", rec.Body.String())
	}
}

func TestHandleBackupSyncValid(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	sync := BackupSync{
		Provider: "veeam",
		TenantID: "tenant-789",
		DeviceID: "device-456",
		Status:   "success",
	}

	payload, _ := json.Marshal(sync)

	req := httptest.NewRequest("POST", "/api/v1/integrations/backup/sync", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.HandleBackupSync(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp["status"] != "received" {
		t.Errorf("expected status 'received', got %v", resp["status"])
	}
}

func TestHandleBackupSyncMissingDeviceID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	sync := BackupSync{
		Provider: "veeam",
		TenantID: "tenant-789",
		// Missing DeviceID
		Status: "success",
	}

	payload, _ := json.Marshal(sync)

	req := httptest.NewRequest("POST", "/api/v1/integrations/backup/sync", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.HandleBackupSync(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePSAWebhookValid(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	psa := PSAWebhook{
		Provider: "halopsa",
		TenantID: "tenant-789",
		Action:   "created",
		TicketID: "ticket-123",
		Subject:  "Server down",
	}

	payload, _ := json.Marshal(psa)

	req := httptest.NewRequest("POST", "/api/v1/integrations/psa/webhooks", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.HandlePSAWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp["status"] != "received" {
		t.Errorf("expected status 'received', got %v", resp["status"])
	}
}

func TestHandlePSAWebhookMissingTenant(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	psa := PSAWebhook{
		Provider: "halopsa",
		// Missing TenantID
		Action: "created",
	}

	payload, _ := json.Marshal(psa)

	req := httptest.NewRequest("POST", "/api/v1/integrations/psa/webhooks", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.HandlePSAWebhook(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestNormalizeSeverity(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"critical", "critical"},
		{"Critical_Severity", "critical"},
		{"severe", "critical"},
		{"high", "high"},
		{"high_severity", "high"},
		{"medium", "medium"},
		{"moderate", "medium"},
		{"low", "low"},
		{"informational", "informational"},
		{"info", "informational"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSeverity(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeSeverity(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
