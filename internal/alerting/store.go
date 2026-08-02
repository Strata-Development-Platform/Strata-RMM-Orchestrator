package alerting

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) LoadRules(ctx context.Context) ([]*Rule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, type, enabled, severity,
		       COALESCE(metric_name, ''), COALESCE(condition, ''), COALESCE(threshold, 0),
		       COALESCE(EXTRACT(EPOCH FROM timeout), 0), COALESCE(device_id::text, ''),
		       COALESCE(EXTRACT(EPOCH FROM cooldown), 0),
		       COALESCE(channels, '[]'::jsonb), COALESCE(template, ''),
		       created_at, updated_at
		FROM alert_rules
	`)
	if err != nil {
		return nil, fmt.Errorf("query rules: %w", err)
	}
	defer rows.Close()

	var rules []*Rule
	for rows.Next() {
		var r Rule
		var channelsJSON []byte
		var timeoutSecs, cooldownSecs float64

		err := rows.Scan(
			&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Enabled, &r.Severity,
			&r.MetricName, &r.Condition, &r.Threshold,
			&timeoutSecs, &r.DeviceID,
			&cooldownSecs,
			&channelsJSON, &r.Template,
			&r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}

		r.Timeout = time.Duration(timeoutSecs) * time.Second
		r.Cooldown = time.Duration(cooldownSecs) * time.Second
		if err := json.Unmarshal(channelsJSON, &r.Channels); err != nil {
			return nil, fmt.Errorf("decode rule channels: %w", err)
		}

		rules = append(rules, &r)
	}
	return rules, nil
}

func (s *Store) SaveRule(ctx context.Context, rule *Rule) error {
	now := time.Now()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now

	channelsJSON, err := json.Marshal(rule.Channels)
	if err != nil {
		return fmt.Errorf("encode rule channels: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_rules (id, tenant_id, name, type, enabled, severity, metric_name, condition, threshold, timeout, device_id, cooldown, channels, template, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, make_interval(secs => $10),
		        NULLIF($11, '')::uuid, make_interval(secs => $12), $13, $14, $15, $16)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, type = EXCLUDED.type, enabled = EXCLUDED.enabled, severity = EXCLUDED.severity,
			metric_name = EXCLUDED.metric_name, condition = EXCLUDED.condition,
			threshold = EXCLUDED.threshold, timeout = EXCLUDED.timeout,
			device_id = EXCLUDED.device_id, cooldown = EXCLUDED.cooldown,
			channels = EXCLUDED.channels, template = EXCLUDED.template,
			updated_at = EXCLUDED.updated_at
		WHERE alert_rules.tenant_id = EXCLUDED.tenant_id
	`, rule.ID, rule.TenantID, rule.Name, rule.Type, rule.Enabled, rule.Severity,
		rule.MetricName, rule.Condition, rule.Threshold,
		int64(rule.Timeout.Seconds()), rule.DeviceID, int64(rule.Cooldown.Seconds()),
		channelsJSON, rule.Template, rule.CreatedAt, rule.UpdatedAt,
	)
	return requireAffected(err, result, "alert rule")
}

func (s *Store) DeleteRule(ctx context.Context, tenantID, ruleID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE tenant_id = $1 AND id = $2`, tenantID, ruleID)
	return requireAffected(err, result, "alert rule")
}

func (s *Store) ListRules(ctx context.Context, tenantID string) ([]*Rule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, type, enabled, severity,
		       COALESCE(metric_name, ''), COALESCE(condition, ''), COALESCE(threshold, 0),
		       COALESCE(EXTRACT(EPOCH FROM timeout), 0), COALESCE(device_id::text, ''),
		       COALESCE(EXTRACT(EPOCH FROM cooldown), 0), COALESCE(channels, '[]'::jsonb), COALESCE(template, ''),
		       created_at, updated_at
		FROM alert_rules WHERE tenant_id = $1 ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*Rule
	for rows.Next() {
		var r Rule
		var channelsJSON []byte
		var timeoutSecs, cooldownSecs float64
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Enabled, &r.Severity,
			&r.MetricName, &r.Condition, &r.Threshold, &timeoutSecs, &r.DeviceID,
			&cooldownSecs, &channelsJSON, &r.Template, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Timeout = time.Duration(timeoutSecs) * time.Second
		r.Cooldown = time.Duration(cooldownSecs) * time.Second
		if err := json.Unmarshal(channelsJSON, &r.Channels); err != nil {
			return nil, fmt.Errorf("decode rule channels: %w", err)
		}
		rules = append(rules, &r)
	}
	return rules, nil
}

func (s *Store) SaveAlert(ctx context.Context, alert *Alert) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alerts (id, rule_id, tenant_id, device_id, metric_name, value, severity, message, status, fired_at, resolved_at, acknowledged_at, correlation_key)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULLIF($13, ''))
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status, resolved_at = EXCLUDED.resolved_at, acknowledged_at = EXCLUDED.acknowledged_at
	`, alert.ID, alert.RuleID, alert.TenantID, alert.DeviceID, alert.MetricName, alert.Value,
		alert.Severity, alert.Message, alert.Status, alert.FiredAt, alert.ResolvedAt, alert.AcknowledgedAt, alert.CorrelationKey)
	return err
}

func (s *Store) SaveCVEAlert(ctx context.Context, alert *Alert, correlationKey string) (*Alert, bool, error) {
	alert.CorrelationKey = correlationKey
	var persisted Alert
	var created bool
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO alerts (id, rule_id, tenant_id, device_id, metric_name, value, severity, message, status, fired_at, correlation_key)
		VALUES ($1, NULL, $2, $3, $4, $5, $6, $7, 'firing', $8, $9)
		ON CONFLICT (tenant_id, correlation_key) WHERE correlation_key IS NOT NULL AND status IN ('firing', 'acknowledged')
		DO UPDATE SET value = EXCLUDED.value
		RETURNING id, tenant_id, device_id, COALESCE(metric_name, ''), COALESCE(value, 0),
		          severity, message, status, fired_at, resolved_at, acknowledged_at,
		          correlation_key, (xmax = 0)
	`, alert.ID, alert.TenantID, alert.DeviceID, alert.MetricName, alert.Value,
		alert.Severity, alert.Message, alert.FiredAt, correlationKey).Scan(
		&persisted.ID, &persisted.TenantID, &persisted.DeviceID, &persisted.MetricName,
		&persisted.Value, &persisted.Severity, &persisted.Message, &persisted.Status,
		&persisted.FiredAt, &persisted.ResolvedAt, &persisted.AcknowledgedAt,
		&persisted.CorrelationKey, &created,
	)
	if err != nil {
		return nil, false, err
	}
	return &persisted, created, nil
}

func (s *Store) ResolveCVEAlert(ctx context.Context, tenantID, deviceID, correlationKey string, now time.Time) (*Alert, error) {
	var alert Alert
	err := s.db.QueryRowContext(ctx, `
		UPDATE alerts
		SET status = 'resolved', resolved_at = $1,
		    message = 'Resolved: ' || message
		WHERE tenant_id = $2 AND device_id = $3 AND correlation_key = $4
		  AND status IN ('firing', 'acknowledged')
		RETURNING id, tenant_id, device_id, COALESCE(metric_name, ''), COALESCE(value, 0),
		          severity, message, status, fired_at, resolved_at, acknowledged_at, correlation_key
	`, now, tenantID, deviceID, correlationKey).Scan(
		&alert.ID, &alert.TenantID, &alert.DeviceID, &alert.MetricName, &alert.Value,
		&alert.Severity, &alert.Message, &alert.Status, &alert.FiredAt,
		&alert.ResolvedAt, &alert.AcknowledgedAt, &alert.CorrelationKey,
	)
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

func (s *Store) GetActiveAlerts(ctx context.Context, tenantID string) ([]*Alert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(rule_id::text, ''), tenant_id, COALESCE(device_id, ''), COALESCE(metric_name, ''),
		       COALESCE(value, 0), severity, message, status, fired_at, resolved_at, acknowledged_at,
		       COALESCE(correlation_key, '')
		FROM alerts
		WHERE tenant_id = $1 AND status IN ('firing', 'acknowledged')
		ORDER BY fired_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.RuleID, &a.TenantID, &a.DeviceID, &a.MetricName,
			&a.Value, &a.Severity, &a.Message, &a.Status, &a.FiredAt, &a.ResolvedAt, &a.AcknowledgedAt,
			&a.CorrelationKey); err != nil {
			return nil, err
		}
		alerts = append(alerts, &a)
	}
	return alerts, nil
}

func (s *Store) LoadActiveAlertStates(ctx context.Context) ([]*Alert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(rule_id::text, ''), tenant_id, COALESCE(device_id, ''),
		       COALESCE(metric_name, ''), status, fired_at, COALESCE(correlation_key, '')
		FROM alerts
		WHERE status IN ('firing', 'acknowledged') AND correlation_key LIKE 'rule:%'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var alerts []*Alert
	for rows.Next() {
		var alert Alert
		if err := rows.Scan(&alert.ID, &alert.RuleID, &alert.TenantID, &alert.DeviceID,
			&alert.MetricName, &alert.Status, &alert.FiredAt, &alert.CorrelationKey); err != nil {
			return nil, err
		}
		alerts = append(alerts, &alert)
	}
	return alerts, rows.Err()
}

func (s *Store) GetAlertHistory(ctx context.Context, tenantID string, limit, offset int) ([]*Alert, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(rule_id::text, ''), tenant_id, COALESCE(device_id, ''), COALESCE(metric_name, ''),
		       COALESCE(value, 0), severity, message, status, fired_at, resolved_at, acknowledged_at
		FROM alerts
		WHERE tenant_id = $1
		ORDER BY fired_at DESC LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.RuleID, &a.TenantID, &a.DeviceID, &a.MetricName,
			&a.Value, &a.Severity, &a.Message, &a.Status, &a.FiredAt, &a.ResolvedAt, &a.AcknowledgedAt); err != nil {
			return nil, err
		}
		alerts = append(alerts, &a)
	}
	return alerts, nil
}

func (s *Store) UpdateAlertStatus(ctx context.Context, tenantID, alertID string, status AlertStatus) error {
	now := time.Now()
	var q string
	if status == AlertAcknowledged {
		q = `UPDATE alerts SET status = $1, acknowledged_at = $2 WHERE tenant_id = $3 AND id = $4`
	} else {
		q = `UPDATE alerts SET status = $1 WHERE tenant_id = $2 AND id = $3`
		now = time.Time{}
	}
	var result sql.Result
	var err error
	if status == AlertAcknowledged {
		result, err = s.db.ExecContext(ctx, q, status, now, tenantID, alertID)
	} else {
		result, err = s.db.ExecContext(ctx, q, status, tenantID, alertID)
	}
	return requireAffected(err, result, "alert")
}

func requireAffected(err error, result sql.Result, resource string) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%s not found", resource)
	}
	return nil
}
