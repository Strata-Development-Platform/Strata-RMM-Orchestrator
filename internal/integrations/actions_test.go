package integrations

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockJetStreamPublisher implements JetStreamPublisher for testing.
type mockJetStreamPublisher struct {
	lastSubject string
	lastData    []byte
}

func (m *mockJetStreamPublisher) Publish(subject string, data []byte) (*PublishAck, error) {
	m.lastSubject = subject
	m.lastData = data
	return &PublishAck{Subject: subject, Sequence: 1}, nil
}

func TestHandleIsolationValid(t *testing.T) {
	mock := &mockJetStreamPublisher{}
	handler := NewIsolationHandler(mock, nil)

	action := IsolationAction{
		DeviceID: "device-456",
		TenantID: "tenant-789",
		Reason:   "Malware detected",
		Severity: "critical",
		AlertID:  "alert-123",
		Provider: "crowdstrike",
	}

	payload, _ := json.Marshal(action)

	req := httptest.NewRequest("POST", "/api/v1/integrations/isolate", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.HandleIsolation(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp["status"] != "isolated" {
		t.Errorf("expected status 'isolated', got %v", resp["status"])
	}

	if mock.lastSubject != "tenant.tenant-789.cmd.isolate" {
		t.Errorf("expected subject 'tenant.tenant-789.cmd.isolate', got %s", mock.lastSubject)
	}

	var cmd IsolationCommand
	if err := json.Unmarshal(mock.lastData, &cmd); err != nil {
		t.Fatal(err)
	}

	if cmd.DeviceID != "device-456" {
		t.Errorf("expected device_id 'device-456', got %s", cmd.DeviceID)
	}

	if !cmd.Isolate {
		t.Error("expected isolate to be true")
	}
}

func TestHandleIsolationInvalidJSON(t *testing.T) {
	handler := NewIsolationHandler(nil, nil)

	req := httptest.NewRequest("POST", "/api/v1/integrations/isolate", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()

	handler.HandleIsolation(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleIsolationMissingFields(t *testing.T) {
	mock := &mockJetStreamPublisher{}
	handler := NewIsolationHandler(mock, nil)

	action := IsolationAction{
		// Missing DeviceID and TenantID
		Reason: "Test",
	}

	payload, _ := json.Marshal(action)

	req := httptest.NewRequest("POST", "/api/v1/integrations/isolate", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.HandleIsolation(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleIsolationNilPublisher(t *testing.T) {
	handler := NewIsolationHandler(nil, nil)

	action := IsolationAction{
		DeviceID: "device-456",
		TenantID: "tenant-789",
	}

	payload, _ := json.Marshal(action)

	req := httptest.NewRequest("POST", "/api/v1/integrations/isolate", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.HandleIsolation(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}
