package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	jsmsg "github.com/strata-rmm/strata-rmm-orchestrator/internal/messaging/jetstream"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type Dispatcher struct {
	db       *timescale.Client
	nc       *nats.Conn
	logger   *zap.Logger
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
	running  atomic.Bool
	workerID string
}

func NewDispatcher(db *timescale.Client, nc *nats.Conn, logger *zap.Logger) *Dispatcher {
	host, _ := os.Hostname()
	return &Dispatcher{
		db:       db,
		nc:       nc,
		logger:   logger,
		stopCh:   make(chan struct{}),
		workerID: fmt.Sprintf("%s-%s", host, uuid.NewString()),
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	d.wg.Add(1)
	go d.outboxPublisher(ctx)
	d.wg.Add(1)
	go d.reconciliationWorker(ctx)
	if d.nc != nil {
		d.wg.Add(1)
		go d.subscribeResults(ctx)
	}
	d.running.Store(true)
	d.logger.Info("job dispatcher started")
}

func (d *Dispatcher) Stop() {
	d.running.Store(false)
	d.stopOnce.Do(func() { close(d.stopCh) })
	d.wg.Wait()
	d.logger.Info("job dispatcher stopped")
}

// Healthy verifies that the dispatcher is running and both dependencies used by
// its worker loops are currently available. It is intentionally a live check,
// not a startup flag.
func (d *Dispatcher) Healthy(ctx context.Context) error {
	if !d.running.Load() {
		return fmt.Errorf("dispatcher is not running")
	}
	if d.nc == nil || !d.nc.IsConnected() {
		return fmt.Errorf("dispatcher NATS connection is not established")
	}
	if d.db == nil || d.db.DB() == nil {
		return fmt.Errorf("dispatcher database is not configured")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := d.db.DB().PingContext(pingCtx); err != nil {
		return fmt.Errorf("dispatcher database check failed: %w", err)
	}
	return nil
}

func (d *Dispatcher) outboxPublisher(ctx context.Context) {
	defer d.wg.Done()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.withRecoveryReadLock(func() {
				d.ensureQueuedOutbox()
				d.processOutbox()
				d.expireJobs()
				d.expirePendingApprovals()
				d.handleOfflineReconnect()
				d.expireOfflineWork()
			})
		}
	}
}

func (d *Dispatcher) withRecoveryReadLock(work func()) {
	if d.db == nil || d.db.DB() == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := d.db.DB().Conn(ctx)
	if err != nil {
		d.logger.Error("reserve dispatcher recovery-gate connection", zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()
	var acquired bool
	if err := conn.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock_shared($1)`, postgres.GetRecoveryLockID()).Scan(&acquired); err != nil {
		d.logger.Error("inspect dispatcher recovery gate", zap.Error(err))
		return
	}
	if !acquired {
		return
	}
	defer func() {
		var unlocked bool
		if err := conn.QueryRowContext(context.Background(),
			`SELECT pg_advisory_unlock_shared($1)`, postgres.GetRecoveryLockID()).Scan(&unlocked); err != nil {
			d.logger.Error("release dispatcher recovery gate", zap.Error(err))
		}
	}()
	work()
}

func (d *Dispatcher) ensureQueuedOutbox() {
	_, err := d.db.DB().Exec(`
		INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
		SELECT gen_random_uuid(), j.msp_id, j.id, 'job.dispatch',
		       jsonb_build_object(
		         'schema_version', 1,
		         'event_id', j.id::text || ':' || jt.id::text || ':' || (jt.attempt + 1)::text,
		         'job_id', j.id,
		         'target_id', jt.id,
		         'msp_id', j.msp_id,
		         'client_id', j.client_id,
		         'site_id', j.site_id,
		         'device_id', jt.device_id,
		         'agent_id', jt.agent_id,
		         'correlation_id', j.correlation_id,
		         'attempt', jt.attempt + 1,
		         'issued_at', NOW(),
		         'expires_at', j.expires_at,
		         'command_type', j.type,
		         'payload', j.payload
		       ),
		       GREATEST(COALESCE(jt.next_retry_at, NOW()), COALESCE(j.scheduled_for, NOW()))
		FROM job_targets jt
		JOIN jobs j ON j.id = jt.job_id
		WHERE jt.status = 'queued'
		  AND (jt.next_retry_at IS NULL OR jt.next_retry_at <= NOW())
		  AND (j.expires_at IS NULL OR j.expires_at > NOW())
		  AND jt.retry_count <= j.max_retries
		  AND NOT EXISTS (
		    SELECT 1 FROM job_outbox o
		    WHERE o.event_type = 'job.dispatch'
		      AND o.payload->>'target_id' = jt.id::text
		      AND COALESCE((o.payload->>'attempt')::int, 0) = jt.attempt + 1
		  )
		LIMIT 100
	`)
	if err != nil {
		d.logger.Error("ensure queued outbox", zap.Error(err))
	}
}

func (d *Dispatcher) processOutbox() {
	rows, err := d.db.DB().Query(`
		UPDATE job_outbox SET lease_owner = $1, lease_expires = NOW() + INTERVAL '30 seconds',
		                       attempt_count = attempt_count + 1
		WHERE id IN (
			SELECT id FROM job_outbox
			WHERE published_at IS NULL AND available_at <= NOW()
			      AND (lease_expires IS NULL OR lease_expires < NOW())
			ORDER BY created_at ASC LIMIT 20
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, msp_id, aggregate_id, event_type, payload::text
	`, d.workerID)
	if err != nil {
		d.logger.Error("claim outbox", zap.Error(err))
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, mspID, aggregateID, eventType, payloadStr string
		if err := rows.Scan(&id, &mspID, &aggregateID, &eventType, &payloadStr); err != nil {
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
			d.failOutbox(id, fmt.Errorf("invalid payload: %w", err), 1)
			continue
		}

		agentID, _ := payload["agent_id"].(string)
		targetID, _ := payload["target_id"].(string)
		attempt := intFromJSON(payload["attempt"])
		if agentID == "" || targetID == "" {
			d.failOutbox(id, fmt.Errorf("missing agent_id or target_id"), 1)
			continue
		}

		if eventType == "job.cancel" {
			subject := fmt.Sprintf("tenant.%s.cmd.%s.cancel", mspID, agentID)
			if err := d.nc.Publish(subject, []byte(payloadStr)); err != nil {
				d.failOutbox(id, err, 1)
				continue
			}
			if err := d.nc.FlushTimeout(5 * time.Second); err != nil {
				d.failOutbox(id, err, 1)
				continue
			}
			if _, err := d.db.DB().Exec(`
				UPDATE job_outbox
				SET published_at = NOW(), lease_owner = NULL, lease_expires = NULL, last_error = NULL
				WHERE id = $1 AND lease_owner = $2
			`, id, d.workerID); err != nil {
				d.logger.Error("finalize cancellation outbox publish", zap.String("id", id), zap.Error(err))
			}
			continue
		}
		if eventType != "job.dispatch" {
			d.failOutbox(id, fmt.Errorf("unsupported outbox event type %q", eventType), 1)
			continue
		}
		if attempt < 1 {
			d.failOutbox(id, fmt.Errorf("missing attempt"), 1)
			continue
		}

		// Commit the dispatched state before publishing. Otherwise a fast agent
		// can acknowledge or finish while PostgreSQL still reports "queued".
		tx, err := d.db.DB().Begin()
		if err != nil {
			d.failOutbox(id, err, attempt)
			continue
		}
		result, err := tx.Exec(`
			UPDATE job_targets
			SET status = 'dispatched', dispatched_at = COALESCE(dispatched_at, NOW()), attempt = $2,
			    lease_owner = NULL, lease_expires = NOW() + INTERVAL '2 minutes'
			WHERE id = $1 AND status IN ('queued','dispatched') AND (status = 'queued' OR attempt < $2)
		`, targetID, attempt)
		if err == nil {
			var affected int64
			affected, err = result.RowsAffected()
			if err == nil && affected > 0 {
				_, err = tx.Exec(`
					UPDATE jobs SET dispatch_count = dispatch_count + 1,
						status = CASE WHEN status IN ('pending','queued') THEN 'dispatched' ELSE status END,
						updated_at = NOW()
					WHERE id = $1
				`, aggregateID)
			}
		}
		if err != nil {
			_ = tx.Rollback()
			d.failOutbox(id, err, attempt)
			continue
		}
		if err := tx.Commit(); err != nil {
			d.failOutbox(id, err, attempt)
			continue
		}

		subject := fmt.Sprintf("tenant.%s.cmd.%s", mspID, agentID)
		if err := d.nc.Publish(subject, []byte(payloadStr)); err != nil {
			d.failOutbox(id, err, attempt)
			continue
		}
		if err := d.nc.FlushTimeout(5 * time.Second); err != nil {
			d.failOutbox(id, err, attempt)
			continue
		}

		if _, err := d.db.DB().Exec(`
			UPDATE job_outbox
			SET published_at = NOW(), lease_owner = NULL, lease_expires = NULL, last_error = NULL
			WHERE id = $1 AND lease_owner = $2
		`, id, d.workerID); err != nil {
			d.logger.Error("finalize outbox publish", zap.String("id", id), zap.Error(err))
		}
	}
}

func (d *Dispatcher) expireJobs() {
	if _, err := d.db.DB().Exec(`
		UPDATE job_targets SET status = 'expired'
		WHERE status IN ('pending', 'queued', 'dispatched', 'running')
		      AND id IN (
			SELECT jt.id FROM job_targets jt
			JOIN jobs j ON jt.job_id = j.id
			WHERE j.expires_at < NOW()
			LIMIT 50
		)
	`); err != nil {
		d.logger.Error("expire jobs", zap.Error(err))
	}
}

func (d *Dispatcher) reconciliationWorker(ctx context.Context) {
	defer d.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.withRecoveryReadLock(d.reconcile)
		}
	}
}

func (d *Dispatcher) expirePendingApprovals() {
	if _, err := d.db.DB().Exec(`
		UPDATE endpoint_approval_requests SET status = 'expired', updated_at = NOW()
		WHERE status = 'pending' AND expires_at < NOW()
	`); err != nil {
		d.logger.Error("expire pending approvals", zap.Error(err))
	}
}

func (d *Dispatcher) handleOfflineReconnect() {
	_, err := d.db.DB().Exec(`
		UPDATE job_targets jt SET status = 'queued', reconnect_at = NOW()
		FROM devices d
		WHERE jt.device_id::uuid = d.id
		  AND jt.status = 'waiting'
		  AND d.status = 'online'
		  AND jt.approval_status IN ('none', 'approved')
		  AND (jt.offline_at IS NULL OR jt.offline_at < NOW() - INTERVAL '30 seconds')
	`)
	if err != nil {
		d.logger.Error("handle offline reconnect", zap.Error(err))
	}
}

func (d *Dispatcher) expireOfflineWork() {
	if _, err := d.db.DB().Exec(`
		UPDATE job_targets jt SET status = 'expired', error_message = 'expired: waited beyond expiry'
		FROM jobs j
		WHERE jt.job_id = j.id
		  AND jt.status = 'waiting'
		  AND j.expires_at < NOW()
	`); err != nil {
		d.logger.Error("expire offline work", zap.Error(err))
	}
}

func (d *Dispatcher) reconcile() {
	// Claim expired dispatcher leases.
	if _, err := d.db.DB().Exec(`
		UPDATE job_targets SET status = 'queued', lease_owner = NULL, lease_expires = NULL
		WHERE status = 'dispatched' AND lease_owner IS NOT NULL AND lease_expires < NOW()
		      AND id IN (SELECT id FROM job_targets WHERE lease_expires < NOW() LIMIT 50)
	`); err != nil {
		d.logger.Error("recover dispatcher leases", zap.Error(err))
	}
	// Retry timed-out agent execution while attempts remain.
	if _, err := d.db.DB().Exec(`
		UPDATE job_targets jt
		SET status = CASE WHEN jt.retry_count < j.max_retries THEN 'queued' ELSE 'failed' END,
		    retry_count = jt.retry_count + 1,
		    next_retry_at = CASE WHEN jt.retry_count < j.max_retries THEN NOW() + INTERVAL '30 seconds' ELSE NULL END,
		    lease_owner = NULL, lease_expires = NULL, error_message = 'execution acknowledgement timeout'
		FROM jobs j
		WHERE jt.job_id = j.id AND jt.status IN ('dispatched','running')
		  AND jt.lease_expires < NOW()
		  AND (j.expires_at IS NULL OR j.expires_at > NOW())
		  AND jt.id IN (SELECT id FROM job_targets WHERE lease_expires < NOW() LIMIT 50)
	`); err != nil {
		d.logger.Error("recover timed out execution", zap.Error(err))
	}
	// Reconcile only jobs whose targets are all terminal, using the same
	// precedence as the database trigger: failed/expired, then cancelled, then success.
	if _, err := d.db.DB().Exec(`
		UPDATE jobs j SET
			status = CASE
				WHEN EXISTS (SELECT 1 FROM job_targets WHERE job_id = j.id AND status IN ('failed','expired')) THEN 'failed'
				WHEN EXISTS (SELECT 1 FROM job_targets WHERE job_id = j.id AND status = 'cancelled') THEN 'cancelled'
				ELSE 'succeeded'
			END,
			completed_at = NOW(),
			updated_at = NOW(),
			completed_count = (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status = 'succeeded'),
			failed_count = (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status IN ('failed','expired'))
		WHERE j.id IN (
			SELECT jt.job_id FROM job_targets jt
			GROUP BY jt.job_id
			HAVING count(*) = count(*) FILTER (WHERE jt.status IN ('succeeded','failed','cancelled','expired'))
			LIMIT 50
		)
	`); err != nil {
		d.logger.Error("reconcile job aggregates", zap.Error(err))
	}
}

func (d *Dispatcher) subscribeResults(ctx context.Context) {
	defer d.wg.Done()

	js, err := d.nc.JetStream()
	if err != nil {
		d.logger.Error("create agent result JetStream context", zap.Error(err))
		return
	}
	resultSub, err := js.QueueSubscribe("tenant.*.agent.*.result", "orchestrator-job-results", func(msg *nats.Msg) {
		subjectMSP, subjectAgent, ok := subjectIdentity(msg.Subject, "result")
		if !ok {
			_ = msg.Term()
			return
		}
		res, validateErr := ValidateResultEnvelope(msg.Data, subjectMSP, subjectAgent, "")
		if validateErr != nil || res.MessageID == "" {
			d.logger.Warn("terminating malformed durable agent result", zap.Error(validateErr))
			_ = msg.Term()
			return
		}

		processed := false
		d.withRecoveryReadLock(func() {
			processed = d.processAgentResultWithRetry(ctx, msg.Subject, msg.Data)
		})
		if processed {
			_ = msg.Ack()
			return
		}
		_ = msg.Nak()
	},
		nats.Durable("orchestrator_job_results"),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.DeliverAll(),
		nats.BindStream(jsmsg.StreamAgentResults),
	)
	if err != nil {
		d.logger.Error("subscribe durable agent results", zap.Error(err))
		return
	}
	defer func() {
		if err := resultSub.Unsubscribe(); err != nil {
			d.logger.Warn("unsubscribe durable agent results", zap.Error(err))
		}
	}()

	// Agent acknowledgements are advisory/transient. Losing one does not lose
	// terminal work because the durable result path is authoritative.
	ackSub, err := d.nc.Subscribe("tenant.*.agent.*.ack", func(msg *nats.Msg) {
		d.withRecoveryReadLock(func() {
			d.handleAgentAck(msg.Subject, msg.Data)
		})
	})
	if err != nil {
		d.logger.Error("subscribe agent acknowledgements", zap.Error(err))
		return
	}
	defer func() {
		if err := ackSub.Unsubscribe(); err != nil {
			d.logger.Warn("unsubscribe agent acknowledgements", zap.Error(err))
		}
	}()

	select {
	case <-ctx.Done():
	case <-d.stopCh:
	}
}

func subjectIdentity(subject, suffix string) (string, string, bool) {
	parts := strings.Split(subject, ".")
	if len(parts) != 5 || parts[0] != "tenant" || parts[2] != "agent" || parts[4] != suffix {
		return "", "", false
	}
	return parts[1], parts[3], parts[1] != "" && parts[3] != ""
}

func (d *Dispatcher) handleAgentAck(subject string, data []byte) {
	var ack Acknowledgement
	if err := json.Unmarshal(data, &ack); err != nil {
		d.logger.Warn("malformed acknowledgement", zap.Error(err))
		return
	}
	subjectMSP, subjectAgent, ok := subjectIdentity(subject, "ack")
	if !ok || ack.EventID == "" || ack.MessageID == "" || ack.JobID == "" || ack.TargetID == "" ||
		ack.MSPID != subjectMSP || ack.AgentID != subjectAgent || ack.Attempt < 1 {
		d.logger.Warn("rejected acknowledgement identity", zap.String("subject", subject))
		return
	}
	tx, err := d.db.DB().BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		d.logger.Error("begin acknowledgement transaction", zap.Error(err))
		return
	}
	defer func() { _ = tx.Rollback() }()
	var inserted string
	err = tx.QueryRow(`
		INSERT INTO job_inbox (msp_id, message_id, job_id, target_id, event_type, payload)
		VALUES ($1, $2, $3, $4, 'ack', $5)
		ON CONFLICT (msp_id, message_id) DO NOTHING RETURNING id::text
	`, ack.MSPID, ack.MessageID, ack.JobID, ack.TargetID, data).Scan(&inserted)
	if err == sql.ErrNoRows {
		return
	}
	if err != nil {
		d.logger.Error("claim acknowledgement", zap.Error(err))
		return
	}
	var currentStatus, targetAgent, correlationID string
	var currentAttempt int
	err = tx.QueryRow(`
		SELECT jt.status, COALESCE(jt.agent_id,''), jt.attempt, COALESCE(j.correlation_id,'')
		FROM job_targets jt JOIN jobs j ON jt.job_id = j.id
		WHERE jt.id = $1 AND jt.job_id = $2 AND j.msp_id = $3 AND jt.device_id = $4
		FOR NO KEY UPDATE
	`, ack.TargetID, ack.JobID, ack.MSPID, ack.DeviceID).Scan(&currentStatus, &targetAgent, &currentAttempt, &correlationID)
	if err != nil || targetAgent != ack.AgentID || currentAttempt != ack.Attempt || correlationID != ack.CorrelationID {
		d.logger.Warn("acknowledgement ownership mismatch", zap.String("target", ack.TargetID), zap.Error(err))
		return
	}
	nextStatus := ""
	switch ack.Status {
	case AckAccepted:
		nextStatus = "running"
	case AckDuplicate:
		nextStatus = currentStatus
	case AckRejected, AckUnsupported:
		nextStatus = "failed"
	case AckExpired:
		nextStatus = "expired"
	default:
		return
	}
	if nextStatus != currentStatus {
		if err := TransitionJob(currentStatus, nextStatus); err != nil {
			d.logger.Warn("invalid acknowledgement transition", zap.Error(err))
			return
		}
		if _, err := tx.Exec(`
			UPDATE job_targets SET status=$1, acknowledged_at=CASE WHEN $1='running' THEN NOW() ELSE acknowledged_at END,
				completed_at=CASE WHEN $1 IN ('failed','expired') THEN NOW() ELSE completed_at END,
				error_message=CASE WHEN $1='failed' THEN $2 ELSE error_message END
			WHERE id=$3
		`, nextStatus, "target rejected by agent: "+ack.Status, ack.TargetID); err != nil {
			d.logger.Error("apply acknowledgement", zap.Error(err))
			return
		}
	}
	if _, err := tx.Exec(`UPDATE job_inbox SET processed_at=NOW() WHERE id::text=$1`, inserted); err != nil {
		return
	}
	if err := tx.Commit(); err != nil {
		d.logger.Error("commit acknowledgement", zap.Error(err))
	}
}

func (d *Dispatcher) handleAgentResult(subject string, data []byte) {
	subjectMSP, subjectAgent, ok := subjectIdentity(subject, "result")
	if !ok {
		return
	}
	res, err := ValidateResultEnvelope(data, subjectMSP, subjectAgent, "")
	if err != nil || res.MessageID == "" || res.CorrelationID == "" {
		d.logger.Warn("rejected agent result", zap.Error(err))
		return
	}
	tx, err := d.db.DB().BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()
	var inboxID string
	err = tx.QueryRow(`
		INSERT INTO job_inbox (msp_id, message_id, job_id, target_id, event_type, payload)
		VALUES ($1, $2, $3, $4, 'result', $5)
		ON CONFLICT (msp_id, message_id) DO NOTHING RETURNING id::text
	`, res.MSPID, res.MessageID, res.JobID, res.TargetID, data).Scan(&inboxID)
	if err == sql.ErrNoRows {
		d.publishResultReceipt(*res)
		return
	}
	if err != nil {
		return
	}
	var currentStatus, agentID, clientID, siteID, correlationID string
	var currentAttempt int
	err = tx.QueryRow(`
		SELECT jt.status, COALESCE(jt.agent_id,''), jt.attempt, j.client_id::text,
		       COALESCE(j.site_id::text,''), COALESCE(j.correlation_id,'')
		FROM job_targets jt
		JOIN jobs j ON jt.job_id = j.id
		WHERE jt.id = $1 AND jt.job_id = $2 AND j.msp_id = $3 AND jt.device_id = $4
		FOR NO KEY UPDATE
	`, res.TargetID, res.JobID, res.MSPID, res.DeviceID).Scan(&currentStatus, &agentID, &currentAttempt, &clientID, &siteID, &correlationID)
	if err != nil || agentID != res.AgentID || currentAttempt != res.Attempt ||
		clientID != res.ClientID || siteID != res.SiteID || correlationID != res.CorrelationID {
		d.logger.Warn("result ownership mismatch", zap.String("target", res.TargetID), zap.Error(err))
		return
	}
	currentTerminal := currentStatus == "succeeded" || currentStatus == "failed" ||
		currentStatus == "cancelled" || currentStatus == "expired"
	resultTerminal := res.Status == "succeeded" || res.Status == "failed" ||
		res.Status == "cancelled" || res.Status == "expired"
	if currentTerminal && resultTerminal && (currentStatus == res.Status || currentStatus == "cancelled") {
		if _, err := tx.Exec(`UPDATE job_inbox SET processed_at=NOW() WHERE id::text=$1`, inboxID); err != nil {
			return
		}
		if err := tx.Commit(); err != nil {
			return
		}
		d.publishResultReceipt(*res)
		return
	}
	if err := TransitionJob(currentStatus, res.Status); err != nil {
		d.logger.Warn("invalid result transition", zap.Error(err))
		return
	}
	resultJSON, err := json.Marshal(res.Result)
	if err != nil {
		return
	}
	if _, err := tx.Exec(`
		UPDATE job_targets SET status=$1, result=$2, error_message=NULLIF($3,''), exit_code=$4,
			completed_at=NOW(), lease_owner=NULL, lease_expires=NULL WHERE id=$5
	`, res.Status, resultJSON, res.Error, res.ExitCode, res.TargetID); err != nil {
		return
	}
	if _, err := tx.Exec(`UPDATE job_inbox SET processed_at=NOW() WHERE id::text=$1`, inboxID); err != nil {
		return
	}
	if _, err := tx.Exec(`
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
		return
	}
	if err := tx.Commit(); err != nil {
		return
	}
	d.publishResultReceipt(*res)
}

func (d *Dispatcher) publishResultReceipt(res ResultEnvelope) {
	data, err := json.Marshal(map[string]interface{}{
		"schema_version": CurrentSchemaVersion,
		"message_id":     res.MessageID,
		"event_id":       res.EventID,
		"received_at":    time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return
	}
	subject := fmt.Sprintf("tenant.%s.agent.%s.result.ack", res.MSPID, res.AgentID)
	if err := d.nc.Publish(subject, data); err != nil {
		d.logger.Warn("publish result receipt", zap.Error(err))
	}
}

func backoffDuration(attempt int) time.Duration {
	base := time.Second * 30
	max := time.Minute * 30
	d := float64(base) * math.Pow(2, float64(attempt-1))
	jitter := float64(0)
	if value, err := rand.Int(rand.Reader, big.NewInt(int64(base))); err == nil {
		jitter = float64(value.Int64())
	}
	return time.Duration(math.Min(d+jitter, float64(max)))
}

func (d *Dispatcher) failOutbox(id string, publishErr error, attempt int) {
	delay := backoffDuration(attempt)
	_, err := d.db.DB().Exec(`
		UPDATE job_outbox
		SET last_error = $1, lease_owner = NULL, lease_expires = NULL,
		    available_at = NOW() + $2::interval
		WHERE id = $3 AND lease_owner = $4
	`, publishErr.Error(), delay.String(), id, d.workerID)
	if err != nil {
		d.logger.Error("record outbox failure", zap.String("id", id), zap.Error(err))
	}
}

func intFromJSON(value interface{}) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}
