package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

func validProviderProfileValues() postgres.ProviderBusinessProfileValues {
	return postgres.ProviderBusinessProfileValues{
		LegalName:       "  Example Provider LLC  ",
		DisplayName:     " Example Provider ",
		ContactName:     " Ada Operator ",
		SupportEmail:    " SUPPORT@EXAMPLE.COM ",
		BillingEmail:    " BILLING@EXAMPLE.COM ",
		BusinessPhone:   " +1 (415) 555-0123 ",
		WebsiteURL:      "https://Example.COM/support",
		AddressLine1:    " 1 Main Street ",
		AddressLine2:    " Suite 200 ",
		City:            " San Francisco ",
		StateProvince:   " CA ",
		PostalCode:      " 94105 ",
		CountryCode:     " us ",
		DefaultTimezone: " America/Los_Angeles ",
		DefaultLocale:   " en-us ",
		DefaultCurrency: " usd ",
		TaxIdentifier:   " TAX-REFERENCE ",
	}
}

func TestNormalizeProviderProfile(t *testing.T) {
	got, err := normalizeProviderProfile(validProviderProfileValues(), true)
	if err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
	if got.LegalName != "Example Provider LLC" || got.DisplayName != "Example Provider" {
		t.Fatalf("names were not normalized: %+v", got)
	}
	if got.SupportEmail != "support@example.com" || got.BillingEmail != "billing@example.com" {
		t.Fatalf("emails were not normalized: %+v", got)
	}
	if got.CountryCode != "US" || got.DefaultCurrency != "USD" || got.DefaultLocale != "en-US" {
		t.Fatalf("ISO and locale fields were not normalized: %+v", got)
	}
	if got.WebsiteURL != "https://example.com/support" {
		t.Fatalf("website URL = %q", got.WebsiteURL)
	}
}

func TestNormalizeProviderProfileRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*postgres.ProviderBusinessProfileValues)
		wantErr string
	}{
		{name: "required legal name", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.LegalName = " " }, wantErr: "legal_name is required"},
		{name: "display name length", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.DisplayName = strings.Repeat("x", 101) }, wantErr: "display_name must be 100"},
		{name: "email", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.SupportEmail = "Display <support@example.com>" }, wantErr: "support_email must be a valid"},
		{name: "phone", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.BusinessPhone = "call-me" }, wantErr: "business_phone must be a valid"},
		{name: "URL credentials", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.WebsiteURL = "https://user:password@example.com" }, wantErr: "without credentials"},
		{name: "unsafe URL scheme", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.WebsiteURL = "javascript:alert(1)" }, wantErr: "absolute HTTP(S)"},
		{name: "encoded URL control", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.WebsiteURL = "https://example.com/%0A" }, wantErr: "invalid escaped"},
		{name: "country allowlist", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.CountryCode = "ZZ" }, wantErr: "ISO 3166"},
		{name: "currency allowlist", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.DefaultCurrency = "ZZZ" }, wantErr: "ISO 4217"},
		{name: "timezone", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.DefaultTimezone = "Mars/Olympus" }, wantErr: "valid IANA timezone"},
		{name: "locale", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.DefaultLocale = "not_a_locale" }, wantErr: "language or language-region"},
		{name: "control character", mutate: func(v *postgres.ProviderBusinessProfileValues) { v.AddressLine1 = "1 Main\nStreet" }, wantErr: "control characters"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := validProviderProfileValues()
			test.mutate(&values)
			_, err := normalizeProviderProfile(values, true)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestProviderWebsiteRequiresHTTPSOnlyInProduction(t *testing.T) {
	values := validProviderProfileValues()
	values.WebsiteURL = "http://localhost:3000"
	if _, err := normalizeProviderProfile(values, false); err != nil {
		t.Fatalf("development HTTP URL rejected: %v", err)
	}
	if _, err := normalizeProviderProfile(values, true); err == nil || !strings.Contains(err.Error(), "HTTPS in production") {
		t.Fatalf("production HTTP URL error = %v", err)
	}
}

func TestProviderProfileStrictJSONRejectsProtectedAndUnknownFields(t *testing.T) {
	for _, body := range []string{
		`{"setup_completed_at":"2026-01-01T00:00:00Z"}`,
		`{"platform_owner":"attacker"}`,
		`{"subscription_state":"active"}`,
		`{"display_name":"one"}{"display_name":"two"}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/v2/platform/provider/profile", strings.NewReader(body))
		var patch providerProfilePatch
		if err := decodeStrictJSON(req, &patch); err == nil {
			t.Fatalf("protected or malformed body accepted: %s", body)
		}
	}
}

func TestProviderHandlersRejectUnauthorizedAndScopedActors(t *testing.T) {
	validBody := `{"legal_name":"Example","display_name":"Example","contact_name":"Ada","support_email":"support@example.com","billing_email":"billing@example.com","business_phone":"+14155550123","address_line1":"1 Main","city":"City","postal_code":"12345","country_code":"US","default_timezone":"UTC","default_locale":"en-US","default_currency":"USD"}`
	tests := []struct {
		name   string
		roles  string
		userID string
		mspID  string
		want   int
	}{
		{name: "unauthenticated", want: http.StatusUnauthorized},
		{name: "MSP admin", userID: "user-1", roles: "msp_admin", want: http.StatusForbidden},
		{name: "technician", userID: "user-1", roles: "msp_technician", want: http.StatusForbidden},
		{name: "customer user", userID: "user-1", roles: "client_admin", want: http.StatusForbidden},
		{name: "cross-tenant platform owner", userID: "user-1", roles: "platform_owner", mspID: "msp-1", want: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v2/platform/provider/setup", strings.NewReader(validBody))
			ctx := req.Context()
			if test.userID != "" {
				ctx = context.WithValue(ctx, ctxKeyUserID, test.userID)
			}
			if test.roles != "" {
				ctx = context.WithValue(ctx, ctxKeyRole, test.roles)
			}
			if test.mspID != "" {
				ctx = context.WithValue(ctx, ctxKeyMSPID, test.mspID)
			}
			out := httptest.NewRecorder()
			(&APIServer{}).handleCompleteProviderSetup(out, req.WithContext(ctx))
			if out.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", out.Code, test.want, out.Body.String())
			}
		})
	}
}

func TestProviderRoutesAreFailClosedAdminRoutes(t *testing.T) {
	server := &APIServer{}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v2/platform/provider/profile"},
		{http.MethodPost, "/api/v2/platform/provider/setup"},
		{http.MethodPatch, "/api/v2/platform/provider/profile"},
	} {
		if got := server.classifyRoute(route.method, route.path); got != AccessAdmin {
			t.Fatalf("%s %s classified as %v", route.method, route.path, got)
		}
	}
}

func TestSetupStatusContextAllowsMembershipDerivedUserRoles(t *testing.T) {
	const secret = "provider-context-role-test-secret-at-least-32"
	generator := auth.NewTokenGenerator(secret)
	server := &APIServer{tokenGen: generator, allowClaimPrincipal: true}
	for _, role := range []string{"msp_technician", "msp_viewer", "client_admin", "client_viewer"} {
		t.Run(role, func(t *testing.T) {
			token, err := generator.GenerateUserToken("user-1", "tenant-1", "msp-1", "", "", []string{role}, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v2/context", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			out := httptest.NewRecorder()
			server.withAccessControl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(out, req)
			if out.Code != http.StatusOK {
				t.Fatalf("context access status = %d, want 200", out.Code)
			}
		})
	}
}
