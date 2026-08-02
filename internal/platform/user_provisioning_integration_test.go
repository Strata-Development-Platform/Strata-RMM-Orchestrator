//go:build dbintegration

package platform

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

func TestScopedMembershipReplacementPreservesUnmanagedAndSerializes(t *testing.T) {
	raw, client := providerHandlerDatabase(t)
	const (
		secret   = "membership-replacement-test-secret-32-bytes"
		tenantID = "71000000-0000-0000-0000-000000000001"
		actorID  = "71000000-0000-0000-0000-000000000002"
		targetID = "71000000-0000-0000-0000-000000000003"
		mspA     = "71000000-0000-0000-0000-000000000004"
		mspB     = "71000000-0000-0000-0000-000000000005"
	)

	if _, err := raw.Exec(`
		UPDATE platforms
		SET setup_completed_at = NOW(), setup_completed_by = $1
		WHERE id = $2;
		INSERT INTO tenants (id, name, slug, plan)
		VALUES ($3, 'Membership tenant', 'membership-replacement', 'enterprise');
		INSERT INTO msp_tenants (id, name, slug, is_active, onboarding_status) VALUES
			($4, 'MSP A', 'membership-msp-a', true, 'active'),
			($5, 'MSP B', 'membership-msp-b', true, 'active');
		INSERT INTO users (id, tenant_id, email, password_hash, role, email_verified_at) VALUES
			($1, $3, 'membership-actor@example.test', '$2a$10$test', 'admin', NOW()),
			($6, $3, 'membership-target@example.test', '$2a$10$test', 'viewer', NOW());
		INSERT INTO memberships (user_id, scope_type, scope_id, role, status) VALUES
			($1, 'platform', $2, 'platform_owner', 'active'),
			($1, 'msp', $4, 'msp_owner', 'active'),
			($6, 'msp', $4, 'msp_viewer', 'active'),
			($6, 'msp', $5, 'msp_viewer', 'active')
	`, actorID, postgres.SingletonPlatformID, tenantID, mspA, mspB, targetID); err != nil {
		t.Fatalf("seed membership replacement test: %v", err)
	}

	tokenGenerator := auth.NewTokenGenerator(secret)
	server := &APIServer{db: client, tokenGen: tokenGenerator}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/admin/users/{userID}/memberships", server.handleAdminUpdateUserTenants)
	handler := server.withAccessControl(server.withTenantTransaction(mux))
	actorToken := providerUserToken(t, tokenGenerator, actorID, tenantID, mspA, "msp_owner")

	// A selected-MSP manager editing their own MSP role must not revoke their
	// unrelated singleton-platform membership through the RLS self-read branch.
	selfPayload := fmt.Sprintf(`{"memberships":[{"scope_type":"msp","scope_id":%q,"role":"msp_admin"}]}`, mspA)
	self := providerHTTPRequest(handler, http.MethodPut, "/api/v1/admin/users/"+actorID+"/memberships", actorToken, selfPayload)
	if self.Code != http.StatusOK {
		t.Fatalf("self replacement status = %d: %s", self.Code, self.Body.String())
	}
	var platformOwners, actorMSPAdmins int
	if err := raw.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE scope_type = 'platform' AND scope_id = $2 AND role = 'platform_owner'),
			COUNT(*) FILTER (WHERE scope_type = 'msp' AND scope_id = $3 AND role = 'msp_admin')
		FROM memberships
		WHERE user_id = $1 AND status = 'active'
	`, actorID, postgres.SingletonPlatformID, mspA).Scan(&platformOwners, &actorMSPAdmins); err != nil {
		t.Fatal(err)
	}
	if platformOwners != 1 || actorMSPAdmins != 1 {
		t.Fatalf("self replacement changed unrelated authority: platform owners=%d, MSP admins=%d", platformOwners, actorMSPAdmins)
	}

	// Two requests that both passed authentication before either replacement
	// completes must serialize on the target user and leave one active role in
	// the managed scope. The unrelated MSP B membership must survive.
	payloads := []string{
		fmt.Sprintf(`{"memberships":[{"scope_type":"msp","scope_id":%q,"role":"msp_admin"}]}`, mspA),
		fmt.Sprintf(`{"memberships":[{"scope_type":"msp","scope_id":%q,"role":"msp_technician"}]}`, mspA),
	}
	type response struct {
		status int
		body   string
	}
	responses := make(chan response, len(payloads))
	var group sync.WaitGroup
	for _, payload := range payloads {
		payload := payload
		group.Add(1)
		go func() {
			defer group.Done()
			out := providerHTTPRequest(handler, http.MethodPut, "/api/v1/admin/users/"+targetID+"/memberships", actorToken, payload)
			responses <- response{status: out.Code, body: out.Body.String()}
		}()
	}
	group.Wait()
	close(responses)
	for result := range responses {
		if result.status != http.StatusOK {
			t.Fatalf("concurrent replacement status = %d: %s", result.status, result.body)
		}
	}

	var activeA, activeB int
	var roleA string
	if err := raw.QueryRow(`
		SELECT COUNT(*), COALESCE(MIN(role), '')
		FROM memberships
		WHERE user_id = $1 AND scope_type = 'msp' AND scope_id = $2 AND status = 'active'
	`, targetID, mspA).Scan(&activeA, &roleA); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`
		SELECT COUNT(*)
		FROM memberships
		WHERE user_id = $1 AND scope_type = 'msp' AND scope_id = $2
		  AND role = 'msp_viewer' AND status = 'active'
	`, targetID, mspB).Scan(&activeB); err != nil {
		t.Fatal(err)
	}
	if activeA != 1 || (roleA != "msp_admin" && roleA != "msp_technician") {
		t.Fatalf("concurrent replacements left invalid managed state: count=%d role=%q", activeA, roleA)
	}
	if activeB != 1 {
		t.Fatalf("replacement revoked unrelated MSP membership: count=%d", activeB)
	}
}
