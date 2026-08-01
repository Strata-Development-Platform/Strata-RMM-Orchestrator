package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOwnerInvitationTokenUses32RandomBytesAndStoresDigest(t *testing.T) {
	random := bytes.Repeat([]byte{0x5a}, 32)
	raw, digest, err := generateOwnerInvitationToken(bytes.NewReader(random))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 32 {
		t.Fatalf("decoded token bytes = %d, want 32", len(decoded))
	}
	if raw == digest || strings.Contains(digest, raw) || len(digest) != 64 {
		t.Fatalf("raw token and digest are not safely separated: raw_len=%d digest_len=%d", len(raw), len(digest))
	}
	derived, err := digestOwnerInvitationToken(raw)
	if err != nil || derived != digest {
		t.Fatalf("derived digest = %q, want %q (err=%v)", derived, digest, err)
	}
	for _, malformed := range []string{"", "short", raw + "=", strings.Repeat("a", 129)} {
		if _, err := digestOwnerInvitationToken(malformed); !errors.Is(err, errInvitationInvalid) {
			t.Fatalf("malformed token %q error = %v", malformed, err)
		}
	}
}

func TestOwnerActivationPasswordByteBoundaries(t *testing.T) {
	for _, test := range []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "13 rejected", password: strings.Repeat("a", 13), wantErr: true},
		{name: "14 accepted", password: strings.Repeat("a", 14)},
		{name: "72 accepted", password: strings.Repeat("a", 72)},
		{name: "73 rejected", password: strings.Repeat("a", 73), wantErr: true},
		{name: "control rejected", password: "valid-password\nextra", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateOwnerPassword(test.password)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateOwnerPassword() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestOwnerInvitationInspectionMasksEmail(t *testing.T) {
	if got := maskEmail("owner@example.test"); got != "o***@e***.test" {
		t.Fatalf("maskEmail() = %q", got)
	}
}

func TestOwnerActivationMailPlacesTokenInURLFragment(t *testing.T) {
	activation := OwnerActivationMail{
		Recipient: "owner@example.test", MSPName: "Example MSP",
		ActivationURL: "https://rmm.example.test/activate-account#one-time-secret",
		Token:         "one-time-secret", ExpiresAt: time.Unix(1_800_000_000, 0),
	}
	message := buildOwnerActivationMessage("accounts@example.test", activation.Recipient, activation)
	if strings.Contains(message, "?token=") || strings.Contains(message, "?") {
		t.Fatal("activation token was placed in a query string")
	}
	if !strings.Contains(message, "https://rmm.example.test/activate-account#one-time-secret") {
		t.Fatal("activation link is missing from the mail body")
	}
	if strings.Contains(message, "enter this one-time invitation code") {
		t.Fatal("mail body must not present a separate typed code")
	}
}

func TestSMTPMailerConfigurationFailsClosed(t *testing.T) {
	for _, config := range []SMTPAccountMailerConfig{
		{Address: "smtp.example.test:587", FromAddress: "accounts@example.test", Username: "user"},
		{Address: "missing-port", FromAddress: "accounts@example.test"},
		{Address: "smtp.example.test:587", FromAddress: "invalid"},
	} {
		if _, err := NewSMTPAccountMailer(config); err == nil {
			t.Fatalf("NewSMTPAccountMailer(%+v) succeeded", config)
		}
	}
}

func TestTopLevelPlatformInvitationAuthorization(t *testing.T) {
	request := func(userID, roles, mspID string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/v2/platform/msps", nil)
		ctx := context.WithValue(r.Context(), ctxKeyUserID, userID)
		ctx = context.WithValue(ctx, ctxKeyRole, roles)
		ctx = context.WithValue(ctx, ctxKeyMSPID, mspID)
		return r.WithContext(ctx)
	}
	for _, test := range []struct {
		name   string
		req    *http.Request
		wantOK bool
		status int
	}{
		{name: "platform owner", req: request("user-1", "platform_owner", ""), wantOK: true},
		{name: "platform admin", req: request("user-1", "platform_admin", ""), wantOK: true},
		{name: "tenant scoped platform", req: request("user-1", "platform_owner", "msp-1"), status: http.StatusForbidden},
		{name: "MSP owner", req: request("user-1", "msp_owner", "msp-1"), status: http.StatusForbidden},
		{name: "support", req: request("user-1", "platform_support", ""), status: http.StatusForbidden},
		{name: "unauthenticated", req: request("", "", ""), status: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := httptest.NewRecorder()
			if got := authorizeTopLevelPlatformRequest(out, test.req); got != test.wantOK {
				t.Fatalf("authorization = %v, want %v", got, test.wantOK)
			}
			if !test.wantOK && out.Code != test.status {
				t.Fatalf("status = %d, want %d", out.Code, test.status)
			}
		})
	}
}

func TestInvitationRoutesHaveExplicitAccessAndTransactionPolicy(t *testing.T) {
	s := &APIServer{}
	if got := s.classifyRoute(http.MethodPost, "/api/v1/auth/invitations/inspect"); got != AccessPublic {
		t.Fatalf("inspect access = %v", got)
	}
	if got := s.classifyRoute(http.MethodPost, "/api/v1/auth/invitations/accept"); got != AccessPublic {
		t.Fatalf("accept access = %v", got)
	}
	if got := s.classifyRoute(http.MethodPost, "/api/v2/platform/msps/id/owner-invitation"); got != AccessAdmin {
		t.Fatalf("resend access = %v", got)
	}
	if !ownerInvitationOwnsTransaction(httptest.NewRequest(http.MethodPost, "/api/v2/platform/msps", nil)) {
		t.Fatal("MSP creation must own its atomic transaction")
	}
}
