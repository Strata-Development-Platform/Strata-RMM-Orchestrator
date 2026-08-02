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
