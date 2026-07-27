package platform

import (
	"net/http"
	"strings"
)

func getRoles(r *http.Request) []string {
	rolesStr, _ := r.Context().Value(ctxKeyRole).(string)
	if rolesStr == "" {
		return nil
	}
	return strings.Split(rolesStr, ",")
}

func hasAdminRole(roles []string) bool {
	for _, r := range roles {
		switch r {
		case "platform_owner", "platform_admin":
			return true
		}
	}
	return false
}

// AuthorizeMSPAccess checks whether the authenticated principal can access the given MSP.
func (s *APIServer) AuthorizeMSPAccess(w http.ResponseWriter, r *http.Request, mspID string) bool {
	roles := getRoles(r)

	if hasAdminRole(roles) {
		return true
	}
	if mspID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return false
	}
	if hasSupportRole(roles) && s.supportGrantAllows(r, mspID) {
		return true
	}

	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	var allowed bool
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM msp_tenants m
			JOIN memberships mb
			  ON mb.user_id = $1
			 AND mb.scope_type = 'msp'
			 AND mb.scope_id = m.id::text
			 AND mb.status = 'active'
			 AND (mb.expires_at IS NULL OR mb.expires_at > NOW())
			WHERE m.id = $2
			  AND m.is_active = true
		)
	`, userID, mspID).Scan(&allowed)
	if err != nil || !allowed {
		writeAuthorizationDenied(w)
		return false
	}
	return true
}

// AuthorizeClientAccess checks whether the authenticated principal can access the given client.
func (s *APIServer) AuthorizeClientAccess(w http.ResponseWriter, r *http.Request, clientID string) bool {
	roles := getRoles(r)

	if hasAdminRole(roles) {
		return true
	}
	if clientID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return false
	}

	var clientMSPID string
	if err := s.requestDB(r).QueryRowContext(
		r.Context(),
		`SELECT msp_id FROM client_organizations WHERE id = $1`,
		clientID,
	).Scan(&clientMSPID); err != nil {
		writeAuthorizationDenied(w)
		return false
	}
	if hasSupportRole(roles) && s.supportGrantAllows(r, clientMSPID) {
		return true
	}

	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	var allowed bool
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM client_organizations c
			JOIN msp_tenants m ON m.id = c.msp_id AND m.is_active = true
			WHERE c.id = $2
			  AND c.is_active = true
			  AND EXISTS (
				SELECT 1
				FROM memberships mb
				WHERE mb.user_id = $1
				  AND mb.status = 'active'
				  AND (mb.expires_at IS NULL OR mb.expires_at > NOW())
				  AND (
					(mb.scope_type = 'msp' AND mb.scope_id = c.msp_id::text)
					OR (mb.scope_type = 'client' AND mb.scope_id = c.id::text)
				  )
			  )
		)
	`, userID, clientID).Scan(&allowed)
	if err != nil || !allowed {
		writeAuthorizationDenied(w)
		return false
	}
	return true
}

// AuthorizeSiteAccess checks whether the authenticated principal can access the given site.
func (s *APIServer) AuthorizeSiteAccess(w http.ResponseWriter, r *http.Request, siteID string) bool {
	roles := getRoles(r)

	if hasAdminRole(roles) {
		return true
	}
	if siteID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return false
	}

	var siteMSPID string
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT c.msp_id
		FROM sites s
		JOIN client_organizations c ON c.id = s.client_id
		WHERE s.id = $1
	`, siteID).Scan(&siteMSPID); err != nil {
		writeAuthorizationDenied(w)
		return false
	}
	if hasSupportRole(roles) && s.supportGrantAllows(r, siteMSPID) {
		return true
	}

	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	var allowed bool
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM sites s
			JOIN client_organizations c ON c.id = s.client_id AND c.is_active = true
			JOIN msp_tenants m ON m.id = c.msp_id AND m.is_active = true
			WHERE s.id = $2
			  AND s.is_active = true
			  AND EXISTS (
				SELECT 1
				FROM memberships mb
				WHERE mb.user_id = $1
				  AND mb.status = 'active'
				  AND (mb.expires_at IS NULL OR mb.expires_at > NOW())
				  AND (
					(mb.scope_type = 'msp' AND mb.scope_id = c.msp_id::text)
					OR (mb.scope_type = 'client' AND mb.scope_id = c.id::text)
					OR (mb.scope_type = 'site' AND mb.scope_id = s.id::text)
				  )
			  )
		)
	`, userID, siteID).Scan(&allowed)
	if err != nil || !allowed {
		writeAuthorizationDenied(w)
		return false
	}
	return true
}

func writeAuthorizationDenied(w http.ResponseWriter) {
	http.Error(w, `{"error":"resource not found or access denied"}`, http.StatusNotFound)
}
