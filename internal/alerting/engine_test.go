package alerting

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type memoryAlertStore struct {
	mu       sync.Mutex
	alerts   map[string]*Alert
	saves    []*Alert
	cveByKey map[string]*Alert
}

func newMemoryAlertStore() *memoryAlertStore {
	return &memoryAlertStore{alerts: map[string]*Alert{}, cveByKey: map[string]*Alert{}}
}

func (*memoryAlertStore) LoadRules(context.Context) ([]*Rule, error) { return nil, nil }
func (*memoryAlertStore) LoadActiveAlertStates(context.Context) ([]*Alert, error) {
	return nil, nil
}
func (*memoryAlertStore) SaveRule(context.Context, *Rule) error { return nil }
func (*memoryAlertStore) DeleteRule(context.Context, string, string) error {
	return nil
}
func (*memoryAlertStore) ListRules(context.Context, string) ([]*Rule, error) {
	return nil, nil
}
func (*memoryAlertStore) GetActiveAlerts(context.Context, string) ([]*Alert, error) {
	return nil, nil
}
func (*memoryAlertStore) GetAlertHistory(context.Context, string, int, int) ([]*Alert, error) {
	return nil, nil
}
func (*memoryAlertStore) UpdateAlertStatus(context.Context, string, string, AlertStatus) error {
	return nil
}
func (*memoryAlertStore) SaveMaintenanceWindow(context.Context, *MaintenanceWindow) error {
	return nil
}
func (*memoryAlertStore) ListMaintenanceWindows(context.Context, string) ([]*MaintenanceWindow, error) {
	return nil, nil
}
func (*memoryAlertStore) DeleteMaintenanceWindow(context.Context, string) error {
	return nil
}
func (*memoryAlertStore) GetActiveMaintenanceWindows(context.Context, string, string) ([]*MaintenanceWindow, error) {
	return nil, nil
}

func (s *memoryAlertStore) SaveAlert(_ context.Context, alert *Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *alert
	s.alerts[alert.ID] = &copy
	s.saves = append(s.saves, &copy)
	return nil
}

func (s *memoryAlertStore) SaveCVEAlert(_ context.Context, alert *Alert, key string) (*Alert, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.cveByKey[alert.TenantID+":"+key]; existing != nil {
		copy := *existing
		return &copy, false, nil
	}
	copy := *alert
	copy.CorrelationKey = key
	s.cveByKey[alert.TenantID+":"+key] = &copy
	s.alerts[copy.ID] = &copy
	s.saves = append(s.saves, &copy)
	return &copy, true, nil
}

func (s *memoryAlertStore) ResolveCVEAlert(_ context.Context, tenantID, deviceID, key string, now time.Time) (*Alert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	alert := s.cveByKey[tenantID+":"+key]
	if alert == nil || alert.DeviceID != deviceID {
		return nil, fmt.Errorf("alert not found")
	}
	copy := *alert
	copy.Status = AlertResolved
	copy.ResolvedAt = &now
	s.cveByKey[tenantID+":"+key] = &copy
	s.alerts[copy.ID] = &copy
	s.saves = append(s.saves, &copy)
	return &copy, nil
}

func newTestEngine(store alertStore, now time.Time) *Engine {
	engine := NewEngine(nil, nil, store, nil, zap.NewNop())
	engine.now = func() time.Time { return now }
	return engine
}

func validThresholdRule() *Rule {
	return &Rule{
		ID: uuid.NewString(), TenantID: uuid.NewString(), Name: "CPU high",
		Type: RuleTypeThreshold, Enabled: true, Severity: SeverityCritical,
		MetricName: "cpu.percent", Condition: ConditionGTE, Threshold: 90,
		Cooldown: 5 * time.Minute,
	}
}

func TestThresholdLifecycleCorrelatesFireAndResolution(t *testing.T) {
	store := newMemoryAlertStore()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	engine := newTestEngine(store, now)
	rule := validThresholdRule()
	deviceID := uuid.NewString()

	engine.evaluateThreshold(rule, rule.TenantID, deviceID, rule.MetricName, 95, now.UnixNano())
	engine.evaluateThreshold(rule, rule.TenantID, deviceID, rule.MetricName, 99, now.UnixNano())
	engine.now = func() time.Time { return now.Add(time.Minute) }
	engine.evaluateThreshold(rule, rule.TenantID, deviceID, rule.MetricName, 20, now.Add(time.Minute).UnixNano())

	if len(store.saves) != 2 {
		t.Fatalf("expected one firing and one resolution save, got %d", len(store.saves))
	}
	if store.saves[0].ID != store.saves[1].ID {
		t.Fatalf("resolution did not correlate to firing alert: %s != %s", store.saves[0].ID, store.saves[1].ID)
	}
	if _, err := uuid.Parse(store.saves[0].ID); err != nil {
		t.Fatalf("alert ID is not a UUID: %v", err)
	}
	if store.saves[1].Status != AlertResolved || store.saves[1].TenantID != rule.TenantID {
		t.Fatalf("unexpected resolution: %+v", store.saves[1])
	}
}

func TestThresholdCooldownPreventsImmediateRefire(t *testing.T) {
	store := newMemoryAlertStore()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	engine := newTestEngine(store, now)
	rule := validThresholdRule()
	deviceID := uuid.NewString()

	engine.evaluateThreshold(rule, rule.TenantID, deviceID, rule.MetricName, 95, now.UnixNano())
	engine.now = func() time.Time { return now.Add(time.Minute) }
	engine.evaluateThreshold(rule, rule.TenantID, deviceID, rule.MetricName, 10, now.Add(time.Minute).UnixNano())
	engine.now = func() time.Time { return now.Add(2 * time.Minute) }
	engine.evaluateThreshold(rule, rule.TenantID, deviceID, rule.MetricName, 95, now.Add(2*time.Minute).UnixNano())
	if len(store.saves) != 2 {
		t.Fatalf("cooldown allowed immediate refire: %d saves", len(store.saves))
	}
}

func TestThresholdLifecycleRestoresActiveAlertAfterRestart(t *testing.T) {
	store := newMemoryAlertStore()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	engine := newTestEngine(store, now)
	rule := validThresholdRule()
	deviceID := uuid.NewString()
	alertID := uuid.NewString()
	key := rule.ID + ":" + deviceID + ":" + rule.MetricName
	engine.restoreActiveAlerts([]*Alert{{
		ID: alertID, RuleID: rule.ID, TenantID: rule.TenantID, DeviceID: deviceID,
		MetricName: rule.MetricName, Status: AlertFiring, FiredAt: now.Add(-time.Minute),
		CorrelationKey: "rule:" + key,
	}})

	engine.evaluateThreshold(rule, rule.TenantID, deviceID, rule.MetricName, 99, now.UnixNano())
	engine.evaluateThreshold(rule, rule.TenantID, deviceID, rule.MetricName, 10, now.UnixNano())
	if len(store.saves) != 1 || store.saves[0].ID != alertID || store.saves[0].Status != AlertResolved {
		t.Fatalf("restart state did not preserve the active lifecycle: %+v", store.saves)
	}
}

func TestHeartbeatLifecycleResetsStateAndCorrelatesResolution(t *testing.T) {
	store := newMemoryAlertStore()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	engine := newTestEngine(store, now)
	rule := &Rule{ID: uuid.NewString(), TenantID: uuid.NewString(), Name: "Offline", Type: RuleTypeHeartbeat, Enabled: true, Severity: SeverityWarning, Timeout: time.Minute}
	engine.rules[rule.ID] = rule
	deviceID := uuid.NewString()
	engine.evaluateHeartbeat(rule, rule.TenantID, deviceID, now.Add(-2*time.Minute))
	engine.checkStaleHeartbeats()
	engine.now = func() time.Time { return now.Add(time.Second) }
	engine.evaluateHeartbeat(rule, rule.TenantID, deviceID, now.Add(time.Second))

	if len(store.saves) != 2 || store.saves[0].ID != store.saves[1].ID {
		t.Fatalf("heartbeat lifecycle was not correlated: %+v", store.saves)
	}
	state := engine.states[rule.ID+":"+deviceID]
	if state.State != StateOK || state.AlertID != "" {
		t.Fatalf("heartbeat recovery did not reset state: %+v", state)
	}
}

func TestCVEAlertRequiresScopeAndDeduplicates(t *testing.T) {
	store := newMemoryAlertStore()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	engine := newTestEngine(store, now)
	tenantID, deviceID := uuid.NewString(), uuid.NewString()

	engine.FireCVEAlert("", deviceID, "CVE-2026-1", "openssl", "critical", "1", "2")
	engine.FireCVEAlert(tenantID, deviceID, "CVE-2026-1", "openssl", "critical", "1", "2")
	engine.FireCVEAlert(tenantID, deviceID, "CVE-2026-1", "openssl", "critical", "1", "2")
	engine.FireCVEAlert(tenantID, uuid.NewString(), "CVE-2026-1", "openssl", "critical", "1", "2")
	engine.ResolveCVEAlert(tenantID, deviceID, "CVE-2026-1", "openssl")

	if len(store.saves) != 3 {
		t.Fatalf("expected two device-scoped firings and one correlated resolution, got %d", len(store.saves))
	}
	if store.saves[0].ID != store.saves[2].ID || store.saves[2].TenantID != tenantID {
		t.Fatalf("CVE lifecycle lost correlation or tenant: %+v", store.saves)
	}
}

func TestMetricsHandlerSkipsDisabledRuleWithoutDeadlock(t *testing.T) {
	store := newMemoryAlertStore()
	engine := newTestEngine(store, time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC))
	rule := validThresholdRule()
	rule.Enabled = false
	engine.rules[rule.ID] = rule
	message := &nats.Msg{Data: []byte(fmt.Sprintf(
		`{"tenant_id":%q,"device_id":%q,"samples":[{"name":%q,"value":99,"timestamp":1}]}`,
		rule.TenantID, uuid.NewString(), rule.MetricName,
	))}
	done := make(chan struct{})
	go func() {
		engine.handleMetrics(message)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("metrics handler deadlocked")
	}
	if len(store.saves) != 0 {
		t.Fatalf("disabled rule fired %d alert(s)", len(store.saves))
	}
}

func TestMetricsHandlerEvaluatesEnabledRuleWithoutDeadlock(t *testing.T) {
	store := newMemoryAlertStore()
	engine := newTestEngine(store, time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC))
	rule := validThresholdRule()
	engine.rules[rule.ID] = rule
	message := &nats.Msg{Data: []byte(fmt.Sprintf(
		`{"tenant_id":%q,"device_id":%q,"samples":[{"name":%q,"value":99,"timestamp":1}]}`,
		rule.TenantID, uuid.NewString(), rule.MetricName,
	))}
	done := make(chan struct{})
	go func() {
		engine.handleMetrics(message)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("metrics handler deadlocked")
	}
	if len(store.saves) != 1 || store.saves[0].Status != AlertFiring {
		t.Fatalf("enabled rule did not fire exactly once: %+v", store.saves)
	}
}

func TestRuleValidationRejectsIncompleteRules(t *testing.T) {
	rule := validThresholdRule()
	rule.Condition = ""
	rule.Severity = "severe"
	rule.Channels = []ChannelType{ChannelSlack, ChannelSlack}
	if err := rule.Validate(); err == nil {
		t.Fatal("invalid rule passed validation")
	}
}

func TestAlertGroupingCreatesAndUpdatesGroups(t *testing.T) {
	grouping := NewGroupingEngine()

	group1, created1 := grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", time.Now())
	if !created1 {
		t.Fatal("first group should be created")
	}
	if group1.Key.RuleID != "rule1" || group1.Key.DeviceID != "device1" {
		t.Fatalf("unexpected group key: %+v", group1.Key)
	}
	if group1.Count != 1 {
		t.Fatalf("expected count 1, got %d", group1.Count)
	}

	group2, created2 := grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "memory", SeverityCritical, "", time.Now())
	if created2 {
		t.Fatal("second call should not create new group")
	}
	if group1 != group2 {
		t.Fatal("should return same group")
	}
	if group2.Count != 2 {
		t.Fatalf("expected count 2 after second alert, got %d", group2.Count)
	}
}

func TestAlertGroupingTracksMultipleMetrics(t *testing.T) {
	grouping := NewGroupingEngine()

	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", time.Now())
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "memory", SeverityCritical, "", time.Now())
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "disk", SeverityCritical, "", time.Now())

	group := grouping.GetGroup("rule1", "device1")
	if len(group.MetricNames) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(group.MetricNames))
	}

	if group.Status != GroupActive {
		t.Fatalf("expected active status, got %s", group.Status)
	}
}

func TestAlertGroupingResolve(t *testing.T) {
	grouping := NewGroupingEngine()
	grouping.GetOrCreateGroup("rule1", "tenant1", "device1", "cpu", SeverityCritical, "", time.Now())

	grouping.ResolveGroup("rule1", "device1", time.Now())

	group := grouping.GetGroup("rule1", "device1")
	if group.Status != GroupResolved {
		t.Fatalf("expected resolved status, got %s", group.Status)
	}
}

func TestMaintenanceEngineCreatesAndListsWindows(t *testing.T) {
	store := newMemoryMaintenanceStore()
	maintenance := NewMaintenanceEngine(store)

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)

	window, err := maintenance.CreateWindow(context.Background(), "tenant1", "test-window", start, end, []string{"device1", "device2"})
	if err != nil {
		t.Fatalf("failed to create window: %v", err)
	}

	if window.Name != "test-window" {
		t.Fatalf("expected name test-window, got %s", window.Name)
	}

	windows, err := maintenance.ListWindows(context.Background(), "tenant1")
	if err != nil {
		t.Fatalf("failed to list windows: %v", err)
	}

	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}

	if windows[0].ID != window.ID {
		t.Fatalf("expected window ID %s, got %s", window.ID, windows[0].ID)
	}
}

func TestMaintenanceEngineChecksMaintenanceWindow(t *testing.T) {
	store := newMemoryMaintenanceStore()
	maintenance := NewMaintenanceEngine(store)

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)

	maintenance.CreateWindow(context.Background(), "tenant1", "test-window", start, end, []string{"device1"})

	if !maintenance.IsInMaintenance("tenant1", "device1", time.Now()) {
		t.Fatal("device1 should be in maintenance window")
	}

	if maintenance.IsInMaintenance("tenant1", "device2", time.Now()) {
		t.Fatal("device2 should not be in maintenance window")
	}

	if maintenance.IsInMaintenance("tenant2", "device1", time.Now()) {
		t.Fatal("different tenant should not be in maintenance window")
	}
}

type memoryMaintenanceStore struct {
	mu      sync.Mutex
	windows map[string]*MaintenanceWindow
}

func newMemoryMaintenanceStore() *memoryMaintenanceStore {
	return &memoryMaintenanceStore{windows: make(map[string]*MaintenanceWindow)}
}

func (s *memoryMaintenanceStore) SaveMaintenanceWindow(_ context.Context, window *MaintenanceWindow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windows[window.ID] = window
	return nil
}

func (s *memoryMaintenanceStore) ListMaintenanceWindows(_ context.Context, tenantID string) ([]*MaintenanceWindow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []*MaintenanceWindow
	for _, w := range s.windows {
		if tenantID == "" || w.TenantID == tenantID {
			result = append(result, w)
		}
	}
	return result, nil
}

func (s *memoryMaintenanceStore) DeleteMaintenanceWindow(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.windows, id)
	return nil
}

func (s *memoryMaintenanceStore) GetActiveMaintenanceWindows(_ context.Context, tenantID, deviceID string) ([]*MaintenanceWindow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []*MaintenanceWindow
	for _, w := range s.windows {
		if w.TenantID != tenantID {
			continue
		}
		now := time.Now()
		if (now.Equal(w.StartTime) || now.After(w.StartTime)) && (now.Equal(w.EndTime) || now.Before(w.EndTime)) {
			if len(w.DeviceIDs) == 0 {
				result = append(result, w)
			} else {
				for _, d := range w.DeviceIDs {
					if d == deviceID {
						result = append(result, w)
						break
					}
				}
			}
		}
	}
	return result, nil
}

func TestEngineSilencesAlertsDuringMaintenance(t *testing.T) {
	store := newMemoryAlertStore()
	maintenanceStore := newMemoryMaintenanceStore()

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)

	window := &MaintenanceWindow{
		ID:        uuid.NewString(),
		TenantID:  "tenant1",
		Name:      "test-window",
		StartTime: start,
		EndTime:   end,
		DeviceIDs: []string{"device1"},
	}
	maintenanceStore.SaveMaintenanceWindow(context.Background(), window)

	maintenance := NewMaintenanceEngine(maintenanceStore)
	maintenance.LoadWindows([]*MaintenanceWindow{window})

	engine := NewEngine(nil, nil, store, nil, zap.NewNop())
	engine.WithMaintenanceEngine(maintenance)

	rule := &Rule{
		ID: uuid.NewString(), TenantID: "tenant1", Name: "CPU high",
		Type: RuleTypeThreshold, Enabled: true, Severity: SeverityCritical,
		MetricName: "cpu.percent", Condition: ConditionGTE, Threshold: 90,
		Cooldown: 5 * time.Minute,
	}
	engine.rules[rule.ID] = rule

	deviceID := "device1"

	engine.evaluateThreshold(rule, "tenant1", deviceID, "cpu.percent", 95, time.Now().UnixNano())

	if len(store.saves) != 0 {
		t.Fatalf("alert should be silenced during maintenance, got %d saves", len(store.saves))
	}

	group := engine.GroupingEngine().GetGroup(rule.ID, deviceID)
	if group == nil {
		t.Fatal("group should still be created even if silenced")
	}
	if group.Status != GroupSilenced {
		t.Fatalf("expected silenced status, got %s", group.Status)
	}
}

func TestEngineResumesAlertingAfterMaintenance(t *testing.T) {
	store := newMemoryAlertStore()
	maintenanceStore := newMemoryMaintenanceStore()

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(-time.Minute)

	window := &MaintenanceWindow{
		ID:        uuid.NewString(),
		TenantID:  "tenant1",
		Name:      "test-window",
		StartTime: start,
		EndTime:   end,
		DeviceIDs: []string{"device1"},
	}
	maintenanceStore.SaveMaintenanceWindow(context.Background(), window)

	maintenance := NewMaintenanceEngine(maintenanceStore)
	maintenance.LoadWindows([]*MaintenanceWindow{window})

	engine := NewEngine(nil, nil, store, nil, zap.NewNop())
	engine.WithMaintenanceEngine(maintenance)

	rule := &Rule{
		ID: uuid.NewString(), TenantID: "tenant1", Name: "CPU high",
		Type: RuleTypeThreshold, Enabled: true, Severity: SeverityCritical,
		MetricName: "cpu.percent", Condition: ConditionGTE, Threshold: 90,
		Cooldown: 5 * time.Minute,
	}
	engine.rules[rule.ID] = rule

	deviceID := "device1"

	engine.evaluateThreshold(rule, "tenant1", deviceID, "cpu.percent", 95, time.Now().UnixNano())

	if len(store.saves) != 1 {
		t.Fatalf("alert should fire after maintenance, got %d saves", len(store.saves))
	}

	if store.saves[0].Status != AlertFiring {
		t.Fatalf("expected firing status, got %s", store.saves[0].Status)
	}

	group := engine.GroupingEngine().GetGroup(rule.ID, deviceID)
	if group.Status != GroupActive {
		t.Fatalf("expected active status after maintenance, got %s", group.Status)
	}
}
