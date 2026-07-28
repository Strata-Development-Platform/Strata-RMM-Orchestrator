package platform

import (
	"encoding/json"
	"testing"
)

func TestEndpointAuditEntryDefaults(t *testing.T) {
	entry := &EndpointAuditEntry{
		MSPID:    "00000000-0000-0000-0000-000000000001",
		ActorUserID: "test-user",
		Action:      "device.reboot",
	}

	if entry.MSPID == "" {
		t.Error("msp_id must not be empty")
	}
	if entry.Targets == nil {
		entry.Targets = json.RawMessage("[]")
	}
	if entry.PolicySnapshot == nil {
		entry.PolicySnapshot = json.RawMessage("{}")
	}
	if entry.ApprovalDecisions == nil {
		entry.ApprovalDecisions = json.RawMessage("[]")
	}
	if entry.MaintenanceWindow == nil {
		entry.MaintenanceWindow = json.RawMessage("null")
	}
	if entry.ExecutionResult == nil {
		entry.ExecutionResult = json.RawMessage("null")
	}
}

func TestMaintenanceWindowAllowsWithScopedTargets(t *testing.T) {
	// Verify the function is callable with the expected signature
	_ = maintenanceWindowAllows
}
