package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultLifecycleExportLimit = 1000
	maxLifecycleExportLimit     = 5000
)

type lifecycleExportData struct {
	MSP         map[string]interface{}   `json:"msp"`
	Entitlement map[string]interface{}   `json:"entitlement"`
	Clients     []map[string]interface{} `json:"clients"`
	Sites       []map[string]interface{} `json:"sites"`
	Devices     []map[string]interface{} `json:"devices"`
	Memberships []map[string]interface{} `json:"memberships"`
}

func lifecycleExportLimit(raw string) (int, string) {
	if raw == "" {
		return defaultLifecycleExportLimit, ""
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maxLifecycleExportLimit {
		return 0, "limit must be between 1 and 5000"
	}
	return limit, ""
}

func (s *APIServer) handleExportMSP(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	limit, validationError := lifecycleExportLimit(r.URL.Query().Get("limit"))
	if validationError != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": validationError})
		return
	}

	db := s.requestDB(r)
	data := lifecycleExportData{
		Clients:     make([]map[string]interface{}, 0),
		Sites:       make([]map[string]interface{}, 0),
		Devices:     make([]map[string]interface{}, 0),
		Memberships: make([]map[string]interface{}, 0),
	}

	var mspIDValue, name, slug, plan string
	var active bool
	var createdAt time.Time
	if err := db.QueryRowContext(r.Context(), `
		SELECT id, name, slug, plan, is_active, created_at
		FROM msp_tenants WHERE id = $1
	`, mspID).Scan(&mspIDValue, &name, &slug, &plan, &active, &createdAt); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "msp not found"})
		return
	}
	data.MSP = map[string]interface{}{
		"id": mspIDValue, "name": name, "slug": slug, "plan": plan,
		"is_active": active, "created_at": createdAt.UTC(),
	}

	var entitlementPlan, entitlementStatus string
	var graceEndsAt, expiresAt *time.Time
	if err := db.QueryRowContext(r.Context(), `
		SELECT p.slug, pe.status, pe.grace_period_ends_at, pe.expires_at
		FROM plan_entitlements pe JOIN plans p ON p.id = pe.plan_id
		WHERE pe.msp_id = $1
	`, mspID).Scan(&entitlementPlan, &entitlementStatus, &graceEndsAt, &expiresAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "entitlement export unavailable"})
		return
	}
	data.Entitlement = map[string]interface{}{
		"plan_slug": entitlementPlan, "status": entitlementStatus,
		"grace_period_ends_at": graceEndsAt, "expires_at": expiresAt,
	}

	remaining := limit
	clients, err := db.QueryContext(r.Context(), `
		SELECT id, name, slug, is_active, created_at
		FROM client_organizations WHERE msp_id = $1 ORDER BY id LIMIT $2
	`, mspID, remaining)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "client export unavailable"})
		return
	}
	for clients.Next() {
		var id, clientName, clientSlug string
		var clientActive bool
		var clientCreated time.Time
		if err := clients.Scan(&id, &clientName, &clientSlug, &clientActive, &clientCreated); err != nil {
			_ = clients.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "client export failed"})
			return
		}
		data.Clients = append(data.Clients, map[string]interface{}{
			"id": id, "name": clientName, "slug": clientSlug,
			"is_active": clientActive, "created_at": clientCreated.UTC(),
		})
		remaining--
	}
	if err := clients.Err(); err != nil {
		_ = clients.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "client export failed"})
		return
	}
	_ = clients.Close()

	if remaining > 0 {
		sites, err := db.QueryContext(r.Context(), `
			SELECT s.id, s.client_id, s.name, s.slug, s.is_active, s.created_at
			FROM sites s JOIN client_organizations c ON c.id = s.client_id
			WHERE c.msp_id = $1 ORDER BY s.id LIMIT $2
		`, mspID, remaining)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site export unavailable"})
			return
		}
		for sites.Next() {
			var id, clientID, siteName, siteSlug string
			var siteActive bool
			var siteCreated time.Time
			if err := sites.Scan(&id, &clientID, &siteName, &siteSlug, &siteActive, &siteCreated); err != nil {
				_ = sites.Close()
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site export failed"})
				return
			}
			data.Sites = append(data.Sites, map[string]interface{}{
				"id": id, "client_id": clientID, "name": siteName, "slug": siteSlug,
				"is_active": siteActive, "created_at": siteCreated.UTC(),
			})
			remaining--
		}
		if err := sites.Err(); err != nil {
			_ = sites.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "site export failed"})
			return
		}
		_ = sites.Close()
	}

	if remaining > 0 {
		devices, err := db.QueryContext(r.Context(), `
			SELECT id, client_id, site_id, hostname, COALESCE(os, ''), COALESCE(os_version, ''),
			       status, enrolled_at, created_at
			FROM devices WHERE msp_id = $1 ORDER BY id LIMIT $2
		`, mspID, remaining)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device export unavailable"})
			return
		}
		for devices.Next() {
			var id, clientID, hostname, operatingSystem, osVersion, deviceStatus string
			var siteID *string
			var enrolledAt *time.Time
			var deviceCreated time.Time
			if err := devices.Scan(
				&id, &clientID, &siteID, &hostname, &operatingSystem,
				&osVersion, &deviceStatus, &enrolledAt, &deviceCreated,
			); err != nil {
				_ = devices.Close()
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device export failed"})
				return
			}
			data.Devices = append(data.Devices, map[string]interface{}{
				"id": id, "client_id": clientID, "site_id": siteID, "hostname": hostname,
				"os": operatingSystem, "os_version": osVersion, "status": deviceStatus,
				"enrolled_at": enrolledAt, "created_at": deviceCreated.UTC(),
			})
			remaining--
		}
		if err := devices.Err(); err != nil {
			_ = devices.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device export failed"})
			return
		}
		_ = devices.Close()
	}

	if remaining > 0 {
		memberships, err := db.QueryContext(r.Context(), `
			SELECT id, user_id, role, scope_type, scope_id, status, created_at
			FROM memberships
			WHERE (scope_type = 'msp' AND scope_id = $1::text)
			   OR (scope_type = 'client' AND scope_id IN (
			     SELECT id::text FROM client_organizations WHERE msp_id = $1::uuid
			   ))
			   OR (scope_type = 'site' AND scope_id IN (
			     SELECT s.id::text FROM sites s
			     JOIN client_organizations c ON c.id = s.client_id
			     WHERE c.msp_id = $1::uuid
			   ))
			ORDER BY id LIMIT $2
		`, mspID, remaining)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership export unavailable"})
			return
		}
		for memberships.Next() {
			var id, userID, role, scopeType, scopeID, membershipStatus string
			var membershipCreated time.Time
			if err := memberships.Scan(
				&id, &userID, &role, &scopeType, &scopeID, &membershipStatus, &membershipCreated,
			); err != nil {
				_ = memberships.Close()
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership export failed"})
				return
			}
			data.Memberships = append(data.Memberships, map[string]interface{}{
				"id": id, "user_id": userID, "role": role, "scope_type": scopeType,
				"scope_id": scopeID, "status": membershipStatus, "created_at": membershipCreated.UTC(),
			})
			remaining--
		}
		if err := memberships.Err(); err != nil {
			_ = memberships.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership export failed"})
			return
		}
		_ = memberships.Close()
	}

	var totalRecords int
	if err := db.QueryRowContext(r.Context(), `
		SELECT
		  (SELECT COUNT(*) FROM client_organizations WHERE msp_id = $1)
		  + (SELECT COUNT(*) FROM sites s JOIN client_organizations c ON c.id = s.client_id WHERE c.msp_id = $1)
		  + (SELECT COUNT(*) FROM devices WHERE msp_id = $1)
		  + (SELECT COUNT(*) FROM memberships
		     WHERE (scope_type = 'msp' AND scope_id = $1::text)
		        OR (scope_type = 'client' AND scope_id IN (
		          SELECT id::text FROM client_organizations WHERE msp_id = $1::uuid
		        ))
		        OR (scope_type = 'site' AND scope_id IN (
		          SELECT s.id::text FROM sites s
		          JOIN client_organizations c ON c.id = s.client_id
		          WHERE c.msp_id = $1::uuid
		        )))
	`, mspID).Scan(&totalRecords); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "export count unavailable"})
		return
	}

	serialized, err := json.Marshal(data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "export serialization failed"})
		return
	}
	digest := sha256.Sum256(serialized)
	exportedRecords := limit - remaining
	if err := s.auditControlPlane(r, mspID, "msp.exported", "msp", mspID,
		map[string]interface{}{"record_count": exportedRecords, "truncated": totalRecords > exportedRecords}); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="strata-msp-export.json"`)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"schema_version": 1,
		"generated_at":   time.Now().UTC(),
		"record_limit":   limit,
		"record_count":   exportedRecords,
		"total_records":  totalRecords,
		"truncated":      totalRecords > exportedRecords,
		"sha256":         hex.EncodeToString(digest[:]),
		"data":           data,
	})
}
