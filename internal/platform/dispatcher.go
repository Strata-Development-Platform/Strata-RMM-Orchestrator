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
			d.failOutbox(id, fmt.Errorf("missing agent_id or target_id"), maxInt(attempt, 1))
			continue
		}

		if eventType == "job.cancel" {
			subject := fmt.Sprintf("tenant.%s.cmd.%s.cancel", mspID, agentID)
			if err := d.nc.Publish(subject, []byte(payloadStr)); err != nil {
				d.failOutbox(id, err, maxInt(attempt, 1))
				continue
			}
			if err := d.nc.FlushTimeout(5 * time.Second); err != nil {
				d.failOutbox(id, err, maxInt(attempt, 1))
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
			d.failOutbox(id, fmt.Errorf("unsupported outbox event type %q", eventType), maxInt(attempt, 1))
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
