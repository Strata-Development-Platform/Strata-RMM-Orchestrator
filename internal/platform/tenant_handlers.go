package platform

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type createMSPRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Plan string `json:"plan,omitempty"`
}

// --- Platform MSP Management ---

func (s *APIServer) handleListMSPS(w http.ResponseWriter, r *http.Request) {
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT m.id, m.name, m.slug, m.plan, m.is_active, m.created_at,
		       (SELECT COUNT(*) FROM client_organizations co WHERE co.msp_id = m.id) as client_count,
		       (SELECT COUNT(*) FROM devices d JOIN client_organizations co ON d.client_id = co.id WHERE co.msp_id = m.id) as device_count
		FROM msp_tenants m ORDER BY m.created_at DESC
	`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var msps []map[string]interface{}
	for rows.Next() {
		var id, name, slug, plan string
		var isActive bool
		var createdAt time.Time
		var clientCount, deviceCount int
		if err := rows.Scan(&id, &name, &slug, &plan, &isActive, &createdAt, &clientCount, &deviceCount); err != nil {
			continue
		}
		msps = append(msps, map[string]interface{}{
			"id": id, "name": name, "slug": slug, "plan": plan,
			"is_active": isActive, "created_at": createdAt.UTC().Format(time.RFC3339),
			"client_count": clientCount, "device_count": deviceCount,
		})
	}
	if msps == nil {
		msps = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"msps": msps})
}

func (s *APIServer) handleCreateMSP(w http.ResponseWriter, r *http.Request) {
	var req createMSPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and slug required"})
		return
	}
	if req.Plan == "" {
		req.Plan = "free"
	}

	var existingID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT id FROM msp_tenants WHERE slug = $1`, req.Slug).Scan(&existingID)
	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "msp slug already exists", "existing_id": existingID})
		return
	}

	id := uuid.New().String()
	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO msp_tenants (id, name, slug, plan) VALUES ($1, $2, $3, $4)
	`, id, req.Name, req.Slug, req.Plan)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})
}

func (s *APIServer) handleGetMSP(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	var id, name, slug, plan string
	var isActive bool
	var createdAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, name, slug, plan, is_active, created_at FROM msp_tenants WHERE id = $1
	`, mspID).Scan(&id, &name, &slug, &plan, &isActive, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "msp not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": id, "name": name, "slug": slug, "plan": plan,
		"is_active": isActive, "created_at": createdAt.UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleSuspendMSP(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	_, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE msp_tenants SET is_active = false WHERE id = $1`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

func (s *APIServer) handleActivateMSP(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	_, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE msp_tenants SET is_active = true WHERE id = $1`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
	defer rows.Close()

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
	if !s.AuthorizeMSPAccess(w, r, mspID) {
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
	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}
	_, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE client_organizations SET is_active = false WHERE id = $1`, clientID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
	defer rows.Close()

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
	if !s.AuthorizeClientAccess(w, r, clientID) {
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
	if !s.AuthorizeSiteAccess(w, r, siteID) {
		return
	}
	_, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE sites SET is_active = false WHERE id = $1`, siteID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
	defer rows.Close()

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
	if !s.AuthorizeMSPAccess(w, r, mspID) {
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
	id := uuid.New().String()
	_, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO memberships (id, user_id, role, scope_type, scope_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, req.UserID, req.Role, req.ScopeType, req.ScopeID, "api")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})
}
