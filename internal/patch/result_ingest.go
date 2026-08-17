package patch

import (
	"context"
	"database/sql"
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

// maxPatchAttempts treats MaxRetries as retries after the initial attempt. A
// negative policy value is never trusted and still yields one initial attempt.
func maxPatchAttempts(maxRetries int) int {
	if maxRetries < 0 {
		return 1
	}
	return maxRetries + 1
}

// ApplyDevicePatchResult persists a result only after re-resolving the
// deployment target against durable tenant ownership. Duplicate results are
// idempotent, late results cannot regress an installed/reboot-required terminal
// state, and the durable attempt counter is bounded by the deployment policy's
// initial-attempt-plus-retries ceiling.
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
	defer func() { _ = tx.Rollback() }()

	var maxRetries int
	err = tx.QueryRowContext(ctx, `
		SELECT pp.max_retries
		FROM patch_deployments pd
		JOIN patch_policies pp ON pp.id = pd.policy_id AND pp.tenant_id = pd.tenant_id
		JOIN patch_deployment_devices pdd ON pdd.deployment_id = pd.id
		JOIN devices d ON d.id = pdd.device_id
		WHERE pd.id = $1
		  AND pd.tenant_id = $2
		  AND pdd.device_id = $3
		  AND d.tenant_id = pd.tenant_id
	`, deploymentID, tenantID, deviceID).Scan(&maxRetries)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPatchResultScope
	}
	if err != nil {
		return fmt.Errorf("authorize patch result target: %w", err)
	}

	attemptCap := maxPatchAttempts(maxRetries)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO patch_device_states (device_id, deployment_id, patch_id, status, attempts, error, updated_at)
		VALUES ($1, $2, $3, $4, 1, $5, NOW())
		ON CONFLICT (deployment_id, device_id, patch_id) DO UPDATE SET
			status = EXCLUDED.status,
			attempts = LEAST(patch_device_states.attempts + 1, $6),
			error = EXCLUDED.error,
			updated_at = EXCLUDED.updated_at
		WHERE (patch_device_states.status, patch_device_states.error)
		      IS DISTINCT FROM (EXCLUDED.status, EXCLUDED.error)
		  AND NOT (
			patch_device_states.status IN ('installed', 'reboot_required')
			AND patch_device_states.status <> EXCLUDED.status
		  )
	`, deviceID, deploymentID, patchID, status, normalizePatchResultError(resultError), attemptCap)
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
