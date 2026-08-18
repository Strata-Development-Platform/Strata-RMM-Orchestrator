//go:build dbintegration

package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/strata-rmm/strata-rmm-orchestrator/internal/postgresdriver"
	"golang.org/x/crypto/bcrypt"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func providerHandlerDatabase(t *testing.T) (*sql.DB, *timescale.Client) {
	t.Helper()
	base := os.Getenv("TEST_POSTGRES_DSN")
	if base == "" {
		t.Fatal("TEST_POSTGRES_DSN is required")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := fmt.Sprintf("provider_handler_%d", time.Now().UnixNano())
	controlURL := *parsed
	controlURL.Path = "/postgres"
	control, err := sql.Open("postgres", controlURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.Exec("CREATE DATABASE " + databaseName); err != nil {
		_ = control.Close()
		t.Fatal(err)
	}
	testURL := *parsed
	testURL.Path = "/" + databaseName
	raw, err := sql.Open("postgres", testURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.NewSchemaManager(raw).Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	client, err := timescale.NewClient(context.Background(), testURL.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Close()
		_ = raw.Close()
		_, _ = control.Exec("DROP DATABASE IF EXISTS " + databaseName + " WITH (FORCE)")
		_ = control.Close()
	})
	return raw, client
}

func TestProviderProfileHTTPAuthorizationSetupRetryPatchAndContext(t *testing.T) {
	raw, client := providerHandlerDatabase(t)
	const (
		secret       = "provider-handler-test-secret-at-least-32-bytes"
		tenantID     = "68000000-0000-0000-0000-000000000001"
		ownerID      = "68000000-0000-0000-0000-000000000002"
		mspAdminID   = "68000000-0000-0000-0000-000000000003"
		technicianID = "68000000-0000-0000-0000-000000000004"
		customerID   = "68000000-0000-0000-0000-000000000005"
		mspID        = "68000000-0000-0000-0000-000000000006"
	)
	t.Setenv("JWT_SECRET", secret)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO tenants (id, name, slug, plan) VALUES ($1, 'Provider tenant', 'provider-handler', 'enterprise')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO msp_tenants (id, name, slug) VALUES ($1, 'Scoped MSP', 'provider-handler-msp')`, mspID); err != nil {
		t.Fatal(err)
	}
	seedHTTPProviderUser(t, raw, tenantID, ownerID, "owner@example.test", string(passwordHash), "platform_owner", "platform", postgres.SingletonPlatformID)
	seedHTTPProviderUser(t, raw, tenantID, mspAdminID, "msp-admin@example.test", string(passwordHash), "msp_admin", "msp", mspID)
	seedHTTPProviderUser(t, raw, tenantID, technicianID, "technician@example.test", string(passwordHash), "msp_technician", "msp", mspID)
	seedHTTPProviderUser(t, raw, tenantID, customerID, "customer@example.test", string(passwordHash), "msp_viewer", "msp", mspID)

	tokenGenerator := auth.NewTokenGenerator(secret)
	server := &APIServer{db: client, tokenGen: tokenGenerator, requireHTTPSWebsite: true}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/auth/me", server.handleMe)
	mux.HandleFunc("GET /api/v2/context", server.handleContext)
	mux.HandleFunc("GET /api/v2/platform/provider/profile", server.handleGetProviderProfile)
	mux.HandleFunc("POST /api/v2/platform/provider/setup", server.handleCompleteProviderSetup)
	mux.HandleFunc("PATCH /api/v2/platform/provider/profile", server.handleUpdateProviderProfile)
	handler := server.withAccessControl(server.withTenantTransaction(mux))

	ownerToken := providerUserToken(t, tokenGenerator, ownerID, tenantID, "", "platform_owner")
	mspAdminToken := providerUserToken(t, tokenGenerator, mspAdminID, tenantID, mspID, "msp_admin")
	technicianToken := providerUserToken(t, tokenGenerator, technicianID, tenantID, mspID, "msp_technician")
	customerToken := providerUserToken(t, tokenGenerator, customerID, tenantID, mspID, "client_admin")
	crossTenantOwnerToken := providerUserToken(t, tokenGenerator, ownerID, tenantID, mspID, "platform_owner")

	for _, test := range []struct {
		name  string
		token string
		want  int
	}{
		{name: "unauthenticated", want: http.StatusUnauthorized},
		{name: "MSP admin", token: mspAdminToken, want: http.StatusForbidden},
		{name: "technician", token: technicianToken, want: http.StatusForbidden},
		{name: "customer", token: customerToken, want: http.StatusForbidden},
		{name: "cross tenant owner", token: crossTenantOwnerToken, want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := providerHTTPRequest(handler, http.MethodPost, "/api/v2/platform/provider/setup", test.token, providerSetupJSON("Provider"))
			if out.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", out.Code, test.want, out.Body.String())
			}
		})
	}

	created := providerHTTPRequest(handler, http.MethodPost, "/api/v2/platform/provider/setup", ownerToken, providerSetupJSON("Provider"))
	if created.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", created.Code, created.Body.String())
	}
	var createdProfile postgres.ProviderBusinessProfile
	if err := json.Unmarshal(created.Body.Bytes(), &createdProfile); err != nil {
		t.Fatal(err)
	}
	if !createdProfile.SetupComplete || createdProfile.DisplayName != "Provider" || createdProfile.SetupCompletedBy != ownerID {
		t.Fatalf("unexpected setup response: %+v", createdProfile)
	}

	retry := providerHTTPRequest(handler, http.MethodPost, "/api/v2/platform/provider/setup", ownerToken, providerSetupJSON("Provider"))
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status = %d: %s", retry.Code, retry.Body.String())
	}
	conflict := providerHTTPRequest(handler, http.MethodPost, "/api/v2/platform/provider/setup", ownerToken, providerSetupJSON("Different"))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("different retry status = %d: %s", conflict.Code, conflict.Body.String())
	}

	protected := providerHTTPRequest(handler, http.MethodPatch, "/api/v2/platform/provider/profile", ownerToken,
		`{"setup_completed_at":"2026-01-01T00:00:00Z"}`)
	if protected.Code != http.StatusBadRequest {
		t.Fatalf("protected patch status = %d: %s", protected.Code, protected.Body.String())
	}
	patched := providerHTTPRequest(handler, http.MethodPatch, "/api/v2/platform/provider/profile", ownerToken,
		`{"display_name":"Updated Provider","website_url":"https://provider.example.test"}`)
	if patched.Code != http.StatusOK {
		t.Fatalf("patch status = %d: %s", patched.Code, patched.Body.String())
	}
	var patchedProfile postgres.ProviderBusinessProfile
	if err := json.Unmarshal(patched.Body.Bytes(), &patchedProfile); err != nil {
		t.Fatal(err)
	}
	if patchedProfile.SetupCompletedAt == nil || createdProfile.SetupCompletedAt == nil ||
		!patchedProfile.SetupCompletedAt.Equal(*createdProfile.SetupCompletedAt) ||
		patchedProfile.SetupCompletedBy != createdProfile.SetupCompletedBy {
		t.Fatal("PATCH rewrote setup completion metadata")
	}

	contextOut := providerHTTPRequest(handler, http.MethodGet, "/api/v2/context", ownerToken, "")
	if contextOut.Code != http.StatusOK {
		t.Fatalf("context status = %d: %s", contextOut.Code, contextOut.Body.String())
	}
	var actorContext contextResponse
	if err := json.Unmarshal(contextOut.Body.Bytes(), &actorContext); err != nil {
		t.Fatal(err)
	}
	if !actorContext.SetupComplete || actorContext.ProviderDisplayName != "Updated Provider" ||
		actorContext.TenantID != "" || actorContext.SelectedScope.Type != ScopePlatform ||
		primaryEffectiveRole(actorContext.Roles) != "platform_owner" {
		t.Fatalf("unexpected authenticated context: %+v", actorContext)
	}

	meOut := providerHTTPRequest(handler, http.MethodGet, "/api/v1/auth/me", ownerToken, "")
	if meOut.Code != http.StatusOK {
		t.Fatalf("me status = %d: %s", meOut.Code, meOut.Body.String())
	}
	var me loginResponse
	if err := json.Unmarshal(meOut.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Role != "platform_owner" || len(me.Roles) != 1 || me.Roles[0] != "platform_owner" ||
		me.ProviderDisplayName != "Updated Provider" || !me.SetupComplete {
		t.Fatalf("me response is not membership-derived: %+v", me)
	}

	loginOut := httptest.NewRecorder()
	server.handleLogin(loginOut, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"owner@example.test","password":"correct horse battery staple"}`)))
	if loginOut.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", loginOut.Code, loginOut.Body.String())
	}
	var login loginResponse
	if err := json.Unmarshal(loginOut.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.Role != "platform_owner" || len(login.Roles) != 1 || login.Roles[0] != "platform_owner" {
		t.Fatalf("login response is not membership-derived: %+v", login)
	}

	var setupAudits, updateAudits int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM control_plane_audit WHERE action = 'provider.setup_completed'`).Scan(&setupAudits); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM control_plane_audit WHERE action = 'provider.profile_updated'`).Scan(&updateAudits); err != nil {
		t.Fatal(err)
	}
	if setupAudits != 1 || updateAudits != 1 {
		t.Fatalf("audit counts setup=%d update=%d", setupAudits, updateAudits)
	}
}

func seedHTTPProviderUser(t *testing.T, db *sql.DB, tenantID, userID, email, passwordHash, role, scopeType, scopeID string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO users (id, tenant_id, email, password_hash, role, email_verified_at)
		VALUES ($1, $2, $3, $4, 'admin', NOW())
	`, userID, tenantID, email, passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO memberships (user_id, role, scope_type, scope_id, status)
		VALUES ($1, $2, $3, $4, 'active')
	`, userID, role, scopeType, scopeID); err != nil {
		t.Fatal(err)
	}
}

func providerUserToken(t *testing.T, generator *auth.TokenGenerator, userID, tenantID, mspID, role string) string {
	t.Helper()
	token, err := generator.GenerateUserToken(userID, tenantID, mspID, "", "", []string{role}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func providerHTTPRequest(handler http.Handler, method, target, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, request)
	return out
}

func providerSetupJSON(displayName string) string {
	return fmt.Sprintf(`{
		"legal_name":"Provider LLC","display_name":%q,"contact_name":"Ada Operator",
		"support_email":"support@example.test","billing_email":"billing@example.test",
		"business_phone":"+14155550123","website_url":"https://provider.example.test",
		"address_line1":"1 Main Street","city":"Test City","state_province":"CA",
		"postal_code":"94105","country_code":"US","default_timezone":"UTC",
		"default_locale":"en-US","default_currency":"USD"
	}`, displayName)
}
