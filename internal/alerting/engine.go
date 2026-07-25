package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type Engine struct {
	nats      *nats.Conn
	tsdb      *timescale.Client
	logger    *zap.Logger
	store     *Store
	notifier  *Notifier

	mu       sync.RWMutex
	rules    map[string]*Rule // ruleID -> Rule
	states   map[string]*AlertState // ruleID+deviceID -> state
	subs     []*nats.Subscription
}

func NewEngine(nc *nats.Conn, tsdb *timescale.Client, store *Store, notifier *Notifier, logger *zap.Logger) *Engine {
	return &Engine{
		nats:     nc,
		tsdb:     tsdb,
		logger:   logger,
		store:    store,
		notifier: notifier,
		rules:    make(map[string]*Rule),
		states:   make(map[string]*AlertState),
	}
}

func (e *Engine) Start(ctx context.Context) error {
	e.logger.Info("starting alerting engine")

	rules, err := e.store.LoadRules(ctx)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	e.mu.Lock()
	for _, r := range rules {
		e.rules[r.ID] = r
	}
	e.mu.Unlock()
	e.logger.Info("rules loaded", zap.Int("count", len(rules)))

	metricsSub, err := e.nats.Subscribe("tenant.>.agent.>.metrics", e.handleMetrics)
	if err != nil {
		return fmt.Errorf("subscribe metrics: %w", err)
	}
	e.subs = append(e.subs, metricsSub)

	heartbeatSub, err := e.nats.Subscribe("tenant.>.agent.>.heartbeat", e.handleHeartbeat)
	if err != nil {
		return fmt.Errorf("subscribe heartbeat: %w", err)
	}
	e.subs = append(e.subs, heartbeatSub)

	go e.evaluationLoop(ctx)
	go e.cleanupLoop(ctx)

	return nil
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
	defer e.mu.RUnlock()

	for _, r := range e.rules {
		if r.TenantID != payload.TenantID || r.Type != RuleTypeThreshold {
			continue
		}
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
	defer e.mu.RUnlock()

	for _, r := range e.rules {
		if r.TenantID != payload.TenantID || r.Type != RuleTypeHeartbeat {
			continue
		}
		if r.DeviceID == "" || r.DeviceID == payload.DeviceID {
			e.evaluateHeartbeat(r, payload.TenantID, payload.DeviceID, time.Unix(0, payload.Time))
		}
	}
}

func (e *Engine) evaluateThreshold(rule *Rule, tenantID, deviceID, metricName string, value float64, timestamp int64) {
	key := rule.ID + ":" + deviceID + ":" + metricName
	e.mu.Lock()
	state, exists := e.states[key]
	if !exists {
		state = &AlertState{
			RuleID:   rule.ID,
			TenantID: tenantID,
			DeviceID: deviceID,
			State:    StateOK,
		}
		e.states[key] = state
	}
	e.mu.Unlock()

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

	now := time.Now()
	if shouldFire && state.State == StateOK {
		if now.Sub(state.LastFired) < rule.Cooldown {
			return
		}
		alert := &Alert{
			ID:         fmt.Sprintf("%s-%s-%s-%d", rule.ID, deviceID, metricName, now.UnixNano()),
			RuleID:     rule.ID,
			TenantID:   tenantID,
			DeviceID:   deviceID,
			MetricName: metricName,
			Value:      value,
			Severity:   rule.Severity,
			Message:    rule.FormatMessage(deviceID, metricName, value),
			Status:     AlertFiring,
			FiredAt:    now,
		}
		if err := e.fireAlert(alert); err != nil {
			e.logger.Error("fire alert", zap.Error(err))
			return
		}
		state.State = StateFiring
		state.LastFired = now
		state.ConsecutiveFires++
	} else if !shouldFire && state.State == StateFiring {
		alert := &Alert{
			ID:       fmt.Sprintf("%s-%s-%s-%d", rule.ID, deviceID, metricName, now.UnixNano()),
			RuleID:   rule.ID,
			TenantID: tenantID,
			DeviceID: deviceID,
			Message:  fmt.Sprintf("Resolved: %s", rule.FormatMessage(deviceID, metricName, value)),
			Status:   AlertResolved,
			ResolvedAt: &now,
		}
		if err := e.resolveAlert(alert); err != nil {
			e.logger.Error("resolve alert", zap.Error(err))
			return
		}
		state.State = StateOK
		state.ConsecutiveFires = 0
	} else if shouldFire && state.State == StateFiring {
		state.ConsecutiveFires++
	}
}

func (e *Engine) evaluateHeartbeat(rule *Rule, tenantID, deviceID string, lastTime time.Time) {
	key := rule.ID + ":" + deviceID
	e.mu.Lock()
	state, exists := e.states[key]
	if !exists {
		state = &AlertState{
			RuleID:     rule.ID,
			TenantID:   tenantID,
			DeviceID:   deviceID,
			State:      StateOK,
			LastHeard:  lastTime,
		}
		e.states[key] = state
	} else {
		state.LastHeard = lastTime
	}
	e.mu.Unlock()

	if state.State == StateFiring {
		now := time.Now()
		alert := &Alert{
			ID:         fmt.Sprintf("%s-%s-%d", rule.ID, deviceID, now.UnixNano()),
			RuleID:     rule.ID,
			TenantID:   tenantID,
			DeviceID:   deviceID,
			Message:    fmt.Sprintf("Heartbeat restored for device %s", deviceID),
			Status:     AlertResolved,
			ResolvedAt: &now,
		}
		if err := e.resolveAlert(alert); err != nil {
			e.logger.Error("resolve heartbeat alert", zap.Error(err))
		}
	}
}

func (e *Engine) fireAlert(alert *Alert) error {
	if err := e.store.SaveAlert(context.Background(), alert); err != nil {
		return fmt.Errorf("save alert: %w", err)
	}
	if err := e.notifier.Send(context.Background(), alert); err != nil {
		return fmt.Errorf("send notification: %w", err)
	}

	subject := fmt.Sprintf("tenant.%s.alert.%s", alert.TenantID, alert.DeviceID)
	data, _ := json.Marshal(alert)
	if err := e.nats.Publish(subject, data); err != nil {
		e.logger.Warn("publish alert", zap.Error(err))
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

	subject := fmt.Sprintf("tenant.%s.alert.%s", alert.TenantID, alert.DeviceID)
	data, _ := json.Marshal(alert)
	if err := e.nats.Publish(subject, data); err != nil {
		e.logger.Warn("publish alert resolution", zap.Error(err))
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
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now()
	for _, state := range e.states {
		if state.State == StateFiring {
			continue
		}
		rule, ok := e.rules[state.RuleID]
		if !ok || rule.Type != RuleTypeHeartbeat {
			continue
		}
		if state.LastHeard.IsZero() {
			continue
		}
		sinceLast := now.Sub(state.LastHeard)
		if sinceLast > rule.Timeout {
			e.mu.RUnlock()
			alert := &Alert{
				ID:       fmt.Sprintf("%s-%s-%d", rule.ID, state.DeviceID, now.UnixNano()),
				RuleID:   rule.ID,
				TenantID: state.TenantID,
				DeviceID: state.DeviceID,
				Severity: rule.Severity,
				Message:  fmt.Sprintf("Device %s has not reported for %v (timeout: %v)", state.DeviceID, sinceLast.Round(time.Second), rule.Timeout),
				Status:   AlertFiring,
				FiredAt:  now,
			}
			e.mu.RLock()
			state.State = StateFiring
			state.LastFired = now

			if err := e.fireAlert(alert); err != nil {
				e.logger.Error("fire heartbeat alert", zap.Error(err))
			}
			e.mu.RUnlock()
			e.mu.RLock()
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
	alert := &Alert{
		ID:         fmt.Sprintf("cve-%s-%s-%d", cveID, deviceID, time.Now().UnixNano()),
		RuleID:     "cve-detector",
		TenantID:   tenantID,
		DeviceID:   deviceID,
		MetricName: "cve_count",
		Severity:   Severity(severity),
		Message:    fmt.Sprintf("[%s] %s in %s %s — upgrade to %s on device %s", strings.ToUpper(severity), cveID, packageName, currentVersion, fixedVersion, deviceID),
		Status:     AlertFiring,
		FiredAt:    time.Now(),
	}
	e.logger.Warn("CVE alert fired",
		zap.String("cve", cveID),
		zap.String("device", deviceID),
		zap.String("package", packageName),
		zap.String("severity", severity),
	)
	if err := e.store.SaveAlert(context.Background(), alert); err != nil {
		e.logger.Error("save CVE alert", zap.Error(err))
		return
	}
	if err := e.notifier.Send(context.Background(), alert); err != nil {
		e.logger.Warn("send CVE notification", zap.Error(err))
	}
	subject := fmt.Sprintf("tenant.%s.alert.%s", tenantID, deviceID)
	data, _ := json.Marshal(alert)
	if err := e.nats.Publish(subject, data); err != nil {
		e.logger.Warn("publish CVE alert", zap.Error(err))
	}
}

func (e *Engine) ResolveCVEAlert(deviceID, cveID string) {
	now := time.Now()
	alert := &Alert{
		ID:         fmt.Sprintf("cve-%s-%s-%d", cveID, deviceID, now.UnixNano()),
		RuleID:     "cve-detector",
		DeviceID:   deviceID,
		Message:    fmt.Sprintf("Resolved: %s on device %s — package patched", cveID, deviceID),
		Status:     AlertResolved,
		ResolvedAt: &now,
	}
	if err := e.store.SaveAlert(context.Background(), alert); err != nil {
		e.logger.Error("save CVE resolution", zap.Error(err))
	}
	subject := fmt.Sprintf("tenant.%s.alert.%s", "", deviceID)
	data, _ := json.Marshal(alert)
	if err := e.nats.Publish(subject, data); err != nil {
		e.logger.Warn("publish CVE resolution", zap.Error(err))
	}
}

func (e *Engine) AddRule(ctx context.Context, rule *Rule) error {
	if err := e.store.SaveRule(ctx, rule); err != nil {
		return err
	}
	e.mu.Lock()
	e.rules[rule.ID] = rule
	e.mu.Unlock()
	e.logger.Info("rule added", zap.String("rule_id", rule.ID), zap.String("name", rule.Name))
	return nil
}

func (e *Engine) RemoveRule(ctx context.Context, ruleID string) error {
	if err := e.store.DeleteRule(ctx, ruleID); err != nil {
		return err
	}
	e.mu.Lock()
	delete(e.rules, ruleID)
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

func (e *Engine) AcknowledgeAlert(ctx context.Context, alertID string) error {
	return e.store.UpdateAlertStatus(ctx, alertID, AlertAcknowledged)
}
