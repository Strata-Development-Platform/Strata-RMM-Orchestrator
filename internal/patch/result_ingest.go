package patch

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrPatchResultScope = errors.New("patch result is outside deployment scope")

const maxPatchResultErrorBytes = 4096

func patchAgentTransportIdentity(subject, terminal string) (tenantID, deviceID string, err error) {
	parts := strings.Split(subject, ".")
	if len(parts) != 6 || parts[0] != "tenant" || parts[2] != "agent" || parts[4] != "patch" || parts[5] != terminal {
		return "", "", fmt.Errorf("invalid patch %s subject", terminal)
	}
	if parts[1] == "" || parts[3] == "" || strings.ContainsAny(parts[1], "*> ") || strings.ContainsAny(parts[3], "*> ") {
		return "", "", fmt.Errorf("invalid patch %s subject identity", terminal)
	}
	return parts[1], parts[3], nil
}

// patchResultTransportIdentity extracts the authoritative tenant and device
// identity from the subscribed NATS subject. Result payloads must not choose
// either value.
func patchResultTransportIdentity(subject string) (tenantID, deviceID string, err error) {
	return patchAgentTransportIdentity(subject, "result")
}

func patchInventoryTransportIdentity(subject string) (tenantID, deviceID string, err error) {
	return patchAgentTransportIdentity(subject, "inventory")
}

func normalizePatchResultError(value string) string {
	if len(value) <= maxPatchResultErrorBytes {
		return value
	}
	return value[:maxPatchResultErrorBytes]
}

// ApplyDevicePatchResult persists a result only after re-resolving the
// deployment target against durable tenant ownership. Duplicate results are
// idempotent, and a late result cannot regress an installed/reboot-required
// terminal state to a different state.
func (s *Store) ApplyDevicePatchResult(ctx context.Context, tenantID, deviceID, deploymentID, patchID string, status PatchStatus, resultError string) error {
	if s == nil || s.db == nil {
		return errors.New("patch store database is required")
	}
	if tenantID == "" || deviceID == "" || deploymentID == "" || patchID == "" {
		return errors.New("patch result identity is incomplete")
	}
	if !validPatchResultStatus(status) {
		return fmt.Errorf("invalid patch result status: %s", status)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var authorized bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM patch_deployments pd
			JOIN patch_deployment_devices pdd ON pdd.deployment_id = pd.id
			JOIN devices d ON d.id = pdd.device_id
			WHERE pd.id = $1
			  AND pd.tenant_id = $2
			  AND pdd.device_id = $3
			  AND d.tenant_id = pd.tenant_id
		)
	`, deploymentID, tenantID, deviceID).Scan(&authorized); err != nil {
		return fmt.Errorf("authorize patch result target: %w", err)
	}
	if !authorized {
		return ErrPatchResultScope
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO patch_device_states (device_id, deployment_id, patch_id, status, attempts, error, updated_at)
		VALUES ($1, $2, $3, $4, 1, $5, NOW())
		ON CONFLICT (deployment_id, device_id, patch_id) DO UPDATE SET
			status = EXCLUDED.status,
			attempts = patch_device_states.attempts + 1,
			error = EXCLUDED.error,
			updated_at = EXCLUDED.updated_at
		WHERE (patch_device_states.status, patch_device_states.error)
		      IS DISTINCT FROM (EXCLUDED.status, EXCLUDED.error)
		  AND NOT (
			patch_device_states.status IN ('installed', 'reboot_required')
			AND patch_device_states.status <> EXCLUDED.status
		  )
	`, deviceID, deploymentID, patchID, status, normalizePatchResultError(resultError))
	if err != nil {
		return fmt.Errorf("persist patch result: %w", err)
	}
	return tx.Commit()
}

func validPatchResultStatus(status PatchStatus) bool {
	switch status {
	case StatusInstalled, StatusFailed, StatusRebootReq:
		return true
	default:
		return false
	}
}
