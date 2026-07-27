package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
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
}

func NewDispatcher(db *timescale.Client, nc *nats.Conn, logger *zap.Logger) *Dispatcher {
	return &Dispatcher{
		db:     db,
		nc:     nc,
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	d.wg.Add(1)
	go d.outboxPublisher(ctx)
	d.wg.Add(1)
	go d.dispatchWorker(ctx)
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
			d.processOutbox()
		}
	}
}

func (d *Dispatcher) processOutbox() {
	rows, err := d.db.DB().Query(`
		UPDATE job_outbox SET lease_owner = 'dispatcher', lease_expires = NOW() + INTERVAL '30 seconds',
		                       attempt_count = attempt_count + 1
		WHERE id IN (
			SELECT id FROM job_outbox
			WHERE published_at IS NULL AND available_at <= NOW()
			      AND (lease_expires IS NULL OR lease_expires < NOW())
			ORDER BY created_at ASC LIMIT 20
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, msp_id, aggregate_id, event_type, payload::text
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, mspID, aggregateID, eventType, payloadStr string
		if err := rows.Scan(&id, &mspID, &aggregateID, &eventType, &payloadStr); err != nil {
			continue
		}

		var payload map[string]interface{}
		json.Unmarshal([]byte(payloadStr), &payload)

		deviceID, _ := payload["device_id"].(string)
		if deviceID == "" {
			continue
		}

		subject := fmt.Sprintf("tenant.%s.cmd.%s", mspID, deviceID)
		if err := d.nc.Publish(subject, []byte(payloadStr)); err != nil {
			d.logger.Warn("outbox publish failed", zap.String("id", id), zap.Error(err))
			d.db.DB().Exec(`UPDATE job_outbox SET last_error = $1 WHERE id = $2`, err.Error(), id)
			continue
		}

		d.db.DB().Exec(`UPDATE job_outbox SET published_at = NOW(), lease_owner = NULL, lease_expires = NULL WHERE id = $1`, id)
	}

	for rows.NextResultSet() {
	}
}

func (d *Dispatcher) dispatchWorker(ctx context.Context) {
	defer d.wg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.processDispatches()
			d.expireStaleWork()
		}
	}
}

func (d *Dispatcher) processDispatches() {
	rows, err := d.db.DB().Query(`
		UPDATE job_targets SET status = 'dispatched', dispatched_at = NOW(),
		                       attempt = attempt + 1,
		                       lease_owner = 'dispatcher',
		                       lease_expires = CASE WHEN lease_owner IS NULL THEN NOW() + INTERVAL '2 minutes'
		                                            ELSE lease_expires + INTERVAL '1 minute' END
		WHERE id IN (
			SELECT id FROM job_targets
			WHERE status = 'queued'
			      AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			      AND retry_count <= (SELECT COALESCE(max_retries, 3) FROM jobs WHERE id = job_id)
			ORDER BY created_at ASC LIMIT 10
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, job_id, device_id, msp_id
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var targetID, jobID, deviceID, mspID string
		if err := rows.Scan(&targetID, &jobID, &deviceID, &mspID); err != nil {
			continue
		}

		// Build command envelope
		cmd := map[string]interface{}{
			"schema_version": 1,
			"event_id":       uuid.New().String(),
			"job_id":         jobID,
			"target_id":      targetID,
			"msp_id":         mspID,
			"device_id":      deviceID,
			"attempt":        1,
			"issued_at":      time.Now().UTC().Format(time.RFC3339),
			"command_type":   "execute",
		}
		cmdJSON, _ := json.Marshal(cmd)

		subject := fmt.Sprintf("tenant.%s.cmd.%s", mspID, deviceID)
		if err := d.nc.Publish(subject, cmdJSON); err != nil {
			d.logger.Warn("dispatch publish failed", zap.String("target", targetID), zap.Error(err))
		}

		d.db.DB().Exec(`UPDATE jobs SET dispatch_count = dispatch_count + 1, updated_at = NOW() WHERE id = $1`, jobID)
	}
}

func (d *Dispatcher) expireStaleWork() {
	d.db.DB().Exec(`
		UPDATE job_targets SET status = 'expired'
		WHERE status IN ('pending', 'queued', 'dispatched', 'running')
		      AND id IN (
			SELECT jt.id FROM job_targets jt
			JOIN jobs j ON jt.job_id = j.id
			WHERE j.expires_at < NOW() OR (
			      jt.lease_owner IS NOT NULL AND jt.lease_expires < NOW()
			)
			LIMIT 50
		)
	`)
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
	d.db.DB().Exec(`
		UPDATE job_targets SET status = 'queued', lease_owner = NULL, lease_expires = NULL
		WHERE status = 'dispatched' AND lease_owner IS NOT NULL AND lease_expires < NOW()
		      AND id IN (SELECT id FROM job_targets WHERE lease_expires < NOW() LIMIT 50)
	`)
	// Claim expired agent leases (running but no heartbeat)
	d.db.DB().Exec(`
		UPDATE job_targets SET status = 'failed', error_message = 'execution timeout'
		WHERE status = 'running' AND lease_expires < NOW() - INTERVAL '10 minutes'
		      AND id IN (SELECT id FROM job_targets WHERE lease_expires < NOW() LIMIT 50)
	`)
	// Aggregate job state from target states
	d.db.DB().Exec(`
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
	`)
}

func backoffDuration(attempt int) time.Duration {
	base := time.Second * 30
	max := time.Minute * 30
	d := float64(base) * math.Pow(2, float64(attempt-1))
	jitter := rand.Float64() * float64(base)
	return time.Duration(math.Min(d+jitter, float64(max)))
}
