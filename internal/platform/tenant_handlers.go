package platform

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type createMSPRequest struct {
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Plan       string `json:"plan,omitempty"`
	OwnerEmail string `json:"owner_email"`
}

// --- Platform MSP Management ---

func (s *APIServer) handleListMSPS(w http.ResponseWriter, r *http.Request) {
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT m.id, m.name, m.slug, m.plan, m.is_active, m.onboarding_status, m.created_at,
		       (SELECT COUNT(*) FROM client_organizations co WHERE co.msp_id = m.id) as client_count,
		       (SELECT COUNT(*) FROM devices d JOIN client_organizations co ON d.client_id = co.id WHERE co.msp_id = m.id) as device_count,
		       COALESCE((
		           SELECT invitation.delivery_status FROM account_invitations invitation
		           WHERE invitation.msp_id = m.id
		           ORDER BY invitation.created_at DESC LIMIT 1
		       ), '')
		FROM msp_tenants m ORDER BY m.created_at DESC
	`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = rows.Close() }()

	var msps []map[string]interface{}
	for rows.Next() {
		var id, name, slug, plan, onboardingStatus, invitationDeliveryStatus string
		var isActive bool
		var createdAt time.Time
		var clientCount, deviceCount int
		if err := rows.Scan(&id, &name, &slug, &plan, &isActive, &onboardingStatus, &createdAt, &clientCount, &deviceCount, &invitationDeliveryStatus); err != nil {
			continue
		}
		msps = append(msps, map[string]interface{}{
			"id": id, "name": name, "slug": slug, "plan": plan,
			"is_active": isActive, "created_at": createdAt.UTC().Format(time.RFC3339),
			"onboarding_status": onboardingStatus, "owner_invitation_delivery_status": invitationDeliveryStatus,
			"client_count": clientCount, "device_count": deviceCount,
		})
	}
	if msps == nil {
		msps = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"msps": msps})
}

func (s *APIServer) handleCreateMSP(w http.ResponseWriter, r *http.Request) {
	if !authorizeTopLevelPlatformRequest(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, invitationBodyMax)
	var req createMSPRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "owner invitation service unavailable"})
		return
	}
	actorID, _ := r.Context().Value(ctxKeyUserID).(string)
	service := newOwnerInvitationService(s.db.DB(), s.accountMailer, s.publicURL)
	created, err := service.createPendingMSP(r.Context(), createPendingMSPInput{
		Name: req.Name, Slug: req.Slug, Plan: req.Plan, OwnerEmail: req.OwnerEmail, ActorID: actorID,
	})
	if err != nil {
		writeOwnerInvitationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"id": created.MSPID, "status": "pending_owner", "delivery_status": created.DeliveryStatus,
	})
}

func (s *APIServer) handleResendOwnerInvitation(w http.ResponseWriter, r *http.Request) {
	if !authorizeTopLevelPlatformRequest(w, r) {
		return
	}
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "owner invitation service unavailable"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, invitationBodyMax)
	if r.ContentLength != 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body is not allowed"})
		return
	}
	actorID, _ := r.Context().Value(ctxKeyUserID).(string)
	service := newOwnerInvitationService(s.db.DB(), s.accountMailer, s.publicURL)
	rotated, err := service.resend(r.Context(), r.PathValue("mspID"), actorID)
	if err != nil {
		writeOwnerInvitationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "invitation_rotated", "delivery_status": rotated.DeliveryStatus,
	})
}

func (s *APIServer) handleGetMSP(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	var id, name, slug, plan, onboardingStatus, invitationDeliveryStatus string
	var isActive bool
	var createdAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT m.id, m.name, m.slug, m.plan, m.is_active, m.onboarding_status, m.created_at,
		       COALESCE((
		           SELECT invitation.delivery_status FROM account_invitations invitation
		           WHERE invitation.msp_id = m.id
		           ORDER BY invitation.created_at DESC LIMIT 1
		       ), '')
		FROM msp_tenants m WHERE m.id = $1
	`, mspID).Scan(&id, &name, &slug, &plan, &isActive, &onboardingStatus, &createdAt, &invitationDeliveryStatus)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "msp not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": id, "name": name, "slug": slug, "plan": plan,
		"is_active": isActive, "onboarding_status": onboardingStatus,
		"owner_invitation_delivery_status": invitationDeliveryStatus,
		"created_at":                       createdAt.UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleSuspendMSP(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	_, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE msp_tenants SET is_active = false WHERE id = $1`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.auditControlPlane(r, mspID, "msp.suspended", "msp", mspID, nil); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

func (s *APIServer) handleActivateMSP(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE msp_tenants SET is_active = true
		WHERE id = $1 AND onboarding_status = 'active'
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pending owner activation cannot be bypassed"})
		return
	}
	if err := s.auditControlPlane(r, mspID, "msp.activated", "msp", mspID, nil); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
}

// --- Client Management ---

func (s *APIServer) handleListClients(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT co.id, co.name, co.slug, co.is_active, co.created_at,
		       (SELECT COUNT(*) FROM sites s WHERE s.client_id = co.id) as site_count,
		       (SELECT COUNT(*) FROM devices d WHERE d.client_id = co.id) as device_count
		FROM client_organizations co WHERE co.msp_id = $1 ORDER BY co.name ASC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = rows.Close() }()

	var clients []map[string]interface{}
	for rows.Next() {
		var id, name, slug string
		var isActive bool
		var createdAt time.Time
		var siteCount, deviceCount int
		if err := rows.Scan(&id, &name, &slug, &isActive, &createdAt, &siteCount, &deviceCount); err != nil {
			continue
		}
		clients = append(clients, map[string]interface{}{
			"id": id, "name": name, "slug": slug, "is_active": isActive,
			"created_at": createdAt.UTC().Format(time.RFC3339),
			"site_count": siteCount, "device_count": deviceCount,
		})
	}
	if clients == nil {
		clients = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"clients": clients})
}

func (s *APIServer) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and slug required"})
		return
	}
	var existingID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT id FROM client_organizations WHERE msp_id = $1 AND slug = $2`, mspID, req.Slug).Scan(&existingID)
	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "client slug already exists in this MSP", "existing_id": existingID})
		return
	}

	id := uuid.New().String()
	_, err = s.requestDB(r).ExecContext(r.Context(), `
		WITH new_client AS (
			INSERT INTO client_organizations (id, msp_id, name, slug)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		), legacy_tenant AS (
			INSERT INTO tenants (id, name, slug, plan)
			SELECT id, $3, $2 || '-' || $4, 'managed' FROM new_client
			ON CONFLICT (id) DO NOTHING
		)
		INSERT INTO sites (id, client_id, name, slug)
		SELECT gen_random_uuid(), id, 'Default Site', 'default' FROM new_client
	`, id, mspID, req.Name, req.Slug)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.auditControlPlane(r, mspID, "client.created", "client", id,
		map[string]string{"name": req.Name, "slug": req.Slug}); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})
}

func (s *APIServer) handleGetClient(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")
	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}
	var id, mspID, name, slug string
	var isActive bool
	var createdAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, msp_id, name, slug, is_active, created_at FROM client_organizations WHERE id = $1
	`, clientID).Scan(&id, &mspID, &name, &slug, &isActive, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "client not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": id, "msp_id": mspID, "name": name, "slug": slug,
		"is_active": isActive, "created_at": createdAt.UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleArchiveClient(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")
	if _, ok := s.authorizeClientManage(w, r, clientID); !ok {
		return
	}
	_, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE client_organizations SET is_active = false WHERE id = $1`, clientID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	mspID := r.PathValue("mspID")
	if err := s.auditControlPlane(r, mspID, "client.archived", "client", clientID, nil); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// --- Site Management ---

func (s *APIServer) handleListSites(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")
	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT s.id, s.name, s.slug, s.is_active, s.created_at,
		       (SELECT COUNT(*) FROM devices d WHERE d.site_id = s.id) as device_count
		FROM sites s WHERE s.client_id = $1 ORDER BY s.name ASC
	`, clientID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = rows.Close() }()

	var sites []map[string]interface{}
	for rows.Next() {
		var id, name, slug string
		var isActive bool
		var createdAt time.Time
		var deviceCount int
		if err := rows.Scan(&id, &name, &slug, &isActive, &createdAt, &deviceCount); err != nil {
			continue
		}
		sites = append(sites, map[string]interface{}{
			"id": id, "name": name, "slug": slug, "is_active": isActive,
			"created_at":   createdAt.UTC().Format(time.RFC3339),
			"device_count": deviceCount,
		})
	}
	if sites == nil {
		sites = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sites": sites})
}

func (s *APIServer) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")
	if _, ok := s.authorizeClientManage(w, r, clientID); !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and slug required"})
		return
	}
	var existingID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT id FROM sites WHERE client_id = $1 AND slug = $2`, clientID, req.Slug).Scan(&existingID)
	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "site slug already exists in this client", "existing_id": existingID})
		return
	}

	id := uuid.New().String()
	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO sites (id, client_id, name, slug) VALUES ($1, $2, $3, $4)
	`, id, clientID, req.Name, req.Slug)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var mspID string
	_ = s.requestDB(r).QueryRowContext(r.Context(),
		`SELECT msp_id FROM client_organizations WHERE id = $1`, clientID).Scan(&mspID)
	if err := s.auditControlPlane(r, mspID, "site.created", "site", id,
		map[string]string{"client_id": clientID, "name": req.Name, "slug": req.Slug}); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})
}

func (s *APIServer) handleGetSite(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("siteID")
	if !s.AuthorizeSiteAccess(w, r, siteID) {
		return
	}
	var id, clientID, name, slug string
	var isActive bool
	var createdAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, client_id, name, slug, is_active, created_at FROM sites WHERE id = $1
	`, siteID).Scan(&id, &clientID, &name, &slug, &isActive, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "site not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": id, "client_id": clientID, "name": name, "slug": slug,
		"is_active": isActive, "created_at": createdAt.UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleArchiveSite(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("siteID")
	if _, ok := s.authorizeSiteManage(w, r, siteID); !ok {
		return
	}
	_, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE sites SET is_active = false WHERE id = $1`, siteID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var mspID string
	_ = s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT c.msp_id FROM sites s JOIN client_organizations c ON c.id = s.client_id WHERE s.id = $1
	`, siteID).Scan(&mspID)
	if err := s.auditControlPlane(r, mspID, "site.archived", "site", siteID, nil); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// --- Membership Management ---

func (s *APIServer) handleListMemberships(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, user_id, role, scope_type, scope_id, created_at
		FROM memberships WHERE scope_type = 'msp' AND scope_id = $1
		ORDER BY created_at DESC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = rows.Close() }()

	var memberships []map[string]interface{}
	for rows.Next() {
		var id, userID, role, scopeType, scopeID string
		var createdAt time.Time
		if err := rows.Scan(&id, &userID, &role, &scopeType, &scopeID, &createdAt); err != nil {
			continue
		}
		memberships = append(memberships, map[string]interface{}{
			"id": id, "user_id": userID, "role": role,
			"scope_type": scopeType, "scope_id": scopeID,
			"created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}
	if memberships == nil {
		memberships = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"memberships": memberships})
}

func (s *APIServer) handleCreateMembership(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}
	var req struct {
		UserID    string `json:"user_id"`
		Role      string `json:"role"`
		ScopeType string `json:"scope_type"`
		ScopeID   string `json:"scope_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" || req.Role == "" || req.ScopeType == "" || req.ScopeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, role, scope_type, scope_id required"})
		return
	}
	if req.ScopeType != "msp" || req.ScopeID != mspID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "membership scope must match route MSP"})
		return
	}
	allowedRoles := map[string]bool{
		"msp_owner": true, "msp_admin": true, "msp_technician": true, "msp_viewer": true,
	}
	if !allowedRoles[req.Role] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid MSP role"})
		return
	}
	var userActive, quotaAllowed bool
	if err := s.requestDB(r).QueryRowContext(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND is_active = true)`,
		req.UserID,
	).Scan(&userActive); err != nil || !userActive {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "active user not found"})
		return
	}
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT pe.status = 'active' AND (
			p.max_users = 0 OR (
				SELECT COUNT(DISTINCT user_id) FROM memberships
				WHERE scope_type = 'msp' AND scope_id = $1 AND status = 'active'
			) < p.max_users
		)
		FROM plan_entitlements pe JOIN plans p ON p.id = pe.plan_id
		WHERE pe.msp_id::text = $1
	`, mspID).Scan(&quotaAllowed); err != nil || !quotaAllowed {
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "subscription inactive or user quota reached"})
		return
	}
	id := uuid.New().String()
	_, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO memberships (id, user_id, role, scope_type, scope_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, req.UserID, req.Role, req.ScopeType, req.ScopeID, "api")
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "active membership already exists"})
		return
	}
	if err := s.auditControlPlane(r, mspID, "membership.created", "membership", id,
		map[string]string{"user_id": req.UserID, "role": req.Role}); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})
}

func (s *APIServer) handleRevokeMembership(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}
	membershipID := r.PathValue("membershipID")
	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE memberships SET status = 'revoked'
		WHERE id = $1 AND scope_type = 'msp' AND scope_id = $2 AND status = 'active'
	`, membershipID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership revocation failed"})
		return
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active membership not found"})
		return
	}
	if err := s.auditControlPlane(r, mspID, "membership.revoked", "membership", membershipID, nil); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
