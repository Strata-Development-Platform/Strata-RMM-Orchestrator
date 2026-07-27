package platform

import (
	"encoding/json"
	"net/http"
)

type brandingProfile struct {
	ID           string `json:"id"`
	MSPID        string `json:"msp_id"`
	DisplayName  string `json:"display_name"`
	LogoLight    string `json:"logo_light,omitempty"`
	LogoDark     string `json:"logo_dark,omitempty"`
	Favicon      string `json:"favicon,omitempty"`
	PrimaryColor string `json:"primary_color"`
	AccentColor  string `json:"accent_color"`
	SidebarBg    string `json:"sidebar_bg"`
	HeaderBg     string `json:"header_bg"`
	LoginBg      string `json:"login_bg"`
	PortalTitle  string `json:"portal_title"`
	WelcomeText  string `json:"welcome_text"`
	SupportEmail string `json:"support_email,omitempty"`
	TermsURL     string `json:"terms_url,omitempty"`
	PrivacyURL   string `json:"privacy_url,omitempty"`
}

func (s *APIServer) handleGetBranding(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if mspID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no msp context"})
		return
	}

	var bp brandingProfile
	err := s.db.DB().QueryRowContext(r.Context(), `
		SELECT id, msp_id, display_name, COALESCE(logo_light, ''),
		       COALESCE(logo_dark, ''), COALESCE(favicon, ''),
		       primary_color, accent_color, sidebar_bg, header_bg,
		       login_bg, portal_title, welcome_text,
		       COALESCE(support_email, ''), COALESCE(terms_url, ''), COALESCE(privacy_url, '')
		FROM branding_profiles WHERE msp_id = $1
	`, mspID).Scan(&bp.ID, &bp.MSPID, &bp.DisplayName, &bp.LogoLight,
		&bp.LogoDark, &bp.Favicon,
		&bp.PrimaryColor, &bp.AccentColor, &bp.SidebarBg, &bp.HeaderBg,
		&bp.LoginBg, &bp.PortalTitle, &bp.WelcomeText,
		&bp.SupportEmail, &bp.TermsURL, &bp.PrivacyURL)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "branding not found"})
		return
	}
	writeJSON(w, http.StatusOK, bp)
}

func (s *APIServer) handleUpdateBranding(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}

	var req brandingProfile
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	_, err := s.db.DB().ExecContext(r.Context(), `
		UPDATE branding_profiles SET
			display_name = CASE WHEN $2 = '' THEN display_name ELSE $2 END,
			logo_light = CASE WHEN $3 = '' THEN logo_light ELSE $3 END,
			logo_dark = CASE WHEN $4 = '' THEN logo_dark ELSE $4 END,
			favicon = CASE WHEN $5 = '' THEN favicon ELSE $5 END,
			primary_color = CASE WHEN $6 = '' THEN primary_color ELSE $6 END,
			accent_color = CASE WHEN $7 = '' THEN accent_color ELSE $7 END,
			sidebar_bg = CASE WHEN $8 = '' THEN sidebar_bg ELSE $8 END,
			header_bg = CASE WHEN $9 = '' THEN header_bg ELSE $9 END,
			login_bg = CASE WHEN $10 = '' THEN login_bg ELSE $10 END,
			portal_title = CASE WHEN $11 = '' THEN portal_title ELSE $11 END,
			welcome_text = CASE WHEN $12 = '' THEN welcome_text ELSE $12 END,
			support_email = CASE WHEN $13 = '' THEN support_email ELSE $13 END,
			terms_url = CASE WHEN $14 = '' THEN terms_url ELSE $14 END,
			privacy_url = CASE WHEN $15 = '' THEN privacy_url ELSE $15 END,
			updated_at = NOW()
		WHERE msp_id = $1
	`, mspID, req.DisplayName, req.LogoLight, req.LogoDark, req.Favicon,
		req.PrimaryColor, req.AccentColor, req.SidebarBg, req.HeaderBg,
		req.LoginBg, req.PortalTitle, req.WelcomeText,
		req.SupportEmail, req.TermsURL, req.PrivacyURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *APIServer) handleListDomains(w http.ResponseWriter, r *http.Request) {
	mspID := r.Header.Get("X-MSP-ID")
	if mspID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no msp context"})
		return
	}

	rows, err := s.db.DB().QueryContext(r.Context(), `
		SELECT id, hostname, domain_type, verification_status, certificate_status, is_primary
		FROM custom_domains WHERE msp_id = $1 ORDER BY created_at ASC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var domains []map[string]interface{}
	for rows.Next() {
		var id, hostname, domainType, verStatus, certStatus string
		var isPrimary bool
		if err := rows.Scan(&id, &hostname, &domainType, &verStatus, &certStatus, &isPrimary); err != nil {
			continue
		}
		domains = append(domains, map[string]interface{}{
			"id": id, "hostname": hostname, "domain_type": domainType,
			"verification_status": verStatus, "certificate_status": certStatus,
			"is_primary": isPrimary,
		})
	}
	if domains == nil {
		domains = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"domains": domains})
}
