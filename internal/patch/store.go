package patch

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) LoadPolicies(ctx context.Context) ([]*PatchPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, enabled, platforms, approval_mode, severity,
		       maintenance_window, device_filter, max_retries, created_at, updated_at
		FROM patch_policies
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*PatchPolicy
	for rows.Next() {
		var p PatchPolicy
		var platformsJSON, filterJSON []byte
		err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Enabled, &platformsJSON,
			&p.ApprovalMode, &p.Severity, &p.MaintenanceWin, &filterJSON,
			&p.MaxRetries, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return nil, err
		}
		json.Unmarshal(platformsJSON, &p.Platforms)
		json.Unmarshal(filterJSON, &p.DeviceFilter)
		policies = append(policies, &p)
	}
	return policies, nil
}

func (s *Store) SavePolicy(ctx context.Context, policy *PatchPolicy) error {
	platformsJSON, _ := json.Marshal(policy.Platforms)
	filterJSON, _ := json.Marshal(policy.DeviceFilter)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO patch_policies (id, tenant_id, name, enabled, platforms, approval_mode, severity, maintenance_window, device_filter, max_retries, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, enabled = EXCLUDED.enabled, platforms = EXCLUDED.platforms,
			approval_mode = EXCLUDED.approval_mode, severity = EXCLUDED.severity,
			maintenance_window = EXCLUDED.maintenance_window, device_filter = EXCLUDED.device_filter,
			max_retries = EXCLUDED.max_retries, updated_at = EXCLUDED.updated_at
	`, policy.ID, policy.TenantID, policy.Name, policy.Enabled, platformsJSON,
		policy.ApprovalMode, policy.Severity, policy.MaintenanceWin, filterJSON,
		policy.MaxRetries, policy.CreatedAt, policy.UpdatedAt)
	return err
}

func (s *Store) ListPolicies(ctx context.Context, tenantID string) ([]*PatchPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, enabled, platforms, approval_mode, severity,
		       maintenance_window, device_filter, max_retries, created_at, updated_at
		FROM patch_policies WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*PatchPolicy
	for rows.Next() {
		var p PatchPolicy
		var platformsJSON, filterJSON []byte
		rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Enabled, &platformsJSON,
			&p.ApprovalMode, &p.Severity, &p.MaintenanceWin, &filterJSON,
			&p.MaxRetries, &p.CreatedAt, &p.UpdatedAt)
		json.Unmarshal(platformsJSON, &p.Platforms)
		json.Unmarshal(filterJSON, &p.DeviceFilter)
		policies = append(policies, &p)
	}
	return policies, nil
}

func (s *Store) DeletePolicy(ctx context.Context, policyID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM patch_policies WHERE id = $1`, policyID)
	return err
}

func (s *Store) GetPendingDeployments(ctx context.Context) ([]*Deployment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, policy_id, tenant_id, status, device_count, installed, failed, pending,
		       scheduled_for, started_at, completed_at, created_at
		FROM patch_deployments
		WHERE status = 'pending' AND scheduled_for <= NOW()
		ORDER BY scheduled_for ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []*Deployment
	for rows.Next() {
		var d Deployment
		rows.Scan(&d.ID, &d.PolicyID, &d.TenantID, &d.Status, &d.DeviceCount,
			&d.Installed, &d.Failed, &d.Pending, &d.ScheduledFor,
			&d.StartedAt, &d.CompletedAt, &d.CreatedAt)
		deps = append(deps, &d)
	}
	return deps, nil
}

func (s *Store) UpdateDeployment(ctx context.Context, dep *Deployment) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE patch_deployments
		SET status = $1, installed = $2, failed = $3, pending = $4,
		    started_at = $5, completed_at = $6
		WHERE id = $7
	`, dep.Status, dep.Installed, dep.Failed, dep.Pending,
		dep.StartedAt, dep.CompletedAt, dep.ID)
	return err
}

func (s *Store) GetDeploymentDevices(ctx context.Context, deploymentID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT device_id FROM patch_deployment_devices WHERE deployment_id = $1
	`, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []string
	for rows.Next() {
		var d string
		rows.Scan(&d)
		devices = append(devices, d)
	}
	return devices, nil
}

func (s *Store) CreateDeployment(ctx context.Context, dep *Deployment, deviceIDs []string) error {
	if dep.CreatedAt.IsZero() {
		dep.CreatedAt = time.Now()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO patch_deployments (id, policy_id, tenant_id, status, device_count, scheduled_for, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, dep.ID, dep.PolicyID, dep.TenantID, StatusPending, len(deviceIDs), dep.ScheduledFor, dep.CreatedAt)
	if err != nil {
		return err
	}

	for _, deviceID := range deviceIDs {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO patch_deployment_devices (deployment_id, device_id)
			VALUES ($1, $2)
		`, dep.ID, deviceID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ListDeployments(ctx context.Context, tenantID string) ([]*Deployment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, policy_id, tenant_id, status, device_count, installed, failed, pending,
		       scheduled_for, started_at, completed_at, created_at
		FROM patch_deployments WHERE tenant_id = $1 ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []*Deployment
	for rows.Next() {
		var d Deployment
		rows.Scan(&d.ID, &d.PolicyID, &d.TenantID, &d.Status, &d.DeviceCount,
			&d.Installed, &d.Failed, &d.Pending, &d.ScheduledFor,
			&d.StartedAt, &d.CompletedAt, &d.CreatedAt)
		deps = append(deps, &d)
	}
	return deps, nil
}

func (s *Store) UpdateDeviceState(ctx context.Context, state *DevicePatchState) error {
	state.UpdatedAt = time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO patch_device_states (device_id, deployment_id, patch_id, status, attempts, error, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (deployment_id, device_id, patch_id) DO UPDATE SET
			status = EXCLUDED.status, attempts = patch_device_states.attempts + 1,
			error = EXCLUDED.error, updated_at = EXCLUDED.updated_at
	`, state.DeviceID, state.DeploymentID, state.PatchID, state.Status, state.Attempts, state.Error, state.UpdatedAt)
	return err
}

func (s *Store) SaveInventory(ctx context.Context, tenantID, deviceID string, installed, missing []*Patch) error {
	data, _ := json.Marshal(map[string]interface{}{
		"installed": installed,
		"missing":   missing,
	})
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO patch_inventory (tenant_id, device_id, snapshot, created_at)
		VALUES ($1, $2, $3, NOW())
	`, tenantID, deviceID, data)
	return err
}

func (s *Store) GetInventory(ctx context.Context, tenantID, deviceID string) ([]*Patch, []*Patch, error) {
	var snapshotJSON []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT snapshot FROM patch_inventory
		WHERE tenant_id = $1 AND device_id = $2
		ORDER BY created_at DESC LIMIT 1
	`, tenantID, deviceID).Scan(&snapshotJSON)
	if err != nil {
		return nil, nil, err
	}
	var snap struct {
		Installed []*Patch `json:"installed"`
		Missing   []*Patch `json:"missing"`
	}
	json.Unmarshal(snapshotJSON, &snap)
	return snap.Installed, snap.Missing, nil
}

func (s *Store) CountDevices(ctx context.Context, tenantID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE tenant_id = $1`, tenantID).Scan(&count)
	return count, err
}
