package platform

import (
	"encoding/json"
	"testing"
	"time"
)

func validCommand() CommandEnvelope {
	return CommandEnvelope{
		SchemaVersion: 1,
		EventID:       "evt-001",
		JobID:         "job-001",
		TargetID:      "tgt-001",
		MSPID:         "msp-001",
		ClientID:      "client-001",
		DeviceID:      "dev-001",
		AgentID:       "agent-001",
		CorrelationID: "corr-001",
		Attempt:       1,
		IssuedAt:      time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:     time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		CommandType:   "script",
		Payload:       json.RawMessage(`{"script":"echo hello"}`),
	}
}

func TestValidCommandEnvelope(t *testing.T) {
	cmd := validCommand()
	raw, _ := json.Marshal(cmd)
	parsed, err := ValidateCommandEnvelope(raw, "msp-001", "agent-001", "dev-001")
	if err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}
	if parsed.EventID != "evt-001" {
		t.Errorf("wrong event_id: %s", parsed.EventID)
	}
}

func TestMalformedJSON(t *testing.T) {
	_, err := ValidateCommandEnvelope([]byte(`{invalid}`), "", "", "")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestUnsupportedSchemaVersion(t *testing.T) {
	cmd := validCommand()
	cmd.SchemaVersion = 99
	raw, _ := json.Marshal(cmd)
	_, err := ValidateCommandEnvelope(raw, "", "", "")
	if err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestMissingIdentifiers(t *testing.T) {
	fields := []string{"EventID", "JobID", "TargetID", "MSPID"}
	for _, field := range fields {
		cmd := validCommand()
		switch field {
		case "EventID":
			cmd.EventID = ""
		case "JobID":
			cmd.JobID = ""
		case "TargetID":
			cmd.TargetID = ""
		case "MSPID":
			cmd.MSPID = ""
		}
		raw, _ := json.Marshal(cmd)
		_, err := ValidateCommandEnvelope(raw, "", "", "")
		if err == nil {
			t.Errorf("expected error for missing %s", field)
		}
	}
}

func TestWrongMSP(t *testing.T) {
	cmd := validCommand()
	raw, _ := json.Marshal(cmd)
	_, err := ValidateCommandEnvelope(raw, "other-msp", "", "")
	if err == nil {
		t.Error("expected error for wrong MSP")
	}
}

func TestWrongAgent(t *testing.T) {
	cmd := validCommand()
	raw, _ := json.Marshal(cmd)
	_, err := ValidateCommandEnvelope(raw, "msp-001", "other-agent", "")
	if err == nil {
		t.Error("expected error for wrong agent")
	}
}

func TestWrongDevice(t *testing.T) {
	cmd := validCommand()
	raw, _ := json.Marshal(cmd)
	_, err := ValidateCommandEnvelope(raw, "msp-001", "agent-001", "other-dev")
	if err == nil {
		t.Error("expected error for wrong device")
	}
}

func TestExpiredCommand(t *testing.T) {
	cmd := validCommand()
	cmd.ExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(cmd)
	_, err := ValidateCommandEnvelope(raw, "msp-001", "agent-001", "dev-001")
	if err == nil {
		t.Error("expected error for expired command")
	}
}

func TestFutureIssuedCommand(t *testing.T) {
	cmd := validCommand()
	cmd.IssuedAt = time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(cmd)
	_, err := ValidateCommandEnvelope(raw, "msp-001", "agent-001", "dev-001")
	if err == nil {
		t.Error("expected error for future-issued command")
	}
}

func TestUnsupportedCommandType(t *testing.T) {
	cmd := validCommand()
	cmd.CommandType = "unsupported_type"
	raw, _ := json.Marshal(cmd)
	// Validation should pass but handler won't be found
	parsed, err := ValidateCommandEnvelope(raw, "msp-001", "agent-001", "dev-001")
	if err != nil {
		t.Fatalf("validation should pass: %v", err)
	}
	if parsed.CommandType != "unsupported_type" {
		t.Errorf("wrong command type")
	}
}

func TestInvalidAttempt(t *testing.T) {
	cmd := validCommand()
	cmd.Attempt = 0
	raw, _ := json.Marshal(cmd)
	_, err := ValidateCommandEnvelope(raw, "", "", "")
	if err == nil {
		t.Error("expected error for invalid attempt")
	}
}

func TestEmptyCommandType(t *testing.T) {
	cmd := validCommand()
	cmd.CommandType = ""
	raw, _ := json.Marshal(cmd)
	_, err := ValidateCommandEnvelope(raw, "", "", "")
	if err == nil {
		t.Error("expected error for empty command type")
	}
}

func TestResultEnvelope(t *testing.T) {
	res := ResultEnvelope{
		SchemaVersion: 1,
		MessageID:     "msg-001",
		EventID:       "evt-001",
		JobID:         "job-001",
		TargetID:      "tgt-001",
		MSPID:         "msp-001",
		DeviceID:      "dev-001",
		AgentID:       "agent-001",
		CorrelationID: "corr-001",
		Attempt:       1,
		Status:        "succeeded",
		ExitCode:      0,
		CompletedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(res)
	parsed, err := ValidateResultEnvelope(raw, "msp-001", "agent-001", "dev-001")
	if err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
	if parsed.Status != "succeeded" {
		t.Errorf("wrong status: %s", parsed.Status)
	}
}

func TestInvalidResultStatus(t *testing.T) {
	res := ResultEnvelope{
		SchemaVersion: 1,
		MessageID:     "msg-001",
		EventID:       "evt-001",
		JobID:         "job-001",
		TargetID:      "tgt-001",
		MSPID:         "msp-001",
		DeviceID:      "dev-001",
		AgentID:       "agent-001",
		CorrelationID: "corr-001",
		Attempt:       1,
		Status:        "invalid_status",
	}
	raw, _ := json.Marshal(res)
	_, err := ValidateResultEnvelope(raw, "msp-001", "agent-001", "dev-001")
	if err == nil {
		t.Error("expected error for invalid status")
	}
}
