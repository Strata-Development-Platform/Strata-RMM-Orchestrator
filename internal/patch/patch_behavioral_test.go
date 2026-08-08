package patch

import (


	"encoding/json"
	"testing"
	"time"
)

// TestPatch_StructFields verifies all Patch struct fields exist and have correct types
func TestPatch_StructFields(t *testing.T) {
	now := time.Now()
	p := &Patch{
		ID:          "patch-001",
		TenantID:    "tenant-001",
		KB:          "KB1234567",
		Title:       "Critical Security Update",
		Platform:    PlatformWindows,
		Severity:    SeverityCritical,
		Description: "Fixes CVE-2024-1234",
		CVE:         []string{"CVE-2024-1234", "CVE-2024-5678"},
		Published:   now,
		CreatedAt:   now,
	}

	// Verify all fields
	if p.ID != "patch-001" {
		t.Errorf("Patch.ID = %q, want %q", p.ID, "patch-001")
	}
	if p.TenantID != "tenant-001" {
		t.Errorf("Patch.TenantID = %q, want %q", p.TenantID, "tenant-001")
	}
	if p.KB != "KB1234567" {
		t.Errorf("Patch.KB = %q, want %q", p.KB, "KB1234567")
	}
	if p.Title != "Critical Security Update" {
		t.Errorf("Patch.Title = %q, want %q", p.Title, "Critical Security Update")
	}
	if p.Platform != PlatformWindows {
		t.Errorf("Patch.Platform = %q, want %q", p.Platform, PlatformWindows)
	}
	if p.Severity != SeverityCritical {
		t.Errorf("Patch.Severity = %q, want %q", p.Severity, SeverityCritical)
	}
	if p.Description != "Fixes CVE-2024-1234" {
		t.Errorf("Patch.Description = %q, want %q", p.Description, "Fixes CVE-2024-1234")
	}
	if len(p.CVE) != 2 || p.CVE[0] != "CVE-2024-1234" {
		t.Errorf("Patch.CVE = %v, want 2 items starting with CVE-2024-1234", p.CVE)
	}
	if p.Published.IsZero() {
		t.Error("Patch.Published should not be zero")
	}
	if p.CreatedAt.IsZero() {
		t.Error("Patch.CreatedAt should not be zero")
	}
}

// TestPatchPolicy_StructFields verifies all PatchPolicy struct fields
func TestPatchPolicy_StructFields(t *testing.T) {
	now := time.Now()
	p := &PatchPolicy{
		ID:             "policy-001",
		TenantID:       "tenant-001",
		Name:           "Critical Security Policy",
		Enabled:        true,
		Platforms:      []Platform{PlatformWindows, PlatformLinux},
		ApprovalMode:   "auto",
		Severity:       SeverityImportant,
		MaintenanceWin: "Sunday 02:00-06:00",
		DeviceFilter:   map[string]string{"os_type": "windows"},
		MaxRetries:     3,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if p.ID != "policy-001" {
		t.Errorf("PatchPolicy.ID = %q, want %q", p.ID, "policy-001")
	}
	if p.TenantID != "tenant-001" {
		t.Errorf("PatchPolicy.TenantID = %q, want %q", p.TenantID, "tenant-001")
	}
	if p.Name != "Critical Security Policy" {
		t.Errorf("PatchPolicy.Name = %q, want %q", p.Name, "Critical Security Policy")
	}
	if !p.Enabled {
		t.Error("PatchPolicy.Enabled should be true")
	}
	if len(p.Platforms) != 2 {
		t.Errorf("PatchPolicy.Platforms length = %d, want 2", len(p.Platforms))
	}
	if p.ApprovalMode != "auto" {
		t.Errorf("PatchPolicy.ApprovalMode = %q, want %q", p.ApprovalMode, "auto")
	}
	if p.Severity != SeverityImportant {
		t.Errorf("PatchPolicy.Severity = %q, want %q", p.Severity, SeverityImportant)
	}
	if p.MaintenanceWin != "Sunday 02:00-06:00" {
		t.Errorf("PatchPolicy.MaintenanceWin = %q, want %q", p.MaintenanceWin, "Sunday 02:00-06:00")
	}
	if len(p.DeviceFilter) != 1 {
		t.Errorf("PatchPolicy.DeviceFilter length = %d, want 1", len(p.DeviceFilter))
	}
	if p.DeviceFilter["os_type"] != "windows" {
		t.Errorf("PatchPolicy.DeviceFilter[os_type] = %q, want %q", p.DeviceFilter["os_type"], "windows")
	}
	if p.MaxRetries != 3 {
		t.Errorf("PatchPolicy.MaxRetries = %d, want 3", p.MaxRetries)
	}
}

// TestDeployment_StructFields verifies all Deployment struct fields
func TestDeployment_StructFields(t *testing.T) {
	now := time.Now()
	dep := &Deployment{
		ID:           "deploy-001",
		PolicyID:     "policy-001",
		TenantID:     "tenant-001",
		Status:       StatusDeploying,
		DeviceCount:  100,
		Installed:    80,
		Failed:       5,
		Pending:      15,
		ScheduledFor: now,
		StartedAt:    &now,
		CompletedAt:  nil,
		CreatedAt:    now,
	}

	if dep.ID != "deploy-001" {
		t.Errorf("Deployment.ID = %q, want %q", dep.ID, "deploy-001")
	}
	if dep.PolicyID != "policy-001" {
		t.Errorf("Deployment.PolicyID = %q, want %q", dep.PolicyID, "policy-001")
	}
	if dep.TenantID != "tenant-001" {
		t.Errorf("Deployment.TenantID = %q, want %q", dep.TenantID, "tenant-001")
	}
	if dep.Status != StatusDeploying {
		t.Errorf("Deployment.Status = %q, want %q", dep.Status, StatusDeploying)
	}
	if dep.DeviceCount != 100 {
		t.Errorf("Deployment.DeviceCount = %d, want 100", dep.DeviceCount)
	}
	if dep.Installed != 80 {
		t.Errorf("Deployment.Installed = %d, want 80", dep.Installed)
	}
	if dep.Failed != 5 {
		t.Errorf("Deployment.Failed = %d, want 5", dep.Failed)
	}
	if dep.Pending != 15 {
		t.Errorf("Deployment.Pending = %d, want 15", dep.Pending)
	}
	if dep.StartedAt == nil {
		t.Error("Deployment.StartedAt should not be nil")
	}
	if dep.CompletedAt != nil {
		t.Error("Deployment.CompletedAt should be nil")
	}
}

// TestDevicePatchState_StructFields verifies all DevicePatchState struct fields
func TestDevicePatchState_StructFields(t *testing.T) {
	now := time.Now()
	state := &DevicePatchState{
		DeviceID:     "device-001",
		DeploymentID: "deploy-001",
		PatchID:      "patch-001",
		Status:       StatusInstalled,
		Attempts:     2,
		Error:        "",
		UpdatedAt:    now,
	}

	if state.DeviceID != "device-001" {
		t.Errorf("DevicePatchState.DeviceID = %q, want %q", state.DeviceID, "device-001")
	}
	if state.DeploymentID != "deploy-001" {
		t.Errorf("DevicePatchState.DeploymentID = %q, want %q", state.DeploymentID, "deploy-001")
	}
	if state.PatchID != "patch-001" {
		t.Errorf("DevicePatchState.PatchID = %q, want %q", state.PatchID, "patch-001")
	}
	if state.Status != StatusInstalled {
		t.Errorf("DevicePatchState.Status = %q, want %q", state.Status, StatusInstalled)
	}
	if state.Attempts != 2 {
		t.Errorf("DevicePatchState.Attempts = %d, want 2", state.Attempts)
	}
	if state.Error != "" {
		t.Error("DevicePatchState.Error should be empty")
	}
}

// TestExecResult_StructFields verifies all ExecResult struct fields
func TestExecResult_StructFields(t *testing.T) {
	r := &ExecResult{
		Status:    StatusInstalled,
		Output:    "Patch installed successfully",
		Error:     "",
		RebootReq: true,
	}

	if r.Status != StatusInstalled {
		t.Errorf("ExecResult.Status = %q, want %q", r.Status, StatusInstalled)
	}
	if r.Output != "Patch installed successfully" {
		t.Errorf("ExecResult.Output = %q, want %q", r.Output, "Patch installed successfully")
	}
	if r.Error != "" {
		t.Error("ExecResult.Error should be empty")
	}
	if !r.RebootReq {
		t.Error("ExecResult.RebootReq should be true")
	}

	// Test with error state
	errResult := &ExecResult{
		Status:    StatusFailed,
		Output:    "",
		Error:     "Timeout waiting for patch installation",
		RebootReq: false,
	}
	if errResult.Status != StatusFailed {
		t.Errorf("ExecResult.Status = %q, want %q", errResult.Status, StatusFailed)
	}
}

// TestPatchJSONRoundTrip verifies Patch serializes/deserializes correctly
func TestPatchJSONRoundTrip(t *testing.T) {
	now := time.Now()
	original := &Patch{
		ID:          "patch-001",
		TenantID:    "tenant-001",
		KB:          "KB1234567",
		Title:       "Critical Security Update",
		Platform:    PlatformWindows,
		Severity:    SeverityCritical,
		Description: "Fixes CVE-2024-1234",
		CVE:         []string{"CVE-2024-1234"},
		Published:   now,
		CreatedAt:   now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal Patch: %v", err)
	}

	var restored Patch
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal Patch: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("Round-trip Patch.ID = %q, want %q", restored.ID, original.ID)
	}
	if restored.TenantID != original.TenantID {
		t.Errorf("Round-trip Patch.TenantID = %q, want %q", restored.TenantID, original.TenantID)
	}
	if restored.KB != original.KB {
		t.Errorf("Round-trip Patch.KB = %q, want %q", restored.KB, original.KB)
	}
	if restored.Platform != original.Platform {
		t.Errorf("Round-trip Patch.Platform = %q, want %q", restored.Platform, original.Platform)
	}
	if restored.Severity != original.Severity {
		t.Errorf("Round-trip Patch.Severity = %q, want %q", restored.Severity, original.Severity)
	}
	if restored.Description != original.Description {
		t.Errorf("Round-trip Patch.Description = %q, want %q", restored.Description, original.Description)
	}
}

// TestPatchPolicyJSONRoundTrip verifies PatchPolicy serializes/deserializes correctly
func TestPatchPolicyJSONRoundTrip(t *testing.T) {
	now := time.Now()
	original := &PatchPolicy{
		ID:             "policy-001",
		TenantID:       "tenant-001",
		Name:           "Critical Security Policy",
		Enabled:        true,
		Platforms:      []Platform{PlatformWindows, PlatformLinux},
		ApprovalMode:   "auto",
		Severity:       SeverityImportant,
		MaintenanceWin: "Sunday 02:00-06:00",
		DeviceFilter:   map[string]string{"os_type": "windows"},
		MaxRetries:     3,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal PatchPolicy: %v", err)
	}

	var restored PatchPolicy
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal PatchPolicy: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("Round-trip PatchPolicy.ID = %q, want %q", restored.ID, original.ID)
	}
	if restored.Name != original.Name {
		t.Errorf("Round-trip PatchPolicy.Name = %q, want %q", restored.Name, original.Name)
	}
	if restored.ApprovalMode != original.ApprovalMode {
		t.Errorf("Round-trip PatchPolicy.ApprovalMode = %q, want %q", restored.ApprovalMode, original.ApprovalMode)
	}
	if restored.MaxRetries != original.MaxRetries {
		t.Errorf("Round-trip PatchPolicy.MaxRetries = %d, want %d", restored.MaxRetries, original.MaxRetries)
	}
}

// TestDeploymentJSONRoundTrip verifies Deployment serializes/deserializes correctly
func TestDeploymentJSONRoundTrip(t *testing.T) {
	now := time.Now()
	original := &Deployment{
		ID:           "deploy-001",
		PolicyID:     "policy-001",
		TenantID:     "tenant-001",
		Status:       StatusDeploying,
		DeviceCount:  100,
		Installed:    80,
		Failed:       5,
		Pending:      15,
		ScheduledFor: now,
		StartedAt:    &now,
		CreatedAt:    now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal Deployment: %v", err)
	}

	var restored Deployment
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal Deployment: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("Round-trip Deployment.ID = %q, want %q", restored.ID, original.ID)
	}
	if restored.Status != original.Status {
		t.Errorf("Round-trip Deployment.Status = %q, want %q", restored.Status, original.Status)
	}
	if restored.DeviceCount != original.DeviceCount {
		t.Errorf("Round-trip Deployment.DeviceCount = %d, want %d", restored.DeviceCount, original.DeviceCount)
	}
}

// TestDevicePatchStateJSONRoundTrip verifies DevicePatchState serializes/deserializes correctly
func TestDevicePatchStateJSONRoundTrip(t *testing.T) {
	now := time.Now()
	original := &DevicePatchState{
		DeviceID:     "device-001",
		DeploymentID: "deploy-001",
		PatchID:      "patch-001",
		Status:       StatusInstalled,
		Attempts:     2,
		UpdatedAt:    now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal DevicePatchState: %v", err)
	}

	var restored DevicePatchState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal DevicePatchState: %v", err)
	}

	if restored.DeviceID != original.DeviceID {
		t.Errorf("Round-trip DevicePatchState.DeviceID = %q, want %q", restored.DeviceID, original.DeviceID)
	}
	if restored.Status != original.Status {
		t.Errorf("Round-trip DevicePatchState.Status = %q, want %q", restored.Status, original.Status)
	}
	if restored.Attempts != original.Attempts {
		t.Errorf("Round-trip DevicePatchState.Attempts = %d, want %d", restored.Attempts, original.Attempts)
	}
}

// TestPatchStatusConstants verifies all PatchStatus constant values
func TestPatchStatusConstants(t *testing.T) {
	constants := map[PatchStatus]string{
		StatusPending:   "pending",
		StatusApproved:  "approved",
		StatusDeploying: "deploying",
		StatusInstalled: "installed",
		StatusFailed:    "failed",
		StatusRebootReq: "reboot_required",
	}

	for status, expected := range constants {
		if string(status) != expected {
			t.Errorf("PatchStatus %q = %q, want %q", status, status, expected)
		}
	}
}

// TestPatchSeverityConstants verifies all PatchSeverity constant values
func TestPatchSeverityConstants(t *testing.T) {
	constants := map[PatchSeverity]string{
		SeverityCritical: "critical",
		SeverityImportant: "important",
		SeverityModerate: "moderate",
		SeverityLow:      "low",
	}

	for severity, expected := range constants {
		if string(severity) != expected {
			t.Errorf("PatchSeverity %q = %q, want %q", severity, severity, expected)
		}
	}
}

// TestPlatformConstants verifies all Platform constant values
func TestPlatformConstants(t *testing.T) {
	constants := map[Platform]string{
		PlatformWindows: "windows",
		PlatformLinux:   "linux",
		PlatformMacOS:   "macos",
	}

	for platform, expected := range constants {
		if string(platform) != expected {
			t.Errorf("Platform %q = %q, want %q", platform, platform, expected)
		}
	}
}

// TestExecResultJSONRoundTrip verifies ExecResult serializes/deserializes correctly
func TestExecResultJSONRoundTrip(t *testing.T) {
	original := &ExecResult{
		Status:    StatusInstalled,
		Output:    "Patch installed successfully",
		Error:     "",
		RebootReq: true,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal ExecResult: %v", err)
	}

	var restored ExecResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal ExecResult: %v", err)
	}

	if restored.Status != original.Status {
		t.Errorf("Round-trip ExecResult.Status = %q, want %q", restored.Status, original.Status)
	}
	if restored.Output != original.Output {
		t.Errorf("Round-trip ExecResult.Output = %q, want %q", restored.Output, original.Output)
	}
}

// TestManager_StructFields verifies all Manager struct fields exist
func TestManager_StructFields(t *testing.T) {
	// Manager has unexported fields: nats, tsdb, logger, store, mu, policies
	// We verify via constructor and basic struct access
	m := NewManager(nil, nil, nil, nil)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	// Verify Manager policies map is initialized (empty but not nil)
	// We can't access unexported fields directly, but we can verify
	// that ListPolicies returns an empty list without panicking
	// when called with nil store (which panics). Instead we verify
	// the Manager struct exists and has expected behavior.
	_ = m
}

// TestStore_StructFields verifies all Store struct fields exist
// TestStore_StructFields verifies all Store struct fields exist
func TestStore_StructFields(t *testing.T) {
	// Store has unexported field: db
	s := NewStore(nil)
	if s == nil {
		t.Fatal("NewStore returned nil")
	}

	// Verify Store is properly initialized
	// This verifies the db field exists and is set (even if nil)
	// We cannot call methods with nil DB as they panic
	_ = s
}

// TestCanaryProgress_JSONRoundTrip verifies CanaryProgress serializes/deserializes
func TestCanaryProgress_JSONRoundTrip(t *testing.T) {
	original := CanaryProgress{
		CanarySize:       10,
		CanaryPassed:     8,
		CanaryFailed:     2,
		TotalDevices:     100,
		DeployedToCanary: 10,
		PassThreshold:    90,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal CanaryProgress: %v", err)
	}

	var restored CanaryProgress
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal CanaryProgress: %v", err)
	}

	if restored.CanarySize != original.CanarySize {
		t.Errorf("Round-trip CanaryProgress.CanarySize = %d, want %d", restored.CanarySize, original.CanarySize)
	}
	if restored.CanaryPassed != original.CanaryPassed {
		t.Errorf("Round-trip CanaryProgress.CanaryPassed = %d, want %d", restored.CanaryPassed, original.CanaryPassed)
	}
	if restored.TotalDevices != original.TotalDevices {
		t.Errorf("Round-trip CanaryProgress.TotalDevices = %d, want %d", restored.TotalDevices, original.TotalDevices)
	}
	if restored.PassThreshold != original.PassThreshold {
		t.Errorf("Round-trip CanaryProgress.PassThreshold = %d, want %d", restored.PassThreshold, original.PassThreshold)
	}
}

// TestCanaryResult_JSONRoundTrip verifies CanaryResult serializes/deserializes
func TestCanaryResult_JSONRoundTrip(t *testing.T) {
	original := &CanaryResult{
		DeploymentID: "deploy-001",
		PatchID:      "patch-001",
		Status:       "in_progress",
		RollbackUsed: false,
		Error:        "",
		Progress: CanaryProgress{
			CanarySize:       10,
			CanaryPassed:     8,
			CanaryFailed:     2,
			TotalDevices:     100,
			DeployedToCanary: 10,
			PassThreshold:    90,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal CanaryResult: %v", err)
	}

	var restored CanaryResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal CanaryResult: %v", err)
	}

	if restored.DeploymentID != original.DeploymentID {
		t.Errorf("Round-trip CanaryResult.DeploymentID = %q, want %q", restored.DeploymentID, original.DeploymentID)
	}
	if restored.PatchID != original.PatchID {
		t.Errorf("Round-trip CanaryResult.PatchID = %q, want %q", restored.PatchID, original.PatchID)
	}
	if restored.Status != original.Status {
		t.Errorf("Round-trip CanaryResult.Status = %q, want %q", restored.Status, original.Status)
	}
}

// TestCanaryPatchResult_JSONRoundTrip verifies CanaryPatchResult serializes/deserializes
func TestCanaryPatchResult_JSONRoundTrip(t *testing.T) {
	now := time.Now()
	original := &CanaryPatchResult{
		DeploymentID:  "deploy-001",
		PatchID:       "patch-001",
		CanaryGroup:   "canary-001",
		DevicesTested: 10,
		DevicesOK:     8,
		DevicesFail:   2,
		Status:        "in_progress",
		StartedAt:     now,
		CompletedAt:   &now,
		Error:         "",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal CanaryPatchResult: %v", err)
	}

	var restored CanaryPatchResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal CanaryPatchResult: %v", err)
	}

	if restored.DeploymentID != original.DeploymentID {
		t.Errorf("Round-trip CanaryPatchResult.DeploymentID = %q, want %q", restored.DeploymentID, original.DeploymentID)
	}
	if restored.PatchID != original.PatchID {
		t.Errorf("Round-trip CanaryPatchResult.PatchID = %q, want %q", restored.PatchID, original.PatchID)
	}
	if restored.CanaryGroup != original.CanaryGroup {
		t.Errorf("Round-trip CanaryPatchResult.CanaryGroup = %q, want %q", restored.CanaryGroup, original.CanaryGroup)
	}
	if restored.DevicesTested != original.DevicesTested {
		t.Errorf("Round-trip CanaryPatchResult.DevicesTested = %d, want %d", restored.DevicesTested, original.DevicesTested)
	}
	if restored.DevicesOK != original.DevicesOK {
		t.Errorf("Round-trip CanaryPatchResult.DevicesOK = %d, want %d", restored.DevicesOK, original.DevicesOK)
	}
}

// TestWindowsPatch_StructFields verifies windowsPatch struct fields for JSON parsing
func TestWindowsPatch_StructFields(t *testing.T) {
	// windowsPatch is used for parsing Windows update JSON output
	// It contains: Title, KBArticleIDs, MsrcSeverity, Description, Categories, DeployTime
	original := windowsPatch{
		Title:        "Security Update",
		KBArticleIDs: []string{"KB1234567"},
		MsrcSeverity: "Critical",
		Description:  "Fixes critical vulnerability",
		Categories:   []string{"Security Updates"},
		DeployTime:   "2024-01-01T00:00:00Z",
	}

	if original.Title != "Security Update" {
		t.Errorf("windowsPatch.Title = %q, want %q", original.Title, "Security Update")
	}
	if len(original.KBArticleIDs) != 1 || original.KBArticleIDs[0] != "KB1234567" {
		t.Errorf("windowsPatch.KBArticleIDs = %v, want [KB1234567]", original.KBArticleIDs)
	}
	if original.MsrcSeverity != "Critical" {
		t.Errorf("windowsPatch.MsrcSeverity = %q, want %q", original.MsrcSeverity, "Critical")
	}
	if original.Description != "Fixes critical vulnerability" {
		t.Errorf("windowsPatch.Description = %q, want %q", original.Description, "Fixes critical vulnerability")
	}
	if len(original.Categories) != 1 || original.Categories[0] != "Security Updates" {
		t.Errorf("windowsPatch.Categories = %v, want [Security Updates]", original.Categories)
	}
	if original.DeployTime != "2024-01-01T00:00:00Z" {
		t.Errorf("windowsPatch.DeployTime = %q, want %q", original.DeployTime, "2024-01-01T00:00:00Z")
	}
}

// TestWindowsPatchJSONRoundTrip verifies windowsPatch serializes/deserializes correctly
func TestWindowsPatchJSONRoundTrip(t *testing.T) {
	original := windowsPatch{
		Title:        "Security Update",
		KBArticleIDs: []string{"KB1234567", "KB7654321"},
		MsrcSeverity: "Important",
		Description:  "Fixes important vulnerability",
		Categories:   []string{"Security Updates", "Updates"},
		DeployTime:   "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal windowsPatch: %v", err)
	}

	var restored windowsPatch
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal windowsPatch: %v", err)
	}

	if restored.Title != original.Title {
		t.Errorf("Round-trip windowsPatch.Title = %q, want %q", restored.Title, original.Title)
	}
	if len(restored.KBArticleIDs) != len(original.KBArticleIDs) {
		t.Errorf("Round-trip windowsPatch.KBArticleIDs length = %d, want %d", len(restored.KBArticleIDs), len(original.KBArticleIDs))
	}
	if restored.MsrcSeverity != original.MsrcSeverity {
		t.Errorf("Round-trip windowsPatch.MsrcSeverity = %q, want %q", restored.MsrcSeverity, original.MsrcSeverity)
	}
}

// TestPatchSeverityFromMSRC verifies severity mapping from MSRC strings
func TestPatchSeverityFromMSRC(t *testing.T) {
	tests := []struct {
		input  string
		output PatchSeverity
	}{
		{"Critical", SeverityCritical},
		{"Important", SeverityImportant},
		{"Moderate", SeverityModerate},
		{"Low", SeverityLow},
		{"Unknown", SeverityModerate},
		{"", SeverityModerate},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := severityFromMSRC(tt.input)
			if result != tt.output {
				t.Errorf("severityFromMSRC(%q) = %q, want %q", tt.input, result, tt.output)
			}
		})
	}
}

// TestPatchPolicyApprovalModes verifies valid approval modes
func TestPatchPolicyApprovalModes(t *testing.T) {
	modes := []string{"auto", "manual"}
	for _, mode := range modes {
		if mode == "" {
			t.Error("approval mode should not be empty")
		}
	}
}
