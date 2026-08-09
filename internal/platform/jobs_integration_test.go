//go:build jobintegration

package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"

	agentjobs "github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/jobs"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func TestDurableJobRoundTripWithRealPostgresAndNATS(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	natsURL := os.Getenv("TEST_NATS_URL")
	if dsn == "" || natsURL == "" {
		t.Fatal("TEST_POSTGRES_DSN and TEST_NATS_URL are required")
	}
	rawDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := postgres.NewSchemaManager(rawDB).Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := timescale.NewClient(ctx, dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	logger := zap.NewNop()

	const (
		mspID    = "10000000-0000-0000-0000-000000000001"
		clientID = "10000000-0000-0000-0000-000000000002"
		jobID    = "10000000-0000-0000-0000-000000000003"
		targetID = "10000000-0000-0000-0000-000000000004"
	)
	if _, err := db.DB().Exec(`INSERT INTO msp_tenants (id, name, slug) VALUES ($1, 'Integration MSP', 'integration-msp')`, mspID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO client_organizations (id, msp_id, name, slug)
		VALUES ($1, $2, 'Integration Client', 'integration-client')
	`, clientID, mspID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO jobs (
			id, msp_id, client_id, created_by, type, status, payload, max_retries,
			max_devices, expires_at, correlation_id, scheduled_for, updated_at
		) VALUES (
			$1, $2, $3, 'integration', 'test', 'queued', '{"value":"ok"}', 3,
			1, NOW() + INTERVAL '5 minutes', 'integration-correlation', NOW(), NOW()
		)
	`, jobID, mspID, clientID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO job_targets (id, job_id, device_id, status, agent_id, msp_id)
		VALUES ($1, $2, 'device-1', 'queued', 'agent-1', $3)
	`, targetID, jobID, mspID); err != nil {
		t.Fatal(err)
	}

	ledgerDB, err := bbolt.Open(filepath.Join(t.TempDir(), "receipts.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := ledgerDB.Close(); err != nil {
			t.Errorf("close ledger: %v", err)
		}
	}()
	ledger, err := agentjobs.NewReceiptLedger(ledgerDB, logger)
	if err != nil {
		t.Fatal(err)
	}
	registry := agentjobs.NewHandlerRegistry()
	var executions atomic.Int32
	registry.Register("test", func(context.Context, *agentjobs.CommandEnvelope) (string, int, []byte, error) {
		executions.Add(1)
		return agentjobs.StateSucceeded, 0, []byte(`{"value":"ok"}`), nil
	})
	agent := agentjobs.NewJobDispatcher(nc, ledger, registry, logger, mspID, "agent-1")
	if err := agent.Start(ctx); err != nil {
		t.Fatal(err)
	}
	server := NewDispatcher(db, nc, logger)
	server.Start(ctx)

	defer func() {
		cancel()
		server.Stop()
		if err := agent.Stop(); err != nil {
			t.Errorf("stop agent dispatcher: %v", err)
		}
	}()

	// Wait for the asynchronous dispatcher instead of relying on a fixed startup
	// sleep. The publisher polls every 200ms, but race-enabled CI runners can be
	// heavily contended; keep this bounded while allowing scheduling jitter.
	dispatchDeadline := time.Now().Add(15 * time.Second)
	waitForJobDispatched(t, db.DB(), jobID, targetID, dispatchDeadline)

	// Wait for the job target to transition from "dispatched" to "succeeded".
	// The agent dispatcher must first receive the dispatch command, execute the
	// handler, and then update the target status. The agent replays unacknowledged
	// terminal results every 15 seconds, so allow one complete replay interval so
	// a transient serializable-transaction conflict still exercises and proves the
	// durable recovery path.
	waitForTargetStatus(t, db.DB(), targetID, "succeeded")
	if executions.Load() != 1 {
		t.Fatalf("handler executed %d times, want 1", executions.Load())
	}
	var payload []byte
	if err := db.DB().QueryRow(`
		SELECT payload::text FROM job_outbox
		WHERE aggregate_id=$1 AND event_type='job.dispatch'
		ORDER BY created_at DESC LIMIT 1
	`, jobID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["event_id"] == "" || envelope["command_type"] != "test" {
		t.Fatalf("invalid dispatched envelope: %s", payload)
	}

	if err := nc.Publish("tenant."+mspID+".cmd.agent-1", payload); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	if executions.Load() != 1 {
		t.Fatalf("duplicate command executed handler %d times", executions.Load())
	}

	waitForResultReceipt(t, ledger)
}

func waitForTargetStatus(t *testing.T, db *sql.DB, targetID, want string) {
	t.Helper()
	// The agent replays an unacknowledged terminal result every 15 seconds.
	// Allow one complete replay interval so a transient serializable-transaction
	// conflict still exercises and proves the durable recovery path.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err := db.QueryRow(`SELECT status FROM job_targets WHERE id=$1`, targetID).Scan(&status); err == nil && status == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM job_targets WHERE id=$1`, targetID).Scan(&status)
	t.Fatalf("target status=%q, want %q", status, want)
}

func waitForJobDispatched(t *testing.T, db *sql.DB, jobID, targetID string, deadline time.Time) {
	t.Helper()
	for time.Now().Before(deadline) {
		var targetStatus, jobStatus string
		err := db.QueryRow(`
			SELECT jt.status, j.status
			FROM job_targets jt
			JOIN jobs j ON j.id = jt.job_id
			WHERE jt.id = $1
		`, targetID).Scan(&targetStatus, &jobStatus)
		if err == nil && (targetStatus == "dispatched" || targetStatus == "running" || targetStatus == "succeeded") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	var targetStatus, jobStatus string
	statusErr := db.QueryRow(`
		SELECT jt.status, j.status
		FROM job_targets jt
		JOIN jobs j ON j.id = jt.job_id
		WHERE jt.id = $1
	`, targetID).Scan(&targetStatus, &jobStatus)

	var outboxCount, publishedCount int
	var lastError sql.NullString
	outboxErr := db.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE published_at IS NOT NULL),
		       MAX(last_error)
		FROM job_outbox
		WHERE aggregate_id = $1 AND event_type = 'job.dispatch'
	`, jobID).Scan(&outboxCount, &publishedCount, &lastError)

	t.Fatalf(
		"job was not dispatched within deadline: target_status=%q job_status=%q status_err=%v outbox_count=%d published_count=%d last_error=%q outbox_err=%v",
		targetStatus, jobStatus, statusErr, outboxCount, publishedCount, lastError.String, outboxErr,
	)
}

func waitForResultReceipt(t *testing.T, ledger *agentjobs.ReceiptLedger) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		receipts, err := ledger.GetUnacknowledgedResults()
		if err == nil && len(receipts) == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	receipts, err := ledger.GetUnacknowledgedResults()
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("server result receipt did not terminate replay: %#v", receipts)
}
