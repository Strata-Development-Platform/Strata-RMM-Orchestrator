package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

const (
	resultTransactionAttempts = 5
	resultTransactionTimeout  = 15 * time.Second
)

// processAgentResultWithRetry applies a durable result using a bounded,
// cancellation-aware serializable transaction. PostgreSQL is allowed to abort
// serializable transactions when concurrent dispatcher reconciliation/outbox
// work creates a dependency cycle; those aborts are safe to retry because the
// inbox claim and target transition are idempotent and committed atomically.
func (d *Dispatcher) processAgentResultWithRetry(parent context.Context, subject string, data []byte) bool {
	ctx, cancel := context.WithTimeout(parent, resultTransactionTimeout)
	defer cancel()

	// Dispatcher.Stop closes stopCh even when the Start context remains live.
	// Bridge that signal into database contexts so shutdown cannot wait forever
	// on an in-flight result transaction.
	go func() {
		select {
		case <-d.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	for attempt := 0; attempt < resultTransactionAttempts; attempt++ {
		processed, err := d.processAgentResultOnce(ctx, subject, data)
		if err == nil {
			return processed
		}
		if !isRetryableResultTransactionError(err) {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				d.logger.Warn("process durable agent result", zap.Error(err))
			}
			return false
		}
		if attempt == resultTransactionAttempts-1 {
			d.logger.Warn("durable agent result serialization retry exhausted", zap.Error(err))
			return false
		}

		delay := time.Duration(attempt+1) * 25 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return false
		case <-timer.C:
		}
	}
	return false
}

func isRetryableResultTransactionError(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return pqErr.Code == "40001" || pqErr.Code == "40P01"
}

func (d *Dispatcher) processAgentResultOnce(ctx context.Context, subject string, data []byte) (bool, error) {
	subjectMSP, subjectAgent, ok := subjectIdentity(subject, "result")
	if !ok {
		return false, nil
	}
	res, err := ValidateResultEnvelope(data, subjectMSP, subjectAgent, "")
	if err != nil || res.MessageID == "" || res.CorrelationID == "" {
		d.logger.Warn("rejected agent result", zap.Error(err))
		return false, nil
	}

	tx, err := d.db.DB().BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var inboxID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO job_inbox (msp_id, message_id, job_id, target_id, event_type, payload)
		VALUES ($1, $2, $3, $4, 'result', $5)
		ON CONFLICT (msp_id, message_id) DO UPDATE
		SET message_id = EXCLUDED.message_id
		WHERE job_inbox.processed_at IS NULL
		RETURNING id::text
	`, res.MSPID, res.MessageID, res.JobID, res.TargetID, data).Scan(&inboxID)
	if err == sql.ErrNoRows {
		processed, verifyErr := d.resultProcessedContext(ctx, res.MSPID, res.MessageID)
		if verifyErr != nil {
			return false, verifyErr
		}
		if processed {
			d.publishResultReceipt(*res)
		}
		return processed, nil
	}
	if err != nil {
		return false, err
	}

	var currentStatus, agentID, clientID, siteID, correlationID string
	var currentAttempt int
	err = tx.QueryRowContext(ctx, `
		SELECT jt.status, COALESCE(jt.agent_id,''), jt.attempt, j.client_id::text,
		       COALESCE(j.site_id::text,''), COALESCE(j.correlation_id,'')
		FROM job_targets jt
		JOIN jobs j ON jt.job_id = j.id
		WHERE jt.id = $1 AND jt.job_id = $2 AND j.msp_id = $3 AND jt.device_id = $4
		FOR NO KEY UPDATE
	`, res.TargetID, res.JobID, res.MSPID, res.DeviceID).Scan(&currentStatus, &agentID, &currentAttempt, &clientID, &siteID, &correlationID)
	if err != nil {
		return false, err
	}
	if agentID != res.AgentID || currentAttempt != res.Attempt || clientID != res.ClientID ||
		siteID != res.SiteID || correlationID != res.CorrelationID {
		d.logger.Warn("result ownership mismatch", zap.String("target", res.TargetID))
		return false, nil
	}

	currentTerminal := currentStatus == "succeeded" || currentStatus == "failed" ||
		currentStatus == "cancelled" || currentStatus == "expired"
	resultTerminal := res.Status == "succeeded" || res.Status == "failed" ||
		res.Status == "cancelled" || res.Status == "expired"
	if currentTerminal && resultTerminal && (currentStatus == res.Status || currentStatus == "cancelled") {
		if _, err := tx.ExecContext(ctx, `UPDATE job_inbox SET processed_at=NOW() WHERE id::text=$1`, inboxID); err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		d.publishResultReceipt(*res)
		return true, nil
	}
	if err := TransitionJob(currentStatus, res.Status); err != nil {
		d.logger.Warn("invalid result transition", zap.Error(err))
		return false, nil
	}

	resultJSON, err := json.Marshal(res.Result)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE job_targets SET status=$1, result=$2, error_message=NULLIF($3,''), exit_code=$4,
			completed_at=NOW(), lease_owner=NULL, lease_expires=NULL WHERE id=$5
	`, res.Status, resultJSON, res.Error, res.ExitCode, res.TargetID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE job_inbox SET processed_at=NOW() WHERE id::text=$1`, inboxID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE jobs SET
			completed_count = (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status = 'succeeded'),
			failed_count = (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status IN ('failed','expired')),
			status = CASE
				WHEN (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status NOT IN ('succeeded','failed','cancelled','expired')) = 0
				THEN CASE
					WHEN (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status IN ('failed','expired')) > 0 THEN 'failed'
					WHEN (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status = 'cancelled') > 0 THEN 'cancelled'
					ELSE 'succeeded'
				END
				ELSE status
			END,
			completed_at = CASE
				WHEN (SELECT COUNT(*) FROM job_targets WHERE job_id = $1 AND status NOT IN ('succeeded','failed','cancelled','expired')) = 0
				THEN NOW()
				ELSE NULL
			END,
			updated_at = NOW()
		WHERE id = $1
	`, res.JobID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	d.publishResultReceipt(*res)
	return true, nil
}

func (d *Dispatcher) resultProcessedContext(ctx context.Context, mspID, messageID string) (bool, error) {
	var processed bool
	err := d.db.DB().QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM job_inbox
			WHERE msp_id = $1 AND message_id = $2 AND processed_at IS NOT NULL
		)
	`, mspID, messageID).Scan(&processed)
	return processed, err
}
