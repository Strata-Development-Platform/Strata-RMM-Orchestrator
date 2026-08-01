package platform

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

type ScopeType string

const (
	ScopePlatform ScopeType = "platform"
	ScopeMSP      ScopeType = "msp"
	ScopeClient   ScopeType = "client"
	ScopeSite     ScopeType = "site"
)

// AuthorizationScope is the one scope selected for a request. Ancestor IDs are
// identifiers proven from the database, not copied into the principal merely
// because they appeared in a token.
type AuthorizationScope struct {
	Type       ScopeType `json:"type"`
	ID         string    `json:"id"`
	PlatformID string    `json:"platform_id,omitempty"`
	MSPID      string    `json:"msp_id,omitempty"`
	ClientID   string    `json:"client_id,omitempty"`
	SiteID     string    `json:"site_id,omitempty"`
}

type AuthorizationGrant struct {
	Role       string    `json:"role"`
	SourceType ScopeType `json:"source_type"`
	SourceID   string    `json:"source_id"`
	Inherited  bool      `json:"inherited"`
}

// AuthorizationResult is deliberately scope-bound. Roles and permissions are
// effective only inside Selected; Grants records the exact active membership
// that supplied each role. No unrelated membership is retained in this object.
type AuthorizationResult struct {
	Selected    AuthorizationScope   `json:"selected_scope"`
	Grants      []AuthorizationGrant `json:"grants"`
	Roles       []string             `json:"roles"`
	Permissions []string             `json:"permissions"`
}

func (a AuthorizationResult) HasRole(wanted ...string) bool {
	for _, role := range a.Roles {
		for _, candidate := range wanted {
			if role == candidate {
				return true
			}
		}
	}
	return false
}

func (a AuthorizationResult) HasGrant(source ScopeType, roles ...string) bool {
	for _, grant := range a.Grants {
		if grant.SourceType != source {
			continue
		}
		for _, role := range roles {
			if grant.Role == role {
				return true
			}
		}
	}
	return false
}

func (a AuthorizationResult) IsPlatformGlobal() bool {
	return a.Selected.Type == ScopePlatform &&
		a.Selected.ID == postgres.SingletonPlatformID &&
		a.HasGrant(ScopePlatform, "platform_owner", "platform_admin")
}

func (a AuthorizationResult) HasPlatformAdministratorMembership() bool {
	return a.HasGrant(ScopePlatform, "platform_owner", "platform_admin")
}

func (a AuthorizationResult) CanManageSelectedScope() bool {
	switch a.Selected.Type {
	case ScopePlatform:
		return a.IsPlatformGlobal()
	case ScopeMSP:
		return a.HasRole("platform_owner", "platform_admin", "msp_owner", "msp_admin")
	case ScopeClient, ScopeSite:
		return a.HasRole("platform_owner", "platform_admin", "msp_owner", "msp_admin", "client_admin")
	default:
		return false
	}
}

func authorizationFromRequest(r *http.Request) AuthorizationResult {
	if authorization, ok := r.Context().Value(ctxKeyAuthorization).(AuthorizationResult); ok {
		return authorization
	}
	if authorization, ok := r.Context().Value(ctxKeyAuthorization).(*AuthorizationResult); ok && authorization != nil {
		return *authorization
	}

	// Compatibility for isolated handler unit tests that predate the structured
	// context. Production requests always receive ctxKeyAuthorization from the
	// authentication middleware.
	rolesText, _ := r.Context().Value(ctxKeyRole).(string)
	roles := strings.FieldsFunc(rolesText, func(r rune) bool { return r == ',' })
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	clientID, _ := r.Context().Value(ctxKeyClientID).(string)
	siteID, _ := r.Context().Value(ctxKeySiteID).(string)
	scope := inferredAuthorizationScope(mspID, clientID, siteID)
	grants := make([]AuthorizationGrant, 0, len(roles))
	for _, role := range roles {
		grants = append(grants, AuthorizationGrant{Role: role, SourceType: scope.Type, SourceID: scope.ID})
	}
	return newAuthorizationResult(scope, grants)
}

func inferredAuthorizationScope(mspID, clientID, siteID string) AuthorizationScope {
	scope := AuthorizationScope{Type: ScopePlatform, ID: postgres.SingletonPlatformID, PlatformID: postgres.SingletonPlatformID}
	if mspID != "" {
		scope = AuthorizationScope{Type: ScopeMSP, ID: mspID, PlatformID: postgres.SingletonPlatformID, MSPID: mspID}
	}
	if clientID != "" {
		scope.Type, scope.ID, scope.ClientID = ScopeClient, clientID, clientID
	}
	if siteID != "" {
		scope.Type, scope.ID, scope.SiteID = ScopeSite, siteID, siteID
	}
	return scope
}

func newAuthorizationResult(scope AuthorizationScope, grants []AuthorizationGrant) AuthorizationResult {
	sort.Slice(grants, func(i, j int) bool {
		if grants[i].Role != grants[j].Role {
			return grants[i].Role < grants[j].Role
		}
		if grants[i].SourceType != grants[j].SourceType {
			return grants[i].SourceType < grants[j].SourceType
		}
		return grants[i].SourceID < grants[j].SourceID
	})
	roles := make([]string, 0, len(grants))
	for _, grant := range grants {
		roles = appendUnique(roles, grant.Role)
	}
	sort.Strings(roles)
	return AuthorizationResult{
		Selected:    scope,
		Grants:      grants,
		Roles:       roles,
		Permissions: permissionsForRoles(roles),
	}
}

func (s *APIServer) resolveAuthorization(
	ctx context.Context,
	tx *sql.Tx,
	userID, claimedMSPID, claimedClientID, claimedSiteID string,
) (AuthorizationResult, error) {
	scope, err := resolveSelectedScope(ctx, tx, claimedMSPID, claimedClientID, claimedSiteID)
	if err != nil {
		return AuthorizationResult{}, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT role, scope_type, scope_id
		FROM memberships
		WHERE user_id = $1
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > statement_timestamp())
		ORDER BY scope_type, scope_id, role
	`, userID)
	if err != nil {
		return AuthorizationResult{}, fmt.Errorf("load active memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	grants := make([]AuthorizationGrant, 0)
	for rows.Next() {
		var role, scopeTypeText, scopeID string
		if err := rows.Scan(&role, &scopeTypeText, &scopeID); err != nil {
			return AuthorizationResult{}, fmt.Errorf("read active membership: %w", err)
		}
		sourceType := ScopeType(scopeTypeText)
		if !validRoleForScope(sourceType, role) || !membershipAppliesToSelectedScope(sourceType, scopeID, role, scope) {
			continue
		}
		grants = append(grants, AuthorizationGrant{
			Role:       role,
			SourceType: sourceType,
			SourceID:   scopeID,
			Inherited:  sourceType != scope.Type || scopeID != scope.ID,
		})
	}
	if err := rows.Err(); err != nil {
		return AuthorizationResult{}, fmt.Errorf("iterate active memberships: %w", err)
	}
	authorization := newAuthorizationResult(scope, grants)
	if len(authorization.Roles) == 0 {
		return AuthorizationResult{}, fmt.Errorf("selected scope has no effective active membership")
	}
	return authorization, nil
}

func (s *APIServer) resolveInitialAuthorization(
	ctx context.Context,
	tx *sql.Tx,
	userID, legacyTenantID string,
) (AuthorizationResult, error) {
	// Prefer a real singleton-platform membership so provider administrators
	// always begin in the only context where platform-global authority exists.
	if authorization, err := s.resolveAuthorization(ctx, tx, userID, "", "", ""); err == nil {
		return authorization, nil
	}

	// users.tenant_id is a compatibility hint only. It may select a client when
	// the hierarchy and an applicable current membership independently prove it.
	if legacyTenantID != "" {
		var mspID string
		if err := tx.QueryRowContext(ctx, `
			SELECT c.msp_id::text
			FROM client_organizations c
			JOIN msp_tenants m ON m.id = c.msp_id
			WHERE c.id = $1 AND c.is_active = true
			  AND m.is_active = true AND m.onboarding_status = 'active'
		`, legacyTenantID).Scan(&mspID); err == nil {
			if authorization, err := s.resolveAuthorization(ctx, tx, userID, mspID, legacyTenantID, ""); err == nil {
				return authorization, nil
			}
		}
	}

	type membershipCandidate struct {
		scopeType ScopeType
		scopeID   string
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT scope_type, scope_id
		FROM memberships
		WHERE user_id = $1
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > statement_timestamp())
		ORDER BY CASE scope_type WHEN 'msp' THEN 1 WHEN 'client' THEN 2 WHEN 'site' THEN 3 ELSE 4 END,
		         scope_id
	`, userID)
	if err != nil {
		return AuthorizationResult{}, fmt.Errorf("load initial membership candidates: %w", err)
	}
	candidates := make([]membershipCandidate, 0)
	for rows.Next() {
		var candidate membershipCandidate
		if err := rows.Scan(&candidate.scopeType, &candidate.scopeID); err != nil {
			_ = rows.Close()
			return AuthorizationResult{}, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return AuthorizationResult{}, err
	}
	_ = rows.Close()

	for _, candidate := range candidates {
		var mspID, clientID, siteID string
		switch candidate.scopeType {
		case ScopeMSP:
			mspID = candidate.scopeID
			_, _ = tx.ExecContext(ctx, `
				SELECT set_config('app.scope_type', 'msp', true),
				       set_config('app.msp_id', $1, true),
				       set_config('app.client_id', '', true),
				       set_config('app.site_id', '', true)
			`, mspID)
		case ScopeClient:
			clientID = candidate.scopeID
			_, _ = tx.ExecContext(ctx, `
				SELECT set_config('app.scope_type', 'client', true),
				       set_config('app.client_id', $1, true),
				       set_config('app.site_id', '', true)
			`, clientID)
			if err := tx.QueryRowContext(ctx, `SELECT msp_id::text FROM client_organizations WHERE id = $1`, clientID).Scan(&mspID); err != nil {
				continue
			}
			_, _ = tx.ExecContext(ctx, `SELECT set_config('app.msp_id', $1, true)`, mspID)
		case ScopeSite:
			siteID = candidate.scopeID
			_, _ = tx.ExecContext(ctx, `
				SELECT set_config('app.scope_type', 'site', true),
				       set_config('app.client_id', '', true),
				       set_config('app.site_id', $1, true)
			`, siteID)
			if err := tx.QueryRowContext(ctx, `SELECT client_id::text FROM sites WHERE id = $1`, siteID).Scan(&clientID); err != nil {
				continue
			}
			_, _ = tx.ExecContext(ctx, `SELECT set_config('app.client_id', $1, true)`, clientID)
			if err := tx.QueryRowContext(ctx, `SELECT msp_id::text FROM client_organizations WHERE id = $1`, clientID).Scan(&mspID); err != nil {
				continue
			}
			_, _ = tx.ExecContext(ctx, `SELECT set_config('app.msp_id', $1, true)`, mspID)
		default:
			continue
		}
		if authorization, err := s.resolveAuthorization(ctx, tx, userID, mspID, clientID, siteID); err == nil {
			return authorization, nil
		}
	}
	return AuthorizationResult{}, fmt.Errorf("account has no active structurally valid membership")
}

func resolveSelectedScope(ctx context.Context, tx *sql.Tx, mspID, clientID, siteID string) (AuthorizationScope, error) {
	if siteID != "" && (clientID == "" || mspID == "") {
		return AuthorizationScope{}, fmt.Errorf("site scope requires its client and MSP")
	}
	if clientID != "" && mspID == "" {
		return AuthorizationScope{}, fmt.Errorf("client scope requires its MSP")
	}
	if mspID == "" {
		return AuthorizationScope{
			Type: ScopePlatform, ID: postgres.SingletonPlatformID, PlatformID: postgres.SingletonPlatformID,
		}, nil
	}

	scope := AuthorizationScope{Type: ScopeMSP, ID: mspID, PlatformID: postgres.SingletonPlatformID, MSPID: mspID}
	if clientID == "" {
		var active bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM msp_tenants
				WHERE id = $1 AND is_active = true AND onboarding_status = 'active'
			)
		`, mspID).Scan(&active); err != nil || !active {
			return AuthorizationScope{}, fmt.Errorf("MSP scope is suspended or inactive")
		}
		return scope, nil
	}

	var resolvedMSP string
	if siteID == "" {
		if err := tx.QueryRowContext(ctx, `
			SELECT c.msp_id::text
			FROM client_organizations c
			JOIN msp_tenants m ON m.id = c.msp_id
			WHERE c.id = $1 AND c.is_active = true
			  AND m.is_active = true AND m.onboarding_status = 'active'
		`, clientID).Scan(&resolvedMSP); err != nil || resolvedMSP != mspID {
			return AuthorizationScope{}, fmt.Errorf("client is inactive or structurally unrelated to the selected MSP")
		}
		scope.Type, scope.ID, scope.ClientID = ScopeClient, clientID, clientID
		return scope, nil
	}

	var resolvedClient string
	if err := tx.QueryRowContext(ctx, `
		SELECT c.msp_id::text, s.client_id::text
		FROM sites s
		JOIN client_organizations c ON c.id = s.client_id
		JOIN msp_tenants m ON m.id = c.msp_id
		WHERE s.id = $1 AND s.is_active = true AND c.is_active = true
		  AND m.is_active = true AND m.onboarding_status = 'active'
	`, siteID).Scan(&resolvedMSP, &resolvedClient); err != nil || resolvedMSP != mspID || resolvedClient != clientID {
		return AuthorizationScope{}, fmt.Errorf("site is inactive or structurally unrelated to the selected hierarchy")
	}
	scope.Type, scope.ID, scope.ClientID, scope.SiteID = ScopeSite, siteID, clientID, siteID
	return scope, nil
}

func membershipAppliesToSelectedScope(sourceType ScopeType, sourceID, role string, selected AuthorizationScope) bool {
	switch sourceType {
	case ScopePlatform:
		return sourceID == postgres.SingletonPlatformID && platformRoleFlowsTo(role, selected.Type)
	case ScopeMSP:
		return sourceID == selected.MSPID && mspRoleFlowsTo(role, selected.Type)
	case ScopeClient:
		return sourceID == selected.ClientID && clientRoleFlowsTo(role, selected.Type)
	case ScopeSite:
		return sourceID == selected.SiteID && selected.Type == ScopeSite
	default:
		return false
	}
}

// Downward inheritance is an explicit policy table. A role never flows upward
// or sideways, and callers only reach this point after the child hierarchy was
// resolved from active database rows.
func platformRoleFlowsTo(role string, selected ScopeType) bool {
	if selected == ScopePlatform {
		return true
	}
	switch role {
	case "platform_owner", "platform_admin", "platform_support", "platform_billing", "platform_security_auditor", "platform_viewer":
		return selected == ScopeMSP || selected == ScopeClient || selected == ScopeSite
	default:
		return false
	}
}

func mspRoleFlowsTo(role string, selected ScopeType) bool {
	if selected == ScopeMSP {
		return true
	}
	switch role {
	case "msp_owner", "msp_admin", "msp_technician", "msp_viewer":
		return selected == ScopeClient || selected == ScopeSite
	default:
		return false
	}
}

func clientRoleFlowsTo(role string, selected ScopeType) bool {
	if selected == ScopeClient {
		return true
	}
	return selected == ScopeSite && (role == "client_admin" || role == "client_viewer")
}

func validRoleForScope(scope ScopeType, role string) bool {
	switch scope {
	case ScopePlatform:
		switch role {
		case "platform_owner", "platform_admin", "platform_support", "platform_billing", "platform_security_auditor", "platform_viewer":
			return true
		}
	case ScopeMSP:
		switch role {
		case "msp_owner", "msp_admin", "msp_technician", "msp_viewer":
			return true
		}
	case ScopeClient, ScopeSite:
		return role == "client_admin" || role == "client_viewer"
	}
	return false
}
