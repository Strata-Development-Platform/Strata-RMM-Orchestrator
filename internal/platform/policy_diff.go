package platform

import (
	"reflect"
)

type policyDiff struct {
	PolicyID  string                 `json:"policy_id"`
	Version1  int                    `json:"version_1"`
	Version2  int                    `json:"version_2"`
	Changes   []policyChange         `json:"changes"`
	IsChanged bool                   `json:"is_changed"`
}

type policyChange struct {
	Path      string                 `json:"path"`
	OldValue  interface{}            `json:"old_value,omitempty"`
	NewValue  interface{}            `json:"new_value,omitempty"`
	Changed   bool                   `json:"changed"`
}

func computePolicyDiff(layers []policyLayer, v1, v2 int) policyDiff {
	diff := policyDiff{PolicyID: layers[0].ID, Version1: v1, Version2: v2, Changes: []policyChange{}}
	if len(layers) == 0 {
		return diff
	}
	var v1Layer, v2Layer *policyLayer
	for i := range layers {
		if layers[i].Version == v1 {
			v1Layer = &layers[i]
		}
		if layers[i].Version == v2 {
			v2Layer = &layers[i]
		}
	}
	if v1Layer == nil || v2Layer == nil {
		diff.IsChanged = true
		diff.Changes = append(diff.Changes, policyChange{Path: "/", Changed: true})
		return diff
	}
	if v1Layer.Config != nil && v2Layer.Config != nil {
		diff.Changes = append(diff.Changes, compareValues("config", v1Layer.Config, v2Layer.Config)...)
	}
	if (v1Layer.MaintenanceStart != nil && v2Layer.MaintenanceStart == nil) ||
		(v1Layer.MaintenanceStart == nil && v2Layer.MaintenanceStart != nil) ||
		(v1Layer.MaintenanceStart != nil && v2Layer.MaintenanceStart != nil && *v1Layer.MaintenanceStart != *v2Layer.MaintenanceStart) {
		diff.Changes = append(diff.Changes, policyChange{Path: "maintenance_start", OldValue: toString(v1Layer.MaintenanceStart), NewValue: toString(v2Layer.MaintenanceStart), Changed: true})
	}
	if (v1Layer.MaintenanceEnd != nil && v2Layer.MaintenanceEnd == nil) ||
		(v1Layer.MaintenanceEnd == nil && v2Layer.MaintenanceEnd != nil) ||
		(v1Layer.MaintenanceEnd != nil && v2Layer.MaintenanceEnd != nil && *v1Layer.MaintenanceEnd != *v2Layer.MaintenanceEnd) {
		diff.Changes = append(diff.Changes, policyChange{Path: "maintenance_end", OldValue: toString(v1Layer.MaintenanceEnd), NewValue: toString(v2Layer.MaintenanceEnd), Changed: true})
	}
	if (v1Layer.MaintenanceDays != nil && v2Layer.MaintenanceDays == nil) ||
		(v1Layer.MaintenanceDays == nil && v2Layer.MaintenanceDays != nil) ||
		(v1Layer.MaintenanceDays != nil && v2Layer.MaintenanceDays != nil && !stringSlicesEqual(*v1Layer.MaintenanceDays, *v2Layer.MaintenanceDays)) {
		diff.Changes = append(diff.Changes, policyChange{Path: "maintenance_days", OldValue: v1Layer.MaintenanceDays, NewValue: v2Layer.MaintenanceDays, Changed: true})
	}
	if v1Layer.MaintenanceTimezone != v2Layer.MaintenanceTimezone {
		diff.Changes = append(diff.Changes, policyChange{Path: "maintenance_timezone", OldValue: v1Layer.MaintenanceTimezone, NewValue: v2Layer.MaintenanceTimezone, Changed: true})
	}
	diff.IsChanged = len(diff.Changes) > 0
	return diff
}

func compareValues(path string, v1, v2 interface{}) []policyChange {
	changes := []policyChange{}
	m1, ok1 := v1.(map[string]interface{})
	m2, ok2 := v2.(map[string]interface{})
	if ok1 && ok2 {
		allKeys := map[string]bool{}
		for k := range m1 {
			allKeys[k] = true
		}
		for k := range m2 {
			allKeys[k] = true
		}
		for k := range allKeys {
			newPath := path + "/" + k
			if val1, ok := m1[k]; ok {
				if val2, ok := m2[k]; ok {
					changes = append(changes, compareValues(newPath, val1, val2)...)
				} else {
					changes = append(changes, policyChange{Path: newPath, OldValue: val1, Changed: true})
				}
			} else if _, ok := m2[k]; ok {
				changes = append(changes, policyChange{Path: newPath, NewValue: m2[k], Changed: true})
			}
		}
	} else if !reflect.DeepEqual(v1, v2) {
		changes = append(changes, policyChange{Path: path, OldValue: v1, NewValue: v2, Changed: true})
	}
	return changes
}

func toString(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
