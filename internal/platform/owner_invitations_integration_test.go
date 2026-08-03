//go:build dbintegration

package platform

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

type capturingAccountMailer struct {
	mu       sync.Mutex
	messages []OwnerActivationMail
	err      error
}

func (m *capturingAccountMailer) SendOwnerActivation(_ context.Context, message OwnerActivationMail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, message)
	return m.err
}

func (m *capturingAccountMailer) SendReport(_ context.Context, _ string, _ string, _ string, _ []byte) error {
	return nil
}

func (m *capturingAccountMailer) last(t *testing.T) OwnerActivationMail {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		t.Fatal("mailer captured no activation message")
	}
	return m.messages[len(m.messages)-1]
}

func platformActivationExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT set_config('app.role', 'platform_admin', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func seedActivationActors(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	const tenantID = "46800000-0000-0000-0000-000000000001"
	actors := map[string]string{
		"owner":   "46800000-0000-0000-0000-000000000002",
		"admin":   "46800000-0000-0000-0000-000000000003",
		"expired": "46800000-0000-0000-0000-000000000004",
		"support": "46800000-0000-0000-0000-000000000005",
		"msp":     "46800000-0000-0000-0000-000000000006",
	}
	platformActivationExec(t, db, `
		INSERT INTO tenants (id, name, slug, plan)
		VALUES ($1, 'Activation provider', 'activation-provider', 'enterprise')
	`, tenantID)
	platformActivationExec(t, db, `
		INSERT INTO users (id, tenant_id, email, password_hash, role, is_active, email_verified_at) VALUES
			($2, $1, 'provider-owner@example.test', 'redacted', 'viewer', TRUE, NOW()),
			($3, $1, 'provider-admin@example.test', 'redacted', 'viewer', TRUE, NOW()),
			($4, $1, 'expired-admin@example.test', 'redacted', 'viewer', TRUE, NOW()),
			($5, $1, 'support@example.test', 'redacted', 'viewer', TRUE, NOW()),
			($6, $1, 'msp-owner@example.test', 'redacted', 'viewer', TRUE, NOW())
	`, tenantID, actors["owner"], actors["admin"], actors["expired"], actors["support"], actors["msp"])
	platformActivationExec(t, db, `
		INSERT INTO memberships (user_id, role, scope_type, scope_id, status, expires_at) VALUES
			($1, 'platform_owner', 'platform', $6, 'active', NULL),
			($2, 'platform_admin', 'platform', $6, 'active', NULL),
			($3, 'platform_admin', 'platform', $6, 'active', NOW() - INTERVAL '1 minute'),
			($4, 'platform_support', 'platform', $6, 'active', NULL),
			($5, 'msp_owner', 'msp', '46800000-0000-0000-0000-000000000099', 'active', NULL)
	`, actors["owner"], actors["admin"], actors["expired"], actors["support"], actors["msp"], postgres.SingletonPlatformID)
	return actors
}

func TestOwnerActivationLifecycleDeliveryRotationAndLogin(t *testing.T) {
	rawDB, client := providerHandlerDatabase(t)
	actors := seedActivationActors(t, rawDB)
	mailer := &capturingAccountMailer{err: errors.New("delivery unavailable")}
	service := newOwnerInvitationService(rawDB, mailer, "https://rmm.example.test")
	service.bcryptCost = bcrypt.MinCost

	created, err := service.createPendingMSP(context.Background(), createPendingMSPInput{
		Name: "Lifecycle MSP", Slug: "lifecycle-msp", Plan: "starter",
		OwnerEmail: "Lifecycle.Owner@Example.Test", ActorID: actors["owner"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.DeliveryStatus != "failed" {
		t.Fatalf("delivery status = %q, want failed", created.DeliveryStatus)
	}
	firstMail := mailer.last(t)
	if !strings.Contains(firstMail.ActivationURL, "/activate-account#"+firstMail.Token) {
		t.Fatal("activation URL must carry the token in a hash fragment")
	}
	if strings.Contains(firstMail.ActivationURL, "?") {
		t.Fatal("activation URL must not use a query string (fragments avoid request-log exposure)")
	}
	var active bool
	var onboarding, entitlementStatus, storedHash, deliveryStatus string
	if err := rawDB.QueryRow(`
		SELECT m.is_active, m.onboarding_status, entitlement.status,
		       invitation.token_hash, invitation.delivery_status
		FROM msp_tenants m
		JOIN plan_entitlements entitlement ON entitlement.msp_id = m.id
		JOIN account_invitations invitation ON invitation.msp_id = m.id
		WHERE m.id = $1 AND invitation.revoked_at IS NULL
	`, created.MSPID).Scan(&active, &onboarding, &entitlementStatus, &storedHash, &deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if active || onboarding != "pending_owner" || entitlementStatus != "suspended" || deliveryStatus != "failed" {
		t.Fatalf("pending state active=%v onboarding=%q entitlement=%q delivery=%q", active, onboarding, entitlementStatus, deliveryStatus)
	}
	if storedHash == firstMail.Token || strings.Contains(storedHash, firstMail.Token) || len(storedHash) != 64 {
		t.Fatal("database retained raw invitation material")
	}
	inspected, err := service.inspect(context.Background(), firstMail.Token)
	if err != nil || inspected.MSPName != "Lifecycle MSP" || inspected.MaskedEmail != "l***@e***.test" {
		t.Fatalf("inspection = %+v, err=%v", inspected, err)
	}

	server := &APIServer{db: client, tokenGen: auth.NewTokenGenerator("activation-login-secret-at-least-32-bytes")}
	t.Setenv("JWT_SECRET", "activation-login-secret-at-least-32-bytes")
	getMSPOut := httptest.NewRecorder()
	getMSPRequest := httptest.NewRequest(http.MethodGet, "/api/v2/platform/msps/"+created.MSPID, nil)
	getMSPRequest.SetPathValue("mspID", created.MSPID)
	server.handleGetMSP(getMSPOut, getMSPRequest)
	if getMSPOut.Code != http.StatusOK || !strings.Contains(getMSPOut.Body.String(), `"owner_invitation_delivery_status":"failed"`) {
		t.Fatalf("pending MSP response does not expose resend state: %d %s", getMSPOut.Code, getMSPOut.Body.String())
	}
	login := func() *httptest.ResponseRecorder {
		out := httptest.NewRecorder()
		server.handleLogin(out, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"email":"LIFECYCLE.OWNER@example.test","password":"abcdefghijklmn"}`)))
		return out
	}
	if out := login(); out.Code != http.StatusUnauthorized {
		t.Fatalf("pre-activation login status = %d, want 401: %s", out.Code, out.Body.String())
	}
	if mspID, _ := server.resolveMSPByHost("lifecycle-msp." + platformDomain); mspID != "" {
		t.Fatalf("pending MSP resolved by host as %q", mspID)
	}
	workspaceOut := httptest.NewRecorder()
	workspaceRequest := httptest.NewRequest(http.MethodPost, "/api/v2/context", strings.NewReader(`{"msp_id":"`+created.MSPID+`"}`))
	workspaceContext := context.WithValue(workspaceRequest.Context(), ctxKeyUserID, actors["owner"])
	workspaceContext = context.WithValue(workspaceContext, ctxKeyRole, "platform_owner")
	server.handleContextSwitch(workspaceOut, workspaceRequest.WithContext(workspaceContext))
	if workspaceOut.Code != http.StatusNotFound {
		t.Fatalf("pending workspace status = %d, want fail-closed 404", workspaceOut.Code)
	}

	mailer.mu.Lock()
	mailer.err = nil
	mailer.mu.Unlock()
	rotated, err := service.resend(context.Background(), created.MSPID, actors["admin"])
	if err != nil {
		t.Fatal(err)
	}
	if rotated.DeliveryStatus != "delivered" {
		t.Fatalf("rotated delivery = %q", rotated.DeliveryStatus)
	}
	secondMail := mailer.last(t)
	if secondMail.Token == firstMail.Token {
		t.Fatal("resend did not rotate invitation token")
	}
	if _, err := service.inspect(context.Background(), firstMail.Token); !errors.Is(err, errInvitationInvalid) {
		t.Fatalf("old invitation remained valid: %v", err)
	}

	acceptOut := httptest.NewRecorder()
	server.handleAcceptOwnerInvitation(acceptOut, httptest.NewRequest(http.MethodPost, "/api/v1/auth/invitations/accept",
		strings.NewReader(`{"token":"`+secondMail.Token+`","password":"abcdefghijklmn"}`)))
	if acceptOut.Code != http.StatusNoContent || acceptOut.Body.Len() != 0 {
		t.Fatalf("accept status/body = %d/%q, want 204/empty", acceptOut.Code, acceptOut.Body.String())
	}
	if strings.Contains(acceptOut.Body.String(), secondMail.Token) {
		t.Fatal("accept response exposed raw invitation token")
	}

	var userID, passwordHash string
	var tenantNull, verified, invitationAccepted bool
	var ownerCount, activationAudits int
	if err := rawDB.QueryRow(`
		SELECT id::text, password_hash, tenant_id IS NULL, email_verified_at IS NOT NULL
		FROM users WHERE normalized_email = 'lifecycle.owner@example.test'
	`).Scan(&userID, &passwordHash, &tenantNull, &verified); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("abcdefghijklmn")); err != nil {
		t.Fatalf("stored owner password does not authenticate: %v", err)
	}
	if !tenantNull || !verified {
		t.Fatalf("activated user tenant_null=%v verified=%v", tenantNull, verified)
	}
	if err := rawDB.QueryRow(`
		SELECT m.is_active, m.onboarding_status, entitlement.status,
		       (SELECT COUNT(*) FROM memberships
		        WHERE scope_type = 'msp' AND scope_id = m.id::text
		          AND role = 'msp_owner' AND status = 'active'),
		       (SELECT accepted_at IS NOT NULL FROM account_invitations WHERE id = $2),
		       (SELECT COUNT(*) FROM control_plane_audit
		        WHERE msp_id = m.id AND action = 'msp.owner_activated')
		FROM msp_tenants m JOIN plan_entitlements entitlement ON entitlement.msp_id = m.id
		WHERE m.id = $1
	`, created.MSPID, rotated.InvitationID).Scan(&active, &onboarding, &entitlementStatus, &ownerCount, &invitationAccepted, &activationAudits); err != nil {
		t.Fatal(err)
	}
	if !active || onboarding != "active" || entitlementStatus != "active" || ownerCount != 1 || !invitationAccepted || activationAudits != 1 {
		t.Fatalf("activation state active=%v onboarding=%q entitlement=%q owners=%d accepted=%v audits=%d",
			active, onboarding, entitlementStatus, ownerCount, invitationAccepted, activationAudits)
	}
	var auditText string
	if err := rawDB.QueryRow(`
		SELECT string_agg(action || details::text, ' ')
		FROM control_plane_audit WHERE msp_id = $1
	`, created.MSPID).Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"lifecycle.owner@example.test", firstMail.Token, secondMail.Token, "abcdefghijklmn", passwordHash} {
		if strings.Contains(auditText, secret) {
			t.Fatalf("control-plane audit contains prohibited activation data")
		}
	}
	if err := service.accept(context.Background(), secondMail.Token, "abcdefghijklmn"); !errors.Is(err, errInvitationInvalid) {
		t.Fatalf("replayed invitation error = %v", err)
	}
	if out := login(); out.Code != http.StatusOK || strings.Count(out.Body.String(), `"token"`) != 1 {
		t.Fatalf("post-activation login status = %d: %s", out.Code, out.Body.String())
	}
	_ = userID
}

func TestOwnerActivationConcurrentAcceptanceExactlyOnceAndExistingPasswordSafe(t *testing.T) {
	rawDB, _ := providerHandlerDatabase(t)
	actors := seedActivationActors(t, rawDB)
	mailer := &capturingAccountMailer{}
	service := newOwnerInvitationService(rawDB, mailer, "https://rmm.example.test")
	service.bcryptCost = bcrypt.MinCost
	created, err := service.createPendingMSP(context.Background(), createPendingMSPInput{
		Name: "Concurrent MSP", Slug: "concurrent-msp", Plan: "free",
		OwnerEmail: "concurrent@example.test", ActorID: actors["owner"],
	})
	if err != nil {
		t.Fatal(err)
	}
	token := mailer.last(t).Token
	password := strings.Repeat("z", 72)
	start := make(chan struct{})
	errorsOut := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			errorsOut <- service.accept(context.Background(), token, password)
		}()
	}
	close(start)
	var success, rejected int
	for range 2 {
		if err := <-errorsOut; err == nil {
			success++
		} else if errors.Is(err, errInvitationInvalid) {
			rejected++
		} else {
			t.Fatalf("concurrent acceptance error = %v", err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("concurrent results success=%d rejected=%d", success, rejected)
	}
	var users, owners, audits int
	if err := rawDB.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM users WHERE normalized_email = 'concurrent@example.test'),
			(SELECT COUNT(*) FROM memberships WHERE scope_type = 'msp' AND scope_id = $1::uuid::text AND role = 'msp_owner' AND status = 'active'),
			(SELECT COUNT(*) FROM control_plane_audit WHERE msp_id = $1::uuid AND action = 'msp.owner_activated')
	`, created.MSPID).Scan(&users, &owners, &audits); err != nil {
		t.Fatal(err)
	}
	if users != 1 || owners != 1 || audits != 1 {
		t.Fatalf("concurrent committed state users=%d owners=%d audits=%d", users, owners, audits)
	}

	protected, err := service.createPendingMSP(context.Background(), createPendingMSPInput{
		Name: "Protected MSP", Slug: "protected-password-msp", Plan: "free",
		OwnerEmail: "protected@example.test", ActorID: actors["admin"],
	})
	if err != nil {
		t.Fatal(err)
	}
	protectedToken := mailer.last(t).Token
	existingHash, err := bcrypt.GenerateFromPassword([]byte("existing-password-safe"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	platformActivationExec(t, rawDB, `
		INSERT INTO users (tenant_id, email, password_hash, role, is_active, email_verified_at)
		VALUES (NULL, 'protected@example.test', $1, 'viewer', TRUE, NOW())
	`, string(existingHash))
	if err := service.accept(context.Background(), protectedToken, "replacement-password"); !errors.Is(err, errInvitationEmailRegistered) {
		t.Fatalf("existing account acceptance error = %v", err)
	}
	var afterHash string
	if err := rawDB.QueryRow(`SELECT password_hash FROM users WHERE normalized_email = 'protected@example.test'`).Scan(&afterHash); err != nil {
		t.Fatal(err)
	}
	if afterHash != string(existingHash) {
		t.Fatal("existing account password hash changed")
	}
	var protectedActive bool
	if err := rawDB.QueryRow(`SELECT is_active FROM msp_tenants WHERE id = $1`, protected.MSPID).Scan(&protectedActive); err != nil || protectedActive {
		t.Fatalf("protected MSP active=%v err=%v", protectedActive, err)
	}
}

func TestOwnerInvitationServiceAuthorizationAndInvalidStates(t *testing.T) {
	rawDB, _ := providerHandlerDatabase(t)
	actors := seedActivationActors(t, rawDB)
	mailer := &capturingAccountMailer{err: errors.New("undelivered")}
	service := newOwnerInvitationService(rawDB, mailer, "https://rmm.example.test")
	service.bcryptCost = bcrypt.MinCost
	for label, actor := range map[string]string{"expired": actors["expired"], "support": actors["support"], "msp": actors["msp"]} {
		_, err := service.createPendingMSP(context.Background(), createPendingMSPInput{
			Name: "Denied " + label, Slug: "denied-" + label, Plan: "free",
			OwnerEmail: label + "-target@example.test", ActorID: actor,
		})
		if !errors.Is(err, errPlatformAuthorization) {
			t.Fatalf("%s actor error = %v", label, err)
		}
	}
	if _, err := service.createPendingMSP(context.Background(), createPendingMSPInput{
		Name: "Duplicate identity", Slug: "duplicate-identity", Plan: "free",
		OwnerEmail: "PROVIDER-OWNER@EXAMPLE.TEST", ActorID: actors["owner"],
	}); !errors.Is(err, errInvitationEmailRegistered) {
		t.Fatalf("case-insensitive registered email preflight error = %v", err)
	}
	for label, actor := range map[string]string{"owner": actors["owner"], "admin": actors["admin"]} {
		if _, err := service.createPendingMSP(context.Background(), createPendingMSPInput{
			Name: "Allowed " + label, Slug: "allowed-" + label, Plan: "free",
			OwnerEmail: label + "-allowed@example.test", ActorID: actor,
		}); err != nil {
			t.Fatalf("%s actor rejected: %v", label, err)
		}
	}

	expired, err := service.createPendingMSP(context.Background(), createPendingMSPInput{
		Name: "Expired MSP", Slug: "expired-invitation", Plan: "free",
		OwnerEmail: "expired-target@example.test", ActorID: actors["owner"],
	})
	if err != nil {
		t.Fatal(err)
	}
	expiredToken := mailer.last(t).Token
	platformActivationExec(t, rawDB, `
		UPDATE account_invitations
		SET created_at = NOW() - INTERVAL '2 hours', expires_at = NOW() - INTERVAL '1 hour'
		WHERE msp_id = $1
	`, expired.MSPID)
	if _, err := service.inspect(context.Background(), expiredToken); !errors.Is(err, errInvitationInvalid) {
		t.Fatalf("expired inspection error = %v", err)
	}
	if err := service.accept(context.Background(), expiredToken, "valid-password1"); !errors.Is(err, errInvitationInvalid) {
		t.Fatalf("expired acceptance error = %v", err)
	}

	revoked, err := service.createPendingMSP(context.Background(), createPendingMSPInput{
		Name: "Revoked MSP", Slug: "revoked-invitation", Plan: "free",
		OwnerEmail: "revoked-target@example.test", ActorID: actors["admin"],
	})
	if err != nil {
		t.Fatal(err)
	}
	revokedToken := mailer.last(t).Token
	platformActivationExec(t, rawDB, `UPDATE account_invitations SET revoked_at = NOW() WHERE msp_id = $1`, revoked.MSPID)
	if err := service.accept(context.Background(), revokedToken, "valid-password1"); !errors.Is(err, errInvitationInvalid) {
		t.Fatalf("revoked acceptance error = %v", err)
	}
	if err := service.accept(context.Background(), "malformed", "valid-password1"); !errors.Is(err, errInvitationInvalid) {
		t.Fatalf("malformed acceptance error = %v", err)
	}

	atomic, err := service.createPendingMSP(context.Background(), createPendingMSPInput{
		Name: "Atomic MSP", Slug: "atomic-owner-activation", Plan: "free",
		OwnerEmail: "atomic-target@example.test", ActorID: actors["owner"],
	})
	if err != nil {
		t.Fatal(err)
	}
	atomicToken := mailer.last(t).Token
	platformActivationExec(t, rawDB, `
		CREATE OR REPLACE FUNCTION fail_owner_activation_audit()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.action = 'msp.owner_activated' THEN
				RAISE EXCEPTION 'injected owner activation audit failure';
			END IF;
			RETURN NEW;
		END
		$$
	`)
	platformActivationExec(t, rawDB, `
		CREATE TRIGGER fail_owner_activation_audit
		BEFORE INSERT ON control_plane_audit
		FOR EACH ROW EXECUTE FUNCTION fail_owner_activation_audit()
	`)
	activationErr := service.accept(context.Background(), atomicToken, "valid-password1")
	if activationErr == nil || strings.Contains(activationErr.Error(), atomicToken) {
		t.Fatalf("audit failure activation error = %v", activationErr)
	}
	platformActivationExec(t, rawDB, `DROP TRIGGER fail_owner_activation_audit ON control_plane_audit`)
	platformActivationExec(t, rawDB, `DROP FUNCTION fail_owner_activation_audit()`)
	var atomicActive, atomicAccepted bool
	var atomicEntitlement string
	var atomicUsers, atomicOwners int
	if err := rawDB.QueryRow(`
		SELECT m.is_active, entitlement.status,
		       invitation.accepted_at IS NOT NULL,
		       (SELECT COUNT(*) FROM users WHERE normalized_email = 'atomic-target@example.test'),
		       (SELECT COUNT(*) FROM memberships
		        WHERE scope_type = 'msp' AND scope_id = m.id::text
		          AND role = 'msp_owner' AND status = 'active')
		FROM msp_tenants m
		JOIN plan_entitlements entitlement ON entitlement.msp_id = m.id
		JOIN account_invitations invitation ON invitation.msp_id = m.id
		WHERE m.id = $1 AND invitation.revoked_at IS NULL
	`, atomic.MSPID).Scan(&atomicActive, &atomicEntitlement, &atomicAccepted, &atomicUsers, &atomicOwners); err != nil {
		t.Fatal(err)
	}
	if atomicActive || atomicAccepted || atomicEntitlement != "suspended" || atomicUsers != 0 || atomicOwners != 0 {
		t.Fatalf("failed activation partially committed: active=%v accepted=%v entitlement=%q users=%d owners=%d",
			atomicActive, atomicAccepted, atomicEntitlement, atomicUsers, atomicOwners)
	}

	var deniedMSPs int
	if err := rawDB.QueryRow(`SELECT COUNT(*) FROM msp_tenants WHERE slug LIKE 'denied-%'`).Scan(&deniedMSPs); err != nil || deniedMSPs != 0 {
		t.Fatalf("denied actors created %d MSPs (err=%v)", deniedMSPs, err)
	}
}
