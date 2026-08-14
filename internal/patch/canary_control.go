package patch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

var (
	ErrInvalidCanaryPercent = errors.New("canary percent must be between 1 and 100")
	ErrNoCanaryDevices      = errors.New("no tenant-valid deployment devices available for canary")
)

const (
	defaultCanaryPercent          = 10
	defaultCanarySuccessThreshold = 90
	rolloutGroupCanary            = "canary"
	rolloutGroupBroad             = "broad"
)

type CanaryGate struct {
	Total     int
	Completed int
	Succeeded int
	Failed    int
}

func (g CanaryGate) Ready() bool {
	return g.Total > 0 && g.Completed == g.Total
}

func (g CanaryGate) Passes(thresholdPercent int) bool {
	if !g.Ready() || thresholdPercent < 0 || thresholdPercent > 100 {
		return false
	}
	return g.Succeeded*100 >= g.Total*thresholdPercent
}

// GetCanaryDeploymentDevices resolves the deployment's canary set from
// authoritative persisted ownership. A device is eligible only when it is a
// target of the requested deployment and its durable tenant ownership matches
// the deployment tenant. Selection is deterministic to make retries and
// orchestrator restarts converge on the same canary set.
func (s *Store) GetCanaryDeploymentDevices(ctx context.Context, deploymentID string, canaryPercent int) ([]string, error) {
	eligible, err := s.getTenantValidDeploymentDevices(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if canaryPercent < 1 || canaryPercent > 100 {
		return nil, ErrInvalidCanaryPercent
	}
	if len(eligible) == 0 {
		return nil, ErrNoCanaryDevices
	}
	return selectCanarySubset(eligible, canaryPercent), nil
}

func (s *Store) getTenantValidDeploymentDevices(ctx context.Context, deploymentID string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("patch store database is required")
	}
	if deploymentID == "" {
		return nil, errors.New("deployment id is required")
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
		return nil, fmt.Errorf("query tenant-valid deployment devices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var eligible []string
	for rows.Next() {
		var deviceID string
		if err := rows.Scan(&deviceID); err != nil {
			return nil, fmt.Errorf("scan deployment device: %w", err)
		}
		eligible = append(eligible, deviceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deployment devices: %w", err)
	}
	return eligible, nil
}

// PrepareCanaryRollout atomically labels the deterministic canary set and
// resets dispatch metadata before the first rollout publication.
func (s *Store) PrepareCanaryRollout(ctx context.Context, deploymentID string, canaryPercent int) ([]string, error) {
	canary, err := s.GetCanaryDeploymentDevices(ctx, deploymentID, canaryPercent)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE patch_deployment_devices
		SET rollout_group = 'broad', dispatched_at = NULL, dispatch_attempts = 0
		WHERE deployment_id = $1
	`, deploymentID); err != nil {
		return nil, fmt.Errorf("reset rollout targets: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE patch_deployment_devices
		SET rollout_group = 'canary'
		WHERE deployment_id = $1 AND device_id = ANY($2)
	`, deploymentID, pq.Array(canary)); err != nil {
		return nil, fmt.Errorf("mark canary rollout targets: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return canary, nil
}

func (s *Store) GetUndispatchedRolloutDevices(ctx context.Context, deploymentID, rolloutGroup string) ([]string, error) {
	if rolloutGroup != rolloutGroupCanary && rolloutGroup != rolloutGroupBroad {
		return nil, errors.New("invalid patch rollout group")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT pdd.device_id
		FROM patch_deployments pd
		JOIN patch_deployment_devices pdd ON pdd.deployment_id = pd.id
		JOIN devices d ON d.id = pdd.device_id AND d.tenant_id = pd.tenant_id
		WHERE pd.id = $1
		  AND pdd.rollout_group = $2
		  AND pdd.dispatched_at IS NULL
		ORDER BY pdd.device_id ASC
	`, deploymentID, rolloutGroup)
	if err != nil {
		return nil, fmt.Errorf("query undispatched rollout devices: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var devices []string
	for rows.Next() {
		var deviceID string
		if err := rows.Scan(&deviceID); err != nil {
			return nil, err
		}
		devices = append(devices, deviceID)
	}
	return devices, rows.Err()
}

func (s *Store) MarkRolloutDeviceDispatched(ctx context.Context, deploymentID, deviceID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE patch_deployment_devices pdd
		SET dispatched_at = NOW(), dispatch_attempts = dispatch_attempts + 1
		FROM patch_deployments pd, devices d
		WHERE pdd.deployment_id = $1
		  AND pdd.device_id = $2
		  AND pd.id = pdd.deployment_id
		  AND d.id = pdd.device_id
		  AND d.tenant_id = pd.tenant_id
	`, deploymentID, deviceID)
	if err != nil {
		return fmt.Errorf("mark rollout device dispatched: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrPatchResultScope
	}
	return nil
}

func (s *Store) GetRemainingDeploymentDevices(ctx context.Context, deploymentID string, canaryPercent int) ([]string, error) {
	eligible, err := s.getTenantValidDeploymentDevices(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if canaryPercent < 1 || canaryPercent > 100 {
		return nil, ErrInvalidCanaryPercent
	}
	if len(eligible) == 0 {
		return nil, ErrNoCanaryDevices
	}
	canaryCount := len(selectCanarySubset(eligible, canaryPercent))
	remaining := make([]string, len(eligible)-canaryCount)
	copy(remaining, eligible[canaryCount:])
	return remaining, nil
}

func (s *Store) getResultGate(ctx context.Context, deploymentID string, devices []string) (CanaryGate, error) {
	if len(devices) == 0 {
		return CanaryGate{}, nil
	}
	gate := CanaryGate{Total: len(devices)}
	rows, err := s.db.QueryContext(ctx, `
		SELECT pdd.device_id,
		       COUNT(pds.patch_id) > 0 AS reported,
		       COALESCE(BOOL_OR(pds.status = 'failed'), FALSE) AS failed,
		       COALESCE(BOOL_OR(pds.status IN ('installed', 'reboot_required')), FALSE) AS succeeded
		FROM patch_deployment_devices pdd
		LEFT JOIN patch_device_states pds
		  ON pds.deployment_id = pdd.deployment_id
		 AND pds.device_id = pdd.device_id
		 AND pds.status IN ('installed', 'failed', 'reboot_required')
		WHERE pdd.deployment_id = $1
		  AND pdd.device_id = ANY($2)
		GROUP BY pdd.device_id
	`, deploymentID, pq.Array(devices))
	if err != nil {
		return CanaryGate{}, fmt.Errorf("query patch result gate: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var deviceID string
		var reported, failed, succeeded bool
		if err := rows.Scan(&deviceID, &reported, &failed, &succeeded); err != nil {
			return CanaryGate{}, fmt.Errorf("scan patch result gate: %w", err)
		}
		if !reported {
			continue
		}
		gate.Completed++
		if failed {
			gate.Failed++
		} else if succeeded {
			gate.Succeeded++
		}
	}
	if err := rows.Err(); err != nil {
		return CanaryGate{}, fmt.Errorf("iterate patch result gate: %w", err)
	}
	return gate, nil
}

// GetCanaryGate derives canary progress solely from durable result rows for the
// deterministic canary set. A canary device is complete when at least one
// terminal patch result exists; any failed patch makes that device fail.
func (s *Store) GetCanaryGate(ctx context.Context, deploymentID string, canaryPercent int) (CanaryGate, error) {
	devices, err := s.GetCanaryDeploymentDevices(ctx, deploymentID, canaryPercent)
	if err != nil {
		return CanaryGate{}, err
	}
	return s.getResultGate(ctx, deploymentID, devices)
}

func (s *Store) GetDeploymentGate(ctx context.Context, deploymentID string) (CanaryGate, error) {
	devices, err := s.getTenantValidDeploymentDevices(ctx, deploymentID)
	if err != nil {
		return CanaryGate{}, err
	}
	return s.getResultGate(ctx, deploymentID, devices)
}

func (s *Store) getDeploymentsByStatus(ctx context.Context, status PatchStatus) ([]*Deployment, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("patch store database is required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, policy_id, tenant_id, status, device_count, installed, failed, pending,
		       scheduled_for, started_at, completed_at, created_at
		FROM patch_deployments
		WHERE status = $1
		ORDER BY started_at ASC NULLS FIRST, scheduled_for ASC
	`, status)
	if err != nil {
		return nil, fmt.Errorf("query %s deployments: %w", status, err)
	}
	defer func() { _ = rows.Close() }()
	var deployments []*Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.PolicyID, &d.TenantID, &d.Status, &d.DeviceCount,
			&d.Installed, &d.Failed, &d.Pending, &d.ScheduledFor,
			&d.StartedAt, &d.CompletedAt, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan %s deployment: %w", status, err)
		}
		deployments = append(deployments, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s deployments: %w", status, err)
	}
	return deployments, nil
}

func (s *Store) GetCanaryDeployments(ctx context.Context) ([]*Deployment, error) {
	return s.getDeploymentsByStatus(ctx, StatusCanary)
}

func (s *Store) GetDeployingDeployments(ctx context.Context) ([]*Deployment, error) {
	return s.getDeploymentsByStatus(ctx, StatusDeploying)
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

// maintenanceWindowAllows interprets the existing policy window as
// "HH:MM-HH:MM" in local server time. Empty means unrestricted. Invalid values
// fail closed. Equal endpoints represent a full-day window.
func maintenanceWindowAllows(now time.Time, window string) bool {
	window = strings.TrimSpace(window)
	if window == "" {
		return true
	}
	parts := strings.Split(window, "-")
	if len(parts) != 2 {
		return false
	}
	startValue := strings.TrimSpace(parts[0])
	endValue := strings.TrimSpace(parts[1])
	if !validMaintenanceClock(startValue) || !validMaintenanceClock(endValue) {
		return false
	}
	start, err := time.Parse("15:04", startValue)
	if err != nil {
		return false
	}
	end, err := time.Parse("15:04", endValue)
	if err != nil {
		return false
	}
	minute := now.Hour()*60 + now.Minute()
	startMinute := start.Hour()*60 + start.Minute()
	endMinute := end.Hour()*60 + end.Minute()
	if startMinute == endMinute {
		return true
	}
	if startMinute < endMinute {
		return minute >= startMinute && minute < endMinute
	}
	return minute >= startMinute || minute < endMinute
}

func validMaintenanceClock(value string) bool {
	if len(value) != 5 || value[2] != ':' {
		return false
	}
	for _, idx := range []int{0, 1, 3, 4} {
		if value[idx] < '0' || value[idx] > '9' {
			return false
		}
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[3]-'0')*10 + int(value[4]-'0')
	return hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59
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
