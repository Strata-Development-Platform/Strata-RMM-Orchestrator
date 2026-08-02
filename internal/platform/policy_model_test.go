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
