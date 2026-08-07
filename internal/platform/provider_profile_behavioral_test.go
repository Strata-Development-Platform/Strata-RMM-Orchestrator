package platform

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

func TestProviderProfilePatchEmpty(t *testing.T) {
	var patch providerProfilePatch
	assert.True(t, patch.empty())
}

func TestProviderProfilePatchNonEmptyDisplayName(t *testing.T) {
	name := "Acme"
	patch := providerProfilePatch{DisplayName: &name}
	assert.False(t, patch.empty())
}

func TestProviderProfilePatchNonEmptyLegalName(t *testing.T) {
	name := "Acme"
	patch := providerProfilePatch{LegalName: &name}
	assert.False(t, patch.empty())
}

func TestProviderProfilePatchApplyDisplayName(t *testing.T) {
	displayName := "Updated Provider"
	patch := providerProfilePatch{DisplayName: &displayName}

	original := postgres.ProviderBusinessProfileValues{
		DisplayName: "Old Provider",
		LegalName:   "Acme LLC",
	}

	applied := patch.apply(original)
	assert.Equal(t, "Updated Provider", applied.DisplayName)
	assert.Equal(t, "Acme LLC", applied.LegalName)
}

func TestProviderProfilePatchApplyEmail(t *testing.T) {
	email := "new@example.com"
	patch := providerProfilePatch{SupportEmail: &email}

	original := postgres.ProviderBusinessProfileValues{
		SupportEmail: "old@example.com",
	}

	applied := patch.apply(original)
	assert.Equal(t, "new@example.com", applied.SupportEmail)
}

func TestProviderProfilePatchApplyPhone(t *testing.T) {
	phone := "+18005551234"
	patch := providerProfilePatch{BusinessPhone: &phone}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "+18005551234", applied.BusinessPhone)
}

func TestProviderProfilePatchApplyWebsiteURL(t *testing.T) {
	url := "https://example.com"
	patch := providerProfilePatch{WebsiteURL: &url}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "https://example.com", applied.WebsiteURL)
}

func TestProviderProfilePatchApplyCountryCode(t *testing.T) {
	code := "DE"
	patch := providerProfilePatch{CountryCode: &code}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "DE", applied.CountryCode)
}

func TestProviderProfilePatchApplyDefaultCurrency(t *testing.T) {
	currency := "EUR"
	patch := providerProfilePatch{DefaultCurrency: &currency}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "EUR", applied.DefaultCurrency)
}

func TestProviderProfilePatchApplyDefaultTimezone(t *testing.T) {
	tz := "Europe/Berlin"
	patch := providerProfilePatch{DefaultTimezone: &tz}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "Europe/Berlin", applied.DefaultTimezone)
}

func TestProviderProfilePatchApplyDefaultLocale(t *testing.T) {
	locale := "de-DE"
	patch := providerProfilePatch{DefaultLocale: &locale}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "de-DE", applied.DefaultLocale)
}

func TestValidatePlainTextControlCharactersRejected(t *testing.T) {
	err := validatePlainText("field", "has\x00null", 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "control characters")
}

func TestValidatePlainTextMaxLengthRejected(t *testing.T) {
	err := validatePlainText("field", strings.Repeat("x", 101), 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "100 characters or fewer")
}

func TestValidatePlainTextPassesShortValue(t *testing.T) {
	err := validatePlainText("field", "short", 100)
	assert.NoError(t, err)
}

func TestNormalizeProfileTextRequiredMissingRejected(t *testing.T) {
	_, err := normalizeProfileText("legal_name", "", 200, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is required")
}

func TestNormalizeProfileTextOptionalEmptyAccepted(t *testing.T) {
	got, err := normalizeProfileText("address_line2", "", 200, false)
	assert.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestNormalizeProfileTextTrimsWhitespace(t *testing.T) {
	got, err := normalizeProfileText("legal_name", "  Acme  ", 200, true)
	assert.NoError(t, err)
	assert.Equal(t, "Acme", got)
}

func TestNormalizeProfileEmailLowercases(t *testing.T) {
	got, err := normalizeProfileEmail("support_email", "  SUPPORT@EXAMPLE.COM ")
	assert.NoError(t, err)
	assert.Equal(t, "support@example.com", got)
}

func TestNormalizeProfileEmailRejectsNameBeforeAddress(t *testing.T) {
	_, err := normalizeProfileEmail("support_email", "Support <support@example.com>")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "valid email")
}

func TestNormalizeProfileEmailRejectsInvalidEmail(t *testing.T) {
	_, err := normalizeProfileEmail("support_email", "not-an-email")
	assert.Error(t, err)
}

func TestNormalizeProfilePhoneRejectsTooFewDigits(t *testing.T) {
	_, err := normalizeProfilePhone("12345")
	assert.Error(t, err)
}

func TestNormalizeProfilePhoneAcceptsValidPhone(t *testing.T) {
	got, err := normalizeProfilePhone("+14155551234")
	assert.NoError(t, err)
	assert.Equal(t, "+14155551234", got)
}

func TestNormalizeProfilePhoneTrimsWhitespace(t *testing.T) {
	got, err := normalizeProfilePhone(" +14155551234 ")
	assert.NoError(t, err)
	assert.Equal(t, "+14155551234", got)
}

func TestNormalizeProfileURLRejectsEmptyHost(t *testing.T) {
	_, err := normalizeProfileURL("about:", true)
	assert.Error(t, err)
}

func TestNormalizeProfileURLRejectsCredentials(t *testing.T) {
	_, err := normalizeProfileURL("https://user:pass@example.com", true)
	assert.Error(t, err)
}

func TestNormalizeProfileURLAcceptsHTTPS(t *testing.T) {
	got, err := normalizeProfileURL("https://example.com", true)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com", got)
}

func TestNormalizeProfileURLAcceptsHTTPInDevelopment(t *testing.T) {
	got, err := normalizeProfileURL("http://localhost:3000", false)
	assert.NoError(t, err)
	assert.Equal(t, "http://localhost:3000", got)
}

func TestNormalizeProfileURLRequiresHTTPSInProduction(t *testing.T) {
	_, err := normalizeProfileURL("http://example.com", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPS in production")
}

func TestNormalizeProfileURLTrimsWhitespace(t *testing.T) {
	got, err := normalizeProfileURL("  https://example.com  ", true)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com", got)
}

func TestProviderProfilePatchApplyTaxIdentifier(t *testing.T) {
	tax := "DE123456789"
	patch := providerProfilePatch{TaxIdentifier: &tax}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "DE123456789", applied.TaxIdentifier)
}

func TestProviderProfilePatchApplyAddressLine1(t *testing.T) {
	addr := "42 Market Street"
	patch := providerProfilePatch{AddressLine1: &addr}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "42 Market Street", applied.AddressLine1)
}

func TestProviderProfilePatchApplyCity(t *testing.T) {
	city := "Berlin"
	patch := providerProfilePatch{City: &city}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "Berlin", applied.City)
}

func TestProviderProfilePatchApplyPostalCode(t *testing.T) {
	code := "10115"
	patch := providerProfilePatch{PostalCode: &code}

	original := postgres.ProviderBusinessProfileValues{}

	applied := patch.apply(original)
	assert.Equal(t, "10115", applied.PostalCode)
}

func TestProviderProfilePatchMultipleFields(t *testing.T) {
	displayName := "Updated"
	email := "updated@example.com"

	patch := providerProfilePatch{
		DisplayName:  &displayName,
		SupportEmail: &email,
	}

	original := postgres.ProviderBusinessProfileValues{
		DisplayName: "Original",
		SupportEmail: "original@example.com",
	}

	applied := patch.apply(original)
	assert.Equal(t, "Updated", applied.DisplayName)
	assert.Equal(t, "updated@example.com", applied.SupportEmail)
}
