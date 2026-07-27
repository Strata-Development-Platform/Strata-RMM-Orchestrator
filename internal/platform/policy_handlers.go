package platform

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (s *APIServer) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}

	var req struct {
		Name        string                 `json:"name"`
		Category    string                 `json:"category"`
		Description string                 `json:"description"`
		Config      map[string]interface{} `json:"config"`
		ScopeLevel  string                 `json:"scope_level"`
		ClientID    string                 `json:"client_id,omitempty"`
		SiteID      string                 `json:"site_id,omitempty"`
		ParentID    string                 `json:"parent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	if req.ScopeLevel == "" {
		req.ScopeLevel = "msp"
	}

	configJSON, _ := json.Marshal(req.Config)
	id := uuid.New().String()

	var parentID interface{}
	if req.ParentID != "" {
		parentID = req.ParentID
	}
	var clientID interface{}
	if req.ClientID != "" {
		clientID = req.ClientID
	}
	var siteID interface{}
	if req.SiteID != "" {
		siteID = req.SiteID
	}

	_, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO policies (id, msp_id, client_id, site_id, name, category, description, config, scope_level, parent_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, mspID, clientID, siteID, req.Name, req.Category, req.Description, configJSON, req.ScopeLevel, parentID, "api")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "created"})
}

func (s *APIServer) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, name, category, description, scope_level, status, version, created_at
		FROM policies WHERE msp_id = $1
		ORDER BY category, name ASC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var policies []map[string]interface{}
	for rows.Next() {
		var id, name, category, desc, scopeLevel, status string
		var version int
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &category, &desc, &scopeLevel, &status, &version, &createdAt); err != nil {
			continue
		}
		policies = append(policies, map[string]interface{}{
			"id": id, "name": name, "category": category, "description": desc,
			"scope_level": scopeLevel, "status": status, "version": version,
			"created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}
	if policies == nil {
		policies = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"policies": policies})
}

func (s *APIServer) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	policyID := r.PathValue("policyID")

	var id, mspID, name, category, desc, scopeLevel, status, configStr string
	var version int
	var createdAt time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, msp_id, name, category, description, config::text, scope_level, status, version, created_at
		FROM policies WHERE id = $1
	`, policyID).Scan(&id, &mspID, &name, &category, &desc, &configStr, &scopeLevel, &status, &version, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id": id, "msp_id": mspID, "name": name, "category": category,
		"description": desc, "scope_level": scopeLevel, "status": status,
		"version": version, "created_at": createdAt.UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handlePublishPolicy(w http.ResponseWriter, r *http.Request) {
	policyID := r.PathValue("policyID")
	_, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE policies SET status = 'active', version = version + 1, updated_at = NOW()
		WHERE id = $1 AND status = 'draft'
	`, policyID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "published"})
}

func (s *APIServer) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	policyID := r.PathValue("policyID")
	_, err := s.requestDB(r).ExecContext(r.Context(), `DELETE FROM policies WHERE id = $1`, policyID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
