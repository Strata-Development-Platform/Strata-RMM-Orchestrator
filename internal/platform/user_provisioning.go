package platform

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

type scopedMembershipAssignment struct {
	ScopeType ScopeType `json:"scope_type"`
	ScopeID   string    `json:"scope_id"`
	Role      string    `json:"role"`

	resolved AuthorizationScope
}

type createScopedUserRequest struct {
	Email       string                       `json:"email"`
	Password    string                       `json:"password"`
	ScopeType   ScopeType                    `json:"scope_type"`
	ScopeID     string                       `json:"scope_id"`
	Role        string                       `json:"role"`
	Memberships []scopedMembershipAssignment `json:"memberships"`
}

type updateScopedUserRequest struct {
	Memberships []scopedMembershipAssignment `json:"memberships"`
}

func (request createScopedUserRequest) assignments() ([]scopedMembershipAssignment, error) {
	hasSingle := request.ScopeType != "" || request.ScopeID != "" || request.Role != ""
	if hasSingle && len(request.Memberships) > 0 {
		return nil, fmt.Errorf("use either scope_type/scope_id/role or memberships, not both")
	}
	if hasSingle {
		if request.ScopeType == "" || request.ScopeID == "" || request.Role == "" {
			return nil, fmt.Errorf("scope_type, scope_id, and role are required together")
		}
		return []scopedMembershipAssignment{{
			ScopeType: request.ScopeType, ScopeID: request.ScopeID, Role: request.Role,
		}}, nil
	}
	if len(request.Memberships) == 0 {
		return nil, fmt.Errorf("at least one explicit membership is required")
	}
	return request.Memberships, nil
}

func (s *APIServer) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var request createScopedUserRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	normalizedEmail, err := normalizeEmail(request.Email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a valid email is required"})
		return
	}
	if len(request.Password) < 12 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 12 characters"})
		return
	}
	if len(request.Password) > 72 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must not exceed 72 bytes"})
		return
	}
	assignments, err := request.assignments()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	assignments, err = s.validateMembershipAssignments(r, assignments)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	tx, ok := requestTransaction(r)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transactional user provisioning unavailable"})
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password hashing failed"})
		return
	}
	legacyRole, legacyTenantID := legacyMirrorsForAssignments(assignments)
	var userID string
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO users (tenant_id, email, password_hash, role, email_verified_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (normalized_email) DO NOTHING
		RETURNING id::text
	`, nullIfEmpty(legacyTenantID), normalizedEmail, string(passwordHash), legacyRole).Scan(&userID)
	if err == sql.ErrNoRows {
		var active bool
		if err := tx.QueryRowContext(r.Context(), `
			SELECT id::text, is_active AND email_verified_at IS NOT NULL
			FROM users WHERE normalized_email = $1
		`, normalizedEmail).Scan(&userID, &active); err != nil || !active {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already exists", "code": "email_conflict"})
			return
		}
		exact, err := exactActiveAssignments(r, tx, userID, assignments)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "duplicate verification failed"})
			return
		}
		if !exact {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already exists with different memberships", "code": "email_conflict"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id": userID, "status": "exists", "memberships": publicAssignments(assignments),
		})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user creation failed"})
		return
	}
	if err := insertMembershipAssignments(r, tx, userID, assignments); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership creation failed"})
		return
	}
	if err := rebuildLegacyTenantMirror(r, tx, userID, assignments); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compatibility mirror update failed"})
		return
	}
	if err := auditUserProvisioning(r, tx, userID, "user.provisioned", assignments); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user provisioning audit failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id": userID, "status": "created", "memberships": publicAssignments(assignments),
	})
}

func (s *APIServer) handleAdminUpdateUserTenants(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("userID"))
	if _, err := uuid.Parse(userID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid user ID required"})
		return
	}
	var request updateScopedUserRequest
	if err := decodeStrictJSON(r, &request); err != nil || len(request.Memberships) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "memberships with explicit scope_type, scope_id, and role are required",
		})
		return
	}
	assignments, err := s.validateMembershipAssignments(r, request.Memberships)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	tx, ok := requestTransaction(r)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transactional membership update unavailable"})
		return
	}
	var exists bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists); err != nil || !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	exact, err := exactActiveAssignments(r, tx, userID, assignments)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership update failed"})
		return
	}
	if exact {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "unchanged", "memberships": publicAssignments(assignments),
		})
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE memberships
		SET status = 'revoked'
		WHERE user_id = $1 AND status = 'active'
	`, userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership revocation failed"})
		return
	}
	if err := insertMembershipAssignments(r, tx, userID, assignments); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership replacement failed"})
		return
	}
	legacyRole, legacyTenantID := legacyMirrorsForAssignments(assignments)
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE users SET role = $2, tenant_id = $3, updated_at = NOW() WHERE id = $1
	`, userID, legacyRole, nullIfEmpty(legacyTenantID)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "identity compatibility mirror update failed"})
		return
	}
	if err := rebuildLegacyTenantMirror(r, tx, userID, assignments); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compatibility mirror update failed"})
		return
	}
	if err := auditUserProvisioning(r, tx, userID, "user.memberships_updated", assignments); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership update audit failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "updated", "memberships": publicAssignments(assignments),
	})
}

func (s *APIServer) validateMembershipAssignments(r *http.Request, assignments []scopedMembershipAssignment) ([]scopedMembershipAssignment, error) {
	if len(assignments) == 0 || len(assignments) > 100 {
		return nil, fmt.Errorf("between 1 and 100 memberships are required")
	}
	actor := authorizationFromRequest(r)
	seen := make(map[string]struct{}, len(assignments))
	validated := make([]scopedMembershipAssignment, 0, len(assignments))
	for _, assignment := range assignments {
		assignment.ScopeID = strings.TrimSpace(assignment.ScopeID)
		assignment.Role = strings.TrimSpace(assignment.Role)
		if !validRoleForScope(assignment.ScopeType, assignment.Role) {
			return nil, fmt.Errorf("role %q is not legal for %s scope", assignment.Role, assignment.ScopeType)
		}
		if _, err := uuid.Parse(assignment.ScopeID); err != nil {
			return nil, fmt.Errorf("scope_id must be a UUID")
		}
		key := string(assignment.ScopeType) + ":" + assignment.ScopeID
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("only one role may be assigned per scope")
		}
		seen[key] = struct{}{}
		resolved, err := resolveAssignmentScope(r, s.requestDB(r), assignment.ScopeType, assignment.ScopeID)
		if err != nil {
			return nil, err
		}
		assignment.resolved = resolved
		if !actorMayAssign(actor, assignment) {
			return nil, fmt.Errorf("acting administrator may not assign %s in the requested scope", assignment.Role)
		}
		validated = append(validated, assignment)
	}
	sort.Slice(validated, func(i, j int) bool {
		if validated[i].ScopeType != validated[j].ScopeType {
			return validated[i].ScopeType < validated[j].ScopeType
		}
		return validated[i].ScopeID < validated[j].ScopeID
	})
	return validated, nil
}

func resolveAssignmentScope(r *http.Request, db dbExecutor, scopeType ScopeType, scopeID string) (AuthorizationScope, error) {
	switch scopeType {
	case ScopePlatform:
		if scopeID != postgres.SingletonPlatformID {
			return AuthorizationScope{}, fmt.Errorf("platform membership must use the singleton platform ID")
		}
		return AuthorizationScope{Type: ScopePlatform, ID: scopeID, PlatformID: scopeID}, nil
	case ScopeMSP:
		var active bool
		if err := db.QueryRowContext(r.Context(), `
			SELECT EXISTS (SELECT 1 FROM msp_tenants WHERE id = $1 AND is_active = true AND onboarding_status = 'active')
		`, scopeID).Scan(&active); err != nil || !active {
			return AuthorizationScope{}, fmt.Errorf("MSP scope is inactive or unavailable")
		}
		return AuthorizationScope{Type: ScopeMSP, ID: scopeID, PlatformID: postgres.SingletonPlatformID, MSPID: scopeID}, nil
	case ScopeClient:
		var mspID string
		if err := db.QueryRowContext(r.Context(), `
			SELECT c.msp_id::text FROM client_organizations c
			JOIN msp_tenants m ON m.id = c.msp_id
			WHERE c.id = $1 AND c.is_active = true AND m.is_active = true AND m.onboarding_status = 'active'
		`, scopeID).Scan(&mspID); err != nil {
			return AuthorizationScope{}, fmt.Errorf("client scope is inactive or unavailable")
		}
		return AuthorizationScope{Type: ScopeClient, ID: scopeID, PlatformID: postgres.SingletonPlatformID, MSPID: mspID, ClientID: scopeID}, nil
	case ScopeSite:
		var mspID, clientID string
		if err := db.QueryRowContext(r.Context(), `
			SELECT c.msp_id::text, s.client_id::text FROM sites s
			JOIN client_organizations c ON c.id = s.client_id
			JOIN msp_tenants m ON m.id = c.msp_id
			WHERE s.id = $1 AND s.is_active = true AND c.is_active = true
			  AND m.is_active = true AND m.onboarding_status = 'active'
		`, scopeID).Scan(&mspID, &clientID); err != nil {
			return AuthorizationScope{}, fmt.Errorf("site scope is inactive or unavailable")
		}
		return AuthorizationScope{Type: ScopeSite, ID: scopeID, PlatformID: postgres.SingletonPlatformID, MSPID: mspID, ClientID: clientID, SiteID: scopeID}, nil
	default:
		return AuthorizationScope{}, fmt.Errorf("scope_type must be platform, msp, client, or site")
	}
}

func actorMayAssign(actor AuthorizationResult, assignment scopedMembershipAssignment) bool {
	selected := actor.Selected
	target := assignment.resolved
	switch selected.Type {
	case ScopePlatform:
		if !actor.IsPlatformGlobal() {
			return false
		}
		if assignment.ScopeType == ScopePlatform && assignment.Role == "platform_owner" && !actor.HasRole("platform_owner") {
			return false
		}
		return true
	case ScopeMSP:
		if target.MSPID != selected.MSPID || !actor.HasRole("platform_owner", "platform_admin", "msp_owner", "msp_admin") {
			return false
		}
		if assignment.ScopeType == ScopePlatform {
			return false
		}
		return assignment.Role != "msp_owner" || actor.HasRole("platform_owner", "platform_admin", "msp_owner")
	case ScopeClient:
		if !actor.HasRole("client_admin", "msp_owner", "msp_admin", "platform_owner", "platform_admin") {
			return false
		}
		return (assignment.ScopeType == ScopeClient && target.ClientID == selected.ClientID) ||
			(assignment.ScopeType == ScopeSite && target.ClientID == selected.ClientID)
	case ScopeSite:
		return actor.HasRole("client_admin", "msp_owner", "msp_admin", "platform_owner", "platform_admin") &&
			assignment.ScopeType == ScopeSite && target.SiteID == selected.SiteID
	default:
		return false
	}
}

func insertMembershipAssignments(r *http.Request, tx *sql.Tx, userID string, assignments []scopedMembershipAssignment) error {
	actorID, _ := r.Context().Value(ctxKeyUserID).(string)
	for _, assignment := range assignments {
		result, err := tx.ExecContext(r.Context(), `
			INSERT INTO memberships (user_id, role, scope_type, scope_id, created_by, status)
			VALUES ($1, $2, $3, $4, $5, 'active')
			ON CONFLICT (user_id, scope_type, scope_id, role) WHERE status = 'active' DO NOTHING
		`, userID, assignment.Role, assignment.ScopeType, assignment.ScopeID, actorID)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return fmt.Errorf("membership already exists or could not be created")
		}
	}
	return nil
}

func exactActiveAssignments(r *http.Request, tx *sql.Tx, userID string, assignments []scopedMembershipAssignment) (bool, error) {
	wanted := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		wanted[string(assignment.ScopeType)+":"+assignment.ScopeID+":"+assignment.Role] = struct{}{}
	}
	rows, err := tx.QueryContext(r.Context(), `
		SELECT scope_type, scope_id, role
		FROM memberships
		WHERE user_id = $1 AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > statement_timestamp())
	`, userID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	found := make(map[string]struct{})
	for rows.Next() {
		var scopeType, scopeID, role string
		if err := rows.Scan(&scopeType, &scopeID, &role); err != nil {
			return false, err
		}
		found[scopeType+":"+scopeID+":"+role] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(found) != len(wanted) {
		return false, nil
	}
	for key := range wanted {
		if _, ok := found[key]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func legacyMirrorsForAssignments(assignments []scopedMembershipAssignment) (role, tenantID string) {
	role = "viewer"
	for _, assignment := range assignments {
		switch assignment.Role {
		case "platform_owner", "platform_admin", "msp_owner", "msp_admin", "client_admin":
			role = "admin"
		case "msp_technician":
			if role != "admin" {
				role = "technician"
			}
		}
		if tenantID == "" && assignment.resolved.ClientID != "" {
			tenantID = assignment.resolved.ClientID
		}
	}
	return role, tenantID
}

func rebuildLegacyTenantMirror(r *http.Request, tx *sql.Tx, userID string, assignments []scopedMembershipAssignment) error {
	actorID, _ := r.Context().Value(ctxKeyUserID).(string)
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM user_tenant_access WHERE user_id = $1`, userID); err != nil {
		return err
	}
	seen := make(map[string]struct{})
	for _, assignment := range assignments {
		clientID := assignment.resolved.ClientID
		if clientID == "" {
			continue
		}
		if _, ok := seen[clientID]; ok {
			continue
		}
		seen[clientID] = struct{}{}
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO user_tenant_access (user_id, tenant_id, granted_by)
			VALUES ($1, $2, $3)
		`, userID, clientID, actorID); err != nil {
			return err
		}
	}
	return nil
}

func auditUserProvisioning(r *http.Request, tx *sql.Tx, userID, action string, assignments []scopedMembershipAssignment) error {
	actorID, _ := r.Context().Value(ctxKeyUserID).(string)
	public := publicAssignments(assignments)
	details, err := json.Marshal(map[string]interface{}{"memberships": public})
	if err != nil {
		return err
	}
	mspID := ""
	for _, assignment := range assignments {
		if assignment.resolved.MSPID == "" {
			continue
		}
		if mspID == "" {
			mspID = assignment.resolved.MSPID
		} else if mspID != assignment.resolved.MSPID {
			mspID = ""
			break
		}
	}
	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO control_plane_audit (msp_id, actor_user_id, action, resource_type, resource_id, details)
		VALUES ($1, $2, $3, 'user', $4, $5)
	`, nullIfEmpty(mspID), actorID, action, userID, details)
	return err
}

func publicAssignments(assignments []scopedMembershipAssignment) []map[string]string {
	result := make([]map[string]string, 0, len(assignments))
	for _, assignment := range assignments {
		result = append(result, map[string]string{
			"scope_type": string(assignment.ScopeType), "scope_id": assignment.ScopeID, "role": assignment.Role,
		})
	}
	return result
}
