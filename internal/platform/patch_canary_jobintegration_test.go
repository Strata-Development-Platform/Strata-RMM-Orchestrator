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

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/patch"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

func TestPatchCanaryTerminalGateAdvancesOrStopsBroadRollout(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	natsURL := os.Getenv("TEST_NATS_URL")
	if dsn == "" || natsURL == "" {
		t.Fatal("TEST_POSTGRES_DSN and TEST_NATS_URL are required")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := postgres.NewSchemaManager(db).Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if err := postgres.ApplyDurabilitySchema(ctx, db); err != nil {
		t.Fatalf("apply production durability schema: %v", err)
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	const (
		tenantID = "30000000-0000-0000-0000-000000000001"
		mspID    = "30000000-0000-0000-0000-000000000002"
		clientID = "30000000-0000-0000-0000-000000000003"
		siteID   = "30000000-0000-0000-0000-000000000004"
		policyID = "30000000-0000-0000-0000-000000000005"

		passDeploymentID = "30000000-0000-0000-0000-000000000010"
		passCanaryDevice  = "30000000-0000-0000-0000-000000000011"
		passBroadDevice   = "30000000-0000-0000-0000-000000000012"
		passCanaryJob     = "30000000-0000-0000-0000-000000000013"
		passCanaryTarget  = "30000000-0000-0000-0000-000000000014"

		failDeploymentID = "30000000-0000-0000-0000-000000000020"
		failCanaryDevice  = "30000000-0000-0000-0000-000000000021"
		failBroadDevice   = "30000000-0000-0000-0000-000000000022"
		failCanaryJob     = "30000000-0000-0000-0000-000000000023"
		failCanaryTarget  = "30000000-0000-0000-0000-000000000024"
	)

	mustExec := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed canary integration: %v\nquery: %s", err, query)
		}
	}

	mustExec(`INSERT INTO tenants (id, name, slug, plan) VALUES ($1, 'Patch Canary Tenant', 'patch-canary-tenant', 'enterprise')`, tenantID)
	mustExec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Patch Canary MSP', 'patch-canary-msp', TRUE)`, mspID)
	mustExec(`INSERT INTO client_organizations (id, msp_id, name, slug, is_active) VALUES ($1, $2, 'Patch Canary Client', 'patch-canary-client', TRUE)`, clientID, mspID)
	mustExec(`INSERT INTO sites (id, client_id, name, slug, is_active) VALUES ($1, $2, 'Patch Canary Site', 'patch-canary-site', TRUE)`, siteID, clientID)

	for _, device := range []struct {
		id, agent string
	}{
		{passCanaryDevice, "patch-pass-canary"},
		{passBroadDevice, "patch-pass-broad"},
		{failCanaryDevice, "patch-fail-canary"},
		{failBroadDevice, "patch-fail-broad"},
	} {
		mustExec(`
			INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, agent_id, hostname, status, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, $6, 'online', TRUE)
		`, device.id, mspID, clientID, siteID, tenantID, device.agent)
	}

	now := time.Now().UTC()
	mustExec(`
		INSERT INTO patch_policies (
			id, tenant_id, name, enabled, platforms, approval_mode, severity,
			maintenance_window, device_filter, max_retries, created_at, updated_at
		) VALUES ($1, $2, 'Canary Integration', TRUE, '["linux"]'::jsonb, 'automatic',
		          'critical', '', '{}'::jsonb, 2, $3, $3)
	`, policyID, tenantID, now)

	seedDeployment := func(deploymentID, canaryDevice, broadDevice, canaryJob, canaryTarget, terminalStatus string) {
		t.Helper()
		mustExec(`
			INSERT INTO patch_deployments (
				id, policy_id, tenant_id, status, device_count, installed, failed, pending,
				scheduled_for, started_at, created_at
			) VALUES ($1, $2, $3, 'canary', 2, 0, 0, 2, $4, $4, $4)
		`, deploymentID, policyID, tenantID, now.Add(-time.Minute))
		mustExec(`INSERT INTO patch_deployment_patches (deployment_id, patch_id) VALUES ($1, 'KB-INTEGRATION-1')`, deploymentID)
		mustExec(`
			INSERT INTO jobs (
				id, msp_id, client_id, created_by, type, status, payload, max_retries,
				max_devices, expires_at, correlation_id, scheduled_for, completed_at
			) VALUES ($1, $2, $3, 'patch-integration', 'patch_install', $4,
			          '{"patch_ids":["KB-INTEGRATION-1"]}'::jsonb, 2, 1,
			          NOW() + INTERVAL '1 hour', $1::text, NOW(), NOW())
		`, canaryJob, mspID, clientID, terminalStatus)
		mustExec(`
			INSERT INTO job_targets (
				id, job_id, device_id, agent_id, msp_id, status, attempt,
				started_at, completed_at
			) VALUES ($1, $2, $3, $4, $5, $6, 1, NOW(), NOW())
		`, canaryTarget, canaryJob, canaryDevice, "agent-"+canaryDevice[len(canaryDevice)-3:], mspID, terminalStatus)
		// Keep the job target identity authoritative by aligning it to the device's
		// actual registered agent after insertion.
		mustExec(`
			UPDATE job_targets jt
			SET agent_id = d.agent_id
			FROM devices d
			WHERE jt.id = $1 AND d.id = jt.device_id
		`, canaryTarget)
		mustExec(`
			INSERT INTO patch_deployment_devices (
				deployment_id, device_id, rollout_group, dispatched_at,
				dispatch_attempts, job_id, job_target_id
			) VALUES ($1, $2, 'canary', NOW(), 1, $3, $4),
			         ($1, $5, 'broad', NULL, 0, NULL, NULL)
		`, deploymentID, canaryDevice, canaryJob, canaryTarget, broadDevice)
	}

	seedDeployment(passDeploymentID, passCanaryDevice, passBroadDevice, passCanaryJob, passCanaryTarget, "succeeded")
	seedDeployment(failDeploymentID, failCanaryDevice, failBroadDevice, failCanaryJob, failCanaryTarget, "failed")

	store := patch.NewStore(db)
	manager := patch.NewManager(nc, nil, store, zap.NewNop())
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var passStatus, failStatus string
		var passBroadJob, failBroadJob sql.NullString
		err1 := db.QueryRowContext(ctx, `SELECT status FROM patch_deployments WHERE id = $1`, passDeploymentID).Scan(&passStatus)
		err2 := db.QueryRowContext(ctx, `SELECT status FROM patch_deployments WHERE id = $1`, failDeploymentID).Scan(&failStatus)
		err3 := db.QueryRowContext(ctx, `SELECT job_id::text FROM patch_deployment_devices WHERE deployment_id = $1 AND device_id = $2`, passDeploymentID, passBroadDevice).Scan(&passBroadJob)
		err4 := db.QueryRowContext(ctx, `SELECT job_id::text FROM patch_deployment_devices WHERE deployment_id = $1 AND device_id = $2`, failDeploymentID, failBroadDevice).Scan(&failBroadJob)
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil &&
			passStatus == "deploying" && passBroadJob.Valid && passBroadJob.String != "" &&
			failStatus == "failed" && !failBroadJob.Valid {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	var passStatus, failStatus string
	var passBroadJob, failBroadJob sql.NullString
	_ = db.QueryRow(`SELECT status FROM patch_deployments WHERE id = $1`, passDeploymentID).Scan(&passStatus)
	_ = db.QueryRow(`SELECT status FROM patch_deployments WHERE id = $1`, failDeploymentID).Scan(&failStatus)
	_ = db.QueryRow(`SELECT job_id::text FROM patch_deployment_devices WHERE deployment_id = $1 AND device_id = $2`, passDeploymentID, passBroadDevice).Scan(&passBroadJob)
	_ = db.QueryRow(`SELECT job_id::text FROM patch_deployment_devices WHERE deployment_id = $1 AND device_id = $2`, failDeploymentID, failBroadDevice).Scan(&failBroadJob)
	t.Fatalf("canary convergence mismatch: pass_status=%q pass_broad_job=%q fail_status=%q fail_broad_job=%q",
		passStatus, passBroadJob.String, failStatus, failBroadJob.String)
}
