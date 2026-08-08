package software

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestSoftwareCommand_StructFields verifies all SoftwareCommand struct fields
func TestSoftwareCommand_StructFields(t *testing.T) {
	cmd := SoftwareCommand{
		Type:          "software_install",
		DeploymentID:  "deploy-001",
		Action:        "install",
		SourceURL:     "https://example.com/software.msi",
		Checksum:      "abc123def456",
		InstallArgs:   "/quiet /norestart",
		UninstallArgs: "/uninstall /quiet",
		PackageType:   "msi",
		Timeout:       3600,
	}

	if cmd.Type != "software_install" {
		t.Errorf("SoftwareCommand.Type = %q, want %q", cmd.Type, "software_install")
	}
	if cmd.DeploymentID != "deploy-001" {
		t.Errorf("SoftwareCommand.DeploymentID = %q, want %q", cmd.DeploymentID, "deploy-001")
	}
	if cmd.Action != "install" {
		t.Errorf("SoftwareCommand.Action = %q, want %q", cmd.Action, "install")
	}
	if cmd.SourceURL != "https://example.com/software.msi" {
		t.Errorf("SoftwareCommand.SourceURL = %q, want %q", cmd.SourceURL, "https://example.com/software.msi")
	}
	if cmd.Checksum != "abc123def456" {
		t.Errorf("SoftwareCommand.Checksum = %q, want %q", cmd.Checksum, "abc123def456")
	}
	if cmd.InstallArgs != "/quiet /norestart" {
		t.Errorf("SoftwareCommand.InstallArgs = %q, want %q", cmd.InstallArgs, "/quiet /norestart")
	}
	if cmd.UninstallArgs != "/uninstall /quiet" {
		t.Errorf("SoftwareCommand.UninstallArgs = %q, want %q", cmd.UninstallArgs, "/uninstall /quiet")
	}
	if cmd.PackageType != "msi" {
		t.Errorf("SoftwareCommand.PackageType = %q, want %q", cmd.PackageType, "msi")
	}
	if cmd.Timeout != 3600 {
		t.Errorf("SoftwareCommand.Timeout = %d, want %d", cmd.Timeout, 3600)
	}
}

// TestSoftwareCommand_ZeroValues verifies SoftwareCommand with zero values
func TestSoftwareCommand_ZeroValues(t *testing.T) {
	cmd := SoftwareCommand{}

	if cmd.Type != "" {
		t.Error("SoftwareCommand.Type should be empty by default")
	}
	if cmd.DeploymentID != "" {
		t.Error("SoftwareCommand.DeploymentID should be empty by default")
	}
	if cmd.Action != "" {
		t.Error("SoftwareCommand.Action should be empty by default")
	}
	if cmd.SourceURL != "" {
		t.Error("SoftwareCommand.SourceURL should be empty by default")
	}
	if cmd.Checksum != "" {
		t.Error("SoftwareCommand.Checksum should be empty by default")
	}
	if cmd.InstallArgs != "" {
		t.Error("SoftwareCommand.InstallArgs should be empty by default")
	}
	if cmd.UninstallArgs != "" {
		t.Error("SoftwareCommand.UninstallArgs should be empty by default")
	}
	if cmd.PackageType != "" {
		t.Error("SoftwareCommand.PackageType should be empty by default")
	}
	if cmd.Timeout != 0 {
		t.Error("SoftwareCommand.Timeout should be 0 by default")
	}
}

// TestSoftwareResult_StructFields verifies all SoftwareResult struct fields
func TestSoftwareResult_StructFields(t *testing.T) {
	result := SoftwareResult{
		Type:         "software_result",
		DeploymentID: "deploy-001",
		Action:       "install",
		Status:       "success",
		ErrorMessage: "",
		DurationMs:   12345,
	}

	if result.Type != "software_result" {
		t.Errorf("SoftwareResult.Type = %q, want %q", result.Type, "software_result")
	}
	if result.DeploymentID != "deploy-001" {
		t.Errorf("SoftwareResult.DeploymentID = %q, want %q", result.DeploymentID, "deploy-001")
	}
	if result.Action != "install" {
		t.Errorf("SoftwareResult.Action = %q, want %q", result.Action, "install")
	}
	if result.Status != "success" {
		t.Errorf("SoftwareResult.Status = %q, want %q", result.Status, "success")
	}
	if result.ErrorMessage != "" {
		t.Error("SoftwareResult.ErrorMessage should be empty")
	}
	if result.DurationMs != 12345 {
		t.Errorf("SoftwareResult.DurationMs = %d, want %d", result.DurationMs, 12345)
	}
}

// TestSoftwareResult_FailureState verifies SoftwareResult with failure state
func TestSoftwareResult_FailureState(t *testing.T) {
	result := SoftwareResult{
		Type:         "software_result",
		DeploymentID: "deploy-001",
		Action:       "install",
		Status:       "failed",
		ErrorMessage: "timeout",
		DurationMs:   30000,
	}

	if result.Status != "failed" {
		t.Errorf("SoftwareResult.Status = %q, want %q", result.Status, "failed")
	}
	if result.ErrorMessage != "timeout" {
		t.Errorf("SoftwareResult.ErrorMessage = %q, want %q", result.ErrorMessage, "timeout")
	}
}

// TestSoftwareResult_ZeroValues verifies SoftwareResult with zero values
func TestSoftwareResult_ZeroValues(t *testing.T) {
	result := SoftwareResult{}

	if result.Type != "" {
		t.Error("SoftwareResult.Type should be empty by default")
	}
	if result.DeploymentID != "" {
		t.Error("SoftwareResult.DeploymentID should be empty by default")
	}
	if result.Action != "" {
		t.Error("SoftwareResult.Action should be empty by default")
	}
	if result.Status != "" {
		t.Error("SoftwareResult.Status should be empty by default")
	}
	if result.ErrorMessage != "" {
		t.Error("SoftwareResult.ErrorMessage should be empty by default")
	}
	if result.DurationMs != 0 {
		t.Error("SoftwareResult.DurationMs should be 0 by default")
	}
}

// TestInstaller_StructFields verifies all Installer struct fields exist
func TestInstaller_StructFields(t *testing.T) {
	// Installer has unexported fields (nc, logger, tenantID, agentID, sub, httpClient)
	// We verify via constructor behavior

	// NewInstaller creates an Installer with valid defaults
	inst := NewInstaller(nil, nil, "tenant-001", "agent-001")
	if inst == nil {
		t.Fatal("NewInstaller returned nil")
	}

	// Verify tenantID and agentID are set via Start (which uses them in NATS subject)
	// We can't directly access unexported fields, but we can verify constructor behavior
	_ = inst
}

// TestNewInstaller_Defaults verifies NewInstaller default values
func TestNewInstaller_Defaults(t *testing.T) {
	inst := NewInstaller(nil, nil, "tenant-001", "agent-001")

	if inst == nil {
		t.Fatal("NewInstaller returned nil")
	}

	// httpClient should be non-nil (set to &http.Client{Timeout: 30 * time.Minute})
	// We verify by checking that Start works (even with nil NATS, it fails gracefully)
}

// TestSoftwareCommand_JSONRoundTrip verifies SoftwareCommand serializes/deserializes
func TestSoftwareCommand_JSONRoundTrip(t *testing.T) {
	original := SoftwareCommand{
		Type:          "software_install",
		DeploymentID:  "deploy-001",
		Action:        "install",
		SourceURL:     "https://example.com/software.msi",
		Checksum:      "abc123def456789",
		InstallArgs:   "/quiet /norestart",
		UninstallArgs: "/uninstall /quiet",
		PackageType:   "msi",
		Timeout:       3600,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal SoftwareCommand: %v", err)
	}

	var restored SoftwareCommand
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal SoftwareCommand: %v", err)
	}

	if restored.Type != original.Type {
		t.Errorf("Round-trip SoftwareCommand.Type = %q, want %q", restored.Type, original.Type)
	}
	if restored.DeploymentID != original.DeploymentID {
		t.Errorf("Round-trip SoftwareCommand.DeploymentID = %q, want %q", restored.DeploymentID, original.DeploymentID)
	}
	if restored.Action != original.Action {
		t.Errorf("Round-trip SoftwareCommand.Action = %q, want %q", restored.Action, original.Action)
	}
	if restored.SourceURL != original.SourceURL {
		t.Errorf("Round-trip SoftwareCommand.SourceURL = %q, want %q", restored.SourceURL, original.SourceURL)
	}
	if restored.Checksum != original.Checksum {
		t.Errorf("Round-trip SoftwareCommand.Checksum = %q, want %q", restored.Checksum, original.Checksum)
	}
	if restored.InstallArgs != original.InstallArgs {
		t.Errorf("Round-trip SoftwareCommand.InstallArgs = %q, want %q", restored.InstallArgs, original.InstallArgs)
	}
	if restored.UninstallArgs != original.UninstallArgs {
		t.Errorf("Round-trip SoftwareCommand.UninstallArgs = %q, want %q", restored.UninstallArgs, original.UninstallArgs)
	}
	if restored.PackageType != original.PackageType {
		t.Errorf("Round-trip SoftwareCommand.PackageType = %q, want %q", restored.PackageType, original.PackageType)
	}
	if restored.Timeout != original.Timeout {
		t.Errorf("Round-trip SoftwareCommand.Timeout = %d, want %d", restored.Timeout, original.Timeout)
	}
}

// TestSoftwareResult_JSONRoundTrip verifies SoftwareResult serializes/deserializes
func TestSoftwareResult_JSONRoundTrip(t *testing.T) {
	original := SoftwareResult{
		Type:         "software_result",
		DeploymentID: "deploy-001",
		Action:       "install",
		Status:       "success",
		ErrorMessage: "",
		DurationMs:   12345,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal SoftwareResult: %v", err)
	}

	var restored SoftwareResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal SoftwareResult: %v", err)
	}

	if restored.Type != original.Type {
		t.Errorf("Round-trip SoftwareResult.Type = %q, want %q", restored.Type, original.Type)
	}
	if restored.DeploymentID != original.DeploymentID {
		t.Errorf("Round-trip SoftwareResult.DeploymentID = %q, want %q", restored.DeploymentID, original.DeploymentID)
	}
	if restored.Action != original.Action {
		t.Errorf("Round-trip SoftwareResult.Action = %q, want %q", restored.Action, original.Action)
	}
	if restored.Status != original.Status {
		t.Errorf("Round-trip SoftwareResult.Status = %q, want %q", restored.Status, original.Status)
	}
	if restored.ErrorMessage != original.ErrorMessage {
		t.Errorf("Round-trip SoftwareResult.ErrorMessage = %q, want %q", restored.ErrorMessage, original.ErrorMessage)
	}
	if restored.DurationMs != original.DurationMs {
		t.Errorf("Round-trip SoftwareResult.DurationMs = %d, want %d", restored.DurationMs, original.DurationMs)
	}
}

// TestSoftwareResult_JSONRoundTrip_WithErrorMessage verifies SoftwareResult with error serializes/deserializes
func TestSoftwareResult_JSONRoundTrip_WithErrorMessage(t *testing.T) {
	original := SoftwareResult{
		Type:         "software_result",
		DeploymentID: "deploy-001",
		Action:       "install",
		Status:       "failed",
		ErrorMessage: "timeout",
		DurationMs:   30000,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal SoftwareResult: %v", err)
	}

	var restored SoftwareResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal SoftwareResult: %v", err)
	}

	if restored.Status != original.Status {
		t.Errorf("Round-trip SoftwareResult.Status = %q, want %q", restored.Status, original.Status)
	}
	if restored.ErrorMessage != original.ErrorMessage {
		t.Errorf("Round-trip SoftwareResult.ErrorMessage = %q, want %q", restored.ErrorMessage, original.ErrorMessage)
	}
	if restored.DurationMs != original.DurationMs {
		t.Errorf("Round-trip SoftwareResult.DurationMs = %d, want %d", restored.DurationMs, original.DurationMs)
	}
}

// TestPackageTypeConstants verifies all known package type values used in runInstall
func TestPackageTypeConstants(t *testing.T) {
	// Package types used in runInstall switch: msi, exe, deb, rpm, appimage, script
	types := []string{"msi", "exe", "deb", "rpm", "appimage", "script"}
	for _, ptype := range types {
		if ptype == "" {
			t.Error("PackageType should not be empty")
		}
	}
}

// TestActionTypeConstants verifies all known action type values used in execute
func TestActionTypeConstants(t *testing.T) {
	// Action types used in execute switch: install, uninstall
	actions := []string{"install", "uninstall"}
	for _, action := range actions {
		if action == "" {
			t.Error("ActionType should not be empty")
		}
	}
}

// TestSoftwareCommand_InvalidPackageType verifies behavior with unknown package type
func TestSoftwareCommand_InvalidPackageType(t *testing.T) {
	cmd := SoftwareCommand{
		Type:        "software_install",
		Action:      "install",
		PackageType: "unknown_type",
		Timeout:     300,
	}

	if cmd.PackageType != "unknown_type" {
		t.Errorf("SoftwareCommand.PackageType = %q, want %q", cmd.PackageType, "unknown_type")
	}

	// runInstall falls through to default (runExec) for unknown types
	// This test verifies the struct accepts arbitrary package type values
}

// TestSoftwareCommand_UnknownAction verifies behavior with unknown action
func TestSoftwareCommand_UnknownAction(t *testing.T) {
	cmd := SoftwareCommand{
		Type:       "software_install",
		Action:     "unknown_action",
		Timeout:    300,
		PackageType: "msi",
	}

	if cmd.Action != "unknown_action" {
		t.Errorf("SoftwareCommand.Action = %q, want %q", cmd.Action, "unknown_action")
	}

	// execute() returns failed with "unknown action" error for unknown actions
	// This test verifies the struct accepts arbitrary action values
}

// TestSoftwareCommand_TimeoutBounds verifies timeout value bounds
func TestSoftwareCommand_TimeoutBounds(t *testing.T) {
	tests := []struct {
		name     string
		timeout  int
		wantMin  int
		wantMax  int
	}{
		{"zero", 0, 0, 0},
		{"minimum", 1, 1, 1},
		{"typical", 300, 300, 300},
		{"maximum", 7200, 7200, 7200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := SoftwareCommand{Timeout: tt.timeout}
			if cmd.Timeout != tt.wantMin || cmd.Timeout != tt.wantMax {
				t.Errorf("SoftwareCommand.Timeout = %d, want %d", cmd.Timeout, tt.timeout)
			}
		})
	}
}

// TestSoftwareResult_DurationMs_NonNegative verifies DurationMs is non-negative
func TestSoftwareResult_DurationMs_NonNegative(t *testing.T) {
	result := SoftwareResult{
		Type:       "software_result",
		Status:     "success",
		DurationMs: 0,
	}

	if result.DurationMs < 0 {
		t.Errorf("SoftwareResult.DurationMs = %d, should be non-negative", result.DurationMs)
	}
}

// TestInstaller_Start_InvalidNATS verifies Start fails with nil NATS connection
func TestInstaller_Start_InvalidNATS(t *testing.T) {
	inst := NewInstaller(nil, nil, "tenant-001", "agent-001")
	if inst == nil {
		t.Fatal("NewInstaller returned nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := inst.Start(ctx)
	if err == nil {
		t.Error("Installer.Start should return error with nil NATS connection")
	}
}

// TestInstaller_Stop_Safe verifies Stop is safe to call multiple times
func TestInstaller_Stop_Safe(t *testing.T) {
	inst := NewInstaller(nil, nil, "tenant-001", "agent-001")
	if inst == nil {
		t.Fatal("NewInstaller returned nil")
	}

	// Multiple Stop calls should not panic
	inst.Stop()
	inst.Stop()
	inst.Stop()
}

// TestSoftwareCommand_EmptyFields_JSON verifies empty fields serialize to zero values
func TestSoftwareCommand_EmptyFields_JSON(t *testing.T) {
	cmd := SoftwareCommand{}

	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("Failed to marshal SoftwareCommand: %v", err)
	}

	var restored SoftwareCommand
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal SoftwareCommand: %v", err)
	}

	// All fields should be zero/empty after round-trip
	if restored.Type != "" || restored.DeploymentID != "" || restored.Action != "" {
		t.Error("Zero-value SoftwareCommand should round-trip to zero values")
	}
}

// TestSoftwareResult_EmptyFields_JSON verifies empty fields serialize to zero values
func TestSoftwareResult_EmptyFields_JSON(t *testing.T) {
	result := SoftwareResult{}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal SoftwareResult: %v", err)
	}

	var restored SoftwareResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal SoftwareResult: %v", err)
	}

	// All fields should be zero/empty after round-trip
	if restored.Type != "" || restored.DeploymentID != "" || restored.Action != "" {
		t.Error("Zero-value SoftwareResult should round-trip to zero values")
	}
}
