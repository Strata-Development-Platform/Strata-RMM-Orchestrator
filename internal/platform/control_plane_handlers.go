package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var hostnamePattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

func (s *APIServer) handleCreateDomain(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}
	var req struct {
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	req.Hostname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(req.Hostname), "."))
	if !hostnamePattern.MatchString(req.Hostname) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid hostname required"})
		return
	}
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "verification token unavailable"})
		return
	}
	id := uuid.New().String()
	token := hex.EncodeToString(tokenBytes)
	_, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO custom_domains (id, msp_id, hostname, domain_type, verification_token)
		VALUES ($1, $2, $3, 'portal', $4)
	`, id, mspID, req.Hostname, token)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "hostname is already registered"})
		return
	}
	if err := s.auditControlPlane(r, mspID, "domain.created", "custom_domain", id, map[string]string{"hostname": req.Hostname}); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"id": id, "hostname": req.Hostname, "verification_token": token,
		"txt_name": "_strata-verification." + req.Hostname,
	})
}

func (s *APIServer) handleVerifyDomain(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}
	domainID := r.PathValue("domainID")
	var hostname, verificationToken string
	if err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT hostname, verification_token FROM custom_domains
		WHERE id = $1 AND msp_id = $2 AND verification_status IN ('pending', 'failed')
	`, domainID, mspID).Scan(&hostname, &verificationToken); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pending domain not found"})
		return
	}
	lookupContext, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	records, err := net.DefaultResolver.LookupTXT(lookupContext, "_strata-verification."+hostname)
	if err != nil {
		_, _ = s.requestDB(r).ExecContext(r.Context(),
			`UPDATE custom_domains SET verification_status = 'failed', last_check_at = NOW() WHERE id = $1 AND msp_id = $2`,
			domainID, mspID)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "verification TXT record not found"})
		return
	}
	verified := false
	for _, record := range records {
		if strings.TrimSpace(record) == verificationToken ||
			strings.TrimSpace(record) == "strata-verification="+verificationToken {
			verified = true
			break
		}
	}
	if !verified {
		_, _ = s.requestDB(r).ExecContext(r.Context(),
			`UPDATE custom_domains SET verification_status = 'failed', last_check_at = NOW() WHERE id = $1 AND msp_id = $2`,
			domainID, mspID)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "verification TXT record does not match"})
		return
	}
	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE custom_domains
		SET verification_status = 'verified', certificate_status = 'requested',
		    verified_at = NOW(), last_check_at = NOW()
		WHERE id = $1 AND msp_id = $2 AND verification_status IN ('pending', 'failed')
	`, domainID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "domain verification failed"})
		return
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pending domain not found"})
		return
	}
	if err := s.auditControlPlane(r, mspID, "domain.verified", "custom_domain", domainID, nil); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified", "certificate_status": "requested"})
}

func (s *APIServer) handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
	if !s.AuthorizeMSPManage(w, r, mspID) {
		return
	}
	domainID := r.PathValue("domainID")
	result, err := s.requestDB(r).ExecContext(r.Context(),
		`DELETE FROM custom_domains WHERE id = $1 AND msp_id = $2`, domainID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "domain removal failed"})
		return
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
		return
	}
	if err := s.auditControlPlane(r, mspID, "domain.deleted", "custom_domain", domainID, nil); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *APIServer) handleUpdateDomainCertificate(w http.ResponseWriter, r *http.Request) {
	domainID := r.PathValue("domainID")
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Status != "issued" && req.Status != "failed" && req.Status != "expired" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be issued, failed, or expired"})
		return
	}
	var mspID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		UPDATE custom_domains
		SET certificate_status = $2,
		    verification_status = CASE WHEN $2 = 'issued' THEN 'active' ELSE verification_status END,
		    last_check_at = NOW()
		WHERE id = $1 AND verification_status IN ('verified', 'active')
		RETURNING msp_id
	`, domainID, req.Status).Scan(&mspID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "verified domain not found"})
		return
	}
	if err := s.auditControlPlane(r, mspID, "domain.certificate_updated", "custom_domain", domainID,
		map[string]string{"status": req.Status}); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}

func (s *APIServer) handleGetEntitlement(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}
	var response struct {
		PlanSlug        string          `json:"plan_slug"`
		Status          string          `json:"status"`
		GracePeriodEnds *time.Time      `json:"grace_period_ends_at,omitempty"`
		MaxDevices      int             `json:"max_devices"`
		MaxUsers        int             `json:"max_users"`
		DeviceCount     int             `json:"device_count"`
		UserCount       int             `json:"user_count"`
		Features        json.RawMessage `json:"features"`
	}
	var features []byte
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT p.slug, pe.status, pe.grace_period_ends_at, p.max_devices, p.max_users,
		       (SELECT COUNT(*) FROM devices d WHERE d.msp_id = pe.msp_id),
		       (SELECT COUNT(DISTINCT mb.user_id) FROM memberships mb
		        WHERE mb.scope_type = 'msp' AND mb.scope_id = pe.msp_id::text AND mb.status = 'active'),
		       p.features
		FROM plan_entitlements pe JOIN plans p ON p.id = pe.plan_id
		WHERE pe.msp_id = $1
	`, mspID).Scan(&response.PlanSlug, &response.Status, &response.GracePeriodEnds, &response.MaxDevices,
		&response.MaxUsers, &response.DeviceCount, &response.UserCount, &features)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "entitlement not found"})
		return
	}
	response.Features = append(json.RawMessage(nil), features...)
	writeJSON(w, http.StatusOK, response)
}

func (s *APIServer) handleUpdateEntitlement(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	var req struct {
		PlanSlug        string `json:"plan_slug"`
		Status          string `json:"status"`
		GracePeriodDays int    `json:"grace_period_days,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	req.PlanSlug = strings.ToLower(strings.TrimSpace(req.PlanSlug))
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Status == "past_due" {
		if req.GracePeriodDays < 1 || req.GracePeriodDays > 90 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "past_due requires grace_period_days from 1 to 90"})
			return
		}
	} else if req.GracePeriodDays != 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "grace_period_days is only valid for past_due"})
		return
	}
	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE plan_entitlements pe
		SET plan_id = p.id,
		    status = $3,
		    grace_period_ends_at = CASE
		      WHEN $3 = 'past_due' THEN NOW() + make_interval(days => $4)
		      ELSE NULL
		    END,
		    updated_at = NOW()
		FROM plans p
		WHERE pe.msp_id = $1 AND p.slug = $2 AND p.is_active = true
		  AND $3 IN ('active','past_due','suspended','cancelled')
	`, mspID, req.PlanSlug, req.Status, req.GracePeriodDays)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "entitlement update failed"})
		return
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown plan or invalid status"})
		return
	}
	if _, err := s.requestDB(r).ExecContext(r.Context(), `UPDATE msp_tenants SET plan = $2 WHERE id = $1`, mspID, req.PlanSlug); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "MSP plan synchronization failed"})
		return
	}
	if err := s.auditControlPlane(r, mspID, "entitlement.updated", "plan_entitlement", mspID,
		map[string]interface{}{"plan": req.PlanSlug, "status": req.Status, "grace_period_days": req.GracePeriodDays}); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *APIServer) handleUsage(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}
	var devices, users, clients, sites int
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT
		  (SELECT COUNT(*) FROM devices WHERE msp_id = $1),
		  (SELECT COUNT(DISTINCT user_id) FROM memberships WHERE scope_type = 'msp' AND scope_id = $1::text AND status = 'active'),
		  (SELECT COUNT(*) FROM client_organizations WHERE msp_id = $1 AND is_active = true),
		  (SELECT COUNT(*) FROM sites s JOIN client_organizations c ON c.id = s.client_id WHERE c.msp_id = $1 AND s.is_active = true)
	`, mspID).Scan(&devices, &users, &clients, &sites)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "usage unavailable"})
		return
	}
	_, _ = s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO usage_snapshots (msp_id, device_count, user_count, client_count, site_count)
		VALUES ($1, $2, $3, $4, $5)
	`, mspID, devices, users, clients, sites)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"msp_id": mspID, "device_count": devices, "user_count": users,
		"client_count": clients, "site_count": sites, "recorded_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleControlPlaneAudit(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, actor_user_id, action, resource_type, resource_id, details, created_at
		FROM control_plane_audit WHERE msp_id = $1 ORDER BY created_at DESC LIMIT 200
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "audit unavailable"})
		return
	}
	defer func() { _ = rows.Close() }()
	entries := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, actor, action, resourceType, resourceID string
		var details []byte
		var createdAt time.Time
		if err := rows.Scan(&id, &actor, &action, &resourceType, &resourceID, &details, &createdAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "audit unavailable"})
			return
		}
		entries = append(entries, map[string]interface{}{
			"id": id, "actor_user_id": actor, "action": action, "resource_type": resourceType,
			"resource_id": resourceID, "details": json.RawMessage(details),
			"created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "audit unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"entries": entries})
}

func (s *APIServer) handleMSPDevices(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	if !s.AuthorizeMSPAccess(w, r, mspID) {
		return
	}
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT d.id::text, d.tenant_id::text, d.hostname, COALESCE(d.os, ''),
		       COALESCE(d.arch, ''), d.status, COALESCE(d.agent_version, ''),
		       d.last_heartbeat, c.id::text, c.name, s.id::text, s.name
		FROM devices d
		JOIN client_organizations c ON c.id = d.client_id AND c.msp_id = d.msp_id
		JOIN sites s ON s.id = d.site_id AND s.client_id = c.id
		WHERE d.msp_id = $1
		ORDER BY d.hostname
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "devices unavailable"})
		return
	}
	defer func() { _ = rows.Close() }()
	devices := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, tenantID, hostname, osName, arch, status, version, clientID, clientName, siteID, siteName string
		var lastHeartbeat *time.Time
		if err := rows.Scan(&id, &tenantID, &hostname, &osName, &arch, &status, &version,
			&lastHeartbeat, &clientID, &clientName, &siteID, &siteName); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "devices unavailable"})
			return
		}
		device := map[string]interface{}{
			"id": id, "tenant_id": tenantID, "hostname": hostname, "os": osName,
			"arch": arch, "status": status, "agent_version": version,
			"client_id": clientID, "client_name": clientName, "site_id": siteID, "site_name": siteName,
		}
		if lastHeartbeat != nil {
			device["last_heartbeat"] = lastHeartbeat.UTC().Format(time.RFC3339)
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "devices unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
}

func (s *APIServer) handleContextSwitch(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	var req struct {
		ScopeType string `json:"scope_type"`
		MSPID     string `json:"msp_id"`
		ClientID  string `json:"client_id"`
		SiteID    string `json:"site_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.ScopeType == "" {
		if req.MSPID == "" {
			req.ScopeType = string(ScopePlatform)
		} else if req.SiteID != "" {
			req.ScopeType = string(ScopeSite)
		} else if req.ClientID != "" {
			req.ScopeType = string(ScopeClient)
		} else {
			req.ScopeType = string(ScopeMSP)
		}
	}
	if ScopeType(req.ScopeType) == ScopePlatform {
		if req.MSPID != "" || req.ClientID != "" || req.SiteID != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platform scope cannot include child identifiers"})
			return
		}
	} else if req.MSPID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "msp_id required for a child scope"})
		return
	}
	if (ScopeType(req.ScopeType) == ScopeMSP && (req.ClientID != "" || req.SiteID != "")) ||
		(ScopeType(req.ScopeType) == ScopeClient && (req.ClientID == "" || req.SiteID != "")) ||
		(ScopeType(req.ScopeType) == ScopeSite && (req.ClientID == "" || req.SiteID == "")) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope_type does not match the supplied hierarchy"})
		return
	}

	tx, err := s.db.DB().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "workspace transaction unavailable"})
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(r.Context(), `
		SELECT set_config('app.user_id', $1, true),
		       set_config('app.msp_id', $2, true),
		       set_config('app.client_id', $3, true),
		       set_config('app.site_id', $4, true),
		       set_config('app.tenant_id', $3, true),
		       set_config('app.scope_type', $5, true),
		       set_config('app.permission', 'write', true)
	`, userID, req.MSPID, req.ClientID, req.SiteID, req.ScopeType); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "workspace security context unavailable"})
		return
	}
	authorization, err := s.resolveAuthorization(r.Context(), tx, userID, req.MSPID, req.ClientID, req.SiteID)
	if err != nil || authorization.Selected.Type != ScopeType(req.ScopeType) {
		writeAuthorizationDenied(w)
		return
	}
	dbRole := ""
	if authorization.IsPlatformGlobal() {
		dbRole = "platform_admin"
	}
	if _, err := tx.ExecContext(r.Context(), `SELECT set_config('app.role', $1, true)`, dbRole); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "workspace security context unavailable"})
		return
	}
	details, err := json.Marshal(map[string]string{
		"scope_type": req.ScopeType, "msp_id": req.MSPID,
		"client_id": req.ClientID, "site_id": req.SiteID,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "workspace audit unavailable"})
		return
	}
	resourceID := authorization.Selected.ID
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO control_plane_audit (msp_id, actor_user_id, action, resource_type, resource_id, details)
		VALUES ($1, $2, 'context.switched', 'workspace', $3, $4)
	`, nullIfEmpty(authorization.Selected.MSPID), userID, resourceID, details); err != nil {
		writeControlPlaneAuditFailure(w)
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "workspace transaction failed"})
		return
	}
	committed = true

	token, err := s.tokenGen.GenerateUserToken(
		userID, authorization.Selected.ClientID, authorization.Selected.MSPID,
		authorization.Selected.ClientID, authorization.Selected.SiteID,
		authorization.Roles, 8*time.Hour,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "workspace token unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": token, "selected_scope": authorization.Selected,
		"roles": authorization.Roles, "permissions": authorization.Permissions,
		"msp_id": authorization.Selected.MSPID, "client_id": authorization.Selected.ClientID,
		"site_id":    authorization.Selected.SiteID,
		"expires_at": time.Now().Add(8 * time.Hour).UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) auditControlPlane(r *http.Request, mspID, action, resourceType, resourceID string, details interface{}) error {
	if details == nil {
		details = map[string]string{}
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode control-plane audit details: %w", err)
	}
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if _, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO control_plane_audit (msp_id, actor_user_id, action, resource_type, resource_id, details)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, nullIfEmpty(mspID), userID, action, resourceType, resourceID, payload); err != nil {
		return fmt.Errorf("record control-plane audit event: %w", err)
	}
	return nil
}

func writeControlPlaneAuditFailure(w http.ResponseWriter) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "control-plane audit recording failed"})
}
