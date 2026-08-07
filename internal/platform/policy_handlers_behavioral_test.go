package platform

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestPolicyCategories(t *testing.T) {
	for _, cat := range []string{"patch", "alerting", "monitoring", "software", "script", "maintenance", "maintenance_window"} {
		if !policyCategories[cat] {
			t.Errorf("expected policyCategories[%q] to be true", cat)
		}
	}
}

func TestPolicyCategoriesRejectsInvalidCategory(t *testing.T) {
	if policyCategories["invalid-category"] {
		t.Fatal("expected invalid category to be rejected")
	}
}

func TestPolicyScopeRankOrdering(t *testing.T) {
	if policyScopeRank["msp"] >= policyScopeRank["client"] {
		t.Fatal("msp should rank lower than client")
	}
	if policyScopeRank["client"] >= policyScopeRank["site"] {
		t.Fatal("client should rank lower than site")
	}
	if policyScopeRank["site"] >= policyScopeRank["device"] {
		t.Fatal("site should rank lower than device")
	}
}

func TestPolicyScopeRankRejectsInvalidScope(t *testing.T) {
	if _, ok := policyScopeRank["invalid"]; ok {
		t.Fatal("expected invalid scope to be rejected")
	}
}

func TestPolicyInputNormalizeDefaultsMSPScope(t *testing.T) {
	input := policyInput{Name: " test "}
	input.normalize()
	if input.ScopeLevel != "msp" {
		t.Fatalf("expected default scope msp, got %q", input.ScopeLevel)
	}
}

func TestPolicyInputNormalizeLowercasesCategory(t *testing.T) {
	input := policyInput{Category: "PATCH"}
	input.normalize()
	if input.Category != "patch" {
		t.Fatalf("expected lowercased category patch, got %q", input.Category)
	}
}

func TestPolicyInputNormalizeLowercasesScope(t *testing.T) {
	input := policyInput{ScopeLevel: "MSP"}
	input.normalize()
	if input.ScopeLevel != "msp" {
		t.Fatalf("expected lowercased scope msp, got %q", input.ScopeLevel)
	}
}

func TestPolicyInputNormalizeDefaultsMaintenanceTimezone(t *testing.T) {
	input := policyInput{}
	input.normalize()
	if input.MaintenanceTimezone != "UTC" {
		t.Fatalf("expected default maintenance timezone UTC, got %q", input.MaintenanceTimezone)
	}
}

func TestPolicyInputValidateRejectsEmptyName(t *testing.T) {
	input := policyInput{Category: "patch", ScopeLevel: "msp", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestPolicyInputValidateRejectsNameTooLong(t *testing.T) {
	name := strings.Repeat("a", 201)
	input := policyInput{Name: name, Category: "patch", ScopeLevel: "msp", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for name too long")
	}
}

func TestPolicyInputValidateAcceptsNameMaxLength(t *testing.T) {
	name := strings.Repeat("a", 200)
	input := policyInput{Name: name, Category: "patch", ScopeLevel: "msp", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyInputValidateRejectsDescriptionTooLong(t *testing.T) {
	desc := strings.Repeat("a", 2001)
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "msp", Description: desc, Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for description too long")
	}
}

func TestPolicyInputValidateRejectsInvalidCategory(t *testing.T) {
	input := policyInput{Name: "test", Category: "invalid", ScopeLevel: "msp", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
}

func TestPolicyInputValidateRejectsInvalidScopeLevel(t *testing.T) {
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "invalid", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for invalid scope level")
	}
}

func TestPolicyInputValidateRejectsMSPWithChildScope(t *testing.T) {
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "msp", ClientID: "some-id", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for msp with client_id")
	}
}

func TestPolicyInputValidateRejectsClientWithSiteID(t *testing.T) {
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "client", ClientID: "a", SiteID: "b", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for client with site_id")
	}
}

func TestPolicyInputValidateAcceptsClientScope(t *testing.T) {
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "client", ClientID: "some-client-id", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyInputValidateRejectsSiteWithoutClientID(t *testing.T) {
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "site", SiteID: "some-site-id", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for site without client_id")
	}
}

func TestPolicyInputValidateAcceptsSiteScope(t *testing.T) {
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "site", ClientID: "cid", SiteID: "sid", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyInputValidateRejectsDeviceWithoutClientOrSite(t *testing.T) {
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "device", DeviceID: "did", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for device without client/site")
	}
}

func TestPolicyInputValidateAcceptsDeviceScope(t *testing.T) {
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "device", ClientID: "c", SiteID: "s", DeviceID: "d", Config: map[string]interface{}{"a": "b"}}
	err := input.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyInputValidateRejectsEmptyConfig(t *testing.T) {
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "msp", Config: map[string]interface{}{}}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestPolicyInputValidateRejectsConfigTooDeep(t *testing.T) {
	deep := map[string]interface{}{}
	current := deep
	for i := 0; i < 14; i++ {
		next := map[string]interface{}{}
		current["nested"] = next
		current = next
	}
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "msp", Config: deep}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for config too deep")
	}
}



func TestIsValidTimeAcceptsValidTimes(t *testing.T) {
	for _, tt := range []string{"00:00", "08:30", "12:00", "23:59"} {
		if !isValidTime(tt) {
			t.Errorf("expected isValidTime(%q) true", tt)
		}
	}
}

func TestIsValidTimeRejectsInvalidTimes(t *testing.T) {
	for _, tt := range []string{"8:30", "25:00", "12:60", "abcde", "", "12", "12:3", "12-30"} {
		if isValidTime(tt) {
			t.Errorf("expected isValidTime(%q) false", tt)
		}
	}
}

func TestIsValidDayAcceptsAllDays(t *testing.T) {
	for _, day := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		if !isValidDay(day) {
			t.Errorf("expected isValidDay(%q) true", day)
		}
	}
}

func TestIsValidDayRejectsInvalidDay(t *testing.T) {
	if isValidDay("mon") {
		t.Fatal("expected invalid day rejected")
	}
	if isValidDay("MONDAY") {
		t.Fatal("expected uppercase day rejected")
	}
	if isValidDay("invalid") {
		t.Fatal("expected invalid day rejected")
	}
}

func TestPolicyInputValidateMaintenanceWindowRequiresStartBeforeEnd(t *testing.T) {
	start := "18:00"
	end := "06:00"
	input := policyInput{Name: "test", Category: "maintenance_window", ScopeLevel: "msp",
		Config: map[string]interface{}{"a": "b"},
		MaintenanceStart: &start, MaintenanceEnd: &end}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for maintenance start after end")
	}
}

func TestPolicyInputValidateMaintenanceWindowAcceptsValidStartEnd(t *testing.T) {
	start := "06:00"
	end := "18:00"
	input := policyInput{Name: "test", Category: "maintenance_window", ScopeLevel: "msp",
		Config: map[string]interface{}{"a": "b"},
		MaintenanceStart: &start, MaintenanceEnd: &end}
	err := input.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyInputValidateMaintenanceWindowRejectsInvalidStart(t *testing.T) {
	start := "invalid"
	end := "18:00"
	input := policyInput{Name: "test", Category: "maintenance_window", ScopeLevel: "msp",
		Config: map[string]interface{}{"a": "b"},
		MaintenanceStart: &start, MaintenanceEnd: &end}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for invalid maintenance start")
	}
}

func TestPolicyInputValidateMaintenanceDaysRejectsEmpty(t *testing.T) {
	days := []string{}
	input := policyInput{Name: "test", Category: "maintenance_window", ScopeLevel: "msp",
		Config: map[string]interface{}{"a": "b"},
		MaintenanceDays: &days}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for empty maintenance_days")
	}
}

func TestPolicyInputValidateMaintenanceDaysRejectsInvalidDay(t *testing.T) {
	days := []string{"monday", "invalidday"}
	input := policyInput{Name: "test", Category: "maintenance_window", ScopeLevel: "msp",
		Config: map[string]interface{}{"a": "b"},
		MaintenanceDays: &days}
	err := input.validate()
	if err == nil {
		t.Fatal("expected error for invalid maintenance day")
	}
}

func TestPolicyInputValidateMaintenanceDaysAcceptsValidDays(t *testing.T) {
	days := []string{"monday", "wednesday", "friday"}
	input := policyInput{Name: "test", Category: "maintenance_window", ScopeLevel: "msp",
		Config: map[string]interface{}{"a": "b"},
		MaintenanceDays: &days}
	err := input.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyRecordFields(t *testing.T) {
	now := time.Now()
	policy := policyRecord{
		ID:               "id-1",
		MSPID:            "msp-1",
		Name:             "test-policy",
		Category:         "patch",
		Description:      "Test",
		ScopeLevel:       "msp",
		Status:           "draft",
		Version:          1,
		PublishedVersion: nil,
		ValidatedAt:      &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if policy.ID != "id-1" {
		t.Fatalf("policyRecord.ID = %q", policy.ID)
	}
}

func TestPolicyResponseIncludesRequiredFields(t *testing.T) {
	now := time.Now()
	policy := policyRecord{
		ID:           "id-1",
		MSPID:        "msp-1",
		Name:         "test-policy",
		Category:     "patch",
		ScopeLevel:   "msp",
		Status:       "draft",
		Version:      1,
		Config:       map[string]interface{}{"a": "b"},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	resp := policy.policyResponse()
	if resp["id"] != "id-1" {
		t.Fatalf("response id = %v", resp["id"])
	}
	if resp["status"] != "draft" {
		t.Fatalf("response status = %v", resp["status"])
	}
}

func TestPolicyInputToRecordConversionIncludesMaintenanceFields(t *testing.T) {
	start := "06:00"
	end := "18:00"
	record := policyRecord{
		Name:             "test",
		Category:         "maintenance_window",
		ScopeLevel:       "msp",
		MaintenanceStart: &start,
		MaintenanceEnd:   &end,
		MaintenanceTimezone: "UTC",
	}
	input := record.input()
	if input.MaintenanceTimezone != "UTC" {
		t.Fatalf("expected maintenance timezone UTC, got %q", input.MaintenanceTimezone)
	}
}

func TestMergePolicyLayersUsesLowerScopeFirst(t *testing.T) {
	msp := policyLayer{ScopeLevel: "msp", Version: 1, Config: map[string]interface{}{"setting": "msp-value"}}
	device := policyLayer{ScopeLevel: "device", Version: 2, Config: map[string]interface{}{"setting": "device-value"}}
	result := mergePolicyLayers([]policyLayer{msp, device})
	if result["setting"] != "device-value" {
		t.Fatalf("expected device value to override msp value, got %v", result["setting"])
	}
}

func TestMergePolicyLayersPreservesMSPDefaultWhenNoOverride(t *testing.T) {
	msp := policyLayer{ScopeLevel: "msp", Version: 1, Config: map[string]interface{}{"setting": "msp-value"}}
	result := mergePolicyLayers([]policyLayer{msp})
	if result["setting"] != "msp-value" {
		t.Fatalf("expected msp value preserved, got %v", result["setting"])
	}
}

func TestMergeMaintenanceLayers(t *testing.T) {
	start := "06:00"
	end := "18:00"
	layer := policyLayer{
		MaintenanceStart:    &start,
		MaintenanceEnd:      &end,
		MaintenanceDays:     &[]string{"monday"},
		MaintenanceTimezone: "America/New_York",
	}
	result := mergeMaintenanceLayers([]policyLayer{layer})
	if result["maintenance_start"] != start {
		t.Fatalf("maintenance_start = %v", result["maintenance_start"])
	}
	if result["maintenance_end"] != end {
		t.Fatalf("maintenance_end = %v", result["maintenance_end"])
	}
}

func TestMergeMaintenanceLayersSkipsNilValues(t *testing.T) {
	layer := policyLayer{
		MaintenanceTimezone: "UTC",
	}
	result := mergeMaintenanceLayers([]policyLayer{layer})
	if _, ok := result["maintenance_start"]; ok {
		t.Fatal("expected maintenance_start not set for nil layer")
	}
}



func TestComputePolicyDiffMissingVersions(t *testing.T) {
	layers := []policyLayer{{ID: "p1", Version: 1}}
	diff := computePolicyDiff(layers, 1, 2)
	if !diff.IsChanged {
		t.Fatal("expected changed when v2 missing")
	}
}

func TestComputePolicyDiffSameConfig(t *testing.T) {
	config := map[string]interface{}{"setting": "value"}
	layers := []policyLayer{
		{ID: "p1", Version: 1, Config: config},
		{ID: "p1", Version: 2, Config: config},
	}
	diff := computePolicyDiff(layers, 1, 2)
	if diff.IsChanged {
		t.Fatal("expected no change when config identical")
	}
}

func TestComputePolicyDiffDifferentConfig(t *testing.T) {
	layers := []policyLayer{
		{ID: "p1", Version: 1, Config: map[string]interface{}{"setting": "old"}},
		{ID: "p1", Version: 2, Config: map[string]interface{}{"setting": "new"}},
	}
	diff := computePolicyDiff(layers, 1, 2)
	if !diff.IsChanged {
		t.Fatal("expected change when config differs")
	}
}

func TestEffectivePolicyFields(t *testing.T) {
	epp := EffectivePolicy{
		Category:         "patch",
		Config:           map[string]interface{}{"a": "b"},
		Maintenance:      map[string]interface{}{"start": "06:00"},
		ScopeLevel:       "msp",
		PublishedVersion: 1,
	}
	if epp.Category != "patch" {
		t.Fatalf("EffectivePolicy.Category = %q", epp.Category)
	}
}

func TestPolicyEnforcementEngineStructFields(t *testing.T) {
	logger := zap.NewNop()
	engine := NewPolicyEnforcementEngine((*sql.DB)(nil), logger)
	if engine == nil {
		t.Fatal("expected non-nil PolicyEnforcementEngine")
	}
}

func TestPolicySchedulerStructFields(t *testing.T) {
	logger := zap.NewNop()
	engine := NewPolicyEnforcementEngine((*sql.DB)(nil), logger)
	scheduler := NewPolicyScheduler(60*time.Second, engine, logger)
	if scheduler == nil {
		t.Fatal("expected non-nil PolicyScheduler")
	}
}

func TestPolicySchedulerStartStop(t *testing.T) {
	logger := zap.NewNop()
	engine := NewPolicyEnforcementEngine((*sql.DB)(nil), logger)
	scheduler := NewPolicyScheduler(60*time.Second, engine, logger)
	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)
	cancel()
	scheduler.Stop()
}

func TestPolicyAssignmentStructFields(t *testing.T) {
	now := time.Now()
	a := PolicyAssignment{
		ID:          "a-1",
		PolicyID:    "p-1",
		DeviceID:    "d-1",
		PolicyName:  "test-policy",
		Category:    "patch",
		ScopeLevel:  "device",
		EffectiveAt: now,
	}
	if a.PolicyName != "test-policy" {
		t.Fatalf("PolicyAssignment.PolicyName = %q", a.PolicyName)
	}
}

func TestPolicyDiffStructFields(t *testing.T) {
	layers := []policyLayer{
		{ID: "p1", Version: 1, Config: map[string]interface{}{"setting": "old"}},
		{ID: "p1", Version: 2, Config: map[string]interface{}{"setting": "new"}},
	}
	diff := computePolicyDiff(layers, 1, 2)
	if diff.PolicyID != "p1" {
		t.Fatalf("policyDiff.PolicyID = %q", diff.PolicyID)
	}
	if diff.Version1 != 1 {
		t.Fatalf("policyDiff.Version1 = %d", diff.Version1)
	}
	if diff.Version2 != 2 {
		t.Fatalf("policyDiff.Version2 = %d", diff.Version2)
	}
}

func TestPolicyLayerStructFields(t *testing.T) {
	layer := policyLayer{
		ID:                  "p-1",
		ScopeLevel:          "msp",
		Version:             1,
		PublishedVersion:    1,
		Config:              map[string]interface{}{"a": "b"},
		MaintenanceTimezone: "UTC",
	}
	if layer.ScopeLevel != "msp" {
		t.Fatalf("policyLayer.ScopeLevel = %q", layer.ScopeLevel)
	}
}

func TestPolicyInputValidateConfigSizeLimit(t *testing.T) {
	large := make(map[string]interface{})
	for i := 0; i < 50; i++ {
		large[string(rune(i))] = strings.Repeat("x", 1024)
	}
	input := policyInput{Name: "test", Category: "patch", ScopeLevel: "msp", Config: large}
	err := input.validate()
	if err != nil {
		t.Fatalf("unexpected error for moderately sized config: %v", err)
	}
}

func TestPolicyInputValidateAcceptsValidMaintenanceWindow(t *testing.T) {
	start := "06:00"
	end := "18:00"
	days := []string{"sunday"}
	input := policyInput{Name: "test", Category: "maintenance_window", ScopeLevel: "msp",
		Config: map[string]interface{}{"a": "b"},
		MaintenanceStart: &start, MaintenanceEnd: &end, MaintenanceDays: &days, MaintenanceTimezone: "UTC"}
	err := input.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyInputNormalizeTrimsWhitespace(t *testing.T) {
	input := policyInput{Name: "  test  ", Category: " PATCH ", Description: "  desc  ", ScopeLevel: "  DEVICE  "}
	input.normalize()
	if input.Name != "test" {
		t.Fatalf("Name = %q", input.Name)
	}
	if input.Category != "patch" {
		t.Fatalf("Category = %q", input.Category)
	}
	if input.Description != "desc" {
		t.Fatalf("Description = %q", input.Description)
	}
	if input.ScopeLevel != "device" {
		t.Fatalf("ScopeLevel = %q", input.ScopeLevel)
	}
}

func TestPolicyRecordInputConversionPreservesNilMaintenanceDays(t *testing.T) {
	record := policyRecord{
		Name:                "test",
		Category:            "maintenance_window",
		ScopeLevel:          "msp",
		MaintenanceTimezone: "UTC",
	}
	input := record.input()
	if input.MaintenanceDays != nil {
		t.Fatal("expected nil maintenance days")
	}
}

func TestPolicyRecordInputConversionPreservesMaintenanceTimezone(t *testing.T) {
	record := policyRecord{
		Name:                "test",
		Category:            "maintenance_window",
		ScopeLevel:          "msp",
		MaintenanceTimezone: "America/Los_Angeles",
	}
	input := record.input()
	if input.MaintenanceTimezone != "America/Los_Angeles" {
		t.Fatalf("MaintenanceTimezone = %q", input.MaintenanceTimezone)
	}
}

func TestComputePolicyDiffMaintenanceChange(t *testing.T) {
	start1 := "06:00"
	start2 := "08:00"
	layers := []policyLayer{
		{ID: "p1", Version: 1, MaintenanceStart: &start1},
		{ID: "p1", Version: 2, MaintenanceStart: &start2},
	}
	diff := computePolicyDiff(layers, 1, 2)
	if !diff.IsChanged {
		t.Fatal("expected changed when maintenance_start differs")
	}
}

func TestComputePolicyDiffNilVsNonNilMaintenance(t *testing.T) {
	layers := []policyLayer{
		{ID: "p1", Version: 1},
		{ID: "p1", Version: 2, MaintenanceStart: strPtrValue("06:00")},
	}
	diff := computePolicyDiff(layers, 1, 2)
	if !diff.IsChanged {
		t.Fatal("expected changed when maintenance goes from nil to value")
	}
}

func strPtrValue(s string) *string {
	return &s
}

func TestPolicyDiffStructIsChangedFalseWhenNoChanges(t *testing.T) {
	config := map[string]interface{}{"key": "value"}
	layers := []policyLayer{
		{ID: "p1", Version: 1, Config: config},
		{ID: "p1", Version: 2, Config: config},
	}
	diff := computePolicyDiff(layers, 1, 2)
	if diff.IsChanged {
		t.Fatal("expected no changes")
	}
}

func TestPolicyChangeStructFields(t *testing.T) {
	layers := []policyLayer{
		{ID: "p1", Version: 1, Config: map[string]interface{}{"a": "1"}},
		{ID: "p1", Version: 2, Config: map[string]interface{}{"a": "2"}},
	}
	diff := computePolicyDiff(layers, 1, 2)
	if len(diff.Changes) == 0 {
		t.Fatal("expected changes")
	}
}

func TestPolicyEnforcementEngineNilDB(t *testing.T) {
	logger := zap.NewNop()
	engine := NewPolicyEnforcementEngine((*sql.DB)(nil), logger)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestPolicySchedulerNilEngine(t *testing.T) {
	logger := zap.NewNop()
	scheduler := NewPolicyScheduler(1*time.Minute, nil, logger)
	if scheduler == nil {
		t.Fatal("expected non-nil scheduler")
	}
}

func TestPolicySchedulerDefaultInterval(t *testing.T) {
	logger := zap.NewNop()
	engine := NewPolicyEnforcementEngine((*sql.DB)(nil), logger)
	scheduler := NewPolicyScheduler(30*time.Second, engine, logger)
	if scheduler == nil {
		t.Fatal("expected non-nil scheduler")
	}
}

func TestPolicySchedulerStartStopWithNilContext(t *testing.T) {
	logger := zap.NewNop()
	engine := NewPolicyEnforcementEngine((*sql.DB)(nil), logger)
	scheduler := NewPolicyScheduler(1*time.Second, engine, logger)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scheduler.Start(ctx)
	scheduler.Stop()
}

func TestEffectivePolicyEmptyConfig(t *testing.T) {
	ep := EffectivePolicy{Category: "patch", Config: map[string]interface{}{}}
	if ep.Config == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestEffectivePolicyEmptyMaintenance(t *testing.T) {
	ep := EffectivePolicy{Category: "maintenance_window", Maintenance: map[string]interface{}{}}
	if ep.Maintenance == nil {
		t.Fatal("expected non-nil maintenance")
	}
}

func TestPolicyRecordResponseIncludesTimestamps(t *testing.T) {
	now := time.Now()
	policy := policyRecord{
		ID:        "p-1",
		Name:      "test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	resp := policy.policyResponse()
	if _, ok := resp["created_at"]; !ok {
		t.Fatal("expected created_at in response")
	}
	if _, ok := resp["updated_at"]; !ok {
		t.Fatal("expected updated_at in response")
	}
}

func TestPolicyResponseIncludesValidationFlags(t *testing.T) {
	policy := policyRecord{
		ID:          "p-1",
		Name:        "test",
		ValidatedAt: nil,
		PreviewedAt: strPtrTime(time.Now()),
	}
	resp := policy.policyResponse()
	if resp["validated"] != false {
		t.Fatalf("validated = %v", resp["validated"])
	}
}

func strPtrTime(t time.Time) *time.Time {
	return &t
}



func TestPolicyEnforcementEngineHasDBField(t *testing.T) {
	logger := zap.NewNop()
	engine := NewPolicyEnforcementEngine((*sql.DB)(nil), logger)
	if engine == nil {
		t.Fatal("expected engine")
	}
}
