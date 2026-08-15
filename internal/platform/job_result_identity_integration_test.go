//go:build jobintegration

package platform

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	agentjobs "github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/jobs"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func TestStaleOlderAttemptCannotOverwriteCurrentTarget(t *testing.T) {
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

	ctx := context.Background()
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

	const (
		mspID         = "30000000-0000-0000-0000-000000000001"
		clientID      = "30000000-0000-0000-0000-000000000002"
		jobID         = "30000000-0000-0000-0000-000000000003"
		targetID      = "30000000-0000-0000-0000-000000000004"
		deviceID      = "identity-device"
		agentID       = "identity-agent"
		correlationID = "identity-correlation"
	)
	if _, err := db.DB().Exec(`INSERT INTO msp_tenants (id, name, slug) VALUES ($1, 'Identity MSP', 'identity-msp')`, mspID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO client_organizations (id, msp_id, name, slug) VALUES ($1, $2, 'Identity Client', 'identity-client')`, clientID, mspID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO jobs (id, msp_id, client_id, created_by, type, status, payload, max_retries, max_devices, correlation_id, scheduled_for, updated_at)
		VALUES ($1, $2, $3, 'integration', 'test', 'dispatched', '{}'::jsonb, 3, 1, $4, NOW(), NOW())
	`, jobID, mspID, clientID, correlationID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO job_targets (id, job_id, device_id, status, agent_id, msp_id, attempt, retry_count)
		VALUES ($1, $2, $3, 'running', $4, $5, 2, 1)
	`, targetID, jobID, deviceID, agentID, mspID); err != nil {
		t.Fatal(err)
	}

	dispatcher := NewDispatcher(db, nc, zap.NewNop())
	subject := "tenant." + mspID + ".agent." + agentID + ".result"

	_, staleEnvelope, err := agentjobs.MarshalResult(
		"", "event-attempt-1", jobID, targetID, mspID, clientID, "", deviceID, agentID,
		correlationID, 1, agentjobs.StateSucceeded, 0, []byte(`{"attempt":1}`), "",
		time.Now().Add(-time.Minute), time.Now().Add(-30*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.handleAgentResult(subject, staleEnvelope)

	var status string
	var attempt int
	var resultText sql.NullString
	if err := db.DB().QueryRow(`SELECT status, attempt, result::text FROM job_targets WHERE id=$1`, targetID).Scan(&status, &attempt, &resultText); err != nil {
		t.Fatal(err)
	}
	if status != "running" || attempt != 2 {
		t.Fatalf("stale attempt mutated current target: status=%s attempt=%d", status, attempt)
	}
	if resultText.Valid && resultText.String != "null" && resultText.String != "" {
		t.Fatalf("stale attempt wrote result: %s", resultText.String)
	}

	_, currentEnvelope, err := agentjobs.MarshalResult(
		"", "event-attempt-2", jobID, targetID, mspID, clientID, "", deviceID, agentID,
		correlationID, 2, agentjobs.StateSucceeded, 0, []byte(`{"attempt":2}`), "",
		time.Now().Add(-10*time.Second), time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.handleAgentResult(subject, currentEnvelope)

	var jobStatus string
	if err := db.DB().QueryRow(`SELECT status FROM job_targets WHERE id=$1`, targetID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT status FROM jobs WHERE id=$1`, jobID).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || jobStatus != "succeeded" {
		t.Fatalf("current attempt did not converge: target=%s job=%s", status, jobStatus)
	}
}
