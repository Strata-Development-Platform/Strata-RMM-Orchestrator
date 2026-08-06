package alerting

import (
	"fmt"
	"sync"
	"time"
)

type AlertGroupKey struct {
	RuleID   string
	DeviceID string
}

type AlertGroup struct {
	Key         AlertGroupKey `json:"key"`
	TenantID    string        `json:"tenant_id,omitempty"`
	Severity    Severity      `json:"severity,omitempty"`
	Status      GroupStatus   `json:"status"`
	AlertID     string        `json:"alert_id,omitempty"`
	Count       int           `json:"count"`
	FiredAt     time.Time     `json:"fired_at"`
	LastFired   time.Time     `json:"last_fired"`
	MetricNames []string      `json:"metric_names,omitempty"`
}

type GroupStatus string

const (
	GroupActive   GroupStatus = "active"
	GroupSilenced GroupStatus = "silenced"
	GroupResolved GroupStatus = "resolved"
)

type GroupingEngine struct {
	mu     sync.RWMutex
	groups map[AlertGroupKey]*AlertGroup
	now    func() time.Time
}

func NewGroupingEngine() *GroupingEngine {
	return &GroupingEngine{
		groups: make(map[AlertGroupKey]*AlertGroup),
		now:    time.Now,
	}
}

func (g *GroupingEngine) GetOrCreateGroup(ruleID, tenantID, deviceID, metricName string, severity Severity, message string, firedAt time.Time) (*AlertGroup, bool) {
	key := AlertGroupKey{RuleID: ruleID, DeviceID: deviceID}
	g.mu.Lock()
	defer g.mu.Unlock()
	group, exists := g.groups[key]
	if !exists {
		group = &AlertGroup{
			Key:         key,
			TenantID:    tenantID,
			Severity:    severity,
			Status:      GroupActive,
			Count:       0,
			FiredAt:     firedAt,
			LastFired:   firedAt,
			MetricNames: []string{},
		}
		g.groups[key] = group
	}
	group.Count++
	group.LastFired = firedAt
	if severity != "" && group.Severity == "" {
		group.Severity = severity
	}
	if !containsString(group.MetricNames, metricName) {
		group.MetricNames = append(group.MetricNames, metricName)
	}
	return group, !exists
}

func (g *GroupingEngine) UpdateGroupStatus(ruleID, deviceID string, status GroupStatus, alertID string) {
	key := AlertGroupKey{RuleID: ruleID, DeviceID: deviceID}
	g.mu.Lock()
	defer g.mu.Unlock()
	if group, exists := g.groups[key]; exists {
		group.Status = status
		if alertID != "" {
			group.AlertID = alertID
		}
	}
}

func (g *GroupingEngine) GetGroup(ruleID, deviceID string) *AlertGroup {
	key := AlertGroupKey{RuleID: ruleID, DeviceID: deviceID}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.groups[key]
}

func (g *GroupingEngine) GetGroupsByTenant(tenantID string) []*AlertGroup {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var groups []*AlertGroup
	for _, g := range g.groups {
		if tenantID == "" || g.Key.RuleID == tenantID || g.Key.DeviceID == tenantID {
			groups = append(groups, g)
		}
	}
	return groups
}

func (g *GroupingEngine) GetAllGroups() []*AlertGroup {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var groups []*AlertGroup
	for _, g := range g.groups {
		groups = append(groups, g)
	}
	return groups
}

func (g *GroupingEngine) ResolveGroup(ruleID, deviceID string, resolvedAt time.Time) {
	key := AlertGroupKey{RuleID: ruleID, DeviceID: deviceID}
	g.mu.Lock()
	defer g.mu.Unlock()
	if group, exists := g.groups[key]; exists {
		group.Status = GroupResolved
		group.LastFired = resolvedAt
	}
}

func (g *GroupingEngine) DeleteGroup(ruleID, deviceID string) {
	key := AlertGroupKey{RuleID: ruleID, DeviceID: deviceID}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.groups, key)
}

func (g *GroupingEngine) ResolveGroupWithID(groupID, ruleID, deviceID string) {
	key := AlertGroupKey{RuleID: ruleID, DeviceID: deviceID}
	g.mu.Lock()
	defer g.mu.Unlock()
	if group, exists := g.groups[key]; exists {
		group.AlertID = groupID
		group.Status = GroupResolved
	}
}

func (g *GroupingEngine) GroupCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.groups)
}

func (g *GroupingEngine) GetGroupsBySeverity(severity Severity) []*AlertGroup {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var result []*AlertGroup
	for _, grp := range g.groups {
		if grp.Severity == severity && grp.Status == GroupActive {
			result = append(result, grp)
		}
	}
	return result
}

func (g *GroupingEngine) GetActiveGroupsBySeverity(severity Severity) int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	count := 0
	for _, grp := range g.groups {
		if grp.Severity == severity && grp.Status == GroupActive {
			count++
		}
	}
	return count
}

func (g *GroupingEngine) ResolveGroupsByDevice(deviceID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	count := 0
	for key, grp := range g.groups {
		if grp.Key.DeviceID == deviceID && grp.Status == GroupActive {
			grp.Status = GroupResolved
			count++
		}
		if grp.Status == GroupResolved {
			delete(g.groups, key)
		}
	}
	return count
}

func (g *GroupingEngine) GetGroupsByDevice(deviceID string) []*AlertGroup {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var result []*AlertGroup
	for _, grp := range g.groups {
		if grp.Key.DeviceID == deviceID {
			result = append(result, grp)
		}
	}
	return result
}

func (g *GroupingEngine) GetDeviceAlertCount(deviceID string) int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	count := 0
	for _, grp := range g.groups {
		if grp.Key.DeviceID == deviceID && grp.Status == GroupActive {
			count += grp.Count
		}
	}
	return count
}

func (g *GroupingEngine) ResolveAll() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	count := 0
	for _, grp := range g.groups {
		if grp.Status == GroupActive {
			grp.Status = GroupResolved
			count++
		}
	}
	return count
}

type CascadeGroupKey struct {
	TenantID string
	DeviceID string
}

type CascadeGroup struct {
	Key         CascadeGroupKey `json:"key"`
	Groups      []*AlertGroup   `json:"groups"`
	Status      GroupStatus     `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	LastUpdated time.Time       `json:"last_updated"`
}

func (g *GroupingEngine) GetCascadeGroups(tenantID string) []*CascadeGroup {
	g.mu.RLock()
	defer g.mu.RUnlock()
	deviceGroups := make(map[string][]*AlertGroup)
	for _, grp := range g.groups {
		if tenantID == "" || grp.TenantID == tenantID {
			deviceGroups[grp.Key.DeviceID] = append(deviceGroups[grp.Key.DeviceID], grp)
		}
	}
	var result []*CascadeGroup
	for deviceID, groups := range deviceGroups {
		if len(groups) > 0 && groups[0].Status == GroupActive {
			cg := &CascadeGroup{
				Key:         CascadeGroupKey{TenantID: groups[0].TenantID, DeviceID: deviceID},
				Groups:      groups,
				Status:      GroupActive,
				CreatedAt:   groups[0].FiredAt,
				LastUpdated: groups[0].LastFired,
			}
			result = append(result, cg)
		}
	}
	return result
}

func (g *GroupingEngine) GetTimeWindowGroups(window time.Duration) []*AlertGroup {
	g.mu.RLock()
	defer g.mu.RUnlock()
	now := g.now()
	var result []*AlertGroup
	for _, grp := range g.groups {
		if grp.Status == GroupActive && now.Sub(grp.LastFired) <= window {
			result = append(result, grp)
		}
	}
	return result
}

func (g *GroupingEngine) ResolveCascadeGroup(deviceID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	count := 0
	for key, grp := range g.groups {
		if grp.Key.DeviceID == deviceID {
			grp.Status = GroupResolved
			count++
			delete(g.groups, key)
		}
	}
	return count
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func (g *GroupingEngine) FormatGroupMessage(group *AlertGroup) string {
	if len(group.MetricNames) == 0 {
		return fmt.Sprintf("[%s] Alert triggered on device %s", group.Status, group.Key.DeviceID)
	}
	metrics := fmt.Sprintf("%d metrics", len(group.MetricNames))
	if len(group.MetricNames) == 1 {
		metrics = group.MetricNames[0]
	}
	return fmt.Sprintf("[%s] Alert %s on %s for %s (count: %d)",
		group.Status, group.Key.RuleID, group.Key.DeviceID, metrics, group.Count)
}
