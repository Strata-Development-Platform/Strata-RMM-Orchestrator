package platform

import (
	"encoding/json"
	"fmt"
	"strings"
)

var policyCategories = map[string]bool{
	"patch": true, "alerting": true, "monitoring": true,
	"software": true, "script": true, "maintenance": true,
	"maintenance_window": true,
}

var policyScopeRank = map[string]int{"msp": 1, "client": 2, "site": 3, "device": 4}

type policyInput struct {
	Name                string                 `json:"name"`
	Category            string                 `json:"category"`
	Description         string                 `json:"description"`
	Config              map[string]interface{} `json:"config"`
	ScopeLevel          string                 `json:"scope_level"`
	ClientID            string                 `json:"client_id,omitempty"`
	SiteID              string                 `json:"site_id,omitempty"`
	DeviceID            string                 `json:"device_id,omitempty"`
	ParentID            string                 `json:"parent_id,omitempty"`
	MaintenanceStart    *string                `json:"maintenance_start,omitempty"`
	MaintenanceEnd      *string                `json:"maintenance_end,omitempty"`
	MaintenanceDays     *[]string              `json:"maintenance_days,omitempty"`
	MaintenanceTimezone string                 `json:"maintenance_timezone,omitempty"`
}

type policyLayer struct {
	ID                  string                 `json:"id"`
	ScopeLevel          string                 `json:"scope_level"`
	Version             int                    `json:"version"`
	PublishedVersion    int                    `json:"published_version"`
	Config              map[string]interface{} `json:"config"`
	MaintenanceStart    *string                `json:"maintenance_start,omitempty"`
	MaintenanceEnd      *string                `json:"maintenance_end,omitempty"`
	MaintenanceDays     *[]string              `json:"maintenance_days,omitempty"`
	MaintenanceTimezone string                 `json:"maintenance_timezone,omitempty"`
}

func (p *policyInput) normalize() {
	p.Name = strings.TrimSpace(p.Name)
	p.Category = strings.ToLower(strings.TrimSpace(p.Category))
	p.Description = strings.TrimSpace(p.Description)
	p.ScopeLevel = strings.ToLower(strings.TrimSpace(p.ScopeLevel))
	if p.ScopeLevel == "" {
		p.ScopeLevel = "msp"
	}
	if p.MaintenanceTimezone == "" {
		p.MaintenanceTimezone = "UTC"
	}
}

func (p policyInput) validate() error {
	var problems []string
	if p.Name == "" || len(p.Name) > 200 {
		problems = append(problems, "name must contain 1-200 characters")
	}
	if len(p.Description) > 2000 {
		problems = append(problems, "description must not exceed 2000 characters")
	}
	if !policyCategories[p.Category] {
		problems = append(problems, "unsupported category")
	}
	if _, ok := policyScopeRank[p.ScopeLevel]; !ok {
		problems = append(problems, "scope_level must be msp, client, site, or device")
	}
	switch p.ScopeLevel {
	case "msp":
		if p.ClientID != "" || p.SiteID != "" || p.DeviceID != "" {
			problems = append(problems, "msp policies cannot identify a child scope")
		}
	case "client":
		if p.ClientID == "" || p.SiteID != "" || p.DeviceID != "" {
			problems = append(problems, "client policies require only client_id")
		}
	case "site":
		if p.ClientID == "" || p.SiteID == "" || p.DeviceID != "" {
			problems = append(problems, "site policies require client_id and site_id")
		}
	case "device":
		if p.ClientID == "" || p.SiteID == "" || p.DeviceID == "" {
			problems = append(problems, "device policies require client_id, site_id, and device_id")
		}
	}
	if len(p.Config) == 0 {
		problems = append(problems, "config must be a non-empty object")
	} else if encoded, err := json.Marshal(p.Config); err != nil || len(encoded) > 64*1024 {
		problems = append(problems, "config must be valid JSON no larger than 64 KiB")
	} else if policyDepth(p.Config) > 12 {
		problems = append(problems, "config nesting must not exceed 12 levels")
	}
	if p.MaintenanceStart != nil && !isValidTime(*p.MaintenanceStart) {
		problems = append(problems, "maintenance_start must be in HH:MM format")
	}
	if p.MaintenanceEnd != nil && !isValidTime(*p.MaintenanceEnd) {
		problems = append(problems, "maintenance_end must be in HH:MM format")
	}
	if p.MaintenanceStart != nil && p.MaintenanceEnd != nil && *p.MaintenanceStart >= *p.MaintenanceEnd {
		problems = append(problems, "maintenance_start must be before maintenance_end")
	}
	if p.MaintenanceDays != nil {
		if len(*p.MaintenanceDays) == 0 {
			problems = append(problems, "maintenance_days must not be empty")
		} else {
			for _, day := range *p.MaintenanceDays {
				if !isValidDay(day) {
					problems = append(problems, "maintenance_days must contain valid day names (monday-sunday)")
					break
				}
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid policy: %s", strings.Join(problems, "; "))
	}
	return nil
}

func isValidTime(t string) bool {
	if len(t) != 5 {
		return false
	}
	if t[2] != ':' {
		return false
	}
	hours := t[0:2]
	minutes := t[3:5]
	for i := 0; i < 2; i++ {
		if hours[i] < '0' || hours[i] > '9' || minutes[i] < '0' || minutes[i] > '9' {
			return false
		}
	}
	h, m := int(hours[0]-48)*10+int(hours[1]-48), int(minutes[0]-48)*10+int(minutes[1]-48)
	return h < 24 && m < 60
}

func isValidDay(day string) bool {
	days := map[string]bool{"monday": true, "tuesday": true, "wednesday": true, "thursday": true, "friday": true, "saturday": true, "sunday": true}
	return days[day]
}

func policyDepth(value interface{}) int {
	switch typed := value.(type) {
	case map[string]interface{}:
		max := 0
		for _, child := range typed {
			if depth := policyDepth(child); depth > max {
				max = depth
			}
		}
		return max + 1
	case []interface{}:
		max := 0
		for _, child := range typed {
			if depth := policyDepth(child); depth > max {
				max = depth
			}
		}
		return max + 1
	default:
		return 1
	}
}

func mergePolicyLayers(layers []policyLayer) map[string]interface{} {
	effective := map[string]interface{}{}
	for _, layer := range layers {
		mergePolicyMap(effective, layer.Config)
	}
	return effective
}

func mergePolicyMap(destination, source map[string]interface{}) {
	for key, value := range source {
		sourceMap, sourceIsMap := value.(map[string]interface{})
		destinationMap, destinationIsMap := destination[key].(map[string]interface{})
		if sourceIsMap && destinationIsMap {
			mergePolicyMap(destinationMap, sourceMap)
			continue
		}
		destination[key] = clonePolicyValue(value)
	}
}

func clonePolicyValue(value interface{}) interface{} {
	encoded, _ := json.Marshal(value)
	var clone interface{}
	_ = json.Unmarshal(encoded, &clone)
	return clone
}
