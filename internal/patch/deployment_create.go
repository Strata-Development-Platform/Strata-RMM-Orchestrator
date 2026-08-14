package patch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrPatchInventoryUnavailable = errors.New("patch inventory is unavailable for deployment target")
	ErrPatchInventoryInvalid     = errors.New("patch inventory snapshot is invalid")
	ErrPatchSelectionNotMissing  = errors.New("requested patch is not missing on deployment target")
)

func normalizeDeploymentIDs(values []string) []string {
	clean := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		clean = append(clean, value)
	}
	return clean
}

func validatePatchSelectionSnapshot(snapshotJSON []byte, patchIDs []string) error {
	var envelope struct {
		Missing json.RawMessage `json:"missing"`
	}
	if err := json.Unmarshal(snapshotJSON, &envelope); err != nil || len(envelope.Missing) == 0 {
		return ErrPatchInventoryInvalid
	}

	var missing []*Patch
	if err := json.Unmarshal(envelope.Missing, &missing); err != nil {
		return ErrPatchInventoryInvalid
	}
	available := make(map[string]struct{}, len(missing))
	for _, candidate := range missing {
		if candidate == nil {
			continue
		}
		id := strings.TrimSpace(candidate.ID)
		if id != "" {
			available[id] = struct{}{}
		}
	}
	for _, patchID := range patchIDs {
		if _, ok := available[patchID]; !ok {
			return fmt.Errorf("%w: %s", ErrPatchSelectionNotMissing, patchID)
		}
	}
	return nil
}

// CreateDeploymentWithPatches persists the target devices and the exact patch
// IDs approved for the deployment in one transaction. An empty selection is
// rejected so rollout can never degrade into an implicit "install all" action.
// Every selected patch must also be present in the latest authoritative missing
// inventory snapshot for every selected device before any deployment rows are
// committed.
func (s *Store) CreateDeploymentWithPatches(ctx context.Context, dep *Deployment, deviceIDs, patchIDs []string) error {
	if s == nil || s.db == nil {
		return errors.New("patch store database is required")
	}
	if dep == nil || dep.ID == "" || dep.PolicyID == "" || dep.TenantID == "" {
		return errors.New("patch deployment identity is incomplete")
	}
	cleanDevices := normalizeDeploymentIDs(deviceIDs)
	if len(cleanDevices) == 0 {
		return errors.New("patch deployment requires at least one device")
	}
	cleanPatches := normalizeDeploymentIDs(patchIDs)
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

	for _, deviceID := range cleanDevices {
		var snapshotJSON []byte
		err := tx.QueryRowContext(ctx, `
			SELECT pi.snapshot
			FROM patch_inventory pi
			JOIN devices d ON d.id = pi.device_id
			WHERE pi.tenant_id = $1
			  AND pi.device_id = $2
			  AND d.tenant_id = pi.tenant_id
			  AND d.status <> 'disabled'
			ORDER BY pi.created_at DESC
			LIMIT 1
		`, dep.TenantID, deviceID).Scan(&snapshotJSON)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrPatchInventoryUnavailable, deviceID)
		}
		if err != nil {
			return fmt.Errorf("load patch inventory for %s: %w", deviceID, err)
		}
		if err := validatePatchSelectionSnapshot(snapshotJSON, cleanPatches); err != nil {
			return fmt.Errorf("validate patch inventory for %s: %w", deviceID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO patch_deployments (id, policy_id, tenant_id, status, device_count, scheduled_for, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, dep.ID, dep.PolicyID, dep.TenantID, StatusPending, len(cleanDevices), dep.ScheduledFor, dep.CreatedAt); err != nil {
		return err
	}

	for _, deviceID := range cleanDevices {
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
