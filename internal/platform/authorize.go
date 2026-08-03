package platform

import "net/http"

func canManageMSPAtSelectedScope(authorization AuthorizationResult, mspID string) bool {
	switch authorization.Selected.Type {
	case ScopePlatform:
		return authorization.IsPlatformGlobal()
	case ScopeMSP:
		return authorization.Selected.MSPID == mspID &&
			authorization.HasRole("platform_owner", "platform_admin", "msp_owner", "msp_admin")
	default:
		// A child selection never grants access back to its parent MSP.
		return false
	}
}

func canReadMSPAtSelectedScope(authorization AuthorizationResult, mspID string) bool {
	switch authorization.Selected.Type {
	case ScopePlatform:
		return authorization.IsPlatformGlobal()
	case ScopeMSP:
		return authorization.Selected.MSPID == mspID &&
			(authorization.HasRole("platform_owner", "platform_admin") || hasMSPRole(authorization.Roles))
	default:
		return false
	}
}

// AuthorizeMSPAccess authorizes an MSP-level resource only from platform or the
// exact selected MSP. Client/site selections cannot be used to climb upward.
func (s *APIServer) AuthorizeMSPAccess(w http.ResponseWriter, r *http.Request, mspID string) bool {
	if mspID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return false
	}
	authorization := authorizationFromRequest(r)
	var active bool
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM msp_tenants
			WHERE id = $1 AND is_active = true AND onboarding_status = 'active'
		)
	`, mspID).Scan(&active); err != nil || !active {
		writeAuthorizationDenied(w)
		return false
	}
	if canReadMSPAtSelectedScope(authorization, mspID) {
		return true
	}
	if authorization.Selected.Type == ScopeMSP &&
		authorization.Selected.MSPID == mspID &&
		authorization.HasRole("platform_support") && s.supportGrantAllows(r, mspID) {
		return true
	}
	writeAuthorizationDenied(w)
	return false
}

// AuthorizeMSPManage requires a managing role effective in the exact selected
// MSP, or a top-level platform administrator. A child selection never manages
// its parent MSP.
func (s *APIServer) AuthorizeMSPManage(w http.ResponseWriter, r *http.Request, mspID string) bool {
	if mspID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return false
	}
	authorization := authorizationFromRequest(r)
	if !canManageMSPAtSelectedScope(authorization, mspID) {
		writeAuthorizationDenied(w)
		return false
	}
	var active bool
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM msp_tenants
			WHERE id = $1 AND is_active = true AND onboarding_status = 'active'
		)
	`, mspID).Scan(&active); err != nil || !active {
		writeAuthorizationDenied(w)
		return false
	}
	return true
}

func (s *APIServer) authorizeClientManage(w http.ResponseWriter, r *http.Request, clientID string) (string, bool) {
	if clientID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return "", false
	}
	var mspID string
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT c.msp_id::text
		FROM client_organizations c
		JOIN msp_tenants m ON m.id = c.msp_id
		WHERE c.id = $1 AND c.is_active = true
		  AND m.is_active = true AND m.onboarding_status = 'active'
	`, clientID).Scan(&mspID); err != nil {
		writeAuthorizationDenied(w)
		return "", false
	}
	authorization := authorizationFromRequest(r)
	allowed := false
	switch authorization.Selected.Type {
	case ScopePlatform:
		allowed = authorization.IsPlatformGlobal()
	case ScopeMSP:
		allowed = authorization.Selected.MSPID == mspID &&
			authorization.HasRole("platform_owner", "platform_admin", "msp_owner", "msp_admin")
	case ScopeClient:
		allowed = authorization.Selected.ClientID == clientID && authorization.HasRole("client_admin")
	}
	if !allowed {
		writeAuthorizationDenied(w)
		return "", false
	}
	return mspID, true
}

func (s *APIServer) authorizeSiteManage(w http.ResponseWriter, r *http.Request, siteID string) (string, bool) {
	if siteID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return "", false
	}
	var mspID, clientID string
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT c.msp_id::text, s.client_id::text
		FROM sites s
		JOIN client_organizations c ON c.id = s.client_id
		JOIN msp_tenants m ON m.id = c.msp_id
		WHERE s.id = $1 AND s.is_active = true AND c.is_active = true
		  AND m.is_active = true AND m.onboarding_status = 'active'
	`, siteID).Scan(&mspID, &clientID); err != nil {
		writeAuthorizationDenied(w)
		return "", false
	}
	authorization := authorizationFromRequest(r)
	allowed := false
	switch authorization.Selected.Type {
	case ScopePlatform:
		allowed = authorization.IsPlatformGlobal()
	case ScopeMSP:
		allowed = authorization.Selected.MSPID == mspID &&
			authorization.HasRole("platform_owner", "platform_admin", "msp_owner", "msp_admin")
	case ScopeClient:
		allowed = authorization.Selected.ClientID == clientID && authorization.HasRole("client_admin")
	case ScopeSite:
		allowed = authorization.Selected.SiteID == siteID && authorization.HasRole("client_admin")
	}
	if !allowed {
		writeAuthorizationDenied(w)
		return "", false
	}
	return mspID, true
}

// AuthorizeClientManage requires a managing role effective in the exact selected
// client, MSP, or a top-level platform administrator. A child selection never
// manages its parent.
func (s *APIServer) AuthorizeClientManage(w http.ResponseWriter, r *http.Request, clientID string) bool {
	if clientID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return false
	}
	_, ok := s.authorizeClientManage(w, r, clientID)
	return ok
}

// AuthorizeClientAccess authorizes only a client selected directly, or a parent
// platform/MSP scope whose active hierarchy contains that client.
func (s *APIServer) AuthorizeClientAccess(w http.ResponseWriter, r *http.Request, clientID string) bool {
	if clientID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return false
	}
	var mspID string
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT c.msp_id::text
		FROM client_organizations c
		JOIN msp_tenants m ON m.id = c.msp_id
		WHERE c.id = $1 AND c.is_active = true
		  AND m.is_active = true AND m.onboarding_status = 'active'
	`, clientID).Scan(&mspID); err != nil {
		writeAuthorizationDenied(w)
		return false
	}
	authorization := authorizationFromRequest(r)
	allowed := false
	switch authorization.Selected.Type {
	case ScopePlatform:
		allowed = authorization.IsPlatformGlobal()
	case ScopeMSP:
		allowed = authorization.Selected.MSPID == mspID &&
			(authorization.HasRole("platform_owner", "platform_admin") || hasMSPRole(authorization.Roles))
	case ScopeClient:
		allowed = authorization.Selected.ClientID == clientID
	}
	if allowed {
		return true
	}
	if authorization.Selected.Type == ScopeMSP && authorization.Selected.MSPID == mspID &&
		authorization.HasRole("platform_support") && s.supportGrantAllows(r, mspID) {
		return true
	}
	writeAuthorizationDenied(w)
	return false
}

// AuthorizeSiteAccess permits downward access only when the database-proven
// site ancestry is inside the selected platform/MSP/client scope, or the exact
// selected site matches. It never permits a sibling.
func (s *APIServer) AuthorizeSiteAccess(w http.ResponseWriter, r *http.Request, siteID string) bool {
	if siteID == "" || s.db == nil {
		writeAuthorizationDenied(w)
		return false
	}
	var mspID, clientID string
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT c.msp_id::text, s.client_id::text
		FROM sites s
		JOIN client_organizations c ON c.id = s.client_id
		JOIN msp_tenants m ON m.id = c.msp_id
		WHERE s.id = $1 AND s.is_active = true AND c.is_active = true
		  AND m.is_active = true AND m.onboarding_status = 'active'
	`, siteID).Scan(&mspID, &clientID); err != nil {
		writeAuthorizationDenied(w)
		return false
	}
	authorization := authorizationFromRequest(r)
	allowed := false
	switch authorization.Selected.Type {
	case ScopePlatform:
		allowed = authorization.IsPlatformGlobal()
	case ScopeMSP:
		allowed = authorization.Selected.MSPID == mspID &&
			(authorization.HasRole("platform_owner", "platform_admin") || hasMSPRole(authorization.Roles))
	case ScopeClient:
		allowed = authorization.Selected.ClientID == clientID
	case ScopeSite:
		allowed = authorization.Selected.SiteID == siteID
	}
	if allowed {
		return true
	}
	if authorization.Selected.Type == ScopeMSP && authorization.Selected.MSPID == mspID &&
		authorization.HasRole("platform_support") && s.supportGrantAllows(r, mspID) {
		return true
	}
	writeAuthorizationDenied(w)
	return false
}

func writeAuthorizationDenied(w http.ResponseWriter) {
	http.Error(w, `{"error":"resource not found or access denied"}`, http.StatusNotFound)
}
