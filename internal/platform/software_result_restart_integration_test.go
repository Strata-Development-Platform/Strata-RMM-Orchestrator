//go:build jobintegration

package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"

	agentjobs "github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/jobs"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/software"
	jsmsg "github.com/strata-rmm/strata-rmm-orchestrator/internal/messaging/jetstream"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/testsupport"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func TestSoftwareResultRetainedWhilePlatformDispatcherOffline(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	candidateNATS := os.Getenv("TEST_NATS_URL")
	if dsn == "" || candidateNATS == "" {
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
	if err := postgres.ApplyDurabilitySchema(context.Background(), rawDB); err != nil {
		t.Fatalf("apply production durability schema: %v", err)
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

	jsURL, cleanupJS, err := testsupport.EnsureJetStreamURL(context.Background(), candidateNATS)
	if err != nil {
		t.Fatalf("provision JetStream integration endpoint: %v", err)
	}
	defer cleanupJS()
	nc, err := nats.Connect(jsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	logger := zap.NewNop()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := jsmsg.NewStreamManager(js, jsmsg.Default(), logger).EnsureStreams(context.Background()); err != nil {
		t.Fatalf("ensure durable streams: %v", err)
	}

	const (
		tenantID = "40000000-0000-0000-0000-000000000001"
		mspID    = "40000000-0000-0000-0000-000000000002"
		clientID = "40000000-0000-0000-0000-000000000003"
		siteID   = "40000000-0000-0000-0000-000000000004"
		deviceID = "40000000-0000-0000-0000-000000000005"
		agentID  = "40000000-0000-0000-0000-000000000006"
	)

	mustExec := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := db.DB().Exec(query, args...); err != nil {
			t.Fatalf("seed software outage integration: %v\nquery: %s", err, query)
		}
	}
	mustExec(`INSERT INTO tenants (id, name, slug, plan) VALUES ($1, 'Software Outage Tenant', 'software-outage-tenant', 'enterprise')`, tenantID)
	mustExec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Software Outage MSP', 'software-outage-msp', TRUE)`, mspID)
	mustExec(`INSERT INTO client_organizations (id, msp_id, name, slug, is_active) VALUES ($1, $2, 'Software Outage Client', 'software-outage-client', TRUE)`, clientID, mspID)
	mustExec(`INSERT INTO sites (id, client_id, name, slug, is_active) VALUES ($1, $2, 'Software Outage Site', 'software-outage-site', TRUE)`, siteID, clientID)
	mustExec(`
		INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, agent_id, hostname, status, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, 'software-outage-endpoint', 'online', TRUE)
	`, deviceID, mspID, clientID, siteID, tenantID, agentID)

	sideEffect := filepath.Join(t.TempDir(), "software-side-effects.log")
	script := fmt.Sprintf("#!/bin/sh\nprintf 'executed\\n' >> %q\nexit 0\n", sideEffect)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(script))
	}))
	defer server.Close()
	sum := sha256.Sum256([]byte(script))
	checksum := hex.EncodeToString(sum[:])

	var packageID string
	if err := db.DB().QueryRow(`
		INSERT INTO software_packages (
			tenant_id, name, version, description, platform, package_type,
			source_url, checksum, install_args, uninstall_args, detect_command
		) VALUES ($1, 'Outage Script', '1.0', 'jobintegration', 'linux', 'script', $2, $3, '', '', '')
		RETURNING id::text
	`, tenantID, server.URL+"/install.sh", checksum).Scan(&packageID); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]interface{}{
		"package_id": packageID,
		"name":       "Software Result Outage",
		"device_ids": []string{deviceID},
		"action":     "install",
	})
	if err != nil {
		t.Fatal(err)
	}
	api := &APIServer{db: db, logger: logger}
	tx, err := db.DB().BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/software/deployments/"+tenantID, bytes.NewReader(body))
	req.SetPathValue("tenantID", tenantID)
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyDBTransaction, tx))
	rr := httptest.NewRecorder()
	api.handleCreateDurableSoftwareDeployment(rr, req)
	if rr.Code != http.StatusCreated {
		_ = tx.Rollback()
		t.Fatalf("create software deployment status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var created struct {
		DeploymentID string `json:"deployment_id"`
		JobID        string `json:"job_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	var targetID, outboxPayload string
	if err := db.DB().QueryRow(`
		SELECT jt.id::text, o.payload::text
		FROM job_targets jt
		JOIN job_outbox o ON o.aggregate_id = jt.job_id AND o.event_type = 'job.dispatch'
		WHERE jt.job_id = $1
		LIMIT 1
	`, created.JobID).Scan(&targetID, &outboxPayload); err != nil {
		t.Fatal(err)
	}
	var canonicalDispatch struct {
		Attempt int `json:"attempt"`
	}
	if err := json.Unmarshal([]byte(outboxPayload), &canonicalDispatch); err != nil {
		t.Fatalf("decode canonical dispatch envelope: %v", err)
	}
	if canonicalDispatch.Attempt < 1 {
		t.Fatalf("canonical dispatch envelope has invalid attempt %d", canonicalDispatch.Attempt)
	}

	// Model the real processOutbox crash boundary. Production commits the target
	// and job as dispatched before NATS publish so a fast result can never race a
	// database target that still says queued. We deliberately leave published_at
	// unset to represent a crash after publish but before outbox finalization;
	// restart may therefore redeliver the command and must still execute once.
	dispatchTx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := dispatchTx.ExecContext(ctx, `
		UPDATE job_targets
		SET status='dispatched', dispatched_at=COALESCE(dispatched_at, NOW()), attempt=$2,
		    lease_owner=NULL, lease_expires=NOW() + INTERVAL '2 minutes'
		WHERE id=$1 AND status='queued'
	`, targetID, canonicalDispatch.Attempt)
	if err != nil {
		_ = dispatchTx.Rollback()
		t.Fatalf("persist pre-publish target state: %v", err)
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		_ = dispatchTx.Rollback()
		t.Fatalf("persist pre-publish target state affected=%d err=%v", affected, rowsErr)
	}
	if _, err := dispatchTx.ExecContext(ctx, `
		UPDATE jobs
		SET dispatch_count=dispatch_count + 1,
		    status=CASE WHEN status IN ('pending','queued') THEN 'dispatched' ELSE status END,
		    updated_at=NOW()
		WHERE id=$1
	`, created.JobID); err != nil {
		_ = dispatchTx.Rollback()
		t.Fatalf("persist pre-publish job state: %v", err)
	}
	if err := dispatchTx.Commit(); err != nil {
		t.Fatalf("commit pre-publish dispatch state: %v", err)
	}

	ledgerDB, err := bbolt.Open(filepath.Join(t.TempDir(), "software-outage-receipts.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ledgerDB.Close() }()
	ledger, err := agentjobs.NewReceiptLedger(ledgerDB, logger)
	if err != nil {
		t.Fatal(err)
	}
	installer := software.NewInstaller(nc, logger, mspID, agentID)
	registry := agentjobs.NewHandlerRegistry()
	registry.Register("software_install", func(handlerCtx context.Context, command *agentjobs.CommandEnvelope) (string, int, []byte, error) {
		return installer.RunDurableJob(handlerCtx, command.Payload, "install")
	})
	agent := agentjobs.NewJobDispatcher(nc, ledger, registry, logger, mspID, agentID)
	if err := agent.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = agent.Stop() }()

	// The platform dispatcher is intentionally not running here. Publish the
	// transactionally-created canonical outbox envelope to simulate the command
	// leaving the orchestrator immediately before the process dies.
	if err := nc.Publish("tenant."+mspID+".cmd."+agentID, []byte(outboxPayload)); err != nil {
		t.Fatal(err)
	}
	if err := nc.FlushTimeout(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(sideEffect)
		pending, ledgerErr := ledger.GetUnacknowledgedResults()
		if readErr == nil && strings.Count(string(data), "executed\n") == 1 && ledgerErr == nil && len(pending) == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	data, readErr := os.ReadFile(sideEffect)
	pending, ledgerErr := ledger.GetUnacknowledgedResults()
	if readErr != nil || strings.Count(string(data), "executed\n") != 1 || ledgerErr != nil || len(pending) != 1 {
		t.Fatalf("software did not finish with retained result while platform offline: side_effect=%q read_err=%v pending=%d ledger_err=%v", data, readErr, len(pending), ledgerErr)
	}

	var beforeTarget, beforeLegacy string
	if err := db.DB().QueryRow(`SELECT status FROM job_targets WHERE id=$1`, targetID).Scan(&beforeTarget); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT status FROM software_deployment_targets WHERE deployment_id=$1 AND device_id=$2`, created.DeploymentID, deviceID).Scan(&beforeLegacy); err != nil {
		t.Fatal(err)
	}
	if beforeTarget != "dispatched" || beforeLegacy == "success" {
		t.Fatalf("software outage boundary is not production-equivalent: target=%q legacy=%q", beforeTarget, beforeLegacy)
	}

	// A fresh dispatcher instance represents orchestrator restart. Its durable
	// result consumer must replay the retained result. Its outbox worker may also
	// redeliver the original command because published_at was not finalized; the
	// agent receipt ledger must suppress a second side effect and replay the exact
	// persisted terminal result instead.
	restarted := NewDispatcher(db, nc, logger)
	restarted.Start(ctx)
	defer restarted.Stop()

	waitForTargetStatus(t, db.DB(), targetID, "succeeded")
	convergeDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(convergeDeadline) {
		var legacyTarget, deployment string
		err := db.DB().QueryRow(`
			SELECT t.status, d.status
			FROM software_deployment_targets t
			JOIN software_deployments d ON d.id=t.deployment_id
			WHERE t.deployment_id=$1 AND t.device_id=$2
		`, created.DeploymentID, deviceID).Scan(&legacyTarget, &deployment)
		pending, ledgerErr = ledger.GetUnacknowledgedResults()
		data, readErr = os.ReadFile(sideEffect)
		if err == nil && legacyTarget == "success" && deployment == "completed" &&
			ledgerErr == nil && len(pending) == 0 && readErr == nil && strings.Count(string(data), "executed\n") == 1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	var legacyTarget, deployment string
	_ = db.DB().QueryRow(`
		SELECT t.status, d.status FROM software_deployment_targets t
		JOIN software_deployments d ON d.id=t.deployment_id
		WHERE t.deployment_id=$1 AND t.device_id=$2
	`, created.DeploymentID, deviceID).Scan(&legacyTarget, &deployment)
	data, _ = os.ReadFile(sideEffect)
	pending, _ = ledger.GetUnacknowledgedResults()
	t.Fatalf("retained software result did not converge exactly once after dispatcher restart: legacy=%q deployment=%q side_effects=%d pending_results=%d",
		legacyTarget, deployment, strings.Count(string(data), "executed\n"), len(pending))
}
