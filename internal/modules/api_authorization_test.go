package modules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

func newModuleAPIAuthorizer(t *testing.T, permissions []string) (*APIAuthorizer, *IdentityManager, string) {
	t.Helper()
	registry := NewRegistry()
	manifest := validManifest()
	manifest.Permissions = permissions
	manifest.Routes = nil
	installed, err := registry.Install(manifest)
	if err != nil {
		t.Fatalf("install module: %v", err)
	}
	if _, err := registry.Enable(installed.Manifest.ID); err != nil {
		t.Fatalf("enable module: %v", err)
	}
	generator := auth.NewTokenGenerator("01234567890123456789012345678901")
	identities, err := NewIdentityManager(registry, generator, NewMemoryRevocationStore())
	if err != nil {
		t.Fatalf("new identity manager: %v", err)
	}
	authorizer, err := NewAPIAuthorizer(identities)
	if err != nil {
		t.Fatalf("new API authorizer: %v", err)
	}
	return authorizer, identities, installed.Manifest.ID
}

func issueModuleToken(t *testing.T, identities *IdentityManager, moduleID, mspID, clientID, siteID string, permissions []string) string {
	t.Helper()
	credential, err := identities.Issue(context.Background(), ServiceIdentityRequest{
		ModuleID: moduleID, MSPID: mspID, ClientID: clientID, SiteID: siteID,
		Permissions: permissions, TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("issue module token: %v", err)
	}
	return credential.Token
}

func TestAPIAuthorizerAllowsDescendantScopeWithinTokenBoundary(t *testing.T) {
	authorizer, identities, moduleID := newModuleAPIAuthorizer(t, []string{"devices.read"})
	token := issueModuleToken(t, identities, moduleID, "msp-a", "", "", []string{"devices.read"})
	claims, err := authorizer.Authorize(context.Background(), token, APIAuthorizationRequest{
		ModuleID: moduleID, Permission: "devices.read",
		Scope: ResourceScope{MSPID: "msp-a", ClientID: "client-a", SiteID: "site-a"},
	})
	if err != nil {
		t.Fatalf("authorize descendant scope: %v", err)
	}
	if claims.ModuleID != moduleID {
		t.Fatalf("module id = %q, want %q", claims.ModuleID, moduleID)
	}
}

func TestAPIAuthorizerDeniesSiblingAndAncestorScopeEscape(t *testing.T) {
	authorizer, identities, moduleID := newModuleAPIAuthorizer(t, []string{"devices.read"})
	token := issueModuleToken(t, identities, moduleID, "msp-a", "client-a", "site-a", []string{"devices.read"})
	cases := []ResourceScope{
		{MSPID: "msp-b", ClientID: "client-b", SiteID: "site-b"},
		{MSPID: "msp-a", ClientID: "client-b", SiteID: "site-b"},
		{MSPID: "msp-a", ClientID: "client-a", SiteID: "site-b"},
		{MSPID: "msp-a", ClientID: "client-a"},
		{MSPID: "msp-a"},
	}
	for _, scope := range cases {
		_, err := authorizer.Authorize(context.Background(), token, APIAuthorizationRequest{
			ModuleID: moduleID, Permission: "devices.read", Scope: scope,
		})
		if !errors.Is(err, ErrScopeDenied) {
			t.Fatalf("scope %+v error = %v, want ErrScopeDenied", scope, err)
		}
	}
}

func TestAPIAuthorizerDeniesPermissionAndModuleEscalation(t *testing.T) {
	authorizer, identities, moduleID := newModuleAPIAuthorizer(t, []string{"devices.read", "alerts.read"})
	token := issueModuleToken(t, identities, moduleID, "msp-a", "client-a", "", []string{"devices.read"})

	if _, err := authorizer.Authorize(context.Background(), token, APIAuthorizationRequest{
		ModuleID: moduleID, Permission: "alerts.read", Scope: ResourceScope{MSPID: "msp-a", ClientID: "client-a"},
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("permission escalation error = %v", err)
	}
	if _, err := authorizer.Authorize(context.Background(), token, APIAuthorizationRequest{
		ModuleID: "other.module", Permission: "devices.read", Scope: ResourceScope{MSPID: "msp-a", ClientID: "client-a"},
	}); !errors.Is(err, ErrModuleIdentityMismatch) {
		t.Fatalf("module substitution error = %v", err)
	}
}

func TestAPIAuthorizerPropagatesRevocationFailClosed(t *testing.T) {
	authorizer, identities, moduleID := newModuleAPIAuthorizer(t, []string{"devices.read"})
	token := issueModuleToken(t, identities, moduleID, "msp-a", "", "", []string{"devices.read"})
	if err := identities.Revoke(context.Background(), token); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_, err := authorizer.Authorize(context.Background(), token, APIAuthorizationRequest{
		ModuleID: moduleID, Permission: "devices.read", Scope: ResourceScope{MSPID: "msp-a"},
	})
	if !errors.Is(err, ErrIdentityRevoked) {
		t.Fatalf("revoked token error = %v", err)
	}
}

func TestAPIAuthorizerRejectsMalformedTargetHierarchy(t *testing.T) {
	authorizer, identities, moduleID := newModuleAPIAuthorizer(t, []string{"devices.read"})
	token := issueModuleToken(t, identities, moduleID, "msp-a", "", "", []string{"devices.read"})
	_, err := authorizer.Authorize(context.Background(), token, APIAuthorizationRequest{
		ModuleID: moduleID, Permission: "devices.read", Scope: ResourceScope{SiteID: "site-a"},
	})
	if err == nil {
		t.Fatal("malformed target scope unexpectedly authorized")
	}
}
