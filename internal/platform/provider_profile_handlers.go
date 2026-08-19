package platform

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

var (
	phonePattern    = regexp.MustCompile(`^[+0-9][0-9 ().xX+-]*$`)
	localePattern   = regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z]{2}|-[0-9]{3})?$`)
	hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	isoCountries    = stringSet(`
		AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ
		CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR
		GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO
		JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR
		MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO
		RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV
		TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW
	`)
	isoCurrencies = stringSet(`
		AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYN BZD CAD CDF
		CHE CHF CHW CLF CLP CNY COP COU CRC CUC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD
		GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP
		LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB
		PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SLL SOS SRD SSP STN SVC SYP SZL THB
		TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD USN UYI UYU UYW UZS VED VES VND VUV WST XAF XAG XAU XBA XBB XBC
		XBD XCD XDR XOF XPD XPF XPT XSU XUA YER ZAR ZMW ZWG
	`)
)

type providerProfilePatch struct {
	LegalName             *string `json:"legal_name"`
	DisplayName           *string `json:"display_name"`
	ContactName           *string `json:"contact_name"`
	SupportEmail          *string `json:"support_email"`
	BillingEmail          *string `json:"billing_email"`
	BusinessPhone         *string `json:"business_phone"`
	WebsiteURL            *string `json:"website_url"`
	AddressLine1          *string `json:"address_line1"`
	AddressLine2          *string `json:"address_line2"`
	City                  *string `json:"city"`
	StateProvince         *string `json:"state_province"`
	PostalCode            *string `json:"postal_code"`
	CountryCode           *string `json:"country_code"`
	DefaultTimezone       *string `json:"default_timezone"`
	DefaultLocale         *string `json:"default_locale"`
	DefaultCurrency       *string `json:"default_currency"`
	TaxIdentifier         *string `json:"tax_identifier"`
	LogoLightURL          *string `json:"logo_light_url"`
	LogoDarkURL           *string `json:"logo_dark_url"`
	FaviconURL            *string `json:"favicon_url"`
	BrandLightColor       *string `json:"brand_light_color"`
	BrandDarkColor        *string `json:"brand_dark_color"`
	TermsURL              *string `json:"terms_url"`
	PrivacyURL            *string `json:"privacy_url"`
	SupportURL            *string `json:"support_url"`
	PublicSaaSEnabled     *bool   `json:"public_saas_enabled"`
	PublicSaaSHeadline    *string `json:"public_saas_headline"`
	PublicSaaSDescription *string `json:"public_saas_description"`
}

type providerProfileResponse struct {
	postgres.ProviderBusinessProfile
	OutboundEmailStatus string `json:"outbound_email_status"`
}

func stringSet(values string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, value := range strings.Fields(values) {
		set[value] = struct{}{}
	}
	return set
}

func (s *APIServer) providerProfileResponse(profile postgres.ProviderBusinessProfile) providerProfileResponse {
	status := "not_configured"
	if s.accountMailer != nil {
		status = "configured"
	}
	return providerProfileResponse{ProviderBusinessProfile: profile, OutboundEmailStatus: status}
}

func (s *APIServer) authorizeProviderProfile(w http.ResponseWriter, r *http.Request) bool {
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session required"})
		return false
	}
	if !authorizationFromRequest(r).IsPlatformGlobal() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "platform owner or administrator required"})
		return false
	}
	for _, key := range []contextKey{ctxKeyMSPID, ctxKeyClientID, ctxKeySiteID} {
		if scopeID, _ := r.Context().Value(key).(string); scopeID != "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "top-level platform context required"})
			return false
		}
	}
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "identity database unavailable"})
		return false
	}
	allowed, err := postgres.UserCanManageProvider(r.Context(), s.requestDB(r), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "provider authorization unavailable"})
		return false
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "platform owner or administrator required"})
		return false
	}
	return true
}

func (s *APIServer) handleGetProviderProfile(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeProviderProfile(w, r) {
		return
	}
	profile, err := postgres.GetProviderBusinessProfile(r.Context(), s.requestDB(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "provider profile unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, s.providerProfileResponse(profile))
}

func (s *APIServer) handleCompleteProviderSetup(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeProviderProfile(w, r) {
		return
	}
	var values postgres.ProviderBusinessProfileValues
	if err := decodeStrictJSON(r, &values); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	values, err := normalizeProviderProfile(values, s.requireHTTPSWebsite)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tx, ok := requestTransaction(r)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transactional provider setup unavailable"})
		return
	}
	actor, _ := r.Context().Value(ctxKeyUserID).(string)
	profile, created, err := postgres.CompleteProviderSetup(r.Context(), tx, actor, values)
	if errors.Is(err, postgres.ErrProviderSetupAlreadyCompleted) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "provider setup is already complete; use profile update"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "provider setup failed"})
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, s.providerProfileResponse(profile))
}

func (s *APIServer) handleUpdateProviderProfile(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeProviderProfile(w, r) {
		return
	}
	var patch providerProfilePatch
	if err := decodeStrictJSON(r, &patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if patch.empty() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one editable profile field is required"})
		return
	}
	tx, ok := requestTransaction(r)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "transactional provider update unavailable"})
		return
	}
	current, err := postgres.LockProviderBusinessProfile(r.Context(), tx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "provider profile unavailable"})
		return
	}
	values, err := normalizeProviderProfile(patch.apply(current.Values()), s.requireHTTPSWebsite)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	actor, _ := r.Context().Value(ctxKeyUserID).(string)
	profile, _, err := postgres.UpdateProviderBusinessProfile(r.Context(), tx, actor, values)
	if errors.Is(err, postgres.ErrProviderSetupIncomplete) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "provider setup must be completed first"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "provider profile update failed"})
		return
	}
	writeJSON(w, http.StatusOK, s.providerProfileResponse(profile))
}

func requestTransaction(r *http.Request) (*sql.Tx, bool) {
	tx, ok := r.Context().Value(ctxKeyDBTransaction).(*sql.Tx)
	return tx, ok && tx != nil
}

func decodeStrictJSON(r *http.Request, destination interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func normalizeProviderProfile(values postgres.ProviderBusinessProfileValues, requireHTTPS bool) (postgres.ProviderBusinessProfileValues, error) {
	var err error
	if values.LegalName, err = normalizeProfileText("legal_name", values.LegalName, 200, true); err != nil {
		return values, err
	}
	if values.DisplayName, err = normalizeProfileText("display_name", values.DisplayName, 100, true); err != nil {
		return values, err
	}
	if values.ContactName, err = normalizeProfileText("contact_name", values.ContactName, 150, true); err != nil {
		return values, err
	}
	if values.SupportEmail, err = normalizeProfileEmail("support_email", values.SupportEmail); err != nil {
		return values, err
	}
	if values.BillingEmail, err = normalizeProfileEmail("billing_email", values.BillingEmail); err != nil {
		return values, err
	}
	if values.BusinessPhone, err = normalizeProfilePhone(values.BusinessPhone); err != nil {
		return values, err
	}
	if values.WebsiteURL, err = normalizeProfileURLField("website_url", values.WebsiteURL, requireHTTPS, false); err != nil {
		return values, err
	}

	fields := []struct {
		name     string
		value    *string
		max      int
		required bool
	}{
		{"address_line1", &values.AddressLine1, 200, true}, {"address_line2", &values.AddressLine2, 200, false},
		{"city", &values.City, 100, true}, {"state_province", &values.StateProvince, 100, false},
		{"postal_code", &values.PostalCode, 32, true}, {"tax_identifier", &values.TaxIdentifier, 100, false},
		{"public_saas_headline", &values.PublicSaaSHeadline, 160, false},
		{"public_saas_description", &values.PublicSaaSDescription, 2000, false},
	}
	for _, field := range fields {
		*field.value, err = normalizeProfileText(field.name, *field.value, field.max, field.required)
		if err != nil {
			return values, err
		}
	}

	values.CountryCode = strings.ToUpper(strings.TrimSpace(values.CountryCode))
	if _, ok := isoCountries[values.CountryCode]; !ok {
		return values, fmt.Errorf("country_code must be a supported ISO 3166-1 alpha-2 code")
	}
	values.DefaultCurrency = strings.ToUpper(strings.TrimSpace(values.DefaultCurrency))
	if _, ok := isoCurrencies[values.DefaultCurrency]; !ok {
		return values, fmt.Errorf("default_currency must be a supported ISO 4217 code")
	}
	values.DefaultTimezone = strings.TrimSpace(values.DefaultTimezone)
	if err := validatePlainText("default_timezone", values.DefaultTimezone, 64); err != nil {
		return values, err
	}
	if values.DefaultTimezone == "" {
		return values, fmt.Errorf("default_timezone is required")
	}
	if _, err := time.LoadLocation(values.DefaultTimezone); err != nil || values.DefaultTimezone == "Local" {
		return values, fmt.Errorf("default_timezone must be a valid IANA timezone")
	}
	values.DefaultLocale = strings.TrimSpace(values.DefaultLocale)
	if !localePattern.MatchString(values.DefaultLocale) {
		return values, fmt.Errorf("default_locale must be a valid language or language-region tag")
	}
	localeParts := strings.Split(values.DefaultLocale, "-")
	localeParts[0] = strings.ToLower(localeParts[0])
	if len(localeParts) == 2 && len(localeParts[1]) == 2 {
		localeParts[1] = strings.ToUpper(localeParts[1])
	}
	values.DefaultLocale = strings.Join(localeParts, "-")

	values.BrandLightColor = strings.ToUpper(strings.TrimSpace(values.BrandLightColor))
	if !hexColorPattern.MatchString(values.BrandLightColor) {
		return values, fmt.Errorf("brand_light_color must be a #RRGGBB color")
	}
	values.BrandDarkColor = strings.ToUpper(strings.TrimSpace(values.BrandDarkColor))
	if !hexColorPattern.MatchString(values.BrandDarkColor) {
		return values, fmt.Errorf("brand_dark_color must be a #RRGGBB color")
	}

	for _, target := range []struct {
		name     string
		value    *string
		required bool
	}{
		{"logo_light_url", &values.LogoLightURL, false}, {"logo_dark_url", &values.LogoDarkURL, false},
		{"favicon_url", &values.FaviconURL, false}, {"terms_url", &values.TermsURL, true},
		{"privacy_url", &values.PrivacyURL, true}, {"support_url", &values.SupportURL, false},
	} {
		*target.value, err = normalizeProfileURLField(target.name, *target.value, true, target.required)
		if err != nil {
			return values, err
		}
	}
	return values, nil
}

func normalizeProfileText(field, value string, max int, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if err := validatePlainText(field, value, max); err != nil {
		return "", err
	}
	return value, nil
}

func validatePlainText(field, value string, max int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%s must be %d characters or fewer", field, max)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s must not contain control characters", field)
		}
	}
	return nil
}

func normalizeProfileEmail(field, value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if err := validatePlainText(field, value, 254); err != nil {
		return "", err
	}
	address, err := mail.ParseAddress(value)
	if value == "" || err != nil || !strings.EqualFold(address.Address, value) {
		return "", fmt.Errorf("%s must be a valid email address", field)
	}
	return value, nil
}

func normalizeProfilePhone(value string) (string, error) {
	value = strings.TrimSpace(value)
	if err := validatePlainText("business_phone", value, 32); err != nil {
		return "", err
	}
	digits := 0
	for _, character := range value {
		if character >= '0' && character <= '9' {
			digits++
		}
	}
	if !phonePattern.MatchString(value) || digits < 7 {
		return "", fmt.Errorf("business_phone must be a valid phone number")
	}
	return value, nil
}

func normalizeProfileURLField(field, value string, requireHTTPS, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return "", fmt.Errorf("%s is required", field)
		}
		return "", nil
	}
	if err := validatePlainText(field, value, 2048); err != nil {
		return "", err
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.Opaque != "" || parsed.User != nil {
		return "", fmt.Errorf("%s must be an absolute HTTP(S) URL without credentials", field)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s must use HTTP(S)", field)
	}
	if requireHTTPS && parsed.Scheme != "https" {
		if field == "website_url" {
			return "", fmt.Errorf("website_url must use HTTPS in production")
		}
		return "", fmt.Errorf("%s must use HTTPS", field)
	}
	parsed.Host = strings.ToLower(parsed.Host)
	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil || validatePlainText(field, decodedPath, 2048) != nil {
		return "", fmt.Errorf("%s contains invalid escaped characters", field)
	}
	decodedQuery, err := url.QueryUnescape(parsed.RawQuery)
	if err != nil || validatePlainText(field, decodedQuery, 2048) != nil {
		return "", fmt.Errorf("%s contains invalid escaped characters", field)
	}
	return parsed.String(), nil
}

func normalizeProfileURL(value string, requireHTTPS bool) (string, error) {
	return normalizeProfileURLField("website_url", value, requireHTTPS, false)
}

func (patch providerProfilePatch) empty() bool {
	return patch.LegalName == nil && patch.DisplayName == nil && patch.ContactName == nil && patch.SupportEmail == nil &&
		patch.BillingEmail == nil && patch.BusinessPhone == nil && patch.WebsiteURL == nil && patch.AddressLine1 == nil &&
		patch.AddressLine2 == nil && patch.City == nil && patch.StateProvince == nil && patch.PostalCode == nil &&
		patch.CountryCode == nil && patch.DefaultTimezone == nil && patch.DefaultLocale == nil && patch.DefaultCurrency == nil &&
		patch.TaxIdentifier == nil && patch.LogoLightURL == nil && patch.LogoDarkURL == nil && patch.FaviconURL == nil &&
		patch.BrandLightColor == nil && patch.BrandDarkColor == nil && patch.TermsURL == nil && patch.PrivacyURL == nil &&
		patch.SupportURL == nil && patch.PublicSaaSEnabled == nil && patch.PublicSaaSHeadline == nil && patch.PublicSaaSDescription == nil
}

func (patch providerProfilePatch) apply(values postgres.ProviderBusinessProfileValues) postgres.ProviderBusinessProfileValues {
	assignments := []struct {
		input  *string
		target *string
	}{
		{patch.LegalName, &values.LegalName}, {patch.DisplayName, &values.DisplayName}, {patch.ContactName, &values.ContactName},
		{patch.SupportEmail, &values.SupportEmail}, {patch.BillingEmail, &values.BillingEmail}, {patch.BusinessPhone, &values.BusinessPhone},
		{patch.WebsiteURL, &values.WebsiteURL}, {patch.AddressLine1, &values.AddressLine1}, {patch.AddressLine2, &values.AddressLine2},
		{patch.City, &values.City}, {patch.StateProvince, &values.StateProvince}, {patch.PostalCode, &values.PostalCode},
		{patch.CountryCode, &values.CountryCode}, {patch.DefaultTimezone, &values.DefaultTimezone}, {patch.DefaultLocale, &values.DefaultLocale},
		{patch.DefaultCurrency, &values.DefaultCurrency}, {patch.TaxIdentifier, &values.TaxIdentifier}, {patch.LogoLightURL, &values.LogoLightURL},
		{patch.LogoDarkURL, &values.LogoDarkURL}, {patch.FaviconURL, &values.FaviconURL}, {patch.BrandLightColor, &values.BrandLightColor},
		{patch.BrandDarkColor, &values.BrandDarkColor}, {patch.TermsURL, &values.TermsURL}, {patch.PrivacyURL, &values.PrivacyURL},
		{patch.SupportURL, &values.SupportURL}, {patch.PublicSaaSHeadline, &values.PublicSaaSHeadline}, {patch.PublicSaaSDescription, &values.PublicSaaSDescription},
	}
	for _, assignment := range assignments {
		if assignment.input != nil {
			*assignment.target = *assignment.input
		}
	}
	if patch.PublicSaaSEnabled != nil {
		values.PublicSaaSEnabled = *patch.PublicSaaSEnabled
	}
	return values
}
