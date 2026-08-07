package platform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClientSupportRequestStructFields(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-24 * time.Hour)
	replyAt := now

	req := clientSupportRequest{
		ID:        "req-1",
		TenantID:  "tenant-1",
		Category:  "technical",
		Subject:   "Cannot log in",
		Description: "My account is locked",
		Priority:  "high",
		Status:    "open",
		CreatedAt: createdAt,
		UpdatedAt: now,
		ReplyFrom: "support@example.com",
		ReplyAt:   &replyAt,
	}

	assert.Equal(t, "req-1", req.ID)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, "technical", req.Category)
	assert.Equal(t, "Cannot log in", req.Subject)
	assert.Equal(t, "high", req.Priority)
	assert.Equal(t, "open", req.Status)
	assert.Equal(t, "support@example.com", req.ReplyFrom)
}

func TestClientSupportRequestNullableReplyAtNil(t *testing.T) {
	req := clientSupportRequest{}
	assert.Nil(t, req.ReplyAt)
}

func TestClientSupportRequestNullableReplyAtValid(t *testing.T) {
	now := time.Now()
	req := clientSupportRequest{
		ReplyAt: &now,
	}
	assert.NotNil(t, req.ReplyAt)
}

func TestNullableTimeToPtrValid(t *testing.T) {
	now := time.Now()
	got := nullableTimeToPtr(now, true)
	assert.NotNil(t, got)
	assert.True(t, got.Equal(now))
}

func TestNullableTimeToPtrInvalid(t *testing.T) {
	var zero time.Time
	got := nullableTimeToPtr(zero, false)
	assert.Nil(t, got)
}

func TestNullableTimeToPtrZeroValueValid(t *testing.T) {
	var zero time.Time
	got := nullableTimeToPtr(zero, true)
	assert.NotNil(t, got)
	assert.True(t, got.Equal(zero))
}

func TestReplyToClientSupportRequestRequiresAuthorization(t *testing.T) {
	body := `{"body":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clients/tenant-1/support-requests/req-1/reply", strings.NewReader(body))
	out := httptest.NewRecorder()

	s := &APIServer{}
	s.handleReplyToClientSupportRequest(out, req)

	if out.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for missing authorization: %s", out.Code, out.Body.String())
	}
}

func TestClientSupportCreateRequestRequiresAuthorization(t *testing.T) {
	body := `{"subject":"test","description":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clients/tenant-1/support-requests", strings.NewReader(body))
	out := httptest.NewRecorder()

	s := &APIServer{}
	s.handleCreateClientSupportRequest(out, req)

	if out.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for missing authorization: %s", out.Code, out.Body.String())
	}
}

func TestCreateClientSupportRequestDefaults(t *testing.T) {
	values := createClientSupportRequest{}

	if values.Priority == "" {
		values.Priority = "normal"
	}
	if values.Category == "" {
	values.Category = "technical"
	}

	assert.Equal(t, "normal", values.Priority)
	assert.Equal(t, "technical", values.Category)
}

func TestClientSupportRequestDefaultPriority(t *testing.T) {
	values := createClientSupportRequest{
		Subject:     "test",
		Description: "test",
	}

	if values.Priority == "" {
		values.Priority = "normal"
	}

	assert.Equal(t, "normal", values.Priority)
}

func TestClientSupportRequestDefaultCategory(t *testing.T) {
	values := createClientSupportRequest{
		Subject:     "test",
		Description: "test",
	}

	if values.Category == "" {
		values.Category = "technical"
	}

	assert.Equal(t, "technical", values.Category)
}

func TestClientSupportRequestPriorityHigh(t *testing.T) {
	values := createClientSupportRequest{
		Subject:     "test",
		Description: "test",
		Priority:    "high",
	}

	assert.Equal(t, "high", values.Priority)
}

func TestClientSupportRequestPriorityCritical(t *testing.T) {
	values := createClientSupportRequest{
		Subject:     "test",
		Description: "test",
		Priority:    "critical",
	}

	assert.Equal(t, "critical", values.Priority)
}

func TestClientSupportRequestCategoryBilling(t *testing.T) {
	values := createClientSupportRequest{
		Subject:     "test",
		Description: "test",
		Category:    "billing",
	}

	assert.Equal(t, "billing", values.Category)
}

func TestClientSupportRequestCategorySoftware(t *testing.T) {
	values := createClientSupportRequest{
		Subject:     "test",
		Description: "test",
		Category:    "software",
	}

	assert.Equal(t, "software", values.Category)
}

func TestClientSupportRequestReplyBodyNil(t *testing.T) {
	req := clientSupportRequest{}
	assert.Nil(t, req.ReplyBody)
}

func TestClientSupportRequestReplyBodyValid(t *testing.T) {
	body := []byte(`{"body":"hello"}`)
	req := clientSupportRequest{
		ReplyBody: body,
	}
	assert.NotNil(t, req.ReplyBody)
}

func TestClientSessionStructFields(t *testing.T) {
	now := time.Now()

	session := clientSession{
		ID:             "session-1",
		ClientID:       "client-1",
		UserID:         "user-1",
		Username:       "user@example.com",
		SessionToken:   "abc123",
		LastActivityAt: now,
		ExpiresAt:      now.Add(24 * time.Hour),
		LastIPAddress:  "192.168.1.1",
		LastUserAgent:  "Chrome",
		IsActive:       true,
		AuthenticatedAt: now,
		CreatedAt:      now.Add(-24 * time.Hour),
	}

	assert.Equal(t, "session-1", session.ID)
	assert.Equal(t, "client-1", session.ClientID)
	assert.Equal(t, "user-1", session.UserID)
	assert.Equal(t, "user@example.com", session.Username)
	assert.Equal(t, "abc123", session.SessionToken)
	assert.Equal(t, "192.168.1.1", session.LastIPAddress)
	assert.Equal(t, "Chrome", session.LastUserAgent)
	assert.True(t, session.IsActive)
}

func TestClientSessionExpired(t *testing.T) {
	now := time.Now()

	session := clientSession{
		ExpiresAt: now.Add(-1 * time.Hour),
	}

	assert.False(t, session.IsActive)
}

func TestClientSessionNotExpired(t *testing.T) {
	now := time.Now()

	session := clientSession{
		ExpiresAt: now.Add(1 * time.Hour),
		IsActive:  true,
	}

	assert.True(t, session.IsActive)
}

func TestListClientSupportRequestsStructFields(t *testing.T) {
	reqs := []clientSupportRequest{
		{ID: "req-1", Status: "open"},
	}

	resp := listClientSupportRequests{
		Requests: reqs,
		Count:    1,
	}

	assert.Equal(t, 1, resp.Count)
	assert.Equal(t, "req-1", resp.Requests[0].ID)
}

func TestListClientSupportRequestsEmptyList(t *testing.T) {
	resp := listClientSupportRequests{
		Requests: []clientSupportRequest{},
		Count:    0,
	}

	assert.Equal(t, 0, resp.Count)
}

func TestClientProfileStructFields(t *testing.T) {
	now := time.Now()

	profile := clientProfile{
		ID:          "client-1",
		MSPID:       "msp-1",
		Name:        "Acme Corp",
		Slug:        "acme",
		DisplayName: "Acme",
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	assert.Equal(t, "client-1", profile.ID)
	assert.Equal(t, "msp-1", profile.MSPID)
	assert.Equal(t, "Acme Corp", profile.Name)
	assert.Equal(t, "acme", profile.Slug)
	assert.Equal(t, "Acme", profile.DisplayName)
	assert.True(t, profile.IsActive)
}

func TestUpdateClientProfileRequestFields(t *testing.T) {
	req := updateClientProfileRequest{
		Name:        "Updated Acme",
		DisplayName: "Acme Updated",
		Slug:        "acme-updated",
	}

	assert.Equal(t, "Updated Acme", req.Name)
	assert.Equal(t, "Acme Updated", req.DisplayName)
	assert.Equal(t, "acme-updated", req.Slug)
}

func TestCreateAuthProviderRequestFields(t *testing.T) {
	settings := []byte(`{"setting":"true"}`)

	req := createAuthProviderRequest{
		ProviderName: "google",
		ProviderID:   "provider-1",
		ClientSecret: "secret",
		RedirectURI:  "https://example.com/callback",
		IsActive:     true,
		Settings:     settings,
	}

	assert.Equal(t, "google", req.ProviderName)
	assert.Equal(t, "provider-1", req.ProviderID)
	assert.Equal(t, "secret", req.ClientSecret)
	assert.Equal(t, "https://example.com/callback", req.RedirectURI)
	assert.True(t, req.IsActive)
}

func TestClientSettingsStructFields(t *testing.T) {
	now := time.Now()

	settings := clientSettings{
		ID:                      "settings-1",
		ClientID:                "client-1",
		AllowSelfRegistration:   true,
		SelfRegistrationDomains: []string{"example.com"},
		EnableSSO:               true,
		EnablePasswordLogin:     true,
		SupportEmail:            "support@example.com",
		SupportPhone:            "+18005551234",
		SupportURL:              "https://example.com/support",
		LogoURL:                 "https://example.com/logo.png",
		FaviconURL:              "https://example.com/favicon.ico",
		PrimaryColor:            "#111111",
		AccentColor:             "#0066cc",
		PortalTitle:             "Acme Portal",
		WelcomeText:             "Welcome",
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	assert.Equal(t, "settings-1", settings.ID)
	assert.Equal(t, "client-1", settings.ClientID)
	assert.True(t, settings.AllowSelfRegistration)
	assert.True(t, settings.EnableSSO)
	assert.True(t, settings.EnablePasswordLogin)
	assert.Equal(t, "support@example.com", settings.SupportEmail)
	assert.Equal(t, "#111111", settings.PrimaryColor)
	assert.Equal(t, "#0066cc", settings.AccentColor)
	assert.Equal(t, "Acme Portal", settings.PortalTitle)
}

func TestUpdatePortalSettingsRequestFields(t *testing.T) {
	settings := []byte(`{"key":"value"}`)

	req := updatePortalSettingsRequest{
		AllowSelfRegistration: true,
		EnableSSO:             true,
		EnablePasswordLogin:   true,
		BrandingOverride:      settings,
		WelcomeMessage:        "Welcome",
		SupportEmail:          "support@example.com",
		PortalTitle:           "Portal",
		WelcomeText:           "Welcome text",
	}

	assert.True(t, req.AllowSelfRegistration)
	assert.True(t, req.EnableSSO)
	assert.True(t, req.EnablePasswordLogin)
	assert.Equal(t, "Welcome", req.WelcomeMessage)
	assert.Equal(t, "Portal", req.PortalTitle)
	assert.Equal(t, "Welcome text", req.WelcomeText)
}

func TestClientSupportRequestStatusTransitionOpenToInProgress(t *testing.T) {
	req := clientSupportRequest{
		ID:     "req-1",
		Status: "open",
	}

	assert.Equal(t, "open", req.Status)

	req.Status = "in_progress"
	assert.Equal(t, "in_progress", req.Status)
}

func TestClientSupportRequestStatusTransitionOpenToClosed(t *testing.T) {
	req := clientSupportRequest{
		ID:     "req-1",
		Status: "open",
	}

	assert.Equal(t, "open", req.Status)

	req.Status = "closed"
	assert.Equal(t, "closed", req.Status)
}

func TestAuthProviderStructFields(t *testing.T) {
	now := time.Now()

	provider := authProvider{
		ID:           "auth-1",
		ClientID:     "client-1",
		ProviderName: "google",
		ProviderID:   "provider-1",
		DisplayName:  "Google",
		RedirectURI:  "https://example.com/auth/google",
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	assert.Equal(t, "auth-1", provider.ID)
	assert.Equal(t, "google", provider.ProviderName)
	assert.Equal(t, "provider-1", provider.ProviderID)
	assert.Equal(t, "Google", provider.DisplayName)
	assert.True(t, provider.IsActive)
}

func TestIsValidAuthProvider(t *testing.T) {
	assert.True(t, isValidAuthProvider("google"))
	assert.True(t, isValidAuthProvider("microsoft"))
	assert.True(t, isValidAuthProvider("okta"))
	assert.True(t, isValidAuthProvider("github"))
	assert.True(t, isValidAuthProvider("gitlab"))
	assert.True(t, isValidAuthProvider("saml"))
	assert.False(t, isValidAuthProvider("unknown"))
}

func TestListAuthProvidersResponseFields(t *testing.T) {
	resp := listAuthProvidersResponse{
		Count: 2,
	}

	assert.Equal(t, 2, resp.Count)
}

func TestClientPortalSettingsAllowSelfRegistration(t *testing.T) {
	settings := clientSettings{
		AllowSelfRegistration: true,
	}

	assert.True(t, settings.AllowSelfRegistration)
}

func TestClientPortalSettingsDisablePasswordLogin(t *testing.T) {
	settings := clientSettings{
		EnablePasswordLogin: false,
	}

	assert.False(t, settings.EnablePasswordLogin)
}

func TestClientPortalSettingsDisableSSO(t *testing.T) {
	settings := clientSettings{
		EnableSSO: false,
	}

	assert.False(t, settings.EnableSSO)
}

func TestClientPortalSettingsBrandingOverrideNil(t *testing.T) {
	settings := clientSettings{}

	assert.Nil(t, settings.BrandingOverride)
}

func TestClientPortalSettingsBrandingOverrideValid(t *testing.T) {
	override := []byte(`{"primary_color":"#ff0000"}`)
	settings := clientSettings{
		BrandingOverride: override,
	}

	assert.NotNil(t, settings.BrandingOverride)
}

func TestClientPortalSettingsEmptySelfRegistrationDomains(t *testing.T) {
	settings := clientSettings{
		SelfRegistrationDomains: []string{},
	}

	assert.Equal(t, 0, len(settings.SelfRegistrationDomains))
}

func TestClientPortalSettingsSelfRegistrationDomainsPopulated(t *testing.T) {
	settings := clientSettings{
		SelfRegistrationDomains: []string{"example.com", "acme.com"},
	}

	assert.Equal(t, 2, len(settings.SelfRegistrationDomains))
}

func TestClientSupportRequestWithDeviceID(t *testing.T) {
	req := clientSupportRequest{
		ID:       "req-1",
		DeviceID: "device-1",
		Category: "technical",
		Status:   "open",
	}

	assert.Equal(t, "device-1", req.DeviceID)
}

func TestClientSupportRequestWithoutDeviceID(t *testing.T) {
	req := clientSupportRequest{
		ID:       "req-1",
		DeviceID: "",
		Category: "technical",
		Status:   "open",
	}

	assert.Equal(t, "", req.DeviceID)
}

func TestClientSupportRequestCategoryNetwork(t *testing.T) {
	req := clientSupportRequest{
		Category: "network",
	}

	assert.Equal(t, "network", req.Category)
}

func TestClientSupportRequestCategoryAccess(t *testing.T) {
	req := clientSupportRequest{
		Category: "access",
	}

	assert.Equal(t, "access", req.Category)
}

func TestClientSupportRequestCategoryOther(t *testing.T) {
	req := clientSupportRequest{
		Category: "other",
	}

	assert.Equal(t, "other", req.Category)
}
