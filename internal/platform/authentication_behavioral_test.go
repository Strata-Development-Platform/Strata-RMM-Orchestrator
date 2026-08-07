package platform

import (
	"encoding/base32"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

func TestLoginRequestStructFields(t *testing.T) {
	req := loginRequest{
		Email:    "user@example.com",
		Password: "correct-password",
	}

	if req.Email != "user@example.com" {
		t.Fatalf("loginRequest.Email = %q", req.Email)
	}
	if req.Password != "correct-password" {
		t.Fatalf("loginRequest.Password = %q", req.Password)
	}
}

func TestLoginResponseStructFields(t *testing.T) {
	resp := loginResponse{
		Token:               "jwt-token",
		UserID:              "user-1",
		Email:               "user@example.com",
		Role:                "msp_owner",
		Roles:               []string{"msp_owner", "msp_admin"},
		Permissions:         []string{"devices.write", "devices.read"},
		SelectedScope:       AuthorizationScope{Type: ScopeMSP, ID: "msp-1"},
		Grants:              []AuthorizationGrant{{Role: "msp_owner", SourceType: ScopeMSP, SourceID: "msp-1"}},
		TenantID:            "tenant-1",
		ProviderDisplayName: "Example MSP",
		SetupComplete:       true,
		ExpiresAt:           time.Now().Add(8 * time.Hour),
	}

	if resp.Token != "jwt-token" {
		t.Fatalf("loginResponse.Token = %q", resp.Token)
	}
	if resp.UserID != "user-1" {
		t.Fatalf("loginResponse.UserID = %q", resp.UserID)
	}
	if resp.Role != "msp_owner" {
		t.Fatalf("loginResponse.Role = %q", resp.Role)
	}
	if resp.ProviderDisplayName != "Example MSP" {
		t.Fatalf("loginResponse.ProviderDisplayName = %q", resp.ProviderDisplayName)
	}
	if !resp.SetupComplete {
		t.Fatal("loginResponse.SetupComplete = false")
	}
	if resp.ExpiresAt.IsZero() {
		t.Fatal("loginResponse.ExpiresAt is zero")
	}
}

func TestTenantInfoStructFields(t *testing.T) {
	info := tenantInfo{
		ID:   "tenant-1",
		Name: "Acme Corp",
		Slug: "acme",
	}

	if info.ID != "tenant-1" {
		t.Fatalf("tenantInfo.ID = %q", info.ID)
	}
	if info.Name != "Acme Corp" {
		t.Fatalf("tenantInfo.Name = %q", info.Name)
	}
	if info.Slug != "acme" {
		t.Fatalf("tenantInfo.Slug = %q", info.Slug)
	}
}

func TestPrincipalStructFields(t *testing.T) {
	principal := Principal{
		UserID:         "user-1",
		Email:          "user@example.com",
		TokenID:        "token-1",
		TokenUse:       "user",
		PlatformID:     "platform-1",
		MSPID:          "msp-1",
		ClientID:       "client-1",
		SiteID:         "site-1",
		LegacyTenantID: "legacy-1",
		AuthMethod:     "password",
		SupportGrantID: "grant-1",
	}

	if principal.UserID != "user-1" {
		t.Fatalf("Principal.UserID = %q", principal.UserID)
	}
	if principal.TokenUse != "user" {
		t.Fatalf("Principal.TokenUse = %q", principal.TokenUse)
	}
	if principal.AuthMethod != "password" {
		t.Fatalf("Principal.AuthMethod = %q", principal.AuthMethod)
	}
}

func TestAuthorizationScopeFields(t *testing.T) {
	scope := AuthorizationScope{
		Type:       ScopeMSP,
		ID:         "msp-1",
		PlatformID: "platform-1",
		MSPID:      "msp-1",
	}

	if scope.Type != ScopeMSP {
		t.Fatalf("AuthorizationScope.Type = %v", scope.Type)
	}
	if scope.ID != "msp-1" {
		t.Fatalf("AuthorizationScope.ID = %q", scope.ID)
	}
	if scope.MSPID != "msp-1" {
		t.Fatalf("AuthorizationScope.MSPID = %q", scope.MSPID)
	}
}

func TestAuthorizationGrantFields(t *testing.T) {
	grant := AuthorizationGrant{
		Role:       "msp_admin",
		SourceType: ScopeMSP,
		SourceID:   "msp-1",
		Inherited:  false,
	}

	if grant.Role != "msp_admin" {
		t.Fatalf("AuthorizationGrant.Role = %q", grant.Role)
	}
	if grant.SourceType != ScopeMSP {
		t.Fatalf("AuthorizationGrant.SourceType = %v", grant.SourceType)
	}
	if grant.Inherited {
		t.Fatal("AuthorizationGrant.Inherited = true")
	}
}

func TestAuthorizationResultFields(t *testing.T) {
	result := AuthorizationResult{
		Selected: AuthorizationScope{Type: ScopeMSP, ID: "msp-1"},
		Grants: []AuthorizationGrant{
			{Role: "msp_admin", SourceType: ScopeMSP, SourceID: "msp-1"},
		},
		Roles:       []string{"msp_admin"},
		Permissions: []string{"devices.read"},
	}

	if result.Selected.Type != ScopeMSP {
		t.Fatalf("AuthorizationResult.Selected.Type = %v", result.Selected.Type)
	}
	if len(result.Roles) != 1 {
		t.Fatalf("AuthorizationResult.Roles length = %d", len(result.Roles))
	}
	if len(result.Permissions) == 0 {
		t.Fatal("AuthorizationResult.Permissions is empty")
	}
}

func TestAuthorizationResultHasRole(t *testing.T) {
	result := AuthorizationResult{
		Roles: []string{"msp_owner", "msp_admin"},
	}

	if !result.HasRole("msp_owner") {
		t.Fatal("HasRole(msp_owner) = false")
	}
	if !result.HasRole("msp_admin") {
		t.Fatal("HasRole(msp_admin) = false")
	}
	if result.HasRole("platform_owner") {
		t.Fatal("HasRole(platform_owner) = true")
	}
}

func TestAuthorizationResultHasRoleEmptyRoles(t *testing.T) {
	result := AuthorizationResult{
		Roles: []string{},
	}

	if result.HasRole("msp_admin") {
		t.Fatal("HasRole on empty roles")
	}
}

func TestAuthorizationResultHasGrant(t *testing.T) {
	result := AuthorizationResult{
		Grants: []AuthorizationGrant{
			{Role: "msp_admin", SourceType: ScopeMSP, SourceID: "msp-1"},
		},
	}

	if !result.HasGrant(ScopeMSP, "msp_admin") {
		t.Fatal("HasGrant(ScopeMSP, msp_admin) = false")
	}
	if result.HasGrant(ScopeMSP, "platform_admin") {
		t.Fatal("HasGrant wrong role")
	}
}

func TestAuthorizationResultIsPlatformGlobal(t *testing.T) {
	result := AuthorizationResult{
		Selected: AuthorizationScope{
			Type:       ScopePlatform,
			ID:         "00000000-0000-0000-0000-000000000001",
			PlatformID: "00000000-0000-0000-0000-000000000001",
		},
		Grants: []AuthorizationGrant{
			{Role: "platform_admin", SourceType: ScopePlatform, SourceID: "00000000-0000-0000-0000-000000000001"},
		},
	}

	if !result.IsPlatformGlobal() {
		t.Fatal("IsPlatformGlobal = false")
	}
}

func TestAuthorizationResultIsNotPlatformGlobalWrongScope(t *testing.T) {
	result := AuthorizationResult{
		Selected: AuthorizationScope{Type: ScopeMSP, ID: "msp-1"},
		Grants: []AuthorizationGrant{
			{Role: "msp_owner", SourceType: ScopeMSP, SourceID: "msp-1"},
		},
	}

	if result.IsPlatformGlobal() {
		t.Fatal("IsPlatformGlobal true on MSP scope")
	}
}

func TestAuthorizationResultCanManageSelectedScope(t *testing.T) {
	result := AuthorizationResult{
		Selected: AuthorizationScope{Type: ScopeMSP, ID: "msp-1"},
		Roles:    []string{"msp_admin"},
	}

	if !result.CanManageSelectedScope() {
		t.Fatal("CanManageSelectedScope = false for msp_admin")
	}
}

func TestAuthorizationResultCanManageSelectedScopeFailsClosed(t *testing.T) {
	result := AuthorizationResult{
		Selected: AuthorizationScope{Type: ScopeClient, ID: "client-1"},
		Roles:    []string{"platform_viewer"},
	}

	if result.CanManageSelectedScope() {
		t.Fatal("CanManageSelectedScope true for platform_viewer on client")
	}
}

func TestAuthorizationResultHasPlatformAdministratorMembership(t *testing.T) {
	result := AuthorizationResult{
		Grants: []AuthorizationGrant{
			{Role: "platform_owner", SourceType: ScopePlatform, SourceID: "00000000-0000-0000-0000-000000000001"},
		},
	}

	if !result.HasPlatformAdministratorMembership() {
		t.Fatal("HasPlatformAdministratorMembership = false")
	}
}

func TestScopeTypeConstants(t *testing.T) {
	if ScopePlatform != "platform" {
		t.Fatalf("ScopePlatform = %q", ScopePlatform)
	}
	if ScopeMSP != "msp" {
		t.Fatalf("ScopeMSP = %q", ScopeMSP)
	}
	if ScopeClient != "client" {
		t.Fatalf("ScopeClient = %q", ScopeClient)
	}
	if ScopeSite != "site" {
		t.Fatalf("ScopeSite = %q", ScopeSite)
	}
}

func TestMFASecretStructFields(t *testing.T) {
	secret := auth.MFASecret{
		UserID:    "user-1",
		TenantID:  "tenant-1",
		Secret:    "JBSWY3DPEHPK3PXP",
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	if secret.UserID != "user-1" {
		t.Fatalf("auth.MFASecret.UserID = %q", secret.UserID)
	}
	if secret.TenantID != "tenant-1" {
		t.Fatalf("auth.MFASecret.TenantID = %q", secret.TenantID)
	}
	if !secret.Enabled {
		t.Fatal("auth.MFASecret.Enabled = false")
	}
}

func TestMFASecretDisabled(t *testing.T) {
	secret := auth.MFASecret{
		UserID:  "user-1",
		Enabled: false,
	}

	if secret.Enabled {
		t.Fatal("auth.MFASecret.Enabled should be false")
	}
}

func TestMFASecretSecretFieldOmittedFromJSON(t *testing.T) {
	secret := auth.MFASecret{
		UserID: "user-1",
		Secret: "should-not-appear",
	}

	if secret.Secret != "should-not-appear" {
		t.Fatalf("auth.MFASecret.Secret = %q", secret.Secret)
	}
}

func TestEnrollmentTokenStructFields(t *testing.T) {
	token := auth.EnrollmentToken{
		Token:     "enr-tenant-1-1234567890",
		TenantID:  "tenant-1",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Used:      false,
	}

	if token.Token != "enr-tenant-1-1234567890" {
		t.Fatalf("auth.EnrollmentToken.Token = %q", token.Token)
	}
	if token.TenantID != "tenant-1" {
		t.Fatalf("auth.EnrollmentToken.TenantID = %q", token.TenantID)
	}
	if token.ExpiresAt.IsZero() {
		t.Fatal("auth.EnrollmentToken.ExpiresAt is zero")
	}
}

func TestEnrollmentTokenUsedFlag(t *testing.T) {
	token := auth.EnrollmentToken{Used: true}
	if !token.Used {
		t.Fatal("auth.EnrollmentToken.Used should be true")
	}

	token2 := auth.EnrollmentToken{Used: false}
	if token2.Used {
		t.Fatal("auth.EnrollmentToken.Used should be false")
	}
}

func TestClaimsStructFields(t *testing.T) {
	claims := auth.Claims{
		Subject:  "user-1",
		TokenID:  "token-1",
		Issuer:   "strata-rmm",
		Audience: "strata-rmm-api",
		Roles:    []string{"msp_owner"},
	}

	if claims.Subject != "user-1" {
		t.Fatalf("Claims.Subject = %q", claims.Subject)
	}
	if claims.Issuer != "strata-rmm" {
		t.Fatalf("Claims.Issuer = %q", claims.Issuer)
	}
	if claims.Audience != "strata-rmm-api" {
		t.Fatalf("Claims.Audience = %q", claims.Audience)
	}
}



func TestTokenGeneratorGenerateUserToken(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	token, err := gen.GenerateUserToken("user-1", "tenant-1", "msp-1", "client-1", "site-1", []string{"msp_owner"}, time.Hour)
	if err != nil {
		t.Fatalf("GenerateUserToken: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}
	if strings.Contains(token, "test-secret") {
		t.Fatal("token contains JWT secret")
	}
}

func TestTokenGeneratorGenerateAgentToken(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	token, err := gen.GenerateAgentToken("tenant-1", "agent-1", 720*time.Hour)
	if err != nil {
		t.Fatalf("GenerateAgentToken: %v", err)
	}
	if token == "" {
		t.Fatal("agent token is empty")
	}
}

func TestTokenGeneratorGenerateUserTokenEmptyUserID(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	_, err := gen.GenerateUserToken("", "tenant-1", "", "", "", []string{"admin"}, time.Hour)
	if err == nil {
		t.Fatal("expected error for empty userID")
	}
}

func TestTokenGeneratorGenerateUserTokenEmptyRoles(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	_, err := gen.GenerateUserToken("user-1", "tenant-1", "", "", "", []string{}, time.Hour)
	if err == nil {
		t.Fatal("expected error for empty roles")
	}
}

func TestTokenGeneratorGenerateAgentTokenEmptyTenant(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	_, err := gen.GenerateAgentToken("", "agent-1", time.Hour)
	if err == nil {
		t.Fatal("expected error for empty tenantID")
	}
}

func TestTokenGeneratorGenerateAgentTokenEmptyAgentID(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	_, err := gen.GenerateAgentToken("tenant-1", "", time.Hour)
	if err == nil {
		t.Fatal("expected error for empty agentID")
	}
}

func TestTokenGeneratorValidateUserToken(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	token, err := gen.GenerateUserToken("user-1", "tenant-1", "msp-1", "", "", []string{"msp_owner"}, time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := gen.Validate(token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("Claims.Subject = %q", claims.Subject)
	}
	if claims.Issuer != "strata-rmm" {
		t.Fatalf("Claims.Issuer = %q", claims.Issuer)
	}
	if claims.TokenUse != "user" {
		t.Fatalf("Claims.TokenUse = %q", claims.TokenUse)
	}
}

func TestTokenGeneratorValidateAgentToken(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	token, err := gen.GenerateAgentToken("tenant-1", "agent-1", time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := gen.Validate(token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if claims.TokenUse != "agent" {
		t.Fatalf("Claims.TokenUse = %q", claims.TokenUse)
	}
	if claims.Subject != "agent-1" {
		t.Fatalf("Claims.Subject = %q", claims.Subject)
	}
}

func TestTokenGeneratorValidateExpiredToken(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	token, err := gen.GenerateUserToken("user-1", "tenant-1", "", "", "", []string{"admin"}, -1*time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	_, err = gen.Validate(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestTokenGeneratorValidateWrongSecret(t *testing.T) {
	gen1 := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing-one")
	gen2 := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing-two")
	token, err := gen1.GenerateUserToken("user-1", "tenant-1", "", "", "", []string{"admin"}, time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	_, err = gen2.Validate(token)
	if err == nil {
		t.Fatal("expected validation error with different secret")
	}
}

func TestTokenGeneratorValidateInvalidAlgorithm(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	token, err := gen.GenerateUserToken("user-1", "tenant-1", "", "", "", []string{"admin"}, time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) < 3 {
		t.Fatal("invalid token parts")
	}
	headerJSON := `{"alg":"HS512","typ":"JWT"}`
	encoded := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	invalidToken := encoded + "." + parts[1] + "." + parts[2]
	_, err = gen.Validate(invalidToken)
	if err == nil {
		t.Fatal("expected error for HS512")
	}
}

func TestTokenGeneratorValidateInvalidTokenType(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	token, err := gen.GenerateUserToken("user-1", "tenant-1", "", "", "", []string{"admin"}, time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) < 3 {
		t.Fatal("invalid token parts")
	}
	headerJSON := `{"alg":"HS256","typ":"JWE"}`
	encoded := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	invalidToken := encoded + "." + parts[1] + "." + parts[2]
	_, err = gen.Validate(invalidToken)
	if err == nil {
		t.Fatal("expected error for typ=JWE")
	}
}

func TestTokenGeneratorValidateInvalidTokenFormat(t *testing.T) {
	gen := auth.NewTokenGenerator("test-secret-that-is-long-enough-for-testing")
	_, err := gen.Validate("not-a-jwt")
	if err == nil {
		t.Fatal("expected error for invalid token format")
	}
}

func TestEnrollmentManagerCreateToken(t *testing.T) {
	manager := auth.NewEnrollmentManager("test-secret-that-is-long-enough-for-testing")
	token, err := manager.CreateEnrollmentToken("tenant-1", 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateEnrollmentToken: %v", err)
	}
	if token.TenantID != "tenant-1" {
		t.Fatalf("EnrollmentToken.TenantID = %q", token.TenantID)
	}
	if token.ExpiresAt.IsZero() {
		t.Fatal("EnrollmentToken.ExpiresAt is zero")
	}
}

func TestEnrollmentManagerValidateTokenNotFound(t *testing.T) {
	manager := auth.NewEnrollmentManager("test-secret-that-is-long-enough-for-testing")
	_, err := manager.ValidateEnrollmentToken("nonexistent-token")
	if err == nil {
		t.Fatal("expected error for nonexistent token")
	}
}

func TestEnrollmentManagerValidateTokenExpired(t *testing.T) {
	manager := auth.NewEnrollmentManager("test-secret-that-is-long-enough-for-testing")
	token, err := manager.CreateEnrollmentToken("tenant-1", -1*time.Hour)
	if err != nil {
		t.Fatalf("CreateEnrollmentToken: %v", err)
	}
	_, err = manager.ValidateEnrollmentToken(token.Token)
	if err == nil {
		t.Fatal("expected error for expired enrollment token")
	}
}

func TestEnrollmentManagerValidateTokenUsed(t *testing.T) {
	manager := auth.NewEnrollmentManager("test-secret-that-is-long-enough-for-testing")
	token, err := manager.CreateEnrollmentToken("tenant-1", 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateEnrollmentToken: %v", err)
	}
	if _, err := manager.ValidateEnrollmentToken(token.Token); err != nil {
		t.Fatalf("first validate: %v", err)
	}
	_, err = manager.ValidateEnrollmentToken(token.Token)
	if err == nil {
		t.Fatal("expected error for already used enrollment token")
	}
}

func TestTOTPManagerGenerateSecret(t *testing.T) {
	manager := auth.NewTOTPManager()
	secret, err := manager.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if secret == "" {
		t.Fatal("secret is empty")
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	if len(decoded) != 20 {
		t.Fatalf("decoded secret length = %d, want 20", len(decoded))
	}
}

func TestTOTPManagerProvisioningURIParams(t *testing.T) {
	manager := auth.NewTOTPManager()
	uri := manager.ProvisioningURI("secret123", "user@example.com", "Strata")
	if !strings.Contains(uri, "otpauth://totp/") {
		t.Fatalf("ProvisioningURI missing otpauth prefix: %q", uri)
	}
	if !strings.Contains(uri, "secret=secret123") {
		t.Fatalf("ProvisioningURI missing secret: %q", uri)
	}
	if !strings.Contains(uri, "issuer=Strata") {
		t.Fatalf("ProvisioningURI missing issuer: %q", uri)
	}
	if !strings.Contains(uri, "algorithm=SHA1") {
		t.Fatalf("ProvisioningURI missing algorithm: %q", uri)
	}
	if !strings.Contains(uri, "digits=6") {
		t.Fatalf("ProvisioningURI missing digits: %q", uri)
	}
	if !strings.Contains(uri, "period=30") {
		t.Fatalf("ProvisioningURI missing period: %q", uri)
	}
}

func TestTOTPManagerGenerateCodeLength(t *testing.T) {
	manager := auth.NewTOTPManager()
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(make([]byte, 20))
	code, err := manager.GenerateCode(secret, time.Unix(1234567890, 0))
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("code length = %d, want 6", len(code))
	}
}

func TestTOTPManagerValidateCodeCorrectWindow(t *testing.T) {
	manager := auth.NewTOTPManager()
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(make([]byte, 20))
	now := time.Unix(1234567890, 0)
	code, err := manager.GenerateCode(secret, now)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	ok, err := manager.ValidateCode(secret, code, now)
	if err != nil {
		t.Fatalf("ValidateCode: %v", err)
	}
	if !ok {
		t.Fatal("ValidateCode should succeed for current window")
	}
}

func TestTOTPManagerValidateCodeStrictWindow(t *testing.T) {
	manager := auth.NewTOTPManager()
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(make([]byte, 20))
	now := time.Unix(1234567890, 0)
	code, err := manager.GenerateCode(secret, now)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	ok, err := manager.ValidateCodeStrict(secret, code, now, 1)
	if err != nil {
		t.Fatalf("ValidateCodeStrict: %v", err)
	}
	if !ok {
		t.Fatal("ValidateCodeStrict should succeed within window")
	}
}

func TestTOTPManagerGenerateCodeNegativeTimeRejected(t *testing.T) {
	manager := auth.NewTOTPManager()
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(make([]byte, 20))
	_, err := manager.GenerateCode(secret, time.Unix(-1000, 0))
	if err == nil {
		t.Fatal("expected error for negative unix time")
	}
}

func TestAPIKeyAuthMiddlewareMissingKey(t *testing.T) {
	authenticator := auth.NewAPIKeyAuth()
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAPIKeyAuthMiddlewareInvalidKey(t *testing.T) {
	authenticator := auth.NewAPIKeyAuth()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "invalid-key")
	w := httptest.NewRecorder()
	authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAPIKeyAuthMiddlewareValidKey(t *testing.T) {
	authenticator := auth.NewAPIKeyAuth()
	authenticator.SetKey("valid-key", "tenant-1")
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "valid-key")
	w := httptest.NewRecorder()
	authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestAPIKeyAuthRevokeKey(t *testing.T) {
	authenticator := auth.NewAPIKeyAuth()
	authenticator.SetKey("key-1", "tenant-1")
	authenticator.RevokeKey("key-1")
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "key-1")
	w := httptest.NewRecorder()
	authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status after revoke = %d, want 403", w.Code)
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	auth.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	expected := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Cache-Control":          "no-store",
		"Content-Security-Policy": "default-src 'none'",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, want := range expected {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestPrimaryEffectiveRolePriority(t *testing.T) {
	if primaryEffectiveRole([]string{"platform_owner"}) != "platform_owner" {
		t.Fatal("expected platform_owner")
	}
	if primaryEffectiveRole([]string{"msp_admin", "platform_owner"}) != "platform_owner" {
		t.Fatal("platform_owner should win over msp_admin")
	}
	if primaryEffectiveRole([]string{"msp_owner", "msp_admin"}) != "msp_owner" {
		t.Fatal("msp_owner should win over msp_admin")
	}
	if primaryEffectiveRole([]string{"viewer"}) != "viewer" {
		t.Fatal("viewer should be returned as fallback")
	}
	if primaryEffectiveRole([]string{}) != "" {
		t.Fatal("empty roles should return empty string")
	}
}

func TestPrimaryEffectiveRolePrefersHigherPlatformRole(t *testing.T) {
	if got := primaryEffectiveRole([]string{"platform_admin", "platform_owner"}); got != "platform_owner" {
		t.Fatalf("platform_owner should be preferred: got %q", got)
	}
	if got := primaryEffectiveRole([]string{"msp_admin", "msp_owner"}); got != "msp_owner" {
		t.Fatalf("msp_owner should be preferred: got %q", got)
	}
}

func TestOwnerActivationMailFields(t *testing.T) {
	mail := OwnerActivationMail{
		Recipient:     "owner@example.com",
		MSPName:       "Example MSP",
		ActivationURL: "https://rmm.example.com/activate-account#token",
		Token:         "token",
		ExpiresAt:     time.Now().Add(72 * time.Hour),
	}
	if mail.Recipient != "owner@example.com" {
		t.Fatalf("OwnerActivationMail.Recipient = %q", mail.Recipient)
	}
	if mail.MSPName != "Example MSP" {
		t.Fatalf("OwnerActivationMail.MSPName = %q", mail.MSPName)
	}
}

func TestInspectedOwnerInvitationFields(t *testing.T) {
	inv := inspectedOwnerInvitation{
		MSPName:     "Example MSP",
		MaskedEmail: "o***@e***.com",
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}
	if inv.MSPName != "Example MSP" {
		t.Fatalf("inspectedOwnerInvitation.MSPName = %q", inv.MSPName)
	}
	if inv.MaskedEmail != "o***@e***.com" {
		t.Fatalf("inspectedOwnerInvitation.MaskedEmail = %q", inv.MaskedEmail)
	}
}

func TestOwnerInvitationDeliveryFields(t *testing.T) {
	delivery := ownerInvitationDelivery{
		MSPID:          "msp-1",
		InvitationID:   "invitation-1",
		DeliveryStatus: "delivered",
	}
	if delivery.MSPID != "msp-1" {
		t.Fatalf("ownerInvitationDelivery.MSPID = %q", delivery.MSPID)
	}
	if delivery.DeliveryStatus != "delivered" {
		t.Fatalf("ownerInvitationDelivery.DeliveryStatus = %q", delivery.DeliveryStatus)
	}
}

func TestCreatePendingMSPInputFields(t *testing.T) {
	input := createPendingMSPInput{
		Name:       "Example MSP",
		Slug:       "example",
		Plan:       "business",
		OwnerEmail: "owner@example.com",
		ActorID:    "actor-1",
	}
	if input.Name != "Example MSP" {
		t.Fatalf("createPendingMSPInput.Name = %q", input.Name)
	}
	if input.Plan != "business" {
		t.Fatalf("createPendingMSPInput.Plan = %q", input.Plan)
	}
}

func TestRouteAccessConstants(t *testing.T) {
	if AccessPublic != 0 {
		t.Fatalf("AccessPublic = %d", AccessPublic)
	}
	if AccessAdmin != 3 {
		t.Fatalf("AccessAdmin = %d", AccessAdmin)
	}
	if AccessDenied != 4 {
		t.Fatalf("AccessDenied = %d", AccessDenied)
	}
}

func TestRouteStructFields(t *testing.T) {
	route := Route{
		Method: "POST",
		Path:   "/api/v1/jobs",
		Access: AccessAdmin,
	}
	if route.Method != "POST" {
		t.Fatalf("Route.Method = %q", route.Method)
	}
	if route.Access != AccessAdmin {
		t.Fatalf("Route.Access = %d", route.Access)
	}
}

func TestJWTConfigValidationEmpty(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if err := auth.ValidateJWTConfig(); err == nil {
		t.Fatal("expected error for empty JWT_SECRET")
	}
}

func TestJWTConfigValidationTooShort(t *testing.T) {
	t.Setenv("JWT_SECRET", "short")
	if err := auth.ValidateJWTConfig(); err == nil {
		t.Fatal("expected error for short JWT_SECRET")
	}
}

func TestJWTConfigValidationMinLength(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-secret-that-is-long-enou")
	if err := auth.ValidateJWTConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJWTConfigValidationPreviousTooShort(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-secret-that-is-long-enou")
	t.Setenv("JWT_SECRET_PREVIOUS", "short")
	if err := auth.ValidateJWTConfig(); err == nil {
		t.Fatal("expected error for short JWT_SECRET_PREVIOUS")
	}
}

func TestJWTConfigValidationPreviousSameAsCurrent(t *testing.T) {
	secret := "this-is-a-secret-that-is-long-enou"
	t.Setenv("JWT_SECRET", secret)
	t.Setenv("JWT_SECRET_PREVIOUS", secret)
	if err := auth.ValidateJWTConfig(); err == nil {
		t.Fatal("expected error for identical JWT_SECRET_PREVIOUS")
	}
}

func TestLoginHandlerRejectsMissingCredentials(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleLogin(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestLoginHandlerRejectsMissingBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleLogin(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestLogoutHandlerReturnsNoContent(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleLogout(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestMeHandlerRejectsMissingPrincipal(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleMe(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAgentRegisterHandlerRejectsMissingHostname(t *testing.T) {
	body := `{"enrollment_token":"tok"}`
	req := httptest.NewRequest("POST", "/api/v1/agent/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleAgentRegister(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAgentRegisterHandlerRejectsMissingEnrollmentTokenOrDeploymentID(t *testing.T) {
	body := `{"hostname":"host"}`
	req := httptest.NewRequest("POST", "/api/v1/agent/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleAgentRegister(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAgentConfigHandlerRejectsMissingDeploymentID(t *testing.T) {
	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/agent/config", strings.NewReader(body))
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleAgentConfig(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestSetUpdateSourceHandlerRejectsInvalidSource(t *testing.T) {
	body := `{"update_source":"invalid","update_channel":"stable"}`
	req := httptest.NewRequest("PUT", "/api/v1/tenants/tenant-1/update-source", strings.NewReader(body))
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleSetUpdateSource(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestSetUpdateSourceHandlerRejectsInvalidChannel(t *testing.T) {
	body := `{"update_source":"github","update_channel":"invalid"}`
	req := httptest.NewRequest("PUT", "/api/v1/tenants/tenant-1/update-source", strings.NewReader(body))
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleSetUpdateSource(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}


func TestCreateCustomerHandlerRejectsMissingName(t *testing.T) {
	body := `{"slug":"acme"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/customers", strings.NewReader(body))
	w := httptest.NewRecorder()
	s := &APIServer{}
	s.handleAdminCreateCustomer(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestClientSessionRequestFields(t *testing.T) {
	req := createSessionRequest{
		Username:    "user@example.com",
		Password:    "password",
		ProviderID:  "provider-1",
		RedirectURI: "https://example.com/callback",
	}
	if req.Username != "user@example.com" {
		t.Fatalf("createSessionRequest.Username = %q", req.Username)
	}
	if req.ProviderID != "provider-1" {
		t.Fatalf("createSessionRequest.ProviderID = %q", req.ProviderID)
	}
}

func TestClientSessionRequestMissingCredentials(t *testing.T) {
	req := createSessionRequest{}
	if req.Username != "" {
		t.Fatal("Username should be empty")
	}
	if req.Password != "" {
		t.Fatal("Password should be empty")
	}
	if req.ProviderID != "" {
		t.Fatal("ProviderID should be empty")
	}
}

func TestClientProfileUpdateRequestFields(t *testing.T) {
	req := updateClientProfileRequest{
		Name:        "Updated Corp",
		DisplayName: "Updated",
		Slug:        "updated",
	}
	if req.Name != "Updated Corp" {
		t.Fatalf("updateClientProfileRequest.Name = %q", req.Name)
	}
}


