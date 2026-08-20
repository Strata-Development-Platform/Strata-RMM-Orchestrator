package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"
)

const (
	SingletonPlatformID                 = "00000000-0000-0000-0000-000000000001"
	CurrentProviderSetupContractVersion = 2
)

var (
	ErrProviderSetupAlreadyCompleted = errors.New("provider setup is already complete with different values")
	ErrProviderSetupIncomplete       = errors.New("provider setup is incomplete")
)

// ProviderBusinessProfile contains only the singleton provider's editable
// business identity and its immutable setup-completion metadata. Secret-bearing
// delivery configuration is deliberately excluded from this browser-readable
// record.
type ProviderBusinessProfile struct {
	ID                    string     `json:"id"`
	Slug                  string     `json:"slug"`
	LegalName             string     `json:"legal_name"`
	DisplayName           string     `json:"display_name"`
	ContactName           string     `json:"contact_name"`
	SupportEmail          string     `json:"support_email"`
	BillingEmail          string     `json:"billing_email"`
	BusinessPhone         string     `json:"business_phone"`
	WebsiteURL            string     `json:"website_url,omitempty"`
	AddressLine1          string     `json:"address_line1"`
	AddressLine2          string     `json:"address_line2,omitempty"`
	City                  string     `json:"city"`
	StateProvince         string     `json:"state_province,omitempty"`
	PostalCode            string     `json:"postal_code"`
	CountryCode           string     `json:"country_code"`
	DefaultTimezone       string     `json:"default_timezone"`
	DefaultLocale         string     `json:"default_locale"`
	DefaultCurrency       string     `json:"default_currency"`
	TaxIdentifier         string     `json:"tax_identifier,omitempty"`
	LogoLightURL          string     `json:"logo_light_url,omitempty"`
	LogoDarkURL           string     `json:"logo_dark_url,omitempty"`
	FaviconURL            string     `json:"favicon_url,omitempty"`
	BrandLightColor       string     `json:"brand_light_color"`
	BrandDarkColor        string     `json:"brand_dark_color"`
	TermsURL              string     `json:"terms_url"`
	PrivacyURL            string     `json:"privacy_url"`
	SupportURL            string     `json:"support_url,omitempty"`
	PublicSaaSEnabled     bool       `json:"public_saas_enabled"`
	PublicSaaSHeadline    string     `json:"public_saas_headline,omitempty"`
	PublicSaaSDescription string     `json:"public_saas_description,omitempty"`
	SetupContractVersion  int        `json:"setup_contract_version"`
	SetupComplete         bool       `json:"setup_complete"`
	SetupCompletedAt      *time.Time `json:"setup_completed_at,omitempty"`
	SetupCompletedBy      string     `json:"setup_completed_by,omitempty"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// ProviderBusinessProfileValues is the complete set of editable non-secret
// values. API patch handlers merge partial input into a complete value before
// persistence.
type ProviderBusinessProfileValues struct {
	LegalName             string `json:"legal_name"`
	DisplayName           string `json:"display_name"`
	ContactName           string `json:"contact_name"`
	SupportEmail          string `json:"support_email"`
	BillingEmail          string `json:"billing_email"`
	BusinessPhone         string `json:"business_phone"`
	WebsiteURL            string `json:"website_url"`
	AddressLine1          string `json:"address_line1"`
	AddressLine2          string `json:"address_line2"`
	City                  string `json:"city"`
	StateProvince         string `json:"state_province"`
	PostalCode            string `json:"postal_code"`
	CountryCode           string `json:"country_code"`
	DefaultTimezone       string `json:"default_timezone"`
	DefaultLocale         string `json:"default_locale"`
	DefaultCurrency       string `json:"default_currency"`
	TaxIdentifier         string `json:"tax_identifier"`
	LogoLightURL          string `json:"logo_light_url"`
	LogoDarkURL           string `json:"logo_dark_url"`
	FaviconURL            string `json:"favicon_url"`
	BrandLightColor       string `json:"brand_light_color"`
	BrandDarkColor        string `json:"brand_dark_color"`
	TermsURL              string `json:"terms_url"`
	PrivacyURL            string `json:"privacy_url"`
	SupportURL            string `json:"support_url"`
	PublicSaaSEnabled     bool   `json:"public_saas_enabled"`
	PublicSaaSHeadline    string `json:"public_saas_headline"`
	PublicSaaSDescription string `json:"public_saas_description"`
}

type providerProfileQueryer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func (p ProviderBusinessProfile) Values() ProviderBusinessProfileValues {
	return ProviderBusinessProfileValues{
		LegalName: p.LegalName, DisplayName: p.DisplayName, ContactName: p.ContactName,
		SupportEmail: p.SupportEmail, BillingEmail: p.BillingEmail, BusinessPhone: p.BusinessPhone,
		WebsiteURL: p.WebsiteURL, AddressLine1: p.AddressLine1, AddressLine2: p.AddressLine2,
		City: p.City, StateProvince: p.StateProvince, PostalCode: p.PostalCode,
		CountryCode: p.CountryCode, DefaultTimezone: p.DefaultTimezone,
		DefaultLocale: p.DefaultLocale, DefaultCurrency: p.DefaultCurrency,
		TaxIdentifier: p.TaxIdentifier, LogoLightURL: p.LogoLightURL, LogoDarkURL: p.LogoDarkURL,
		FaviconURL: p.FaviconURL, BrandLightColor: p.BrandLightColor, BrandDarkColor: p.BrandDarkColor,
		TermsURL: p.TermsURL, PrivacyURL: p.PrivacyURL, SupportURL: p.SupportURL,
		PublicSaaSEnabled: p.PublicSaaSEnabled, PublicSaaSHeadline: p.PublicSaaSHeadline,
		PublicSaaSDescription: p.PublicSaaSDescription,
	}
}

func GetProviderBusinessProfile(ctx context.Context, db providerProfileQueryer) (ProviderBusinessProfile, error) {
	return getProviderBusinessProfile(ctx, db, false)
}

func LockProviderBusinessProfile(ctx context.Context, tx *sql.Tx) (ProviderBusinessProfile, error) {
	return getProviderBusinessProfile(ctx, tx, true)
}

func getProviderBusinessProfile(ctx context.Context, db providerProfileQueryer, lock bool) (ProviderBusinessProfile, error) {
	query := `
		SELECT id::text, slug, legal_name, display_name, contact_name,
		       support_email, billing_email, business_phone, website_url,
		       address_line1, address_line2, city, state_province, postal_code,
		       country_code, default_timezone, default_locale, default_currency,
		       tax_identifier, provider_logo_light_url, provider_logo_dark_url,
		       provider_favicon_url, provider_brand_light_color, provider_brand_dark_color,
		       provider_terms_url, provider_privacy_url, provider_support_url,
		       public_saas_enabled, public_saas_headline, public_saas_description,
		       setup_contract_version, setup_completed_at,
		       COALESCE(setup_completed_by::text, ''), updated_at
		FROM platforms
		WHERE id = $1
	`
	if lock {
		query += " FOR UPDATE"
	}

	var profile ProviderBusinessProfile
	var completedAt sql.NullTime
	err := db.QueryRowContext(ctx, query, SingletonPlatformID).Scan(
		&profile.ID, &profile.Slug, &profile.LegalName, &profile.DisplayName,
		&profile.ContactName, &profile.SupportEmail, &profile.BillingEmail,
		&profile.BusinessPhone, &profile.WebsiteURL, &profile.AddressLine1,
		&profile.AddressLine2, &profile.City, &profile.StateProvince,
		&profile.PostalCode, &profile.CountryCode, &profile.DefaultTimezone,
		&profile.DefaultLocale, &profile.DefaultCurrency, &profile.TaxIdentifier,
		&profile.LogoLightURL, &profile.LogoDarkURL, &profile.FaviconURL,
		&profile.BrandLightColor, &profile.BrandDarkColor, &profile.TermsURL,
		&profile.PrivacyURL, &profile.SupportURL, &profile.PublicSaaSEnabled,
		&profile.PublicSaaSHeadline, &profile.PublicSaaSDescription,
		&profile.SetupContractVersion, &completedAt, &profile.SetupCompletedBy, &profile.UpdatedAt,
	)
	if err != nil {
		return ProviderBusinessProfile{}, fmt.Errorf("read provider business profile: %w", err)
	}
	if completedAt.Valid {
		completed := completedAt.Time.UTC()
		profile.SetupCompletedAt = &completed
	}
	profile.SetupComplete = completedAt.Valid && profile.SetupContractVersion >= CurrentProviderSetupContractVersion
	profile.UpdatedAt = profile.UpdatedAt.UTC()
	return profile, nil
}

func UserCanManageProvider(ctx context.Context, db providerProfileQueryer, userID string) (bool, error) {
	var allowed bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM memberships
			WHERE user_id = $1
			  AND scope_type = 'platform'
			  AND scope_id = $2
			  AND role IN ('platform_owner', 'platform_admin')
			  AND status = 'active'
			  AND (expires_at IS NULL OR expires_at > NOW())
		)
	`, userID, SingletonPlatformID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("verify provider administrator membership: %w", err)
	}
	return allowed, nil
}

func CompleteProviderSetup(ctx context.Context, tx *sql.Tx, actorUserID string, values ProviderBusinessProfileValues) (ProviderBusinessProfile, bool, error) {
	current, err := getProviderBusinessProfile(ctx, tx, true)
	if err != nil {
		return ProviderBusinessProfile{}, false, err
	}
	if current.SetupComplete {
		if reflect.DeepEqual(current.Values(), values) {
			return current, false, nil
		}
		return ProviderBusinessProfile{}, false, ErrProviderSetupAlreadyCompleted
	}

	if _, err := tx.ExecContext(ctx, providerProfileUpdateSQL(true), providerProfileUpdateArgs(values, actorUserID, true)...); err != nil {
		return ProviderBusinessProfile{}, false, fmt.Errorf("complete provider business profile: %w", err)
	}
	if err := insertProviderAudit(ctx, tx, actorUserID, "provider.setup_completed", map[string]interface{}{
		"profile_schema_version": CurrentProviderSetupContractVersion,
	}); err != nil {
		return ProviderBusinessProfile{}, false, err
	}
	profile, err := getProviderBusinessProfile(ctx, tx, false)
	if err != nil {
		return ProviderBusinessProfile{}, false, err
	}
	return profile, true, nil
}

func UpdateProviderBusinessProfile(ctx context.Context, tx *sql.Tx, actorUserID string, values ProviderBusinessProfileValues) (ProviderBusinessProfile, bool, error) {
	current, err := getProviderBusinessProfile(ctx, tx, true)
	if err != nil {
		return ProviderBusinessProfile{}, false, err
	}
	if !current.SetupComplete {
		return ProviderBusinessProfile{}, false, ErrProviderSetupIncomplete
	}
	changedFields := changedProviderFields(current.Values(), values)
	if len(changedFields) == 0 {
		return current, false, nil
	}

	if _, err := tx.ExecContext(ctx, providerProfileUpdateSQL(false), providerProfileUpdateArgs(values, actorUserID, false)...); err != nil {
		return ProviderBusinessProfile{}, false, fmt.Errorf("update provider business profile: %w", err)
	}
	if err := insertProviderAudit(ctx, tx, actorUserID, "provider.profile_updated", map[string]interface{}{
		"changed_fields": changedFields,
	}); err != nil {
		return ProviderBusinessProfile{}, false, err
	}
	profile, err := getProviderBusinessProfile(ctx, tx, false)
	if err != nil {
		return ProviderBusinessProfile{}, false, err
	}
	return profile, true, nil
}

func providerProfileUpdateSQL(completing bool) string {
	completion := ""
	if completing {
		completion = ", setup_completed_at = NOW(), setup_completed_by = $29::uuid, setup_contract_version = $30"
	}
	return `
		UPDATE platforms SET
			legal_name = $1, display_name = $2, contact_name = $3,
			support_email = $4, billing_email = $5, business_phone = $6,
			website_url = $7, address_line1 = $8, address_line2 = $9,
			city = $10, state_province = $11, postal_code = $12,
			country_code = $13, default_timezone = $14, default_locale = $15,
			default_currency = $16, tax_identifier = $17,
			provider_logo_light_url = $18, provider_logo_dark_url = $19,
			provider_favicon_url = $20, provider_brand_light_color = $21,
			provider_brand_dark_color = $22, provider_terms_url = $23,
			provider_privacy_url = $24, provider_support_url = $25,
			public_saas_enabled = $26, public_saas_headline = $27,
			public_saas_description = $28, updated_at = NOW()` + completion + `
		WHERE id = '` + SingletonPlatformID + `'::uuid
	`
}

func providerProfileUpdateArgs(values ProviderBusinessProfileValues, actorUserID string, completing bool) []interface{} {
	args := []interface{}{
		values.LegalName, values.DisplayName, values.ContactName,
		values.SupportEmail, values.BillingEmail, values.BusinessPhone,
		values.WebsiteURL, values.AddressLine1, values.AddressLine2,
		values.City, values.StateProvince, values.PostalCode,
		values.CountryCode, values.DefaultTimezone, values.DefaultLocale,
		values.DefaultCurrency, values.TaxIdentifier, values.LogoLightURL,
		values.LogoDarkURL, values.FaviconURL, values.BrandLightColor,
		values.BrandDarkColor, values.TermsURL, values.PrivacyURL, values.SupportURL,
		values.PublicSaaSEnabled, values.PublicSaaSHeadline, values.PublicSaaSDescription,
	}
	if completing {
		args = append(args, actorUserID, CurrentProviderSetupContractVersion)
	}
	return args
}

func changedProviderFields(before, after ProviderBusinessProfileValues) []string {
	fields := make([]string, 0)
	beforeValue := reflect.ValueOf(before)
	afterValue := reflect.ValueOf(after)
	typeInfo := beforeValue.Type()
	for i := 0; i < beforeValue.NumField(); i++ {
		if !reflect.DeepEqual(beforeValue.Field(i).Interface(), afterValue.Field(i).Interface()) {
			fields = append(fields, typeInfo.Field(i).Tag.Get("json"))
		}
	}
	sort.Strings(fields)
	return fields
}

func insertProviderAudit(ctx context.Context, tx *sql.Tx, actorUserID, action string, details interface{}) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode provider audit details: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO control_plane_audit (
			msp_id, actor_user_id, action, resource_type, resource_id, details
		)
		VALUES (NULL, $1, $2, 'platform', $3, $4)
	`, actorUserID, action, SingletonPlatformID, payload); err != nil {
		return fmt.Errorf("record provider audit event: %w", err)
	}
	return nil
}
