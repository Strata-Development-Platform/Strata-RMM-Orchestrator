package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type Engine struct {
	nats     *nats.Conn
	tsdb     *timescale.Client
	logger   *zap.Logger
	store    alertStore
	notifier *Notifier
	now      func() time.Time

	mu            sync.RWMutex
	rules         map[string]*Rule       // ruleID -> Rule
	states        map[string]*AlertState // ruleID+deviceID -> state
	subs          []*nats.Subscription
	grouping      *GroupingEngine
	maintenance   *MaintenanceEngine
	maintenanceMu sync.RWMutex
	maintenanceWindows map[string]*MaintenanceWindow // id -> window (for quick lookup)
}

type alertStore interface {
	LoadRules(context.Context) ([]*Rule, error)
	LoadActiveAlertStates(context.Context) ([]*Alert, error)
	SaveRule(context.Context, *Rule) error
	DeleteRule(context.Context, string, string) error
	ListRules(context.Context, string) ([]*Rule, error)
	SaveAlert(context.Context, *Alert) error
	GetActiveAlerts(context.Context, string) ([]*Alert, error)
	GetAlertHistory(context.Context, string, int, int) ([]*Alert, error)
	UpdateAlertStatus(context.Context, string, string, AlertStatus) error
	SaveCVEAlert(context.Context, *Alert, string) (*Alert, bool, error)
	ResolveCVEAlert(context.Context, string, string, string, time.Time) (*Alert, error)
	
	// Maintenance window operations
	SaveMaintenanceWindow(context.Context, *MaintenanceWindow) error
	ListMaintenanceWindows(context.Context, string) ([]*MaintenanceWindow, error)
	DeleteMaintenanceWindow(context.Context, string) error
	GetActiveMaintenanceWindows(context.Context, string, string) ([]*MaintenanceWindow, error)
}

func NewEngine(nc *nats.Conn, tsdb *timescale.Client, store alertStore, notifier *Notifier, logger *zap.Logger) *Engine {
	return &Engine{
		nats:     nc,
		tsdb:     tsdb,
		logger:   logger,
		store:    store,
		notifier: notifier,
		now:      time.Now,
		rules:    make(map[string]*Rule),
		states:   make(map[string]*AlertState),
		grouping: NewGroupingEngine(),
	}
}

func (e *Engine) WithMaintenanceEngine(m *MaintenanceEngine) *Engine {
	e.maintenance = m
	e.maintenanceWindows = make(map[string]*MaintenanceWindow)
	return e
}

func (e *Engine) Start(ctx context.Context) error {
	e.logger.Info("starting alerting engine")

	rules, err := e.store.LoadRules(ctx)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	activeAlerts, err := e.store.LoadActiveAlertStates(ctx)
	if err != nil {
		return fmt.Errorf("load active alerts: %w", err)
	}
	e.mu.Lock()
	for _, r := range rules {
		e.rules[r.ID] = r
	}
	e.mu.Unlock()
	e.restoreActiveAlerts(activeAlerts)
	
	if e.maintenance != nil {
		if err := e.maintenance.Start(ctx); err != nil {
			e.logger.Warn("failed to start maintenance engine", zap.Error(err))
		}
	}
	e.logger.Info("alerting state loaded", zap.Int("rules", len(rules)), zap.Int("active_alerts", len(activeAlerts)))

	metricsSub, err := e.nats.Subscribe("tenant.*.agent.*.metrics", e.handleMetrics)
	if err != nil {
		return fmt.Errorf("subscribe metrics: %w", err)
	}
	e.subs = append(e.subs, metricsSub)

	heartbeatSub, err := e.nats.Subscribe("tenant.*.agent.*.heartbeat", e.handleHeartbeat)
	if err != nil {
		return fmt.Errorf("subscribe heartbeat: %w", err)
	}
	e.subs = append(e.subs, heartbeatSub)

	go e.evaluationLoop(ctx)
	go e.cleanupLoop(ctx)

	return nil
}

func (e *Engine) restoreActiveAlerts(activeAlerts []*Alert) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, alert := range activeAlerts {
		if alert.RuleID == "" || !strings.HasPrefix(alert.CorrelationKey, "rule:") {
			continue
		}
		key := strings.TrimPrefix(alert.CorrelationKey, "rule:")
		e.states[key] = &AlertState{
			RuleID: alert.RuleID, TenantID: alert.TenantID, DeviceID: alert.DeviceID,
			State: StateFiring, LastFired: alert.FiredAt, AlertID: alert.ID,
			MetricName: alert.MetricName,
		}
	}
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, sub := range e.subs {
		if err := sub.Unsubscribe(); err != nil {
			e.logger.Warn("unsubscribe", zap.Error(err))
		}
	}
}

func (e *Engine) handleMetrics(m *nats.Msg) {
	var payload struct {
		TenantID string `json:"tenant_id"`
		DeviceID string `json:"device_id"`
		Samples  []struct {
			Name      string            `json:"name"`
			Value     float64           `json:"value"`
			Tags      map[string]string `json:"tags"`
			Timestamp int64             `json:"timestamp"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(m.Data, &payload); err != nil {
		e.logger.Warn("invalid metrics payload", zap.Error(err))
		return
	}
	if payload.TenantID == "" || payload.DeviceID == "" {
		return
	}

	e.mu.RLock()
	rules := make([]*Rule, 0, len(e.rules))
	for _, r := range e.rules {
		if r.Enabled && r.TenantID == payload.TenantID && r.Type == RuleTypeThreshold {
			rules = append(rules, r)
		}
	}
	e.mu.RUnlock()

	for _, r := range rules {
		for _, s := range payload.Samples {
			if !r.MatchesMetric(s.Name) {
				continue
			}
			e.evaluateThreshold(r, payload.TenantID, payload.DeviceID, s.Name, s.Value, s.Timestamp)
		}
	}
}

func (e *Engine) handleHeartbeat(m *nats.Msg) {
	var payload struct {
		TenantID string `json:"tenant_id"`
		DeviceID string `json:"device_id"`
		Status   string `json:"status"`
		Time     int64  `json:"time"`
	}
	if err := json.Unmarshal(m.Data, &payload); err != nil {
		return
	}
	if payload.TenantID == "" || payload.DeviceID == "" {
		return
	}

	e.mu.RLock()
	rules := make([]*Rule, 0, len(e.rules))
	for _, r := range e.rules {
		if r.Enabled && r.TenantID == payload.TenantID && r.Type == RuleTypeHeartbeat &&
			(r.DeviceID == "" || r.DeviceID == payload.DeviceID) {
			rules = append(rules, r)
		}
	}
	e.mu.RUnlock()

	for _, r := range rules {
		e.evaluateHeartbeat(r, payload.TenantID, payload.DeviceID, time.Unix(0, payload.Time))
	}
}

func (e *Engine) evaluateThreshold(rule *Rule, tenantID, deviceID, metricName string, value float64, timestamp int64) {
	key := rule.ID + ":" + deviceID + ":" + metricName
	var shouldFire bool
	switch rule.Condition {
	case ConditionGTE:
		shouldFire = value >= rule.Threshold
	case ConditionGT:
		shouldFire = value > rule.Threshold
	case ConditionLTE:
		shouldFire = value <= rule.Threshold
	case ConditionLT:
		shouldFire = value < rule.Threshold
	case ConditionEQ:
		shouldFire = value == rule.Threshold
	case ConditionNEQ:
		shouldFire = value != rule.Threshold
	}

	now := e.now()
	e.mu.Lock()
	state, exists := e.states[key]
	if !exists {
		state = &AlertState{RuleID: rule.ID, TenantID: tenantID, DeviceID: deviceID, State: StateOK, MetricName: metricName}
		e.states[key] = state
	}
	
	_, _ = e.grouping.GetOrCreateGroup(rule.ID, tenantID, deviceID, metricName, rule.Severity, "", now)
	
	e.maintenanceMu.RLock()
	isInMaintenance := e.maintenance != nil && e.maintenance.IsInMaintenance(tenantID, deviceID, now)
	e.maintenanceMu.RUnlock()
	
	if shouldFire && state.State == StateOK {
		if now.Sub(state.LastFired) < rule.Cooldown {
			e.mu.Unlock()
			return
		}
		if isInMaintenance {
			e.mu.Unlock()
			e.logger.Info("alert silenced by maintenance window",
				zap.String("rule", rule.ID),
				zap.String("device", deviceID),
				zap.String("metric", metricName),
				zap.Float64("value", value))
			e.grouping.UpdateGroupStatus(rule.ID, deviceID, GroupSilenced, "")
			return
		}
		alertID := uuid.NewString()
		state.State = StateFiring
		state.LastFired = now
		state.ConsecutiveFires = 1
		state.AlertID = alertID
		e.mu.Unlock()
		alert := &Alert{
			ID:             alertID,
			RuleID:         rule.ID,
			TenantID:       tenantID,
			DeviceID:       deviceID,
			MetricName:     metricName,
			Value:          value,
			Severity:       rule.Severity,
			Message:        rule.FormatMessage(deviceID, metricName, value),
			Status:         AlertFiring,
			FiredAt:        now,
			CorrelationKey: "rule:" + key,
			Channels:       rule.Channels,
		}
		if err := e.fireAlert(alert); err != nil {
			e.logger.Error("fire alert", zap.Error(err))
			e.rollbackFiringTransition(key, alertID)
			return
		}
	} else if !shouldFire && state.State == StateFiring {
		alertID, firedAt := state.AlertID, state.LastFired
		state.State = StateOK
		state.ConsecutiveFires = 0
		state.AlertID = ""
		e.mu.Unlock()
		alert := &Alert{
			ID:             alertID,
			RuleID:         rule.ID,
			TenantID:       tenantID,
			DeviceID:       deviceID,
			MetricName:     metricName,
			Value:          value,
			Severity:       rule.Severity,
			Message:        fmt.Sprintf("Resolved: %s", rule.FormatMessage(deviceID, metricName, value)),
			Status:         AlertResolved,
			FiredAt:        firedAt,
			ResolvedAt:     &now,
			CorrelationKey: "rule:" + key,
			Channels:       rule.Channels,
		}
		if err := e.resolveAlert(alert); err != nil {
			e.logger.Error("resolve alert", zap.Error(err))
			e.restoreFiringTransition(key, alertID, metricName)
			return
		}
		e.grouping.ResolveGroup(rule.ID, deviceID, now)
	} else if shouldFire && state.State == StateFiring {
		state.ConsecutiveFires++
		e.mu.Unlock()
	} else {
		e.mu.Unlock()
	}
}

func (e *Engine) evaluateHeartbeat(rule *Rule, tenantID, deviceID string, lastTime time.Time) {
	key := rule.ID + ":" + deviceID
	e.mu.Lock()
	state, exists := e.states[key]
	if !exists {
		state = &AlertState{
			RuleID:    rule.ID,
			TenantID:  tenantID,
			DeviceID:  deviceID,
			State:     StateOK,
			LastHeard: lastTime,
		}
		e.states[key] = state
	} else if lastTime.After(state.LastHeard) {
		state.LastHeard = lastTime
	}
	
	if state.State == StateFiring {
		now := e.now()
		alertID, firedAt := state.AlertID, state.LastFired
		state.State = StateOK
		state.AlertID = ""
		state.ConsecutiveFires = 0
		e.mu.Unlock()
		alert := &Alert{
			ID:             alertID,
			RuleID:         rule.ID,
			TenantID:       tenantID,
			DeviceID:       deviceID,
			Severity:       rule.Severity,
			Message:        fmt.Sprintf("Heartbeat restored for device %s", deviceID),
			Status:         AlertResolved,
			FiredAt:        firedAt,
			ResolvedAt:     &now,
			CorrelationKey: "rule:" + key,
			Channels:       rule.Channels,
		}
		if err := e.resolveAlert(alert); err != nil {
			e.logger.Error("resolve heartbeat alert", zap.Error(err))
			e.restoreFiringTransition(key, alertID, "")
		}
		e.grouping.ResolveGroup(rule.ID, deviceID, now)
		return
	}
	e.mu.Unlock()
}

func (e *Engine) rollbackFiringTransition(key, alertID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if state := e.states[key]; state != nil && state.AlertID == alertID {
		state.State = StateOK
		state.AlertID = ""
		state.ConsecutiveFires = 0
	}
}

func (e *Engine) restoreFiringTransition(key, alertID, metricName string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if state := e.states[key]; state != nil && state.State == StateOK && state.AlertID == "" {
		state.State = StateFiring
		state.AlertID = alertID
		state.MetricName = metricName
	}
}

func (e *Engine) fireAlert(alert *Alert) error {
	if err := e.store.SaveAlert(context.Background(), alert); err != nil {
		return fmt.Errorf("save alert: %w", err)
	}
	if e.notifier != nil {
		if err := e.notifier.Send(context.Background(), alert); err != nil {
			e.logger.Warn("send notification", zap.Error(err), zap.String("alert_id", alert.ID))
		}
	}

	if e.nats != nil {
		subject := fmt.Sprintf("tenant.%s.alert.%s", alert.TenantID, alert.DeviceID)
		data, _ := json.Marshal(alert)
		if err := e.nats.Publish(subject, data); err != nil {
			e.logger.Warn("publish alert", zap.Error(err))
		}
	}

	e.logger.Warn("alert fired",
		zap.String("rule", alert.RuleID),
		zap.String("device", alert.DeviceID),
		zap.String("metric", alert.MetricName),
		zap.Float64("value", alert.Value),
		zap.String("severity", string(alert.Severity)),
	)
	return nil
}

func (e *Engine) resolveAlert(alert *Alert) error {
	if err := e.store.SaveAlert(context.Background(), alert); err != nil {
		return fmt.Errorf("save alert: %w", err)
	}
	if e.notifier != nil {
		if err := e.notifier.Send(context.Background(), alert); err != nil {
			e.logger.Warn("send resolution notification", zap.Error(err), zap.String("alert_id", alert.ID))
		}
	}

	if e.nats != nil {
		subject := fmt.Sprintf("tenant.%s.alert.%s", alert.TenantID, alert.DeviceID)
		data, _ := json.Marshal(alert)
		if err := e.nats.Publish(subject, data); err != nil {
			e.logger.Warn("publish alert resolution", zap.Error(err))
		}
	}

	e.logger.Info("alert resolved",
		zap.String("rule", alert.RuleID),
		zap.String("device", alert.DeviceID),
	)
	return nil
}

func (e *Engine) evaluationLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.checkStaleHeartbeats()
		}
	}
}

func (e *Engine) checkStaleHeartbeats() {
	now := e.now()
	type pendingAlert struct {
		key   string
		alert *Alert
	}
	var pending []pendingAlert
	e.mu.Lock()
	for key, state := range e.states {
		rule, ok := e.rules[state.RuleID]
		if state.State != StateOK || !ok || !rule.Enabled || rule.Type != RuleTypeHeartbeat ||
			state.LastHeard.IsZero() || now.Sub(state.LastHeard) <= rule.Timeout ||
			now.Sub(state.LastFired) < rule.Cooldown {
			continue
		}
		
		e.maintenanceMu.RLock()
		isInMaintenance := e.maintenance != nil && e.maintenance.IsInMaintenance(state.TenantID, state.DeviceID, now)
		e.maintenanceMu.RUnlock()
		
		if isInMaintenance {
			e.logger.Info("heartbeat alert silenced by maintenance window",
				zap.String("rule", rule.ID),
				zap.String("device", state.DeviceID))
			continue
		}
		
		alertID := uuid.NewString()
		state.State = StateFiring
		state.LastFired = now
		state.AlertID = alertID
		state.ConsecutiveFires = 1
		pending = append(pending, pendingAlert{key: key, alert: &Alert{
			ID: alertID, RuleID: rule.ID, TenantID: state.TenantID, DeviceID: state.DeviceID,
			Severity: rule.Severity,
			Message:  fmt.Sprintf("Device %s has not reported for %v (timeout: %v)", state.DeviceID, now.Sub(state.LastHeard).Round(time.Second), rule.Timeout),
			Status:   AlertFiring, FiredAt: now, CorrelationKey: "rule:" + key, Channels: rule.Channels,
		}})
	}
	e.mu.Unlock()

	for _, item := range pending {
		if err := e.fireAlert(item.alert); err != nil {
			e.logger.Error("fire heartbeat alert", zap.Error(err))
			e.rollbackFiringTransition(item.key, item.alert.ID)
		}
	}
}

func (e *Engine) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.mu.Lock()
			for key, state := range e.states {
				if state.State == StateOK && time.Since(state.LastFired) > 24*time.Hour {
					delete(e.states, key)
				}
			}
			e.mu.Unlock()
		}
	}
}

func (e *Engine) FireCVEAlert(tenantID, deviceID, cveID, packageName, severity, currentVersion, fixedVersion string) {
	if tenantID == "" || deviceID == "" || cveID == "" || packageName == "" {
		e.logger.Error("reject CVE alert without correlation scope")
		return
	}
	
	now := e.now()
	e.maintenanceMu.RLock()
	isInMaintenance := e.maintenance != nil && e.maintenance.IsInMaintenance(tenantID, deviceID, now)
	e.maintenanceMu.RUnlock()
	
	if isInMaintenance {
		e.logger.Info("CVE alert silenced by maintenance window",
			zap.String("cve", cveID),
			zap.String("device", deviceID))
		return
	}
	
	alert := &Alert{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		DeviceID:   deviceID,
		MetricName: "cve_count",
		Severity:   Severity(severity),
		Message:    fmt.Sprintf("[%s] %s in %s %s — upgrade to %s on device %s", strings.ToUpper(severity), cveID, packageName, currentVersion, fixedVersion, deviceID),
		Status:     AlertFiring,
		FiredAt:    now,
	}
	correlationKey := "cve:" + deviceID + ":" + cveID + ":" + packageName
	e.logger.Warn("CVE alert fired",
		zap.String("cve", cveID),
		zap.String("device", deviceID),
		zap.String("package", packageName),
		zap.String("severity", severity),
	)
	persisted, created, err := e.store.SaveCVEAlert(context.Background(), alert, correlationKey)
	if err != nil {
		e.logger.Error("save CVE alert", zap.Error(err))
		return
	}
	if !created {
		return
	}
	alert = persisted
	if e.notifier != nil {
		if err := e.notifier.Send(context.Background(), alert); err != nil {
			e.logger.Warn("send CVE notification", zap.Error(err))
		}
	}
	if e.nats != nil {
		subject := fmt.Sprintf("tenant.%s.alert.%s", tenantID, deviceID)
		data, _ := json.Marshal(alert)
		if err := e.nats.Publish(subject, data); err != nil {
			e.logger.Warn("publish CVE alert", zap.Error(err))
		}
	}
}

func (e *Engine) ResolveCVEAlert(tenantID, deviceID, cveID, packageName string) {
	if tenantID == "" || deviceID == "" || cveID == "" || packageName == "" {
		e.logger.Error("reject CVE resolution without correlation scope")
		return
	}
	alert, err := e.store.ResolveCVEAlert(context.Background(), tenantID, deviceID, "cve:"+deviceID+":"+cveID+":"+packageName, e.now())
	if err != nil {
		e.logger.Error("save CVE resolution", zap.Error(err))
		return
	}
	if e.notifier != nil {
		if err := e.notifier.Send(context.Background(), alert); err != nil {
			e.logger.Warn("send CVE resolution", zap.Error(err))
		}
	}
	if e.nats != nil {
		subject := fmt.Sprintf("tenant.%s.alert.%s", tenantID, deviceID)
		data, _ := json.Marshal(alert)
		if err := e.nats.Publish(subject, data); err != nil {
			e.logger.Warn("publish CVE resolution", zap.Error(err))
		}
	}
}

func (e *Engine) AddRule(ctx context.Context, rule *Rule) error {
	if rule.ID == "" {
		rule.ID = uuid.NewString()
	}
	if err := rule.Validate(); err != nil {
		return err
	}
	if err := e.store.SaveRule(ctx, rule); err != nil {
		return err
	}
	e.mu.Lock()
	e.rules[rule.ID] = rule
	e.mu.Unlock()
	e.logger.Info("rule added", zap.String("rule_id", rule.ID), zap.String("name", rule.Name))
	return nil
}

func (e *Engine) RemoveRule(ctx context.Context, tenantID, ruleID string) error {
	if err := e.store.DeleteRule(ctx, tenantID, ruleID); err != nil {
		return err
	}
	e.mu.Lock()
	delete(e.rules, ruleID)
	for key, state := range e.states {
		if state.RuleID == ruleID {
			delete(e.states, key)
		}
	}
	e.mu.Unlock()
	e.logger.Info("rule removed", zap.String("rule_id", ruleID))
	return nil
}

func (e *Engine) ListRules(ctx context.Context, tenantID string) ([]*Rule, error) {
	return e.store.ListRules(ctx, tenantID)
}

func (e *Engine) GetActiveAlerts(ctx context.Context, tenantID string) ([]*Alert, error) {
	return e.store.GetActiveAlerts(ctx, tenantID)
}

func (e *Engine) GetAlertHistory(ctx context.Context, tenantID string, limit, offset int) ([]*Alert, error) {
	return e.store.GetAlertHistory(ctx, tenantID, limit, offset)
}

func (e *Engine) AcknowledgeAlert(ctx context.Context, tenantID, alertID string) error {
	return e.store.UpdateAlertStatus(ctx, tenantID, alertID, AlertAcknowledged)
}

func (e *Engine) CreateMaintenanceWindow(ctx context.Context, tenantID, name string, startTime, endTime time.Time, deviceIDs []string) (*MaintenanceWindow, error) {
	if e.maintenance == nil {
		return nil, fmt.Errorf("maintenance engine not initialized")
	}
	return e.maintenance.CreateWindow(ctx, tenantID, name, startTime, endTime, deviceIDs)
}

func (e *Engine) DeleteMaintenanceWindow(ctx context.Context, windowID string) error {
	if e.maintenance == nil {
		return fmt.Errorf("maintenance engine not initialized")
	}
	return e.maintenance.DeleteWindow(ctx, windowID)
}

func (e *Engine) ListMaintenanceWindows(ctx context.Context, tenantID string) ([]*MaintenanceWindow, error) {
	if e.maintenance == nil {
		return nil, fmt.Errorf("maintenance engine not initialized")
	}
	return e.maintenance.ListWindows(ctx, tenantID)
}

func (e *Engine) IsDeviceInMaintenance(ctx context.Context, tenantID, deviceID string) (bool, error) {
	if e.maintenance == nil {
		return false, fmt.Errorf("maintenance engine not initialized")
	}
	windows, err := e.maintenance.GetActiveWindows(ctx, tenantID, deviceID)
	if err != nil {
		return false, err
	}
	return len(windows) > 0, nil
}

func (e *Engine) GetMaintenanceWindowsForDevice(ctx context.Context, tenantID, deviceID string) ([]*MaintenanceWindow, error) {
	if e.maintenance == nil {
		return nil, fmt.Errorf("maintenance engine not initialized")
	}
	return e.maintenance.GetActiveWindows(ctx, tenantID, deviceID)
}

func (e *Engine) GroupingEngine() *GroupingEngine {
	return e.grouping
}

func (e *Engine) MaintenanceEngine() *MaintenanceEngine {
	return e.maintenance
}
