//go:build phase8fintegration

package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func phase8FDatabase(t *testing.T) (*sql.DB, *timescale.Client) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("TEST_POSTGRES_DSN is required")
	}
	raw, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := postgres.NewSchemaManager(raw).Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	client, err := timescale.NewClient(context.Background(), dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Close()
		_ = raw.Close()
	})
	return raw, client
}

func phase8FRequest(t *testing.T, db *sql.DB, method, target, body, mspID string) (*http.Request, *sql.Tx) {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`
		SELECT set_config('app.user_id', 'phase8f-operator', true),
		       set_config('app.role', 'platform_admin', true),
		       set_config('app.permission', 'write', true)
	`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.SetPathValue("mspID", mspID)
	ctx := context.WithValue(req.Context(), ctxKeyDBTransaction, tx)
	ctx = context.WithValue(ctx, ctxKeyUserID, "phase8f-operator")
	return req.WithContext(ctx), tx
}

func TestPhase8FOffboardingAndBoundedExport(t *testing.T) {
	raw, client := phase8FDatabase(t)
	server := &APIServer{db: client}
	const (
		mspID    = "81000000-0000-0000-0000-000000000001"
		clientID = "81000000-0000-0000-0000-000000000002"
		siteID   = "81000000-0000-0000-0000-000000000003"
		deviceID = "81000000-0000-0000-0000-000000000004"
		tenantID = "81000000-0000-0000-0000-000000000005"
		userID   = "81000000-0000-0000-0000-000000000006"
	)
	seed := []struct {
		query string
		args  []interface{}
	}{
		{`INSERT INTO tenants (id, name, slug, plan) VALUES ($1, 'Lifecycle tenant', 'lifecycle-tenant', 'managed')`, []interface{}{tenantID}},
		{`INSERT INTO users (id, tenant_id, email, password_hash, role, email_verified_at) VALUES ($1, $2, 'owner@example.test', 'redacted-hash', 'admin', NOW())`, []interface{}{userID, tenantID}},
		{`INSERT INTO msp_tenants (id, name, slug, plan) VALUES ($1, 'Lifecycle MSP', 'lifecycle-msp', 'free')`, []interface{}{mspID}},
		{`INSERT INTO plan_entitlements (msp_id, plan_id) VALUES ($1, '00000000-0000-0000-0000-000000000001')`, []interface{}{mspID}},
		{`INSERT INTO client_organizations (id, msp_id, name, slug) VALUES ($1, $2, 'Lifecycle Client', 'lifecycle-client')`, []interface{}{clientID, mspID}},
		{`INSERT INTO sites (id, client_id, name, slug) VALUES ($1, $2, 'Lifecycle Site', 'lifecycle-site')`, []interface{}{siteID, clientID}},
		{`INSERT INTO devices (id, tenant_id, msp_id, client_id, site_id, hostname, public_ip, status) VALUES ($1, $2, $3, $4, $5, 'lifecycle-device', '192.0.2.10', 'online')`, []interface{}{deviceID, tenantID, mspID, clientID, siteID}},
		{`INSERT INTO memberships (user_id, role, scope_type, scope_id, status) VALUES ($1, 'msp_owner', 'msp', $2, 'active')`, []interface{}{userID, mspID}},
		{`INSERT INTO enrollment_tokens_v2 (msp_id, client_id, site_id, token_hash, expires_at) VALUES ($1, $2, $3, 'must-not-export', NOW() + INTERVAL '1 day')`, []interface{}{mspID, clientID, siteID}},
		{`INSERT INTO custom_domains (msp_id, hostname, verification_token, verification_status) VALUES ($1, 'lifecycle.example.test', 'must-not-export', 'active')`, []interface{}{mspID}},
	}
	for _, statement := range seed {
		if _, err := raw.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	req, tx := phase8FRequest(t, raw, http.MethodPost,
		"/api/v2/platform/msps/"+mspID+"/offboarding",
		`{"reason":"contract ended","retention_days":30}`, mspID)
	out := httptest.NewRecorder()
	server.handleOffboardMSP(out, req)
	if out.Code != http.StatusOK {
		_ = tx.Rollback()
		t.Fatalf("offboarding status %d: %s", out.Code, out.Body.String())
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var mspActive, tokenRevoked bool
	var entitlementStatus, membershipStatus, deviceStatus, domainStatus, lifecycleState string
	if err := raw.QueryRow(`SELECT is_active FROM msp_tenants WHERE id = $1`, mspID).Scan(&mspActive); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT status FROM plan_entitlements WHERE msp_id = $1`, mspID).Scan(&entitlementStatus); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT status FROM memberships WHERE scope_type = 'msp' AND scope_id = $1`, mspID).Scan(&membershipStatus); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT is_revoked FROM enrollment_tokens_v2 WHERE msp_id = $1`, mspID).Scan(&tokenRevoked); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT status FROM devices WHERE id = $1`, deviceID).Scan(&deviceStatus); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT verification_status FROM custom_domains WHERE msp_id = $1`, mspID).Scan(&domainStatus); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT state FROM msp_offboarding WHERE msp_id = $1`, mspID).Scan(&lifecycleState); err != nil {
		t.Fatal(err)
	}
	if mspActive || !tokenRevoked || entitlementStatus != "cancelled" ||
		membershipStatus != "revoked" || deviceStatus != "disabled" ||
		domainStatus != "suspended" || lifecycleState != "access_revoked" {
		t.Fatalf("offboarding did not revoke all access: active=%v token=%v entitlement=%s membership=%s device=%s domain=%s state=%s",
			mspActive, tokenRevoked, entitlementStatus, membershipStatus, deviceStatus, domainStatus, lifecycleState)
	}

	exportReq, exportTx := phase8FRequest(t, raw, http.MethodGet,
		"/api/v2/platform/msps/"+mspID+"/export?limit=2", "", mspID)
	exportOut := httptest.NewRecorder()
	server.handleExportMSP(exportOut, exportReq)
	if exportOut.Code != http.StatusOK {
		_ = exportTx.Rollback()
		t.Fatalf("export status %d: %s", exportOut.Code, exportOut.Body.String())
	}
	if err := exportTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(exportOut.Body.String(), "must-not-export") ||
		strings.Contains(exportOut.Body.String(), "192.0.2.10") {
		t.Fatal("export leaked a credential or IP address")
	}
	var exported struct {
		RecordCount int    `json:"record_count"`
		Truncated   bool   `json:"truncated"`
		SHA256      string `json:"sha256"`
	}
	if err := json.Unmarshal(exportOut.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.RecordCount != 2 || !exported.Truncated || len(exported.SHA256) != 64 {
		t.Fatalf("unexpected bounded export metadata: %+v", exported)
	}

	var auditCount int
	if err := raw.QueryRow(`
		SELECT COUNT(*) FROM control_plane_audit
		WHERE msp_id = $1 AND action IN ('msp.offboarded', 'msp.exported')
	`, mspID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("audit events = %d, want 2", auditCount)
	}
}
