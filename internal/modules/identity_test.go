package modules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

func TestIdentityManagerIssuesLeastPrivilegeModuleToken(t *testing.T) {
	registry := enabledTestRegistry(t)
	manager := newTestIdentityManager(t, registry, NewMemoryRevocationStore())

	credential, err := manager.Issue(context.Background(), ServiceIdentityRequest{
		ModuleID:    validManifest().ID,
		MSPID:       "msp-1",
		ClientID:    "client-1",
		SiteID:      "site-1",
		Permissions: []string{"devices.read"},
		TTL:         2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("issue identity: %v", err)
	}
	if credential.Token == "" || credential.TokenID == "" || credential.ExpiresAt.IsZero() {
		t.Fatalf("incomplete credential: %+v", credential)
	}

	claims, err := manager.Validate(context.Background(), credential.Token)
	if err != nil {
		t.Fatalf("validate identity: %v", err)
	}
	if claims.TokenUse != "module" || claims.ModuleID != validManifest().ID {
		t.Fatalf("unexpected module claims: %+v", claims)
	}
	if claims.MSPID != "msp-1" || claims.ClientID != "client-1" || claims.SiteID != "site-1" {
		t.Fatalf("scope mismatch: %+v", claims)
	}
	if len(claims.Permissions) != 1 || claims.Permissions[0] != "devices.read" {
		t.Fatalf("permissions = %v", claims.Permissions)
	}
}

func TestIdentityManagerRejectsPrivilegeEscalationAndInvalidScope(t *testing.T) {
	registry := enabledTestRegistry(t)
	manager := newTestIdentityManager(t, registry, NewMemoryRevocationStore())

	_, err := manager.Issue(context.Background(), ServiceIdentityRequest{
		ModuleID:    validManifest().ID,
		MSPID:       "msp-1",
		Permissions: []string{"reports.write"},
	})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("undeclared permission error = %v", err)
	}

	_, err = manager.Issue(context.Background(), ServiceIdentityRequest{
		ModuleID:    validManifest().ID,
		ClientID:    "client-1",
		Permissions: []string{"devices.read"},
	})
	if err == nil {
		t.Fatal("client scope without MSP unexpectedly accepted")
	}

	_, err = manager.Issue(context.Background(), ServiceIdentityRequest{
		ModuleID:    validManifest().ID,
		MSPID:       "msp-1",
		SiteID:      "site-1",
		Permissions: []string{"devices.read"},
	})
	if err == nil {
		t.Fatal("site scope without client unexpectedly accepted")
	}
}

func TestIdentityManagerDisablingOrQuarantiningModuleInvalidatesToken(t *testing.T) {
	for _, transition := range []struct {
		name string
		fn   func(*Registry, string) error
	}{
		{name: "disabled", fn: func(r *Registry, id string) error {
			_, err := r.Disable(id, "maintenance")
			return err
		}},
		{name: "quarantined", fn: func(r *Registry, id string) error {
			_, err := r.Quarantine(id, "runtime failure")
			return err
		}},
	} {
		t.Run(transition.name, func(t *testing.T) {
			registry := enabledTestRegistry(t)
			manager := newTestIdentityManager(t, registry, NewMemoryRevocationStore())
			credential, err := manager.Issue(context.Background(), ServiceIdentityRequest{
				ModuleID:    validManifest().ID,
				Permissions: []string{"devices.read"},
			})
			if err != nil {
				t.Fatalf("issue: %v", err)
			}
			if err := transition.fn(registry, validManifest().ID); err != nil {
				t.Fatalf("transition: %v", err)
			}
			if _, err := manager.Validate(context.Background(), credential.Token); !errors.Is(err, ErrPermissionDenied) {
				t.Fatalf("outstanding token survived %s module: %v", transition.name, err)
			}
		})
	}
}

func TestIdentityManagerRevocationIsImmediateAndFailClosed(t *testing.T) {
	registry := enabledTestRegistry(t)
	store := NewMemoryRevocationStore()
	manager := newTestIdentityManager(t, registry, store)
	credential, err := manager.Issue(context.Background(), ServiceIdentityRequest{
		ModuleID:    validManifest().ID,
		Permissions: []string{"devices.read"},
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := manager.Revoke(context.Background(), credential.Token); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := manager.Validate(context.Background(), credential.Token); !errors.Is(err, ErrIdentityRevoked) {
		t.Fatalf("revoked token validation = %v", err)
	}

	failing := newTestIdentityManager(t, registry, failingRevocationStore{})
	credential, err = manager.Issue(context.Background(), ServiceIdentityRequest{
		ModuleID:    validManifest().ID,
		Permissions: []string{"devices.read"},
	})
	if err != nil {
		t.Fatalf("issue second token: %v", err)
	}
	if _, err := failing.Validate(context.Background(), credential.Token); err == nil {
		t.Fatal("revocation store outage failed open")
	}
}

func TestIdentityManagerRejectsUserToken(t *testing.T) {
	registry := enabledTestRegistry(t)
	generator := auth.NewTokenGenerator(testJWTSecret)
	manager, err := NewIdentityManager(registry, generator, NewMemoryRevocationStore())
	if err != nil {
		t.Fatal(err)
	}
	userToken, err := generator.GenerateUserToken("user-1", "client-1", "msp-1", "client-1", "site-1", []string{"msp_admin"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Validate(context.Background(), userToken); err == nil {
		t.Fatal("user token accepted as module identity")
	}
}

const testJWTSecret = "module-identity-test-secret-32-bytes-minimum-value"

func enabledTestRegistry(t *testing.T) *Registry {
	t.Helper()
	registry := NewRegistry()
	manifest := validManifest()
	if _, err := registry.Install(manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Enable(manifest.ID); err != nil {
		t.Fatal(err)
	}
	return registry
}

func newTestIdentityManager(t *testing.T, registry *Registry, store RevocationStore) *IdentityManager {
	t.Helper()
	manager, err := NewIdentityManager(registry, auth.NewTokenGenerator(testJWTSecret), store)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

type failingRevocationStore struct{}

func (failingRevocationStore) Revoke(context.Context, string, time.Time) error {
	return errors.New("revocation backend unavailable")
}

func (failingRevocationStore) IsRevoked(context.Context, string) (bool, error) {
	return false, errors.New("revocation backend unavailable")
}
