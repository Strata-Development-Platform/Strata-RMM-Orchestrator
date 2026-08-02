package platform

import (
	"testing"
)

func TestDefaultApprovalPolicyForDestructiveActions(t *testing.T) {
	tests := []struct {
		action       string
		wantRequired bool
	}{
		{"reboot", true},
		{"shutdown", true},
		{"process_kill", true},
		{"refresh", false},
		{"service_start", false},
		{"service_stop", false},
		{"service_restart", false},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			p := defaultApprovalPolicy(tt.action)
			if p.ApprovalRequired != tt.wantRequired {
				t.Errorf("defaultApprovalPolicy(%q).ApprovalRequired = %v, want %v", tt.action, p.ApprovalRequired, tt.wantRequired)
			}
			if p.MinApprovers < 1 {
				t.Error("MinApprovers must be >= 1")
			}
			if len(p.AllowedRoles) == 0 {
				t.Error("AllowedRoles must not be empty")
			}
			if p.ApprovalExpiresSec <= 0 {
				t.Error("ApprovalExpiresSec must be positive")
			}
		})
	}
}

func TestApprovalStateTransitions(t *testing.T) {
	tests := []struct {
		from string
		to   string
		ok   bool
	}{
		{"pending", "approved", true},
		{"pending", "rejected", true},
		{"pending", "cancelled", true},
		{"pending", "expired", true},
		{"approved", "dispatched", true},
		{"approved", "expired", true},
		{"rejected", "approved", false},
		{"rejected", "dispatched", false},
		{"cancelled", "approved", false},
		{"expired", "approved", false},
		{"dispatched", "approved", false},
		{"pending", "dispatched", false},
		{"approved", "cancelled", false},
		{"unknown", "approved", false},
	}
	for _, tt := range tests {
		t.Run(tt.from+"_"+tt.to, func(t *testing.T) {
			err := transitionApprovalStatus(tt.from, tt.to)
			if tt.ok && err != nil {
				t.Errorf("transitionApprovalStatus(%q, %q) = %v, want nil", tt.from, tt.to, err)
			}
			if !tt.ok && err == nil {
				t.Errorf("transitionApprovalStatus(%q, %q) = nil, want error", tt.from, tt.to)
			}
		})
	}
}

func TestIsActionSupportedByCapabilities(t *testing.T) {
	tests := []struct {
		action string
		cap    *AgentCapability
		want   bool
	}{
		{
			action: "refresh",
			cap: &AgentCapability{
				SupportedJobTypes: []string{"device.refresh", "device.reboot"},
			},
			want: true,
		},
		{
			action: "reboot",
			cap: &AgentCapability{
				SupportedJobTypes: []string{"device.refresh"},
			},
			want: false,
		},
		{
			action: "reboot",
			cap:    nil,
			want:   false,
		},
		{
			action: "unknown_action",
			cap: &AgentCapability{
				SupportedJobTypes: []string{"device.reboot"},
			},
			want: false,
		},
		{
			action: "service_restart",
			cap: &AgentCapability{
				SupportedJobTypes: []string{"device.service_restart"},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			got := isActionSupportedByCapabilities(tt.action, tt.cap)
			if got != tt.want {
				t.Errorf("isActionSupportedByCapabilities(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}

func TestValidateDeviceInventoryResult(t *testing.T) {
	tests := []struct {
		name    string
		payload InventoryResultPayload
		wantErr bool
	}{
		{
			name: "valid payload",
			payload: InventoryResultPayload{
				SchemaVersion:  1,
				DeviceID:       "00000000-0000-0000-0000-000000000001",
				AgentID:        "00000000-0000-0000-0000-000000000002",
				CollectionTime: "2026-01-01T00:00:00Z",
			},
			wantErr: false,
		},
		{
			name: "invalid schema version",
			payload: InventoryResultPayload{
				SchemaVersion: 0,
				DeviceID:      "00000000-0000-0000-0000-000000000001",
				AgentID:       "00000000-0000-0000-0000-000000000002",
			},
			wantErr: true,
		},
		{
			name: "missing device id",
			payload: InventoryResultPayload{
				SchemaVersion: 1,
				DeviceID:      "",
				AgentID:       "00000000-0000-0000-0000-000000000002",
			},
			wantErr: true,
		},
		{
			name: "missing agent id",
			payload: InventoryResultPayload{
				SchemaVersion: 1,
				DeviceID:      "00000000-0000-0000-0000-000000000001",
			},
			wantErr: true,
		},
		{
			name: "invalid uuid",
			payload: InventoryResultPayload{
				SchemaVersion: 1,
				DeviceID:      "not-a-uuid",
				AgentID:       "00000000-0000-0000-0000-000000000002",
			},
			wantErr: true,
		},
		{
			name: "future collection time",
			payload: InventoryResultPayload{
				SchemaVersion:  1,
				DeviceID:       "00000000-0000-0000-0000-000000000001",
				AgentID:        "00000000-0000-0000-0000-000000000002",
				CollectionTime: "2099-07-28T00:00:00Z",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeviceInventoryResult(&tt.payload)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
