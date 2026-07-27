package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token             string       `json:"token"`
	UserID            string       `json:"user_id"`
	Email             string       `json:"email"`
	Role              string       `json:"role"`
	AccessibleTenants []tenantInfo `json:"accessible_tenants"`
	ExpiresAt         time.Time    `json:"expires_at"`
}

type tenantInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *APIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password required"})
		return
	}

	db := s.requestDB(r)
	var userID, tenantID, role, passwordHash string
	err := db.QueryRowContext(r.Context(), `
		SELECT id, tenant_id, email, role, password_hash
		FROM users WHERE email = $1 AND is_active = true
	`, req.Email).Scan(&userID, &tenantID, &req.Email, &role, &passwordHash)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logAuditAuth("", req.Email, r.RemoteAddr, false, "user not found")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		s.logAuditAuth(userID, req.Email, r.RemoteAddr, false, "wrong password")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	var mspID string
	db.QueryRowContext(r.Context(), `SELECT msp_id FROM client_organizations WHERE id = $1`, tenantID).Scan(&mspID)

	if s.tokenGen == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authentication is not configured"})
		return
	}
	ttl := 8 * time.Hour
	token, err := s.tokenGen.GenerateUserToken(userID, tenantID, mspID, "", "", []string{role}, ttl)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token generation failed"})
		return
	}

	accessibleTenants, _ := s.getAccessibleTenants(r, userID, role, tenantID)

	db.ExecContext(r.Context(), `UPDATE users SET last_login = NOW() WHERE id = $1`, userID)
	s.logAuditAuth(userID, req.Email, r.RemoteAddr, true, "login")

	writeJSON(w, http.StatusOK, loginResponse{
		Token:             token,
		UserID:            userID,
		Email:             req.Email,
		Role:              role,
		AccessibleTenants: accessibleTenants,
		ExpiresAt:         time.Now().Add(ttl),
	})
}

func (s *APIServer) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(ctxKeyUserID).(string)
	if !ok || principal == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session required"})
		return
	}

	db := s.requestDB(r)
	var userID, email, role, tenantID string
	var isActive bool
	err := db.QueryRowContext(r.Context(), `
		SELECT id, email, role, tenant_id, is_active FROM users WHERE id = $1
	`, principal).Scan(&userID, &email, &role, &tenantID, &isActive)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if !isActive {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account disabled"})
		return
	}

	accessibleTenants, _ := s.getAccessibleTenants(r, userID, role, tenantID)

	writeJSON(w, http.StatusOK, loginResponse{
		Token:             r.Header.Get("Authorization"),
		UserID:            userID,
		Email:             email,
		Role:              role,
		AccessibleTenants: accessibleTenants,
	})
}

func (s *APIServer) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	db := s.requestDB(r)
	rows, err := db.QueryContext(r.Context(), `
		SELECT u.id, u.email, u.role, u.is_active, u.last_login, u.created_at,
		       COALESCE(json_agg(json_build_object('id', t.id, 'name', t.name, 'slug', t.slug))
		                FILTER (WHERE t.id IS NOT NULL), '[]') as accessible_tenants
		FROM users u
		LEFT JOIN user_tenant_access uta ON u.id = uta.user_id
		LEFT JOIN tenants t ON uta.tenant_id = t.id
		GROUP BY u.id
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id, email, role string
		var isActive bool
		var createdAt time.Time
		var lastLoginNull sql.NullTime
		var tenantsJSON []byte

		if err := rows.Scan(&id, &email, &role, &isActive, &lastLoginNull, &createdAt, &tenantsJSON); err != nil {
			continue
		}
		user := map[string]interface{}{
			"id":         id,
			"email":      email,
			"role":       role,
			"is_active":  isActive,
			"created_at": createdAt,
		}
		if lastLoginNull.Valid {
			user["last_login"] = lastLoginNull.Time
		}
		if len(tenantsJSON) > 0 {
			var tenants []map[string]interface{}
			json.Unmarshal(tenantsJSON, &tenants)
			user["accessible_tenants"] = tenants
		}
		users = append(users, user)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": users})
}

func (s *APIServer) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email     string   `json:"email"`
		Password  string   `json:"password"`
		Role      string   `json:"role"`
		TenantIDs []string `json:"tenant_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Email == "" || req.Password == "" || req.Role == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email, password, role required"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "hash failed"})
		return
	}

	db := s.requestDB(r)
	var userID string
	err = db.QueryRowContext(r.Context(), `
		INSERT INTO users (email, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id
	`, req.Email, string(hash), req.Role).Scan(&userID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already exists"})
		return
	}

	for _, tenantID := range req.TenantIDs {
		db.ExecContext(r.Context(), `
			INSERT INTO user_tenant_access (user_id, tenant_id) VALUES ($1, $2) ON CONFLICT DO NOTHING
		`, userID, tenantID)
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": userID, "status": "created"})
}

func (s *APIServer) handleAdminCreateCustomer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Slug       string `json:"slug"`
		Plan       string `json:"plan"`
		AdminEmail string `json:"admin_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}

	db := s.requestDB(r)
	deploymentID := generateDeploymentID(req.Name)
	slug := req.Slug
	if slug == "" {
		slug = strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
	}
	if req.Plan == "" {
		req.Plan = "free"
	}

	var tenantID string
	err := db.QueryRowContext(r.Context(), `
		INSERT INTO tenants (name, slug, plan, deployment_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, req.Name, slug, req.Plan, deploymentID).Scan(&tenantID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "slug already exists"})
		return
	}

	if req.AdminEmail != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte("changeme123"), bcrypt.DefaultCost)
		db.ExecContext(r.Context(), `
			INSERT INTO users (tenant_id, email, password_hash, role)
			VALUES ($1, $2, $3, 'admin')
		`, tenantID, req.AdminEmail, string(hash))
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            tenantID,
		"name":          req.Name,
		"slug":          slug,
		"plan":          req.Plan,
		"deployment_id": deploymentID,
		"status":        "created",
	})
}

func (s *APIServer) handleAdminUpdateUserTenants(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	var req struct {
		TenantIDs []string `json:"tenant_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	db := s.requestDB(r)
	if _, err := db.ExecContext(r.Context(), `DELETE FROM user_tenant_access WHERE user_id = $1`, userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant access update failed"})
		return
	}
	for _, tenantID := range req.TenantIDs {
		if _, err := db.ExecContext(r.Context(), `INSERT INTO user_tenant_access (user_id, tenant_id) VALUES ($1, $2)`, userID, tenantID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant access update failed"})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *APIServer) handlePlatformOverview(w http.ResponseWriter, r *http.Request) {
	db := s.requestDB(r)

	var totalDevices, onlineDevices, activeAlerts, openCVEs int
	db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM devices`).Scan(&totalDevices)
	db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM devices WHERE status = 'online'`).Scan(&onlineDevices)

	var criticalAlerts int
	db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM alerts WHERE status = 'firing' AND severity = 'critical'
	`).Scan(&criticalAlerts)
	db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM alerts WHERE status = 'firing'`).Scan(&activeAlerts)

	db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM device_vulnerabilities WHERE status = 'open' AND severity IN ('critical', 'high')
	`).Scan(&openCVEs)

	var totalCustomers int
	db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tenants`).Scan(&totalCustomers)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_devices":   totalDevices,
		"online_devices":  onlineDevices,
		"offline_devices": totalDevices - onlineDevices,
		"active_alerts":   activeAlerts,
		"critical_alerts": criticalAlerts,
		"open_cves":       openCVEs,
		"total_customers": totalCustomers,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handlePlatformCustomers(w http.ResponseWriter, r *http.Request) {
	db := s.requestDB(r)
	rows, err := db.QueryContext(r.Context(), `
		SELECT t.id, t.name, t.slug, t.plan, t.is_active, t.created_at,
		       COALESCE(t.deployment_id, '') as deployment_id,
		       (SELECT COUNT(*) FROM devices d WHERE d.tenant_id = t.id) as device_count,
		       (SELECT COUNT(*) FROM devices d WHERE d.tenant_id = t.id AND d.status = 'online') as online_count,
		       (SELECT COUNT(*) FROM alerts a WHERE a.tenant_id = t.id AND a.status = 'firing') as alert_count,
		       (SELECT COUNT(*) FROM device_vulnerabilities dv
		        JOIN devices d ON dv.device_id = d.id
		        WHERE d.tenant_id = t.id AND dv.status = 'open') as cve_count
		FROM tenants t
		ORDER BY t.name ASC
	`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var customers []map[string]interface{}
	for rows.Next() {
		var id, name, slug, plan string
		var isActive bool
		var createdAt time.Time
		var deviceCount, onlineCount, alertCount, cveCount int

		var deploymentID string
		if err := rows.Scan(&id, &name, &slug, &plan, &isActive, &createdAt, &deploymentID, &deviceCount, &onlineCount, &alertCount, &cveCount); err != nil {
			continue
		}
		customers = append(customers, map[string]interface{}{
			"id":            id,
			"name":          name,
			"slug":          slug,
			"plan":          plan,
			"is_active":     isActive,
			"device_count":  deviceCount,
			"online_count":  onlineCount,
			"alert_count":   alertCount,
			"cve_count":     cveCount,
			"deployment_id": deploymentID,
			"created_at":    createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"customers": customers})
}

func (s *APIServer) getAccessibleTenants(r *http.Request, userID, role, tenantID string) ([]tenantInfo, error) {
	db := s.db.DB()
	if role == "admin" {
		rows, err := db.Query(`SELECT id, name, slug FROM tenants ORDER BY name`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var tenants []tenantInfo
		for rows.Next() {
			var t tenantInfo
			rows.Scan(&t.ID, &t.Name, &t.Slug)
			tenants = append(tenants, t)
		}
		return tenants, nil
	}

	rows, err := db.Query(`
		SELECT t.id, t.name, t.slug FROM tenants t
		JOIN user_tenant_access uta ON t.id = uta.tenant_id
		WHERE uta.user_id = $1
		ORDER BY t.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tenants []tenantInfo
	for rows.Next() {
		var t tenantInfo
		rows.Scan(&t.ID, &t.Name, &t.Slug)
		tenants = append(tenants, t)
	}
	return tenants, nil
}

func (s *APIServer) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeploymentID    string `json:"deployment_id"`
		EnrollmentToken string `json:"enrollment_token"`
		Hostname        string `json:"hostname"`
		OS              string `json:"os"`
		Arch            string `json:"arch"`
		Version         string `json:"version"`
		PublicKey       string `json:"public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Hostname == "" || (req.EnrollmentToken == "" && req.DeploymentID == "") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hostname and enrollment_token required"})
		return
	}
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	tx, err := s.db.DB().BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database transaction unavailable"})
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var tenantID, mspID, clientID, siteID string
	if req.EnrollmentToken != "" {
		hash := sha256.Sum256([]byte(req.EnrollmentToken))
		tokenHash := hex.EncodeToString(hash[:])
		if _, err := tx.ExecContext(r.Context(), `SELECT set_config('app.enrollment_hash', $1, true)`, tokenHash); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database security context unavailable"})
			return
		}
		err = tx.QueryRowContext(r.Context(), `
			UPDATE enrollment_tokens_v2 et
			SET use_count = use_count + 1
			FROM msp_tenants m, client_organizations c
			WHERE et.token_hash = $1
			  AND et.is_revoked = false
			  AND et.expires_at > NOW()
			  AND et.use_count < et.max_uses
			  AND m.id = et.msp_id AND m.is_active = true
			  AND c.id = et.client_id AND c.msp_id = et.msp_id AND c.is_active = true
			RETURNING et.client_id::text, et.msp_id::text, et.client_id::text,
				COALESCE(
					et.site_id::text,
					(SELECT s.id::text FROM sites s WHERE s.client_id = et.client_id AND s.is_active = true ORDER BY s.created_at LIMIT 1)
				)
		`, tokenHash).Scan(&tenantID, &mspID, &clientID, &siteID)
	} else {
		if os.Getenv("STRATA_ALLOW_LEGACY_DEPLOYMENT_ENROLLMENT") != "true" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "enrollment token required"})
			return
		}
		err = tx.QueryRowContext(r.Context(), `
			SELECT t.id::text, c.msp_id::text, c.id::text,
			       COALESCE((SELECT s.id::text FROM sites s WHERE s.client_id = c.id AND s.is_active = true ORDER BY s.created_at LIMIT 1), '')
			FROM tenants t
			JOIN client_organizations c ON c.id = t.id
			WHERE t.deployment_id = $1 AND t.is_active = true AND c.is_active = true
		`, req.DeploymentID).Scan(&tenantID, &mspID, &clientID, &siteID)
	}
	if err != nil || siteID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid enrollment or no active site"})
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		SELECT set_config('app.msp_id', $1, true), set_config('app.tenant_id', $2, true)
	`, mspID, tenantID); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database security context unavailable"})
		return
	}

	agentID := uuid.NewString()
	pubKey, err := hex.DecodeString(req.PublicKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "public_key must be hexadecimal"})
		return
	}
	var deviceID string
	agentVersion := req.Version
	if agentVersion == "" {
		agentVersion = "0.0.0"
	}

	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO devices (tenant_id, msp_id, client_id, site_id, agent_id, hostname, os, arch, agent_version, status, enrolled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'online', NOW())
		RETURNING id
	`, tenantID, mspID, clientID, siteID, agentID, req.Hostname, req.OS, req.Arch, agentVersion).Scan(&deviceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create device failed"})
		return
	}

	if _, err = tx.ExecContext(r.Context(), `
		INSERT INTO agent_registrations (deployment_id, device_id, agent_id, public_key, hostname, os, arch, ip_address, approved)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true)
	`, req.DeploymentID, deviceID, agentID, pubKey, req.Hostname, req.OS, req.Arch, r.RemoteAddr); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "register agent failed"})
		return
	}

	if s.tokenGen == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authentication is not configured"})
		return
	}
	token, err := s.tokenGen.GenerateAgentToken(tenantID, agentID, 720*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token generation failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "registration transaction failed"})
		return
	}
	committed = true

	natsURLs := []string{}
	if s.nats != nil {
		natsURLs = append(natsURLs, s.nats.ConnectedUrl())
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"device_id": deviceID,
		"agent_id":  agentID,
		"tenant_id": tenantID,
		"token":     token,
		"nats_urls": natsURLs,
		"interval":  60,
	})
}

func (s *APIServer) handleAgentConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeploymentID string `json:"deployment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeploymentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "deployment_id required"})
		return
	}

	var source, channel string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT update_source, update_channel FROM tenants WHERE deployment_id = $1 AND is_active = true
	`, req.DeploymentID).Scan(&source, &channel)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid deployment_id"})
		return
	}

	manifestURL := "https://releases.strata-rmm.io"
	if source == "server" {
		manifestURL = fmt.Sprintf("http://%s", r.Host)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"update_source":  source,
		"update_channel": channel,
		"manifest_url":   manifestURL,
		"check_interval": 86400,
		"server_time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleDeviceUpdate(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	deviceID := r.PathValue("deviceID")

	var source, channel string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT update_source, update_channel FROM tenants WHERE id = $1
	`, tenantID).Scan(&source, &channel)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}

	cmdPayload, _ := json.Marshal(map[string]interface{}{
		"type":    "update",
		"action":  "check",
		"channel": channel,
		"source":  source,
	})

	subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, deviceID)
	if err := s.nats.Publish(subject, cmdPayload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "update_triggered"})
}

func (s *APIServer) handleDeviceUpdateAll(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")

	var source, channel string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT update_source, update_channel FROM tenants WHERE id = $1
	`, tenantID).Scan(&source, &channel)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `SELECT id FROM devices WHERE tenant_id = $1 AND status = 'online'`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	count := 0
	cmdPayload, _ := json.Marshal(map[string]interface{}{
		"type":    "update",
		"action":  "check",
		"channel": channel,
		"source":  source,
	})

	for rows.Next() {
		var deviceID string
		rows.Scan(&deviceID)
		subject := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, deviceID)
		s.nats.Publish(subject, cmdPayload)
		count++
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "update_triggered",
		"count":  fmt.Sprintf("%d", count),
	})
}

func (s *APIServer) handleSetUpdateSource(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	var req struct {
		Source  string `json:"update_source"`
		Channel string `json:"update_channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Source != "github" && req.Source != "server" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source must be github or server"})
		return
	}
	if req.Channel != "stable" && req.Channel != "beta" && req.Channel != "alpha" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel must be stable, beta, or alpha"})
		return
	}

	_, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE tenants SET update_source = $1, update_channel = $2 WHERE id = $3
	`, req.Source, req.Channel, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *APIServer) handleDeviceVersion(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, hostname, os, agent_version, status, last_heartbeat
		FROM devices WHERE tenant_id = $1
		ORDER BY hostname ASC
	`, tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var id, hostname, os, agentVersion, status string
		var lastHeartbeat sql.NullTime
		if err := rows.Scan(&id, &hostname, &os, &agentVersion, &status, &lastHeartbeat); err != nil {
			continue
		}
		d := map[string]interface{}{
			"id": id, "hostname": hostname, "os": os,
			"agent_version": agentVersion, "status": status,
		}
		if lastHeartbeat.Valid {
			d["last_heartbeat"] = lastHeartbeat.Time
		}
		devices = append(devices, d)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
}

func generateDeploymentID(name string) string {
	b := make([]byte, 4)
	rand.Read(b)
	suffix := hex.EncodeToString(b)
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	for _, c := range []string{".", "_", "'", "\"", "/", "\\"} {
		slug = strings.ReplaceAll(slug, c, "")
	}
	if len(slug) > 20 {
		slug = slug[:20]
	}
	return fmt.Sprintf("%s-%s", slug, suffix)
}

func (s *APIServer) logAuditAuth(userID, email, ip string, success bool, details string) {
	if s.db == nil {
		return
	}
	db := s.db.DB()
	detailsJSON, _ := json.Marshal(map[string]string{"email": email, "detail": details})
	db.Exec(`INSERT INTO audit_auth (user_id, action, ip_address, success, details) VALUES ($1, $2, $3, $4, $5)`,
		nullIfEmpty(userID), "login", ip, success, string(detailsJSON))
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
