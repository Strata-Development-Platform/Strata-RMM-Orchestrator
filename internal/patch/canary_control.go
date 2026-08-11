package patch

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrInvalidCanaryPercent = errors.New("canary percent must be between 1 and 100")
	ErrNoCanaryDevices      = errors.New("no tenant-valid deployment devices available for canary")
)

// GetCanaryDeploymentDevices resolves the deployment's canary set from
// authoritative persisted ownership. A device is eligible only when it is a
// target of the requested deployment and its durable tenant ownership matches
// the deployment tenant. Selection is deterministic to make retries and
// orchestrator restarts converge on the same canary set.
func (s *Store) GetCanaryDeploymentDevices(ctx context.Context, deploymentID string, canaryPercent int) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("patch store database is required")
	}
	if deploymentID == "" {
		return nil, errors.New("deployment id is required")
	}
	if canaryPercent < 1 || canaryPercent > 100 {
		return nil, ErrInvalidCanaryPercent
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT pdd.device_id
		FROM patch_deployments pd
		JOIN patch_deployment_devices pdd ON pdd.deployment_id = pd.id
		JOIN devices d ON d.id = pdd.device_id
		WHERE pd.id = $1
		  AND d.tenant_id = pd.tenant_id
		ORDER BY pdd.device_id ASC
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query canary deployment devices: %w", err)
	}
	defer rows.Close()

	var eligible []string
	for rows.Next() {
		var deviceID string
		if err := rows.Scan(&deviceID); err != nil {
			return nil, fmt.Errorf("scan canary deployment device: %w", err)
		}
		eligible = append(eligible, deviceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate canary deployment devices: %w", err)
	}
	if len(eligible) == 0 {
		return nil, ErrNoCanaryDevices
	}

	return selectCanarySubset(eligible, canaryPercent), nil
}

// selectCanarySubset selects ceil(percent * n / 100), with at least one device
// whenever the deployment has an eligible target. Input order is preserved;
// the production query supplies a stable device-ID order.
func selectCanarySubset(deviceIDs []string, canaryPercent int) []string {
	if len(deviceIDs) == 0 || canaryPercent < 1 || canaryPercent > 100 {
		return nil
	}
	count := (len(deviceIDs)*canaryPercent + 99) / 100
	if count < 1 {
		count = 1
	}
	if count > len(deviceIDs) {
		count = len(deviceIDs)
	}
	selected := make([]string, count)
	copy(selected, deviceIDs[:count])
	return selected
}

// CanaryDeploymentDevices is the Manager-level production boundary for canary
// selection. Endpoint executors intentionally do not receive database handles
// or tenant identifiers from untrusted agent payloads.
func (m *Manager) CanaryDeploymentDevices(ctx context.Context, deploymentID string, canaryPercent int) ([]string, error) {
	if m == nil || m.store == nil {
		return nil, errors.New("patch manager store is required")
	}
	return m.store.GetCanaryDeploymentDevices(ctx, deploymentID, canaryPercent)
}
