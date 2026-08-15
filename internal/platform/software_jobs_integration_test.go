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

func TestDurableSoftwareDeploymentLifecycleWithRealPostgresAndJetStream(t *testing.T) {
	if os.Getenv("TEST_POSTGRES_DSN") == "" || os.Getenv("TEST_NATS_URL") == "" {
		t.Fatal("TEST_POSTGRES_DSN and TEST_NATS_URL are required")
	}

	dsn := os.Getenv("TEST_POSTGRES_DSN")
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

	jetStreamURL, cleanupJetStream, err := testsupport.EnsureJetStreamURL(context.Background(), os.Getenv("TEST_NATS_URL"))
	if err != nil {
		t.Fatalf("provision JetStream integration endpoint: %v", err)
	}
	defer cleanupJetStream()
	nc, err := nats.Connect(jetStreamURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	logger := zap.NewNop()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("JetStream is required for software durability integration: %v", err)
	}
	if err := jsmsg.NewStreamManager(js, jsmsg.Default(), logger).EnsureStreams(context.Background()); err != nil {
		t.Fatalf("provision durable streams: %v", err)
	}

	const (
		tenantID = "20000000-0000-0000-0000-000000000001"
		mspID    = "20000000-0000-0000-0000-000000000002"
		clientID = "20000000-0000-0000-0000-000000000003"
		siteID   = "20000000-0000-0000-0000-000000000004"
		deviceID = "20000000-0000-0000-0000-000000000005"
		agentID  = "20000000-0000-0000-0000-000000000006"
	)

	if _, err := db.DB().Exec(`INSERT INTO tenants (id, name, slug, plan) VALUES ($1, 'Software Tenant', 'software-tenant', 'enterprise')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Software MSP', 'software-msp', TRUE)`, mspID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO client_organizations (id, msp_id, name, slug, is_active) VALUES ($1, $2, 'Software Client', 'software-client', TRUE)`, clientID, mspID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO sites (id, client_id, name, slug, is_active) VALUES ($1, $2, 'Software Site', 'software-site', TRUE)`, siteID, clientID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, agent_id, hostname, status, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, 'software-endpoint', 'online', TRUE)
	`, deviceID, mspID, clientID, siteID, tenantID, agentID); err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(t.TempDir(), "software-installed.marker")
	script := fmt.Sprintf("#!/bin/sh\nprintf installed > %q\n", marker)
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(script))
	}))
	defer fileServer.Close()
	sum := sha256.Sum256([]byte(script))
	checksum := hex.EncodeToString(sum[:])

	var packageID string
	if err := db.DB().QueryRow(`
		INSERT INTO software_packages (
			tenant_id, name, version, description, platform, package_type,
			source_url, checksum, install_args, uninstall_args, detect_command
		) VALUES ($1, 'Integration Script', '1.0', 'jobintegration', 'linux', 'script', $2, $3, '', '', '')
		RETURNING id::text
	`, tenantID, fileServer.URL+"/install.sh", checksum).Scan(&packageID); err != nil {
		t.Fatal(err)
	}

	requestBody, err := json.Marshal(map[string]interface{}{
		"package_id":    packageID,
		"name":          "Durable Software E2E",
		"device_ids":    []string{deviceID},
		"action":        "install",
		"schedule_type": "now",
	})
	if err != nil {
		t.Fatal(err)
	}

	api := &APIServer{db: db, logger: logger}
	tx, err := db.DB().BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/software/deployments/"+tenantID, bytes.NewReader(requestBody))
	req.SetPathValue("tenantID", tenantID)
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyDBTransaction, tx))
	recorder := httptest.NewRecorder()
	api.handleCreateDurableSoftwareDeployment(recorder, req)
	if recorder.Code != http.StatusCreated {
		_ = tx.Rollback()
		t.Fatalf("create deployment status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var created struct {
		DeploymentID string `json:"deployment_id"`
		JobID        string `json:"job_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.DeploymentID == "" || created.JobID == "" {
		t.Fatalf("missing durable identifiers: %s", recorder.Body.String())
	}

	var targetID string
	if err := db.DB().QueryRow(`SELECT id::text FROM job_targets WHERE job_id = $1`, created.JobID).Scan(&targetID); err != nil {
		t.Fatal(err)
	}

	ledgerDB, err := bbolt.Open(filepath.Join(t.TempDir(), "software-receipts.db"), 0600, nil)
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
	agentDispatcher := agentjobs.NewJobDispatcher(nc, ledger, registry, logger, mspID, agentID)
	if err := agentDispatcher.Start(ctx); err != nil {
		t.Fatal(err)
	}
	platformDispatcher := NewDispatcher(db, nc, logger)
	platformDispatcher.Start(ctx)
	defer func() {
		cancel()
		platformDispatcher.Stop()
		if err := agentDispatcher.Stop(); err != nil {
			t.Errorf("stop software agent dispatcher: %v", err)
		}
	}()

	waitForTargetStatus(t, db.DB(), targetID, "succeeded")

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var legacyTargetStatus, legacyDeploymentStatus string
		err := db.DB().QueryRow(`
			SELECT t.status, d.status
			FROM software_deployment_targets t
			JOIN software_deployments d ON d.id = t.deployment_id
			WHERE t.deployment_id = $1 AND t.device_id = $2
		`, created.DeploymentID, deviceID).Scan(&legacyTargetStatus, &legacyDeploymentStatus)
		if err == nil && legacyTargetStatus == "success" && legacyDeploymentStatus == "completed" {
			if data, statErr := os.ReadFile(marker); statErr != nil || string(data) != "installed" {
				t.Fatalf("software side effect missing after successful durable lifecycle: data=%q err=%v", data, statErr)
			}
			waitForResultReceipt(t, ledger)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	var targetStatus, deploymentStatus string
	_ = db.DB().QueryRow(`SELECT status FROM software_deployment_targets WHERE deployment_id=$1 AND device_id=$2`, created.DeploymentID, deviceID).Scan(&targetStatus)
	_ = db.DB().QueryRow(`SELECT status FROM software_deployments WHERE id=$1`, created.DeploymentID).Scan(&deploymentStatus)
	t.Fatalf("software legacy aggregate did not converge: target=%q deployment=%q", targetStatus, deploymentStatus)
}
