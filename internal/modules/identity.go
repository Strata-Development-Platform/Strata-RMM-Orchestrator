package modules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

const DefaultServiceIdentityTTL = 5 * time.Minute

var ErrIdentityRevoked = errors.New("module service identity revoked")

type ServiceIdentityRequest struct {
	ModuleID    string
	MSPID       string
	ClientID    string
	SiteID      string
	Permissions []string
	TTL         time.Duration
}

type ServiceCredential struct {
	Token     string
	TokenID   string
	ExpiresAt time.Time
}

// RevocationStore is intentionally separate from JWT signature validation.
// Implementations used in production must be shared/durable enough for every
// API replica to observe revocation. Any store error is treated fail closed.
type RevocationStore interface {
	Revoke(context.Context, string, time.Time) error
	IsRevoked(context.Context, string) (bool, error)
}

type IdentityManager struct {
	registry    *Registry
	tokens      *auth.TokenGenerator
	revocations RevocationStore
}

func NewIdentityManager(registry *Registry, tokens *auth.TokenGenerator, revocations RevocationStore) (*IdentityManager, error) {
	if registry == nil {
		return nil, errors.New("module identity registry is required")
	}
	if tokens == nil {
		return nil, errors.New("module identity token generator is required")
	}
	if revocations == nil {
		return nil, errors.New("module identity revocation store is required")
	}
	return &IdentityManager{registry: registry, tokens: tokens, revocations: revocations}, nil
}

func (m *IdentityManager) Issue(ctx context.Context, req ServiceIdentityRequest) (ServiceCredential, error) {
	module, err := m.registry.Get(req.ModuleID)
	if err != nil {
		return ServiceCredential{}, err
	}
	if module.State != StateEnabled {
		return ServiceCredential{}, ErrPermissionDenied
	}
	if err := validateServiceScope(req.MSPID, req.ClientID, req.SiteID); err != nil {
		return ServiceCredential{}, err
	}
	permissions, err := m.validateRequestedPermissions(req.ModuleID, req.Permissions)
	if err != nil {
		return ServiceCredential{}, err
	}
	ttl := req.TTL
	if ttl == 0 {
		ttl = DefaultServiceIdentityTTL
	}
	token, err := m.tokens.GenerateModuleToken(req.ModuleID, req.MSPID, req.ClientID, req.SiteID, permissions, ttl)
	if err != nil {
		return ServiceCredential{}, fmt.Errorf("issue module service token: %w", err)
	}
	claims, err := m.tokens.Validate(token)
	if err != nil {
		return ServiceCredential{}, fmt.Errorf("validate issued module service token: %w", err)
	}
	return ServiceCredential{
		Token:     token,
		TokenID:   claims.TokenID,
		ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
	}, nil
}

func (m *IdentityManager) Validate(ctx context.Context, token string) (*auth.Claims, error) {
	claims, err := m.tokens.Validate(token)
	if err != nil {
		return nil, err
	}
	if claims.TokenUse != "module" {
		return nil, errors.New("module service identity required")
	}
	if err := validateServiceScope(claims.MSPID, claims.ClientID, claims.SiteID); err != nil {
		return nil, err
	}
	module, err := m.registry.Get(claims.ModuleID)
	if err != nil {
		return nil, err
	}
	if module.State != StateEnabled {
		return nil, ErrPermissionDenied
	}
	for _, permission := range claims.Permissions {
		if err := m.registry.RequirePermission(claims.ModuleID, permission); err != nil {
			return nil, ErrPermissionDenied
		}
	}
	revoked, err := m.revocations.IsRevoked(ctx, claims.TokenID)
	if err != nil {
		return nil, fmt.Errorf("verify module service token revocation: %w", err)
	}
	if revoked {
		return nil, ErrIdentityRevoked
	}
	return claims, nil
}

func (m *IdentityManager) Revoke(ctx context.Context, token string) error {
	claims, err := m.tokens.Validate(token)
	if err != nil {
		return err
	}
	if claims.TokenUse != "module" {
		return errors.New("module service identity required")
	}
	if err := m.revocations.Revoke(ctx, claims.TokenID, time.Unix(claims.ExpiresAt, 0).UTC()); err != nil {
		return fmt.Errorf("revoke module service token: %w", err)
	}
	return nil
}

func (m *IdentityManager) validateRequestedPermissions(moduleID string, requested []string) ([]string, error) {
	seen := make(map[string]struct{}, len(requested))
	permissions := make([]string, 0, len(requested))
	for _, permission := range requested {
		if permission == "" {
			return nil, errors.New("module service permission may not be empty")
		}
		if _, duplicate := seen[permission]; duplicate {
			return nil, fmt.Errorf("duplicate module service permission %q", permission)
		}
		if err := m.registry.RequirePermission(moduleID, permission); err != nil {
			return nil, ErrPermissionDenied
		}
		seen[permission] = struct{}{}
		permissions = append(permissions, permission)
	}
	return permissions, nil
}

func validateServiceScope(mspID, clientID, siteID string) error {
	if siteID != "" && clientID == "" {
		return errors.New("module site scope requires client scope")
	}
	if clientID != "" && mspID == "" {
		return errors.New("module client scope requires MSP scope")
	}
	return nil
}

// MemoryRevocationStore is suitable for unit tests and single-process
// development only. Production module execution must use a shared store so a
// revocation is observed by every API replica and survives process restart.
type MemoryRevocationStore struct {
	mu      sync.Mutex
	revoked map[string]time.Time
	now     func() time.Time
}

func NewMemoryRevocationStore() *MemoryRevocationStore {
	return &MemoryRevocationStore{revoked: make(map[string]time.Time), now: time.Now}
}

func (s *MemoryRevocationStore) Revoke(_ context.Context, tokenID string, expiresAt time.Time) error {
	if tokenID == "" {
		return errors.New("module service token id is required")
	}
	if expiresAt.IsZero() {
		return errors.New("module service token expiry is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[tokenID] = expiresAt
	return nil
}

func (s *MemoryRevocationStore) IsRevoked(_ context.Context, tokenID string) (bool, error) {
	if tokenID == "" {
		return false, errors.New("module service token id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.revoked[tokenID]
	if !ok {
		return false, nil
	}
	if !expiresAt.After(s.now()) {
		delete(s.revoked, tokenID)
		return false, nil
	}
	return true, nil
}
