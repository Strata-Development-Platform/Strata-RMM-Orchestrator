package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// Logger is the logging interface for the role policy binder.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
}

// RolePolicyBinder manages automatic binding of device roles to monitoring definitions
// and creates alert rules when new roles are detected on devices.
type RolePolicyBinder struct {
	db     *sql.DB
	js     nats.JetStreamContext
	logger Logger
}

// RolePolicyBinding tracks a device role to monitoring definition binding.
type RolePolicyBinding struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"device_id"`
	TenantID  string    `json:"tenant_id"`
	Role      string    `json:"role"`
	BoundRules []string `json:"bound_rules"`
	BoundAt   time.Time `json:"bound_at"`
}

// MonitorEvent represents a role detection event from telemetry.
type MonitorEvent struct {
	AgentID  string   `json:"agent_id"`
	TenantID string   `json:"tenant_id"`
	DeviceID string   `json:"device_id"`
	Roles    []string `json:"roles"`
	Timestamp time.Time `json:"timestamp"`
}

// NewRolePolicyBinder creates a new binder.
func NewRolePolicyBinder(db *sql.DB, js nats.JetStreamContext, logger Logger) *RolePolicyBinder {
	return &RolePolicyBinder{
		db:     db,
		js:     js,
		logger: logger,
	}
}

// ProcessMonitorEvent processes a role detection event from telemetry check-in.
// It binds detected roles to monitoring definitions and creates alert rules.
func (b *RolePolicyBinder) ProcessMonitorEvent(ctx context.Context, event MonitorEvent) error {
	if len(event.Roles) == 0 {
		return nil // No roles to bind
	}

	// Bind each detected role to matching monitoring definitions
	for _, role := range event.Roles {
		if err := b.bindRoleToDevice(ctx, event.DeviceID, event.TenantID, role); err != nil {
			b.logger.Error("binding role to device",
				"device_id", event.DeviceID,
				"tenant_id", event.TenantID,
				"role", role,
				"error", err,
			)
		}
	}

	return nil
}

// bindRoleToDevice binds a device to a role, finding matching monitoring definitions
// and creating alert rules if they don't already exist.
func (b *RolePolicyBinder) bindRoleToDevice(ctx context.Context, deviceID, tenantID, role string) error {
	if deviceID == "" || tenantID == "" || role == "" {
		return fmt.Errorf("empty device_id, tenant_id, or role")
	}

	// Find matching monitoring definitions for this role
	definitions, err := b.findDefinitionsForRole(ctx, tenantID, role)
	if err != nil {
		return fmt.Errorf("find definitions: %w", err)
	}

	if len(definitions) == 0 {
		b.logger.Info("no monitoring definitions for role", "role", role, "tenant", tenantID)
		return nil
	}

	// Extract definition IDs
	defIDs := make([]string, len(definitions))
	for i, d := range definitions {
		defIDs[i] = d.ID
	}

	// Upsert device_role_bindings
	boundRules := make([]string, len(defIDs))
	copy(boundRules, defIDs)
	if err := b.upsertDeviceRoleBinding(ctx, deviceID, tenantID, role, boundRules); err != nil {
		return fmt.Errorf("upsert binding: %w", err)
	}

	// Create alert rules for each matching definition
	for _, def := range definitions {
		if err := b.ensureAlertRuleExists(ctx, deviceID, tenantID, def); err != nil {
			b.logger.Error("ensure alert rule", "role", role, "definition_id", def.ID, "error", err)
		}
	}

	return nil
}

// findDefinitionsForRole returns monitoring definitions that match the given role.
func (b *RolePolicyBinder) findDefinitionsForRole(ctx context.Context, tenantID, role string) ([]*MonitoringDefinition, error) {
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, description, roles, rule_type, metric_name, condition,
			threshold, timeout, severity, cooldown, enabled, created_at, updated_at
		FROM monitoring_definitions
		WHERE tenant_id = $1 AND enabled = true AND roles @> ARRAY[$2::text]
	`, tenantID, role)
	if err != nil {
		return nil, fmt.Errorf("query definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var defs []*MonitoringDefinition
	for rows.Next() {
		var d MonitoringDefinition
		var timeout, cooldown string
		if err := rows.Scan(
			&d.ID, &d.TenantID, &d.Name, &d.Description, &d.Roles,
			&d.RuleType, &d.MetricName, &d.Condition, &d.Threshold,
			&timeout, &d.Severity, &cooldown, &d.Enabled,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan definition: %w", err)
		}
		defs = append(defs, &d)
	}

	return defs, nil
}

// upsertDeviceRoleBinding creates or updates a device role binding record.
func (b *RolePolicyBinder) upsertDeviceRoleBinding(ctx context.Context, deviceID, tenantID, role string, boundRules []string) error {
	_, err := b.db.ExecContext(ctx, `
		INSERT INTO device_role_bindings (device_id, tenant_id, role, bound_rules, bound_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (device_id, role)
		DO UPDATE SET bound_rules = EXCLUDED.bound_rules, updated_at = NOW()
	`, deviceID, tenantID, role, boundRules)
	if err != nil {
		return fmt.Errorf("upsert binding: %w", err)
	}
	return nil
}

// MonitoringDefinition represents a monitoring definition from the database.
type MonitoringDefinition struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Roles       []string  `json:"roles"`
	RuleType    string    `json:"rule_type"`
	MetricName  string    `json:"metric_name"`
	Condition   string    `json:"condition"`
	Threshold   float64   `json:"threshold"`
	Severity    string    `json:"severity"`
	Cooldown    string    `json:"cooldown"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ensureAlertRuleExists creates an alert rule in the database for a monitoring definition
// if it doesn't already exist for this device.
func (b *RolePolicyBinder) ensureAlertRuleExists(ctx context.Context, deviceID, tenantID string, def *MonitoringDefinition) error {
	// Check if an alert rule already exists for this device + metric_name
	var count int
	err := b.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM alert_rules
		WHERE tenant_id = $1 AND device_id = $2 AND metric_name = $3
	`, tenantID, deviceID, def.MetricName).Scan(&count)
	if err != nil {
		return fmt.Errorf("check alert rule: %w", err)
	}

	if count > 0 {
		return nil // Rule already exists
	}

	// Create the alert rule
	_, err = b.db.ExecContext(ctx, `
		INSERT INTO alert_rules (tenant_id, device_id, name, type, metric_name, condition, threshold, severity, cooldown, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		tenantID,
		deviceID,
		def.Name,
		def.RuleType,
		def.MetricName,
		def.Condition,
		def.Threshold,
		def.Severity,
		def.Cooldown,
		def.Enabled,
	)
	if err != nil {
		return fmt.Errorf("create alert rule: %w", err)
	}

	b.logger.Info("auto-created alert rule for role",
		"device_id", deviceID,
		"role", def.Roles[0],
		"rule_name", def.Name,
	)

	return nil
}
