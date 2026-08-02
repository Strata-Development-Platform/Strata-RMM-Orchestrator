package platform

import (
	"reflect"
	"testing"
)

func TestPolicyValidationEnforcesCategoryScopeAndConfig(t *testing.T) {
	valid := policyInput{Name: "Linux patch baseline", Category: "patch", ScopeLevel: "client", ClientID: "client-1", Config: map[string]interface{}{"approval": "manual"}}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	for name, input := range map[string]policyInput{
		"unknown category": {Name: "bad", Category: "unknown", ScopeLevel: "msp", Config: map[string]interface{}{"a": true}},
		"empty config":     {Name: "bad", Category: "patch", ScopeLevel: "msp", Config: map[string]interface{}{}},
		"scope mismatch":   {Name: "bad", Category: "patch", ScopeLevel: "site", ClientID: "client-1", Config: map[string]interface{}{"a": true}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := input.validate(); err == nil {
				t.Fatal("invalid policy passed validation")
			}
		})
	}
}

func TestMergePolicyLayersUsesRecursiveMostSpecificWins(t *testing.T) {
	layers := []policyLayer{
		{ScopeLevel: "msp", Config: map[string]interface{}{"enabled": true, "scan": map[string]interface{}{"hour": float64(2), "reboot": false}}},
		{ScopeLevel: "client", Config: map[string]interface{}{"scan": map[string]interface{}{"hour": float64(4)}}},
		{ScopeLevel: "device", Config: map[string]interface{}{"enabled": false, "scan": map[string]interface{}{"reboot": true}}},
	}
	want := map[string]interface{}{"enabled": false, "scan": map[string]interface{}{"hour": float64(4), "reboot": true}}
	if got := mergePolicyLayers(layers); !reflect.DeepEqual(got, want) {
		t.Fatalf("effective config = %#v, want %#v", got, want)
	}
}

func TestMaintenanceWindowValidation(t *testing.T) {
	for name, input := range map[string]policyInput{
		"valid maintenance window": {
			Name:       "Weekly maintenance",
			Category:   "maintenance_window",
			ScopeLevel: "client",
			ClientID:   "client-1",
			Config:     map[string]interface{}{"enabled": true},
			MaintenanceStart:  strPtr("02:00"),
			MaintenanceEnd:    strPtr("06:00"),
			MaintenanceDays:   &[]string{"monday", "tuesday", "wednesday", "thursday", "friday"},
			MaintenanceTimezone: "America/New_York",
		},
		"invalid start time format": {
			Name:       "bad",
			Category:   "maintenance_window",
			ScopeLevel: "msp",
			Config:     map[string]interface{}{"enabled": true},
			MaintenanceStart:  strPtr("2:00pm"),
		},
		"invalid end time format": {
			Name:       "bad",
			Category:   "maintenance_window",
			ScopeLevel: "msp",
			Config:     map[string]interface{}{"enabled": true},
			MaintenanceEnd: strPtr("0600"),
		},
		"start after end": {
			Name:       "bad",
			Category:   "maintenance_window",
			ScopeLevel: "msp",
			Config:     map[string]interface{}{"enabled": true},
			MaintenanceStart: strPtr("08:00"),
			MaintenanceEnd:   strPtr("06:00"),
		},
		"invalid day": {
			Name:       "bad",
			Category:   "maintenance_window",
			ScopeLevel: "msp",
			Config:     map[string]interface{}{"enabled": true},
			MaintenanceDays: &[]string{"funday"},
		},
		"empty days": {
			Name:       "bad",
			Category:   "maintenance_window",
			ScopeLevel: "msp",
			Config:     map[string]interface{}{"enabled": true},
			MaintenanceDays: &[]string{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := input.validate()
			if name == "valid maintenance window" {
				if err != nil {
					t.Fatalf("valid policy rejected: %v", err)
				}
			} else if err == nil {
				t.Fatal("invalid policy passed validation")
			}
		})
	}
}

func TestMergePolicyLayersWithMaintenance(t *testing.T) {
	days1 := []string{"monday", "tuesday"}
	days2 := []string{"wednesday", "thursday"}
	layers := []policyLayer{
		{ScopeLevel: "msp", Config: map[string]interface{}{"enabled": true}, MaintenanceStart: strPtr("02:00"), MaintenanceEnd: strPtr("06:00"), MaintenanceDays: &days1, MaintenanceTimezone: "UTC"},
		{ScopeLevel: "client", Config: map[string]interface{}{}, MaintenanceStart: strPtr("03:00"), MaintenanceEnd: strPtr("07:00"), MaintenanceDays: &days2, MaintenanceTimezone: "America/New_York"},
	}
	want := map[string]interface{}{"enabled": true}
	wantMgmt := map[string]interface{}{"maintenance_start": "03:00", "maintenance_end": "07:00", "maintenance_days": days2, "maintenance_timezone": "America/New_York"}
	if got := mergePolicyLayers(layers); !reflect.DeepEqual(got, want) {
		t.Fatalf("effective config = %#v, want %#v", got, want)
	}
	effectiveMgmt := mergeMaintenanceLayers(layers)
	if effectiveMgmt["maintenance_start"] != wantMgmt["maintenance_start"] {
		t.Fatalf("effective maintenance_start = %v, want %v", effectiveMgmt["maintenance_start"], wantMgmt["maintenance_start"])
	}
	if effectiveMgmt["maintenance_end"] != wantMgmt["maintenance_end"] {
		t.Fatalf("effective maintenance_end = %v, want %v", effectiveMgmt["maintenance_end"], wantMgmt["maintenance_end"])
	}
	if !reflect.DeepEqual(effectiveMgmt["maintenance_days"], wantMgmt["maintenance_days"]) {
		t.Fatalf("effective maintenance_days = %v, want %v", effectiveMgmt["maintenance_days"], wantMgmt["maintenance_days"])
	}
	if effectiveMgmt["maintenance_timezone"] != wantMgmt["maintenance_timezone"] {
		t.Fatalf("effective maintenance_timezone = %v, want %v", effectiveMgmt["maintenance_timezone"], wantMgmt["maintenance_timezone"])
	}
}

func TestPolicyDiffComputation(t *testing.T) {
	days1 := []string{"monday", "tuesday"}
	days2 := []string{"wednesday", "thursday", "friday"}
	layers := []policyLayer{
		{ID: "policy-1", ScopeLevel: "msp", Config: map[string]interface{}{"enabled": true, "retry": 3}, MaintenanceStart: strPtr("02:00"), MaintenanceEnd: strPtr("06:00"), MaintenanceDays: &days1, MaintenanceTimezone: "UTC", Version: 1},
		{ID: "policy-1", ScopeLevel: "msp", Config: map[string]interface{}{"enabled": true, "retry": 5}, MaintenanceStart: strPtr("03:00"), MaintenanceEnd: strPtr("07:00"), MaintenanceDays: &days2, MaintenanceTimezone: "America/New_York", Version: 2},
	}
	diff := computePolicyDiff(layers, 1, 2)
	if !diff.IsChanged {
		t.Fatal("diff should detect changes")
	}
	if diff.Version1 != 1 || diff.Version2 != 2 {
		t.Fatalf("expected versions 1 and 2, got %d and %d", diff.Version1, diff.Version2)
	}
	foundConfig := false
	foundMaintenance := false
	for _, c := range diff.Changes {
		if c.Path == "config/retry" {
			foundConfig = true
		}
		if c.Path == "maintenance_start" || c.Path == "maintenance_end" || c.Path == "maintenance_days" || c.Path == "maintenance_timezone" {
			foundMaintenance = true
		}
	}
	if !foundConfig {
		t.Error("expected config change detected")
	}
	if !foundMaintenance {
		t.Error("expected maintenance changes detected")
	}
}

func strPtr(s string) *string {
	return &s
}
