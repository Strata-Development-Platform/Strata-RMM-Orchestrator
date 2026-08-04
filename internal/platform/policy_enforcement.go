package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// policyRow represents a policy record from the database.
type policyRow struct {
	ID, Name, Category, ScopeLevel string
	Config                         json.RawMessage
	ClientID, SiteID, DeviceID     *string
	PublishedVersion               int
}

// PolicyEnforcementEngine applies published policies to matching endpoints.
type PolicyEnforcementEngine struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewPolicyEnforcementEngine creates a new policy enforcement engine.
func NewPolicyEnforcementEngine(db *sql.DB, logger *zap.Logger) *PolicyEnforcementEngine {
	return &PolicyEnforcementEngine{db: db, logger: logger}
}

// PolicyAssignment represents a policy assigned to an endpoint.
type PolicyAssignment struct {
	ID          string    `json:"id"`
	PolicyID    string    `json:"policy_id"`
	DeviceID    string    `json:"device_id"`
	PolicyName  string    `json:"policy_name"`
	Category    string    `json:"category"`
	ScopeLevel  string    `json:"scope_level"`
	EffectiveAt time.Time `json:"effective_at"`
}

// ApplyPoliciesToDevices evaluates active policies and assigns them to
// matching devices. It uses the existing policy_assignments table to track
// which devices have which policies.
func (e *PolicyEnforcementEngine) ApplyPoliciesToDevices(ctx context.Context, mspID string) error {
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Set request-scoped tenant ID for RLS
	_, err = tx.ExecContext(ctx, "SET LOCAL app.tenant_id = NULLIF($1, '')", mspID)
	if err != nil {
		return fmt.Errorf("set tenant context: %w", err)
	}

	// Get all active policies for this MSP
	rows, err := tx.QueryContext(ctx, `
		SELECT id, name, category, config, scope_level, client_id, site_id, device_id, published_version
		FROM policies
		WHERE msp_id = $1 AND status = 'active' AND published_version IS NOT NULL
		ORDER BY scope_level, id
	`, mspID)
	if err != nil {
		return fmt.Errorf("query active policies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	policies := []policyRow{}
	for rows.Next() {
		var p policyRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.Config, &p.ScopeLevel,
			&p.ClientID, &p.SiteID, &p.DeviceID, &p.PublishedVersion); err != nil {
			return fmt.Errorf("scan policy: %w", err)
		}
		policies = append(policies, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate policies: %w", err)
	}

	if len(policies) == 0 {
		return tx.Commit()
	}

	// For each policy, find matching devices and assign
	for _, p := range policies {
		assignedDevices, err := e.findMatchingDevices(ctx, tx, mspID, p)
		if err != nil {
			e.logger.Error("finding matching devices", zap.String("policy", p.ID), zap.Error(err))
			continue
		}

		for _, deviceID := range assignedDevices {
			if err := e.assignPolicyToDevice(ctx, tx, p.ID, deviceID, p.Name, p.Category, p.ScopeLevel); err != nil {
				e.logger.Error("assigning policy to device", zap.String("policy", p.ID), zap.String("device", deviceID), zap.Error(err))
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit assignments: %w", err)
	}

	return nil
}

// findMatchingDevices returns device IDs that match the policy scope.
func (e *PolicyEnforcementEngine) findMatchingDevices(ctx context.Context, tx *sql.Tx, mspID string, p policyRow) ([]string, error) {
	switch p.ScopeLevel {
	case "device":
		if p.DeviceID != nil && *p.DeviceID != "" {
			return []string{*p.DeviceID}, nil
		}
		return nil, nil
	case "site":
		if p.SiteID != nil && *p.SiteID != "" {
			return e.findDevicesForSite(ctx, tx, *p.SiteID)
		}
	case "client":
		if p.ClientID != nil && *p.ClientID != "" {
			return e.findDevicesForClient(ctx, tx, mspID, *p.ClientID)
		}
	case "msp", "platform":
		return e.findDevicesForMSP(ctx, tx, mspID)
	}
	return nil, nil
}

func (e *PolicyEnforcementEngine) findDevicesForMSP(ctx context.Context, tx *sql.Tx, mspID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM devices WHERE msp_id = $1 AND is_active = true
	`, mspID)
	if err != nil {
		return nil, fmt.Errorf("query devices for MSP: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var devices []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return devices, nil
}

func (e *PolicyEnforcementEngine) findDevicesForClient(ctx context.Context, tx *sql.Tx, mspID, clientID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT d.id FROM devices d
		JOIN client_organizations c ON c.id = d.client_id
		WHERE d.client_id = $1 AND c.msp_id = $2 AND d.is_active = true
	`, clientID, mspID)
	if err != nil {
		return nil, fmt.Errorf("query devices for client: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var devices []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return devices, nil
}

func (e *PolicyEnforcementEngine) findDevicesForSite(ctx context.Context, tx *sql.Tx, siteID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM devices WHERE site_id = $1 AND is_active = true
	`, siteID)
	if err != nil {
		return nil, fmt.Errorf("query devices for site: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var devices []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return devices, nil
}

// assignPolicyToDevice assigns a policy to a device.
func (e *PolicyEnforcementEngine) assignPolicyToDevice(ctx context.Context, tx *sql.Tx, policyID, deviceID, policyName, category, scopeLevel string) error {
	// Upsert the assignment
	_, err := tx.ExecContext(ctx, `
		INSERT INTO policy_assignments (id, policy_id, device_id, effective_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (id) DO NOTHING
	`, uuid.New().String(), policyID, deviceID)
	if err != nil {
		return fmt.Errorf("insert assignment: %w", err)
	}
	return nil
}

// GetDeviceAssignments returns all policies assigned to a device.
func (e *PolicyEnforcementEngine) GetDeviceAssignments(ctx context.Context, deviceID string) ([]PolicyAssignment, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT pa.id, pa.policy_id, p.name, p.category, p.scope_level, pa.effective_at
		FROM policy_assignments pa
		JOIN policies p ON p.id = pa.policy_id
		WHERE pa.device_id = $1 AND p.status = 'active'
		ORDER BY pa.effective_at DESC
	`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("query device assignments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var assignments []PolicyAssignment
	for rows.Next() {
		var a PolicyAssignment
		if err := rows.Scan(&a.ID, &a.PolicyID, &a.PolicyName, &a.Category, &a.ScopeLevel, &a.EffectiveAt); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		assignments = append(assignments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assignments: %w", err)
	}
	return assignments, nil
}
