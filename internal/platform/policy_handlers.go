package platform

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type policyRecord struct {
	ID, MSPID, Name, Category, Description, ScopeLevel, Status string
	ClientID, SiteID, DeviceID, ParentID                       string
	Config                                                     map[string]interface{}
	Version                                                    int
	PublishedVersion                                           *int
	ValidatedAt, PreviewedAt                                   *time.Time
	CreatedAt, UpdatedAt                                       time.Time
}

func (p policyRecord) input() policyInput {
	return policyInput{Name: p.Name, Category: p.Category, Description: p.Description, Config: p.Config,
		ScopeLevel: p.ScopeLevel, ClientID: p.ClientID, SiteID: p.SiteID, DeviceID: p.DeviceID, ParentID: p.ParentID}
}

func (s *APIServer) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}
	var req policyInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	req.normalize()
	if err := req.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.validatePolicyScope(r, mspID, req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config is not valid JSON"})
		return
	}
	id := uuid.NewString()
	actor, _ := r.Context().Value(ctxKeyUserID).(string)
	_, err = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO policies (id, msp_id, client_id, site_id, device_id, name, category, description, config, scope_level, parent_id, created_by)
		VALUES ($1, $2, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid,
		        $6, $7, $8, $9, $10, NULLIF($11, '')::uuid, $12)
	`, id, mspID, req.ClientID, req.SiteID, req.DeviceID, req.Name, req.Category,
		req.Description, configJSON, req.ScopeLevel, req.ParentID, actor)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create policy failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "draft", "version": 1})
}

func (s *APIServer) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	record, err := s.loadPolicy(r, r.PathValue("policyID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, record.MSPID) {
		return
	}
	if record.Status == "archived" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "archived policies cannot be edited"})
		return
	}
	var req policyInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	req.normalize()
	if err := req.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.validatePolicyScope(r, record.MSPID, req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	configJSON, _ := json.Marshal(req.Config)
	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE policies SET name=$1, category=$2, description=$3, config=$4, scope_level=$5,
		 client_id=NULLIF($6, '')::uuid, site_id=NULLIF($7, '')::uuid, device_id=NULLIF($8, '')::uuid,
		 parent_id=NULLIF($9, '')::uuid, status='draft', version=version+1,
		 validated_at=NULL, previewed_at=NULL, updated_at=NOW()
		WHERE id=$10 AND msp_id=$11
	`, req.Name, req.Category, req.Description, configJSON, req.ScopeLevel, req.ClientID,
		req.SiteID, req.DeviceID, req.ParentID, record.ID, record.MSPID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update policy failed"})
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": record.ID, "status": "draft", "version": record.Version + 1})
}

func (s *APIServer) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, name, category, description, scope_level, status, version,
		       COALESCE(client_id::text, ''), COALESCE(site_id::text, ''), COALESCE(device_id::text, ''),
		       published_version, validated_at, previewed_at, created_at, updated_at
		FROM policies WHERE msp_id = $1 ORDER BY category, name, version DESC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list policies failed"})
		return
	}
	defer func() { _ = rows.Close() }()
	policies := []map[string]interface{}{}
	for rows.Next() {
		var id, name, category, description, scope, status, clientID, siteID, deviceID string
		var version int
		var publishedVersion *int
		var validatedAt, previewedAt *time.Time
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &category, &description, &scope, &status, &version,
			&clientID, &siteID, &deviceID, &publishedVersion, &validatedAt, &previewedAt, &createdAt, &updatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan policy failed"})
			return
		}
		policies = append(policies, map[string]interface{}{"id": id, "name": name, "category": category,
			"description": description, "scope_level": scope, "status": status, "version": version,
			"client_id": clientID, "site_id": siteID, "device_id": deviceID,
			"published_version": publishedVersion,
			"validated":         validatedAt != nil, "previewed": previewedAt != nil,
			"created_at": createdAt.UTC().Format(time.RFC3339), "updated_at": updatedAt.UTC().Format(time.RFC3339)})
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list policies failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"policies": policies})
}

func (s *APIServer) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	record, err := s.loadPolicy(r, r.PathValue("policyID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, record.MSPID) {
		return
	}
	writeJSON(w, http.StatusOK, record.policyResponse())
}

func (s *APIServer) handleValidatePolicy(w http.ResponseWriter, r *http.Request) {
	record, err := s.loadPolicy(r, r.PathValue("policyID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, record.MSPID) {
		return
	}
	if record.Status != "draft" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "only draft policies can be validated"})
		return
	}
	input := record.input()
	if err := input.validate(); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := s.validatePolicyScope(r, record.MSPID, input); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if _, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE policies SET validated_at=NOW(), previewed_at=NULL, updated_at=NOW() WHERE id=$1 AND msp_id=$2`, record.ID, record.MSPID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "validate policy failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "validated", "version": record.Version})
}

func (s *APIServer) handlePreviewPolicy(w http.ResponseWriter, r *http.Request) {
	record, err := s.loadPolicy(r, r.PathValue("policyID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, record.MSPID) {
		return
	}
	if record.Status != "draft" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "only draft policies can be previewed"})
		return
	}
	if record.ValidatedAt == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "policy must be validated before preview"})
		return
	}
	target := record.input()
	var override struct {
		ClientID string `json:"client_id"`
		SiteID   string `json:"site_id"`
		DeviceID string `json:"device_id"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&override); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid preview target"})
			return
		}
	}
	if override.ClientID != "" {
		target.ClientID = override.ClientID
	}
	if override.SiteID != "" {
		target.SiteID = override.SiteID
	}
	if override.DeviceID != "" {
		target.DeviceID = override.DeviceID
	}
	if target.DeviceID != "" {
		target.ScopeLevel = "device"
	} else if target.SiteID != "" {
		target.ScopeLevel = "site"
	} else if target.ClientID != "" {
		target.ScopeLevel = "client"
	}
	if err := s.validatePolicyScope(r, record.MSPID, target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	layers, err := s.loadPolicyLayers(r, record, target)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "preview policy failed"})
		return
	}
	if _, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE policies SET previewed_at=NOW(), updated_at=NOW() WHERE id=$1 AND msp_id=$2`, record.ID, record.MSPID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "record policy preview failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"policy_id": record.ID, "version": record.Version,
		"target": map[string]string{"client_id": target.ClientID, "site_id": target.SiteID, "device_id": target.DeviceID},
		"layers": layers, "effective_config": mergePolicyLayers(layers)})
}

func (s *APIServer) handlePublishPolicy(w http.ResponseWriter, r *http.Request) {
	record, err := s.loadPolicy(r, r.PathValue("policyID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, record.MSPID) {
		return
	}
	if record.Status != "draft" || record.ValidatedAt == nil || record.PreviewedAt == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "draft policy must be validated and previewed before publish"})
		return
	}
	actor, _ := r.Context().Value(ctxKeyUserID).(string)
	configJSON, _ := json.Marshal(record.Config)
	var published int
	err = s.requestDB(r).QueryRowContext(r.Context(), `
		WITH activated AS (
			UPDATE policies SET status='active', published_version=version,
			       published_config=config, updated_at=NOW()
			WHERE id=$1 AND msp_id=$2 AND status='draft' AND version=$3
			RETURNING id
		), revision AS (
			INSERT INTO policy_revisions (policy_id,msp_id,version,name,category,description,config,scope_level,client_id,site_id,device_id,published_by)
			SELECT $1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,NULLIF($10,'')::uuid,NULLIF($11,'')::uuid,$12
			FROM activated RETURNING id
		)
		SELECT COUNT(*) FROM revision
	`, record.ID, record.MSPID, record.Version, record.Name, record.Category, record.Description,
		configJSON, record.ScopeLevel, record.ClientID, record.SiteID, record.DeviceID, actor).Scan(&published)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "publish policy failed"})
		return
	}
	if published != 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "policy changed during publish"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "published", "version": record.Version})
}

func (s *APIServer) handlePolicyRevisions(w http.ResponseWriter, r *http.Request) {
	record, err := s.loadPolicy(r, r.PathValue("policyID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}
	if !s.AuthorizeMSPAccess(w, r, record.MSPID) {
		return
	}
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT version,name,category,description,config::text,scope_level,
		       COALESCE(client_id::text,''),COALESCE(site_id::text,''),COALESCE(device_id::text,''),published_by,published_at
		FROM policy_revisions WHERE policy_id=$1 AND msp_id=$2 ORDER BY version DESC
	`, record.ID, record.MSPID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list policy revisions failed"})
		return
	}
	defer func() { _ = rows.Close() }()
	revisions := []map[string]interface{}{}
	for rows.Next() {
		var version int
		var name, category, description, configText, scope, clientID, siteID, deviceID, actor string
		var publishedAt time.Time
		if err := rows.Scan(&version, &name, &category, &description, &configText, &scope,
			&clientID, &siteID, &deviceID, &actor, &publishedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan policy revision failed"})
			return
		}
		var config map[string]interface{}
		_ = json.Unmarshal([]byte(configText), &config)
		revisions = append(revisions, map[string]interface{}{"version": version, "name": name,
			"category": category, "description": description, "config": config, "scope_level": scope,
			"client_id": clientID, "site_id": siteID, "device_id": deviceID,
			"published_by": actor, "published_at": publishedAt.UTC().Format(time.RFC3339)})
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list policy revisions failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"revisions": revisions})
}

func (s *APIServer) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	record, err := s.loadPolicy(r, r.PathValue("policyID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
		return
	}
	if !s.AuthorizeMSPManage(w, r, record.MSPID) {
		return
	}
	if record.PublishedVersion != nil {
		_, err = s.requestDB(r).ExecContext(r.Context(), `UPDATE policies SET status='archived', updated_at=NOW() WHERE id=$1 AND msp_id=$2`, record.ID, record.MSPID)
	} else {
		_, err = s.requestDB(r).ExecContext(r.Context(), `DELETE FROM policies WHERE id=$1 AND msp_id=$2`, record.ID, record.MSPID)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete policy failed"})
		return
	}
	status := "deleted"
	if record.PublishedVersion != nil {
		status = "archived"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *APIServer) loadPolicy(r *http.Request, policyID string) (policyRecord, error) {
	if _, err := uuid.Parse(policyID); err != nil {
		return policyRecord{}, sql.ErrNoRows
	}
	var record policyRecord
	var configText string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id,msp_id,name,category,description,config::text,scope_level,status,version,
		       COALESCE(client_id::text,''),COALESCE(site_id::text,''),COALESCE(device_id::text,''),
		       COALESCE(parent_id::text,''),published_version,validated_at,previewed_at,created_at,updated_at
		FROM policies WHERE id=$1
	`, policyID).Scan(&record.ID, &record.MSPID, &record.Name, &record.Category, &record.Description,
		&configText, &record.ScopeLevel, &record.Status, &record.Version, &record.ClientID,
		&record.SiteID, &record.DeviceID, &record.ParentID, &record.PublishedVersion, &record.ValidatedAt, &record.PreviewedAt,
		&record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return policyRecord{}, err
	}
	if err := json.Unmarshal([]byte(configText), &record.Config); err != nil {
		return policyRecord{}, err
	}
	return record, nil
}

func (p policyRecord) policyResponse() map[string]interface{} {
	return map[string]interface{}{"id": p.ID, "msp_id": p.MSPID, "name": p.Name, "category": p.Category,
		"description": p.Description, "config": p.Config, "scope_level": p.ScopeLevel, "status": p.Status,
		"version": p.Version, "client_id": p.ClientID, "site_id": p.SiteID, "device_id": p.DeviceID,
		"parent_id": p.ParentID, "validated": p.ValidatedAt != nil, "previewed": p.PreviewedAt != nil,
		"published_version": p.PublishedVersion,
		"created_at":        p.CreatedAt.UTC().Format(time.RFC3339), "updated_at": p.UpdatedAt.UTC().Format(time.RFC3339)}
}

func (s *APIServer) validatePolicyScope(r *http.Request, mspID string, input policyInput) error {
	for _, id := range []string{input.ClientID, input.SiteID, input.DeviceID, input.ParentID} {
		if id != "" {
			if _, err := uuid.Parse(id); err != nil {
				return errors.New("scope identifiers must be UUIDs")
			}
		}
	}
	var valid bool
	switch input.ScopeLevel {
	case "msp":
		valid = true
	case "client":
		_ = s.requestDB(r).QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM client_organizations WHERE id=$1 AND msp_id=$2 AND is_active=true)`, input.ClientID, mspID).Scan(&valid)
	case "site":
		_ = s.requestDB(r).QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM sites s JOIN client_organizations c ON c.id=s.client_id WHERE s.id=$1 AND c.id=$2 AND c.msp_id=$3 AND s.is_active=true AND c.is_active=true)`, input.SiteID, input.ClientID, mspID).Scan(&valid)
	case "device":
		_ = s.requestDB(r).QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM devices WHERE id=$1 AND site_id=$2 AND client_id=$3 AND msp_id=$4)`, input.DeviceID, input.SiteID, input.ClientID, mspID).Scan(&valid)
	}
	if !valid {
		return errors.New("policy scope is outside the active MSP hierarchy")
	}
	if input.ParentID != "" {
		var parentScope string
		err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT scope_level FROM policies WHERE id=$1 AND msp_id=$2 AND category=$3 AND status <> 'archived'`, input.ParentID, mspID, input.Category).Scan(&parentScope)
		if err != nil || policyScopeRank[parentScope] >= policyScopeRank[input.ScopeLevel] {
			return errors.New("parent policy must be an active ancestor in the same MSP and category")
		}
	}
	return nil
}

func (s *APIServer) loadPolicyLayers(r *http.Request, current policyRecord, target policyInput) ([]policyLayer, error) {
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id,scope_level,
		       CASE WHEN id=$3 THEN version ELSE published_version END,
		       (CASE WHEN id=$3 THEN config ELSE published_config END)::text
		FROM policies
		WHERE msp_id=$1 AND category=$2 AND status <> 'archived'
		  AND (published_version IS NOT NULL OR id=$3)
		  AND (scope_level='msp'
		    OR (scope_level='client' AND client_id=NULLIF($4,'')::uuid)
		    OR (scope_level='site' AND site_id=NULLIF($5,'')::uuid)
		    OR (scope_level='device' AND device_id=NULLIF($6,'')::uuid))
		ORDER BY CASE scope_level WHEN 'msp' THEN 1 WHEN 'client' THEN 2 WHEN 'site' THEN 3 WHEN 'device' THEN 4 END, id
	`, current.MSPID, current.Category, current.ID, target.ClientID, target.SiteID, target.DeviceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	layers := []policyLayer{}
	for rows.Next() {
		var layer policyLayer
		var configText string
		if err := rows.Scan(&layer.ID, &layer.ScopeLevel, &layer.Version, &configText); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(configText), &layer.Config); err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}
	return layers, rows.Err()
}
