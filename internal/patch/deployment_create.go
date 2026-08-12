package patch

import (
	"context"
	"errors"
	"strings"
	"time"
)

// CreateDeploymentWithPatches persists the target devices and the exact patch
// IDs approved for the deployment in one transaction. An empty selection is
// rejected so rollout can never degrade into an implicit "install all" action.
func (s *Store) CreateDeploymentWithPatches(ctx context.Context, dep *Deployment, deviceIDs, patchIDs []string) error {
	if s == nil || s.db == nil {
		return errors.New("patch store database is required")
	}
	if dep == nil || dep.ID == "" || dep.PolicyID == "" || dep.TenantID == "" {
		return errors.New("patch deployment identity is incomplete")
	}
	if len(deviceIDs) == 0 {
		return errors.New("patch deployment requires at least one device")
	}
	cleanPatches := make([]string, 0, len(patchIDs))
	seen := make(map[string]struct{}, len(patchIDs))
	for _, patchID := range patchIDs {
		patchID = strings.TrimSpace(patchID)
		if patchID == "" {
			continue
		}
		if _, exists := seen[patchID]; exists {
			continue
		}
		seen[patchID] = struct{}{}
		cleanPatches = append(cleanPatches, patchID)
	}
	if len(cleanPatches) == 0 {
		return ErrNoDeploymentPatches
	}
	if dep.CreatedAt.IsZero() {
		dep.CreatedAt = time.Now().UTC()
	}
	if dep.ScheduledFor.IsZero() {
		dep.ScheduledFor = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO patch_deployments (id, policy_id, tenant_id, status, device_count, scheduled_for, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, dep.ID, dep.PolicyID, dep.TenantID, StatusPending, len(deviceIDs), dep.ScheduledFor, dep.CreatedAt); err != nil {
		return err
	}

	for _, deviceID := range deviceIDs {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO patch_deployment_devices (deployment_id, device_id)
			SELECT $1, d.id
			FROM devices d
			WHERE d.id::text = $2
			  AND d.tenant_id = $3
			  AND d.status <> 'disabled'
		`, dep.ID, deviceID, dep.TenantID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrPatchResultScope
		}
	}

	for _, patchID := range cleanPatches {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO patch_deployment_patches (deployment_id, patch_id)
			VALUES ($1, $2)
		`, dep.ID, patchID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
