package patch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNoDeploymentPatches  = errors.New("patch deployment has no selected patches")
	ErrPatchRetriesExhausted = errors.New("patch deployment retry budget exhausted")
)

// GetDeploymentPatchIDs returns the explicit approved patch selection for a
// deployment. Patch rollout never falls back to "install everything" because
// doing so would bypass approval semantics.
func (s *Store) GetDeploymentPatchIDs(ctx context.Context, deploymentID string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("patch store database is required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT patch_id
		FROM patch_deployment_patches
		WHERE deployment_id = $1
		ORDER BY patch_id ASC
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query deployment patches: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var patchIDs []string
	for rows.Next() {
		var patchID string
		if err := rows.Scan(&patchID); err != nil {
			return nil, fmt.Errorf("scan deployment patch: %w", err)
		}
		patchIDs = append(patchIDs, patchID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deployment patches: %w", err)
	}
	if len(patchIDs) == 0 {
		return nil, ErrNoDeploymentPatches
	}
	return patchIDs, nil
}

// QueuePatchRolloutDevice creates one ordinary durable job target for one patch
// deployment device. The platform dispatcher owns envelope construction,
// outbox publication, retries, reconnect recovery, expiry, ACK handling, and
// stale-attempt result rejection. The queue transaction re-resolves device and
// policy state authoritatively. For a configured maintenance window, the job
// expires at the active window boundary; an expired prior target may be replaced
// in a later window only if the global policy attempt budget still has room.
func (s *Store) QueuePatchRolloutDevice(ctx context.Context, deploymentID, deviceID string, requestedMaxRetries int) error {
	if s == nil || s.db == nil {
		return errors.New("patch store database is required")
	}
	if deploymentID == "" || deviceID == "" {
		return errors.New("patch deployment and device are required")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin durable patch job: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var policyID, mspID, clientID, siteID, agentID, maintenanceWindow string
	var existingJobID, existingTargetID, existingTargetStatus sql.NullString
	var consumedAttempts, existingRetryCount, durableMaxRetries int
	err = tx.QueryRowContext(ctx, `
		SELECT pd.policy_id::text,
		       d.msp_id::text,
		       d.client_id::text,
		       COALESCE(d.site_id::text, ''),
		       d.agent_id::text,
		       COALESCE(pp.maintenance_window, ''),
		       pp.max_retries,
		       pdd.job_id::text,
		       pdd.job_target_id::text,
		       jt.status,
		       COALESCE(jt.retry_count, 0),
		       pdd.dispatch_attempts
		FROM patch_deployments pd
		JOIN patch_policies pp ON pp.id = pd.policy_id AND pp.tenant_id = pd.tenant_id
		JOIN patch_deployment_devices pdd ON pdd.deployment_id = pd.id
		JOIN devices d ON d.id = pdd.device_id
		LEFT JOIN job_targets jt ON jt.id = pdd.job_target_id AND jt.job_id = pdd.job_id
		WHERE pd.id = $1
		  AND pdd.device_id = $2
		  AND d.tenant_id = pd.tenant_id
		  AND d.msp_id IS NOT NULL
		  AND d.client_id IS NOT NULL
		  AND d.agent_id IS NOT NULL
		  AND d.status <> 'disabled'
		FOR UPDATE OF pdd
	`, deploymentID, deviceID).Scan(
		&policyID, &mspID, &clientID, &siteID, &agentID, &maintenanceWindow,
		&durableMaxRetries, &existingJobID, &existingTargetID, &existingTargetStatus,
		&existingRetryCount, &consumedAttempts,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPatchResultScope
	}
	if err != nil {
		return fmt.Errorf("resolve durable patch target: %w", err)
	}
	_ = requestedMaxRetries // The persisted policy is authoritative at dispatch time.
	if durableMaxRetries < 0 {
		durableMaxRetries = 0
	}
	if durableMaxRetries > 10 {
		durableMaxRetries = 10
	}
	attemptCap := maxPatchAttempts(durableMaxRetries)

	if existingJobID.Valid || existingTargetID.Valid {
		if !existingJobID.Valid || !existingTargetID.Valid {
			return errors.New("patch rollout durable job mapping is incomplete")
		}
		if !existingTargetStatus.Valid || existingTargetStatus.String != "expired" {
			return tx.Commit()
		}
		consumedAttempts += existingRetryCount
		if consumedAttempts >= attemptCap {
			return ErrPatchRetriesExhausted
		}
	}
	if consumedAttempts >= attemptCap {
		return ErrPatchRetriesExhausted
	}

	now := time.Now().UTC()
	expiresAt, err := maintenanceWindowDeadline(now, maintenanceWindow)
	if err != nil {
		return err
	}
	remainingRetries := attemptCap - consumedAttempts - 1
	if remainingRetries < 0 {
		remainingRetries = 0
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT patch_id
		FROM patch_deployment_patches
		WHERE deployment_id = $1
		ORDER BY patch_id ASC
	`, deploymentID)
	if err != nil {
		return fmt.Errorf("query selected patches: %w", err)
	}
	var patchIDs []string
	for rows.Next() {
		var patchID string
		if err := rows.Scan(&patchID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan selected patch: %w", err)
		}
		patchIDs = append(patchIDs, patchID)
	}
	rowsErr := rows.Err()
	if err := rows.Close(); err != nil && rowsErr == nil {
		rowsErr = err
	}
	if rowsErr != nil {
		return fmt.Errorf("read selected patches: %w", rowsErr)
	}
	if len(patchIDs) == 0 {
		return ErrNoDeploymentPatches
	}

	payload, err := json.Marshal(map[string]interface{}{
		"patch_ids":     patchIDs,
		"deployment_id": deploymentID,
		"policy_id":     policyID,
	})
	if err != nil {
		return fmt.Errorf("encode durable patch payload: %w", err)
	}

	jobID := uuid.NewString()
	targetID := uuid.NewString()
	correlationID := uuid.NewString()
	cycle := consumedAttempts + 1
	idempotencyKey := fmt.Sprintf("patch:%s:%s:%d", deploymentID, deviceID, cycle)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO jobs (
			id, msp_id, client_id, site_id, created_by, type, status, priority,
			payload, idempotency_key, max_retries, max_devices, expires_at,
			correlation_id, scheduled_for, request_hash
		)
		VALUES ($1, $2, $3, NULLIF($4, '')::uuid, 'patch-manager', 'patch_install',
		        'queued', 0, $5, $6, $7, 1, $8, $9, $10, $11)
	`, jobID, mspID, clientID, siteID, payload,
		idempotencyKey, remainingRetries, expiresAt.UTC(),
		correlationID, now, idempotencyKey)
	if err != nil {
		return fmt.Errorf("create durable patch job: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO job_targets (id, job_id, device_id, agent_id, msp_id, status)
		VALUES ($1, $2, $3, $4, $5, 'queued')
	`, targetID, jobID, deviceID, agentID, mspID); err != nil {
		return fmt.Errorf("create durable patch job target: %w", err)
	}

	newConsumedAttempts := consumedAttempts + 1
	result, err := tx.ExecContext(ctx, `
		UPDATE patch_deployment_devices
		SET job_id = $1,
		    job_target_id = $2,
		    dispatched_at = NOW(),
		    dispatch_attempts = $5
		WHERE deployment_id = $3 AND device_id = $4
	`, jobID, targetID, deploymentID, deviceID, newConsumedAttempts)
	if err != nil {
		return fmt.Errorf("map durable patch job target: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect durable patch target mapping: %w", err)
	}
	if affected != 1 {
		return ErrPatchResultScope
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit durable patch job: %w", err)
	}
	return nil
}

// GetRolloutJobGate derives rollout completion from the generic durable job
// target state. One target represents the aggregate patch installation command
// for one device, so canary progression cannot occur after only one of several
// selected patches reports.
func (s *Store) GetRolloutJobGate(ctx context.Context, deploymentID, rolloutGroup string) (CanaryGate, error) {
	if rolloutGroup != rolloutGroupCanary && rolloutGroup != rolloutGroupBroad {
		return CanaryGate{}, errors.New("invalid patch rollout group")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(jt.status, '')
		FROM patch_deployments pd
		JOIN patch_deployment_devices pdd ON pdd.deployment_id = pd.id
		JOIN devices d ON d.id = pdd.device_id AND d.tenant_id = pd.tenant_id
		LEFT JOIN job_targets jt ON jt.id = pdd.job_target_id AND jt.job_id = pdd.job_id
		WHERE pd.id = $1 AND pdd.rollout_group = $2
		ORDER BY pdd.device_id ASC
	`, deploymentID, rolloutGroup)
	if err != nil {
		return CanaryGate{}, fmt.Errorf("query patch rollout job gate: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var gate CanaryGate
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			return CanaryGate{}, fmt.Errorf("scan patch rollout job gate: %w", err)
		}
		gate.Total++
		switch status {
		case "succeeded":
			gate.Completed++
			gate.Succeeded++
		case "failed", "cancelled":
			gate.Completed++
			gate.Failed++
		case "expired":
			// Expiry at a maintenance boundary is not terminal while the durable
			// retry budget has capacity; QueuePatchRolloutDevice can replace it in
			// the next permitted window. A fully exhausted expired target is
			// classified by GetUndispatchedRolloutDevices/queue as exhausted and
			// remains incomplete until scheduler reconciliation marks failure.
		}
	}
	if err := rows.Err(); err != nil {
		return CanaryGate{}, fmt.Errorf("iterate patch rollout job gate: %w", err)
	}
	return gate, nil
}

func (s *Store) GetDeploymentJobGate(ctx context.Context, deploymentID string) (CanaryGate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(jt.status, '')
		FROM patch_deployments pd
		JOIN patch_deployment_devices pdd ON pdd.deployment_id = pd.id
		JOIN devices d ON d.id = pdd.device_id AND d.tenant_id = pd.tenant_id
		LEFT JOIN job_targets jt ON jt.id = pdd.job_target_id AND jt.job_id = pdd.job_id
		WHERE pd.id = $1
		ORDER BY pdd.device_id ASC
	`, deploymentID)
	if err != nil {
		return CanaryGate{}, fmt.Errorf("query patch deployment job gate: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var gate CanaryGate
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			return CanaryGate{}, fmt.Errorf("scan patch deployment job gate: %w", err)
		}
		gate.Total++
		switch status {
		case "succeeded":
			gate.Completed++
			gate.Succeeded++
		case "failed", "cancelled":
			gate.Completed++
			gate.Failed++
		}
	}
	if err := rows.Err(); err != nil {
		return CanaryGate{}, fmt.Errorf("iterate patch deployment job gate: %w", err)
	}
	return gate, nil
}
