package platform

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

type Dispatcher struct {
	db     *timescale.Client
	nc     *nats.Conn
	logger *zap.Logger
	stopCh chan struct{}
	wg     sync.WaitGroup
	workerID string
}

func NewDispatcher(db *timescale.Client, nc *nats.Conn, logger *zap.Logger) *Dispatcher {
	host, _ := os.Hostname()
	return &Dispatcher{
		db:     db,
		nc:     nc,
		logger: logger,
		stopCh: make(chan struct{}),
		workerID: fmt.Sprintf("%s-%s", host, uuid.NewString()),
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	d.wg.Add(1)
	go d.outboxPublisher(ctx)
	d.wg.Add(1)
	go d.reconciliationWorker(ctx)
	d.logger.Info("job dispatcher started")
}

func (d *Dispatcher) Stop() {
	close(d.stopCh)
	d.wg.Wait()
	d.logger.Info("job dispatcher stopped")
}

func (d *Dispatcher) outboxPublisher(ctx context.Context) {
	defer d.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.ensureQueuedOutbox()
			d.processOutbox()
			d.expireJobs()
		}
	}
}

func (d *Dispatcher) ensureQueuedOutbox() {
	_, err := d.db.DB().Exec(`
		INSERT INTO job_outbox (id, msp_id, aggregate_id, event_type, payload, available_at)
		SELECT gen_random_uuid(), j.msp_id, j.id, 'job.dispatch',
		       jsonb_build_object(
		         'schema_version', 1,
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
		         'type', j.type,
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
		if agentID == "" || targetID == "" || attempt < 1 {
			d.failOutbox(id, fmt.Errorf("missing agent_id, target_id, or attempt"), 1)
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

		tx, err := d.db.DB().Begin()
		if err != nil {
			d.failOutbox(id, err, attempt)
			continue
		}
		if _, err = tx.Exec(`
			UPDATE job_outbox
			SET published_at = NOW(), lease_owner = NULL, lease_expires = NULL, last_error = NULL
			WHERE id = $1 AND lease_owner = $2
		`, id, d.workerID); err == nil {
			_, err = tx.Exec(`
				UPDATE job_targets
				SET status = 'dispatched', dispatched_at = NOW(), attempt = $2,
				    lease_owner = NULL, lease_expires = NOW() + INTERVAL '2 minutes'
				WHERE id = $1 AND status = 'queued' AND attempt < $2
			`, targetID, attempt)
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE jobs SET dispatch_count = dispatch_count + 1, status = 'dispatched', updated_at = NOW() WHERE id = $1`, aggregateID)
		}
		if err != nil {
			_ = tx.Rollback()
			d.logger.Error("finalize outbox publish", zap.String("id", id), zap.Error(err))
			continue
		}
		if err := tx.Commit(); err != nil {
			d.logger.Error("commit outbox publish", zap.String("id", id), zap.Error(err))
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
			d.reconcile()
		}
	}
}

func (d *Dispatcher) reconcile() {
	// Claim expired dispatcher leases
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
		    retry_count = retry_count + 1,
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
	// Aggregate job state from target states
	if _, err := d.db.DB().Exec(`
		UPDATE jobs j SET status = CASE
			WHEN (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status IN ('pending','queued','dispatched','running')) = 0
			     AND (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status = 'failed') > 0
			     AND (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status = 'succeeded') = 0
			THEN 'failed'
			WHEN (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status IN ('pending','queued','dispatched','running')) = 0
			     AND (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status = 'failed') = 0
			THEN 'succeeded'
			WHEN (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status IN ('pending','queued','dispatched','running')) = 0
			     AND (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status = 'failed') > 0
			THEN 'failed'
			ELSE j.status END,
			completed_at = CASE WHEN (SELECT count(*) FROM job_targets WHERE job_id = j.id AND status IN ('pending','queued','dispatched','running')) = 0
			              THEN NOW() ELSE NULL END,
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
