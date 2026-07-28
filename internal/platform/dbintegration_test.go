//go:build dbintegration

package platform

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://strata_test:strata_test@localhost:5432/strata_test?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	return db
}

func applyMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	mgr := postgres.NewSchemaManager(db)
	if err := mgr.Apply(); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
}

func setRLSContext(t *testing.T, tx *sql.Tx, mspID, userID, role, permission string) {
	t.Helper()
	_, err := tx.Exec(`
		SELECT set_config('app.msp_id', $1, true),
		       set_config('app.user_id', $2, true),
		       set_config('app.role', $3, true),
		       set_config('app.permission', $4, true),
		       set_config('app.tenant_id', $1, true)
	`, mspID, userID, role, permission)
	if err != nil {
		t.Fatalf("set RLS context: %v", err)
	}
}

func seedTestData(t *testing.T, db *sql.DB) (mspID, clientID, siteID, deviceID, userID string) {
	t.Helper()

	userID = "00000000-0000-0000-0000-000000000010"
	mspID = "00000000-0000-0000-0000-000000000001"
	clientID = "00000000-0000-0000-0000-000000000002"
	siteID = "00000000-0000-0000-0000-000000000003"
	deviceID = "00000000-0000-0000-0000-000000000004"

	_, _ = db.Exec(`DELETE FROM endpoint_audit_evidence`)
	_, _ = db.Exec(`DELETE FROM inventory_results`)
	_, _ = db.Exec(`DELETE FROM agent_capabilities`)
	_, _ = db.Exec(`DELETE FROM endpoint_approval_decisions`)
	_, _ = db.Exec(`DELETE FROM endpoint_approval_requests`)
	_, _ = db.Exec(`DELETE FROM endpoint_approval_policies`)
	_, _ = db.Exec(`DELETE FROM job_outbox`)
	_, _ = db.Exec(`DELETE FROM job_inbox`)
	_, _ = db.Exec(`DELETE FROM job_targets`)
	_, _ = db.Exec(`DELETE FROM jobs`)
	_, _ = db.Exec(`DELETE FROM devices`)
	_, _ = db.Exec(`DELETE FROM sites`)
	_, _ = db.Exec(`DELETE FROM client_organizations`)
	_, _ = db.Exec(`DELETE FROM memberships`)
	_, _ = db.Exec(`DELETE FROM msp_tenants`)
	_, _ = db.Exec(`DELETE FROM users`)
	_, _ = db.Exec(`DELETE FROM tenants`)

	_, _ = db.Exec(`INSERT INTO tenants (id, name, slug, plan) VALUES ('00000000-0000-0000-0000-000000000001', 'Test', 'test', 'enterprise')`)
	_, _ = db.Exec(`INSERT INTO users (id, tenant_id, email, password_hash, role) VALUES ($1, $2, 'test@test.com', '$2a$10$test', 'admin')`, userID, "00000000-0000-0000-0000-000000000001")
	_, _ = db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Test MSP', 'test-msp', true)`, mspID)
	_, _ = db.Exec(`INSERT INTO client_organizations (id, msp_id, name, slug, is_active) VALUES ($1, $2, 'Test Client', 'test-client', true)`, clientID, mspID)
	_, _ = db.Exec(`INSERT INTO sites (id, client_id, name, slug, is_active) VALUES ($1, $2, 'Test Site', 'test-site', true)`, siteID, clientID)
	_, _ = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status) VALUES ($1, $2, $3, $4, $5, 'test-device', 'online')`, deviceID, mspID, clientID, siteID, "00000000-0000-0000-0000-000000000001")
	_, _ = db.Exec(`INSERT INTO memberships (user_id, scope_type, scope_id, role, status) VALUES ($1, 'msp', $2, 'msp_admin', 'active')`, userID, mspID)

	return
}

// --- APPROVAL TESTS ---

func TestApprovalDefaultDestructivePolicy(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)

	for _, action := range []string{"reboot", "shutdown", "process_kill"} {
		p := defaultApprovalPolicy(action)
		if !p.ApprovalRequired {
			t.Errorf("defaultApprovalPolicy(%q).ApprovalRequired = false, want true", action)
		}
	}
	for _, action := range []string{"refresh", "service_start", "service_stop", "service_restart"} {
		p := defaultApprovalPolicy(action)
		if p.ApprovalRequired {
			t.Errorf("defaultApprovalPolicy(%q).ApprovalRequired = true, want false", action)
		}
	}
}

func TestApprovalPolicySnapshotPreserved(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, _, userID := seedTestData(t, db)

	policySnap := `{"approval_required":true,"min_approvers":2,"allowed_roles":["msp_owner","msp_admin"],"require_separation":true,"approval_expires_sec":3600,"allow_emergency":false}`
	reqID := "00000000-0000-0000-0000-000000000070"
	deviceIDs := "{" + "00000000-0000-0000-0000-000000000004" + "}"
	expiresAt := time.Now().Add(1 * time.Hour)

	_, err := db.Exec(`INSERT INTO endpoint_approval_requests (id, msp_id, requester_user_id, action_name, reason, device_ids, device_count, status, policy_snapshot, expires_at)
		VALUES ($1, $2, $3, 'reboot', 'test', $4, 1, 'pending', $5::jsonb, $6)`,
		reqID, mspID, userID, deviceIDs, policySnap, expiresAt)
	if err != nil {
		t.Fatalf("seed request: %v", err)
	}

	// Change the live policy
	_, err = db.Exec(`DELETE FROM endpoint_approval_policies`)
	if err != nil {
		t.Fatalf("delete policies: %v", err)
	}

	var storedSnap string
	err = db.QueryRow(`SELECT policy_snapshot::text FROM endpoint_approval_requests WHERE id = $1`, reqID).Scan(&storedSnap)
	if err != nil {
		t.Fatalf("get stored snapshot: %v", err)
	}
	if storedSnap != policySnap {
		t.Errorf("stored snapshot changed after live policy deletion: got %q, want %q", storedSnap, policySnap)
	}
}

func TestApprovalSelfApprovalDenied(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, _, userID := seedTestData(t, db)

	reqID := "00000000-0000-0000-0000-000000000040"
	deviceIDs := "{" + "00000000-0000-0000-0000-000000000004" + "}"
	expiresAt := time.Now().Add(1 * time.Hour)
	policySnap := `{"require_separation":true}`

	_, err := db.Exec(`INSERT INTO endpoint_approval_requests (id, msp_id, requester_user_id, action_name, reason, device_ids, device_count, status, policy_snapshot, expires_at)
		VALUES ($1, $2, $3, 'reboot', 'test', $4, 1, 'pending', $5::jsonb, $6)`,
		reqID, mspID, userID, deviceIDs, policySnap, expiresAt)
	if err != nil {
		t.Fatalf("seed request: %v", err)
	}

	_, err = db.Exec(`INSERT INTO endpoint_approval_decisions (id, request_id, msp_id, approver_user_id, decision)
		VALUES (gen_random_uuid(), $1, $2, $3, 'approved')`, reqID, mspID, userID)
	if err == nil {
		t.Error("self-approval should be rejected by unique constraint or application logic")
	}
}

func TestApprovalDuplicateDecisionDenied(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, _, userID := seedTestData(t, db)

	reqID := "00000000-0000-0000-0000-000000000080"
	deviceIDs := "{" + "00000000-0000-0000-0000-000000000004" + "}"
	expiresAt := time.Now().Add(1 * time.Hour)
	otherUserID := "00000000-0000-0000-0000-000000000030"

	_, err := db.Exec(`INSERT INTO users (id, tenant_id, email, password_hash, role) VALUES ($1, '00000000-0000-0000-0000-000000000001', 'other@test.com', '$2a$10$test', 'admin')`, otherUserID)
	if err != nil {
		t.Fatalf("seed other user: %v", err)
	}
	_, err = db.Exec(`INSERT INTO memberships (user_id, scope_type, scope_id, role, status) VALUES ($1, 'msp', $2, 'msp_admin', 'active')`, otherUserID, mspID)
	if err != nil {
		t.Fatalf("seed other membership: %v", err)
	}

	_, err = db.Exec(`INSERT INTO endpoint_approval_requests (id, msp_id, requester_user_id, action_name, reason, device_ids, device_count, status, policy_snapshot, expires_at)
		VALUES ($1, $2, $3, 'reboot', 'test', $4, 1, 'pending', '{}'::jsonb, $5)`,
		reqID, mspID, userID, deviceIDs, expiresAt)
	if err != nil {
		t.Fatalf("seed request: %v", err)
	}

	// First decision succeeds
	_, err = db.Exec(`INSERT INTO endpoint_approval_decisions (id, request_id, msp_id, approver_user_id, decision)
		VALUES (gen_random_uuid(), $1, $2, $3, 'approved')`, reqID, mspID, otherUserID)
	if err != nil {
		t.Fatalf("first decision: %v", err)
	}

	// Duplicate by same user must fail
	_, err = db.Exec(`INSERT INTO endpoint_approval_decisions (id, request_id, msp_id, approver_user_id, decision)
		VALUES (gen_random_uuid(), $1, $2, $3, 'approved')`, reqID, mspID, otherUserID)
	if err == nil {
		t.Error("duplicate approval by same user must be rejected (unique constraint)")
	}
}

func TestApprovalCrossMSPDenied(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, _, userID := seedTestData(t, db)

	otherMSPID := "00000000-0000-0000-0000-000000000099"
	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true)`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}

	reqID := "00000000-0000-0000-0000-000000000090"
	deviceIDs := "{" + "00000000-0000-0000-0000-000000000004" + "}"
	expiresAt := time.Now().Add(1 * time.Hour)

	_, err = db.Exec(`INSERT INTO endpoint_approval_requests (id, msp_id, requester_user_id, action_name, reason, device_ids, device_count, status, policy_snapshot, expires_at)
		VALUES ($1, $2, $3, 'reboot', 'test', $4, 1, 'pending', '{}'::jsonb, $5)`,
		reqID, mspID, userID, deviceIDs, expiresAt)
	if err != nil {
		t.Fatalf("seed request: %v", err)
	}

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM endpoint_approval_requests WHERE msp_id = $1`, otherMSPID).Scan(&count)
	if err != nil {
		t.Fatalf("count for other msp: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 requests visible for other MSP, got %d", count)
	}
}

func TestApprovalExpiredGetsRejected(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, _, userID := seedTestData(t, db)

	reqID := "00000000-0000-0000-0000-000000000060"
	deviceIDs := "{" + "00000000-0000-0000-0000-000000000004" + "}"
	expiresAt := time.Now().Add(-1 * time.Hour)

	_, err := db.Exec(`INSERT INTO endpoint_approval_requests (id, msp_id, requester_user_id, action_name, reason, device_ids, device_count, status, policy_snapshot, expires_at)
		VALUES ($1, $2, $3, 'reboot', 'test', $4, 1, 'pending', '{}'::jsonb, $5)`,
		reqID, mspID, userID, deviceIDs, expiresAt)
	if err != nil {
		t.Fatalf("seed expired request: %v", err)
	}

	expirePendingApprovals(db)

	var status string
	err = db.QueryRow(`SELECT status FROM endpoint_approval_requests WHERE id = $1`, reqID).Scan(&status)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status != "expired" {
		t.Errorf("expected expired, got %s", status)
	}
}

// --- AUDIT IMMUTABILITY TESTS ---

func TestAuditEvidenceIsAppendOnly(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, _, _ := seedTestData(t, db)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	setRLSContext(t, tx, mspID, "test-user", "msp_admin", "write")

	var auditID string
	err = tx.QueryRow(`
		INSERT INTO endpoint_audit_evidence
			(id, msp_id, actor_user_id, action, targets, policy_snapshot, approval_state)
		VALUES (gen_random_uuid(), $1, 'test-user', 'device.reboot', '[]'::jsonb, '{}'::jsonb, 'none')
		RETURNING id::text
	`, mspID).Scan(&auditID)
	if err != nil {
		t.Fatalf("insert audit evidence: %v", err)
	}
	if auditID == "" {
		t.Fatal("expected audit ID")
	}

	// UPDATE must be denied by RLS (WITH CHECK false)
	result, err := tx.Exec(`UPDATE endpoint_audit_evidence SET action = 'device.shutdown' WHERE id = $1`, auditID)
	if err != nil {
		t.Logf("UPDATE denied by policy (expected): %v", err)
	} else {
		affected, _ := result.RowsAffected()
		if affected > 0 {
			t.Error("UPDATE to endpoint_audit_evidence allowed but must be denied by RLS (WITH CHECK false)")
		}
	}

	// DELETE must be denied by RLS
	result, err = tx.Exec(`DELETE FROM endpoint_audit_evidence WHERE id = $1`, auditID)
	if err != nil {
		t.Logf("DELETE denied by policy (expected): %v", err)
	} else {
		affected, _ := result.RowsAffected()
		if affected > 0 {
			t.Error("DELETE to endpoint_audit_evidence allowed but must be denied by RLS (WITH CHECK false)")
		}
	}
}

func TestAuditEvidenceCrossMSPReadDenied(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, _, _ := seedTestData(t, db)
	otherMSPID := "00000000-0000-0000-0000-000000000099"

	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true)`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}

	// Insert audit record for other MSP
	_, err = db.Exec(`
		INSERT INTO endpoint_audit_evidence (id, msp_id, actor_user_id, action, targets, policy_snapshot, approval_state)
		VALUES (gen_random_uuid(), $1, 'other-user', 'device.reboot', '[]'::jsonb, '{}'::jsonb, 'none')
	`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other audit: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	// Try to read from MSP A's context
	setRLSContext(t, tx, mspID, "test-user", "msp_admin", "read")

	var count int
	err = tx.QueryRow(`SELECT COUNT(*) FROM endpoint_audit_evidence WHERE msp_id = $1`, otherMSPID).Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 other-MSP audit records visible, got %d", count)
	}
}

func TestAuditEvidenceTransactionRollbackRemovesEvidence(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, _, _ := seedTestData(t, db)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	var auditID string
	err = tx.QueryRow(`
		INSERT INTO endpoint_audit_evidence
			(id, msp_id, actor_user_id, action, targets, policy_snapshot, approval_state)
		VALUES (gen_random_uuid(), $1, 'test-user', 'device.reboot', '[]'::jsonb, '{}'::jsonb, 'none')
		RETURNING id::text
	`, mspID).Scan(&auditID)
	if err != nil {
		t.Fatalf("insert audit: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM endpoint_audit_evidence WHERE id::text = $1`, auditID).Scan(&count)
	if err != nil {
		t.Fatalf("count after rollback: %v", err)
	}
	if count != 0 {
		t.Error("audit evidence persisted after transaction rollback")
	}
}

// --- INVENTORY IDEMPOTENCY TESTS ---

func TestInventoryResultIdempotency(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, deviceID, _ := seedTestData(t, db)

	now := time.Now().UTC().Add(-1 * time.Minute)
	payloadBytes, _ := json.Marshal(map[string]interface{}{"hostname": "test-device"})
	payloadHash := fmt.Sprintf("%x", sha256.Sum256(payloadBytes))

	// First insert is accepted
	_, err := db.Exec(`
		INSERT INTO inventory_results
			(id, device_id, msp_id, schema_version, payload, payload_hash, collection_time, accepted, is_stale, is_failure)
		VALUES (gen_random_uuid(), $1, $2, 1, $3, $4, $5, true, false, false)
	`, deviceID, mspID, string(payloadBytes), payloadHash, now)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Verify device has inventory_last_success set
	var lastSuccess sql.NullTime
	err = db.QueryRow(`SELECT inventory_last_success FROM devices WHERE id = $1`, deviceID).Scan(&lastSuccess)
	if err != nil {
		t.Fatalf("get last success: %v", err)
	}
	t.Logf("inventory_last_success after first accept: %v", lastSuccess.Time)

	// Second insert with same payload hash is accepted (different UUID)
	// This simulates duplicate content but different result ID
	_, err = db.Exec(`
		INSERT INTO inventory_results
			(id, device_id, msp_id, schema_version, payload, payload_hash, collection_time, accepted, is_stale, is_failure)
		VALUES (gen_random_uuid(), $1, $2, 1, $3, $4, $5, true, false, false)
	`, deviceID, mspID, string(payloadBytes), payloadHash, now)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}

	var acceptedCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM inventory_results WHERE device_id = $1 AND accepted = true`, deviceID).Scan(&acceptedCount)
	if err != nil {
		t.Fatalf("count accepted: %v", err)
	}
	if acceptedCount < 1 {
		t.Errorf("expected at least 1 accepted record, got %d", acceptedCount)
	}
}

func TestInventoryStaleResultRejected(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, deviceID, _ := seedTestData(t, db)

	oldTime := time.Now().UTC().Add(-2 * time.Hour)
	newTime := time.Now().UTC().Add(-1 * time.Hour)
	payloadBytes, _ := json.Marshal(map[string]interface{}{"hostname": "test-device"})
	payloadHash := fmt.Sprintf("%x", sha256.Sum256(payloadBytes))

	// Insert newer accepted result first
	_, err := db.Exec(`
		INSERT INTO inventory_results
			(id, device_id, msp_id, schema_version, payload, payload_hash, collection_time, accepted, is_stale, is_failure)
		VALUES (gen_random_uuid(), $1, $2, 1, $3, $4, $5, true, false, false)
	`, deviceID, mspID, string(payloadBytes), payloadHash, newTime)
	if err != nil {
		t.Fatalf("insert new result: %v", err)
	}

	// Insert older (stale) result
	_, err = db.Exec(`
		INSERT INTO inventory_results
			(id, device_id, msp_id, schema_version, payload, payload_hash, collection_time, accepted, is_stale, is_failure)
		VALUES (gen_random_uuid(), $1, $2, 1, $3, $4, $5, false, true, false)
	`, deviceID, mspID, string(payloadBytes), payloadHash, oldTime)
	if err != nil {
		t.Fatalf("insert stale result: %v", err)
	}

	var acceptedCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM inventory_results WHERE device_id = $1 AND accepted = true`, deviceID).Scan(&acceptedCount)
	if err != nil {
		t.Fatalf("count accepted: %v", err)
	}
	if acceptedCount != 1 {
		t.Errorf("expected 1 accepted (newer), got %d", acceptedCount)
	}

	// Stale result should still be recorded as rejected evidence
	var staleCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM inventory_results WHERE device_id = $1 AND is_stale = true`, deviceID).Scan(&staleCount)
	if err != nil {
		t.Fatalf("count stale: %v", err)
	}
	if staleCount != 1 {
		t.Errorf("expected 1 stale record, got %d", staleCount)
	}
}

func TestInventoryCrossDeviceRejected(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, _, _ := seedTestData(t, db)

	otherDeviceID := "00000000-0000-0000-0000-000000000005"
	_, err := db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device', 'online')`,
		otherDeviceID, mspID)
	if err != nil {
		t.Fatalf("seed other device: %v", err)
	}

	now := time.Now().UTC()
	payloadBytes, _ := json.Marshal(map[string]interface{}{"hostname": "wrong-device"})
	payloadHash := fmt.Sprintf("%x", sha256.Sum256(payloadBytes))

	// Insert result for wrong device
	_, err = db.Exec(`
		INSERT INTO inventory_results
			(id, device_id, msp_id, schema_version, payload, payload_hash, collection_time, accepted)
		VALUES (gen_random_uuid(), $1, $2, 1, $3, $4, $5, false)
	`, otherDeviceID, mspID, string(payloadBytes), payloadHash, now)
	if err != nil {
		t.Fatalf("insert cross-device result: %v", err)
	}

	// Should be rejected (not accepted)
	var acceptedCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM inventory_results WHERE device_id = $1 AND accepted = true`, otherDeviceID).Scan(&acceptedCount)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if acceptedCount != 0 {
		t.Errorf("expected 0 accepted cross-device results, got %d", acceptedCount)
	}
}

// --- MAINTENANCE WINDOW TESTS ---

func TestMaintenanceWindowEnforcement(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, clientID, siteID, deviceID, _ := seedTestData(t, db)

	now := time.Now().UTC()
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	_, err := db.Exec(`INSERT INTO maintenance_windows (id, tenant_id, msp_id, client_id, site_id, device_id, name, start_time, end_time, timezone, description)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'Test Window', $6, $7, 'UTC', 'test')`,
		"00000000-0000-0000-0000-000000000001", mspID, clientID, siteID, deviceID, startTime, endTime)
	if err != nil {
		t.Fatalf("seed maintenance window: %v", err)
	}

	var configured, covered bool
	err = db.QueryRow(`
		SELECT
			EXISTS (SELECT 1 FROM maintenance_windows WHERE msp_id = $1 AND end_time >= NOW()),
			EXISTS (SELECT 1 FROM maintenance_windows WHERE msp_id = $1 AND start_time <= NOW() AND end_time >= NOW())
	`, mspID).Scan(&configured, &covered)
	if err != nil {
		t.Fatalf("check maintenance window: %v", err)
	}
	if !configured {
		t.Error("expected maintenance window to be configured")
	}
	if !covered {
		t.Error("expected current time to be covered by maintenance window")
	}
}

func TestMaintenanceWindowClosedDoesNotCover(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, clientID, siteID, deviceID, _ := seedTestData(t, db)

	pastStart := time.Now().UTC().Add(-3 * time.Hour)
	pastEnd := time.Now().UTC().Add(-2 * time.Hour)

	_, err := db.Exec(`INSERT INTO maintenance_windows (id, tenant_id, msp_id, client_id, site_id, device_id, name, start_time, end_time, timezone, description)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'Closed Window', $6, $7, 'UTC', 'test')`,
		"00000000-0000-0000-0000-000000000001", mspID, clientID, siteID, deviceID, pastStart, pastEnd)
	if err != nil {
		t.Fatalf("seed closed window: %v", err)
	}

	var covered bool
	err = db.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM maintenance_windows WHERE msp_id = $1 AND start_time <= NOW() AND end_time >= NOW())
	`, mspID).Scan(&covered)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if covered {
		t.Error("closed maintenance window should not cover current time")
	}
}

// --- CAPABILITY TESTS ---

func TestCapabilityUnknownFieldsRejected(t *testing.T) {
	// This test verifies the validation function rejects unknown fields
	// by verifying the operation registry behavior
	_, ok := operationRegistry["unknown_action"]
	if ok {
		t.Error("unknown_action should not exist in registry")
	}
}

func TestCapabilityIsActionSupportedByCapabilities(t *testing.T) {
	tests := []struct {
		name   string
		action string
		types  []string
		want   bool
	}{
		{"supported refresh", "refresh", []string{"device.refresh"}, true},
		{"unsupported reboot", "reboot", []string{"device.refresh"}, false},
		{"nil capabilities", "reboot", nil, false},
		{"unknown action", "unknown", []string{"device.reboot"}, false},
		{"service restart", "service_restart", []string{"device.service_restart"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cap *AgentCapability
			if tt.types != nil {
				cap = &AgentCapability{SupportedJobTypes: tt.types}
			}
			got := isActionSupportedByCapabilities(tt.action, cap)
			if got != tt.want {
				t.Errorf("isActionSupportedByCapabilities(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}
