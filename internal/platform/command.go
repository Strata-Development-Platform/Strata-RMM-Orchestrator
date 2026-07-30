package platform

import (
	"encoding/json"
	"fmt"
	"time"
)

const CurrentSchemaVersion = 1

type CommandEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	EventID       string          `json:"event_id"`
	JobID         string          `json:"job_id"`
	TargetID      string          `json:"target_id"`
	MSPID         string          `json:"msp_id"`
	ClientID      string          `json:"client_id,omitempty"`
	SiteID        string          `json:"site_id,omitempty"`
	DeviceID      string          `json:"device_id"`
	AgentID       string          `json:"agent_id,omitempty"`
	CorrelationID string          `json:"correlation_id"`
	Attempt       int             `json:"attempt"`
	IssuedAt      string          `json:"issued_at"`
	ExpiresAt     string          `json:"expires_at"`
	CommandType   string          `json:"command_type"`
	Payload       json.RawMessage `json:"payload"`
}

type Acknowledgement struct {
	SchemaVersion int    `json:"schema_version"`
	MessageID     string `json:"message_id"`
	EventID       string `json:"event_id"`
	JobID         string `json:"job_id"`
	TargetID      string `json:"target_id"`
	MSPID         string `json:"msp_id"`
	DeviceID      string `json:"device_id"`
	AgentID       string `json:"agent_id"`
	CorrelationID string `json:"correlation_id"`
	Attempt       int    `json:"attempt"`
	Status        string `json:"status"`
	Timestamp     string `json:"timestamp"`
}

const (
	AckAccepted    = "accepted"
	AckDuplicate   = "duplicate"
	AckRejected    = "rejected"
	AckExpired     = "expired"
	AckUnsupported = "unsupported"
)

type ResultEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	MessageID     string          `json:"message_id"`
	EventID       string          `json:"event_id"`
	JobID         string          `json:"job_id"`
	TargetID      string          `json:"target_id"`
	MSPID         string          `json:"msp_id"`
	ClientID      string          `json:"client_id,omitempty"`
	SiteID        string          `json:"site_id,omitempty"`
	DeviceID      string          `json:"device_id"`
	AgentID       string          `json:"agent_id"`
	CorrelationID string          `json:"correlation_id"`
	Attempt       int             `json:"attempt"`
	Status        string          `json:"status"`
	ExitCode      int             `json:"exit_code"`
	Result        json.RawMessage `json:"result"`
	Error         string          `json:"error,omitempty"`
	StartedAt     string          `json:"started_at"`
	CompletedAt   string          `json:"completed_at"`
	DurationMs    int64           `json:"duration_ms"`
	Truncated     bool            `json:"truncated,omitempty"`
}

func ValidateCommandEnvelope(raw []byte, enrolledMSPID, enrolledAgentID, enrolledDeviceID string) (*CommandEnvelope, error) {
	var cmd CommandEnvelope
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return nil, fmt.Errorf("malformed command: %w", err)
	}

	if cmd.SchemaVersion <= 0 || cmd.SchemaVersion > CurrentSchemaVersion {
		return nil, fmt.Errorf("unsupported schema version: %d", cmd.SchemaVersion)
	}
	if cmd.EventID == "" || cmd.JobID == "" || cmd.TargetID == "" {
		return nil, fmt.Errorf("missing required identifiers")
	}
	if cmd.MSPID == "" {
		return nil, fmt.Errorf("missing msp_id")
	}
	if enrolledMSPID != "" && cmd.MSPID != enrolledMSPID {
		return nil, fmt.Errorf("msp_id mismatch: %s", cmd.MSPID)
	}
	if enrolledAgentID != "" && cmd.AgentID != "" && cmd.AgentID != enrolledAgentID {
		return nil, fmt.Errorf("agent_id mismatch: %s", cmd.AgentID)
	}
	if enrolledDeviceID != "" && cmd.DeviceID != enrolledDeviceID {
		return nil, fmt.Errorf("device_id mismatch: %s", cmd.DeviceID)
	}
	if cmd.Attempt < 1 {
		return nil, fmt.Errorf("invalid attempt: %d", cmd.Attempt)
	}
	if cmd.CommandType == "" {
		return nil, fmt.Errorf("missing command_type")

	}

	issuedAt, err := time.Parse(time.RFC3339, cmd.IssuedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid issued_at: %w", err)
	}
	if issuedAt.After(time.Now().Add(5 * time.Minute)) {
		return nil, fmt.Errorf("command issued in the future")
	}

	if cmd.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, cmd.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("invalid expires_at: %w", err)
		}
		if time.Now().After(expiresAt) {
			return nil, fmt.Errorf("command expired")
		}
	}

	return &cmd, nil
}

func ValidateResultEnvelope(raw []byte, enrolledMSPID, enrolledAgentID, enrolledDeviceID string) (*ResultEnvelope, error) {
	var res ResultEnvelope
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("malformed result: %w", err)
	}
	if res.SchemaVersion <= 0 || res.SchemaVersion > CurrentSchemaVersion {
		return nil, fmt.Errorf("unsupported schema version: %d", res.SchemaVersion)
	}
	if res.EventID == "" || res.JobID == "" || res.TargetID == "" {
		return nil, fmt.Errorf("missing required identifiers")
	}
	if res.MSPID == "" {
		return nil, fmt.Errorf("missing msp_id")
	}
	if enrolledMSPID != "" && res.MSPID != enrolledMSPID {
		return nil, fmt.Errorf("msp_id mismatch")
	}
	if enrolledAgentID != "" && res.AgentID != enrolledAgentID {
		return nil, fmt.Errorf("agent_id mismatch")
	}
	if enrolledDeviceID != "" && res.DeviceID != enrolledDeviceID {
		return nil, fmt.Errorf("device_id mismatch")
	}
	if res.Attempt < 1 {
		return nil, fmt.Errorf("invalid attempt: %d", res.Attempt)
	}
	validResults := map[string]bool{"succeeded": true, "failed": true, "cancelled": true, "expired": true}
	if !validResults[res.Status] {
		return nil, fmt.Errorf("invalid result status: %s", res.Status)
	}
	return &res, nil
}
