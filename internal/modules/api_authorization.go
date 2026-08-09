package modules

import (
	"context"
	"errors"
	"fmt"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

var (
	ErrModuleIdentityMismatch = errors.New("module identity mismatch")
	ErrScopeDenied            = errors.New("module service scope denied")
)

// ResourceScope describes the organizational scope of the resource a module
// wants to access through a brokered Strata API. Empty child identifiers mean
// the resource is scoped at the nearest populated ancestor.
type ResourceScope struct {
	MSPID    string
	ClientID string
	SiteID   string
}

// APIAuthorizationRequest is transport independent. HTTP and future event
// bridges must pass the module identity expected by the route/subject, the
// capability being exercised, and the scope of the concrete target resource.
type APIAuthorizationRequest struct {
	ModuleID   string
	Permission string
	Scope      ResourceScope
}

// APIAuthorizer is the common authorization boundary for out-of-process module
// requests. It deliberately reuses IdentityManager validation so revocation,
// lifecycle state, current manifest permissions, and token integrity are all
// checked before resource-scope authorization.
type APIAuthorizer struct {
	identities *IdentityManager
}

func NewAPIAuthorizer(identities *IdentityManager) (*APIAuthorizer, error) {
	if identities == nil {
		return nil, errors.New("module identity manager is required")
	}
	return &APIAuthorizer{identities: identities}, nil
}

func (a *APIAuthorizer) Authorize(ctx context.Context, token string, req APIAuthorizationRequest) (*auth.Claims, error) {
	if req.ModuleID == "" {
		return nil, errors.New("module id is required")
	}
	if req.Permission == "" {
		return nil, errors.New("module permission is required")
	}
	if err := validateServiceScope(req.Scope.MSPID, req.Scope.ClientID, req.Scope.SiteID); err != nil {
		return nil, fmt.Errorf("invalid target scope: %w", err)
	}

	claims, err := a.identities.Validate(ctx, token)
	if err != nil {
		return nil, err
	}
	if claims.ModuleID != req.ModuleID {
		return nil, ErrModuleIdentityMismatch
	}
	if !containsPermission(claims.Permissions, req.Permission) {
		return nil, ErrPermissionDenied
	}
	if !scopeAllows(claims.MSPID, claims.ClientID, claims.SiteID, req.Scope) {
		return nil, ErrScopeDenied
	}
	return claims, nil
}

func containsPermission(permissions []string, required string) bool {
	for _, permission := range permissions {
		if permission == required {
			return true
		}
	}
	return false
}

// scopeAllows implements ancestor-to-descendant authorization. A token scoped
// to an MSP may address resources within that MSP; a client-scoped token may
// address only that client and its sites; a site-scoped token may address only
// that site. A scoped token can never escape to an ancestor or sibling scope.
func scopeAllows(tokenMSP, tokenClient, tokenSite string, target ResourceScope) bool {
	if tokenMSP != "" {
		if target.MSPID == "" || target.MSPID != tokenMSP {
			return false
		}
	}
	if tokenClient != "" {
		if target.ClientID == "" || target.ClientID != tokenClient {
			return false
		}
	}
	if tokenSite != "" {
		if target.SiteID == "" || target.SiteID != tokenSite {
			return false
		}
	}
	return true
}
