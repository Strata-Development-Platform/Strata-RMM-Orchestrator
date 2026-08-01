package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
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

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	ownerInvitationTTL = 72 * time.Hour
	invitationBodyMax  = 16 << 10
)

var (
	errInvitationInvalid          = errors.New("invitation is invalid or unavailable")
	errInvitationEmailRegistered  = errors.New("owner email is already registered")
	errInvitationAlreadyDelivered = errors.New("owner invitation was already delivered")
	errMSPSlugConflict            = errors.New("MSP slug already exists")
	errPlanUnavailable            = errors.New("plan is unavailable")
	errPlatformAuthorization      = errors.New("top-level platform administrator required")

	mspSlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

type OwnerInvitationService struct {
	db         *sql.DB
	mailer     AccountMailer
	publicURL  string
	random     io.Reader
	now        func() time.Time
	bcryptCost int
}

type createPendingMSPInput struct {
	Name       string
	Slug       string
	Plan       string
	OwnerEmail string
	ActorID    string
}

type ownerInvitationDelivery struct {
	MSPID          string
	InvitationID   string
	DeliveryStatus string
}

type inspectedOwnerInvitation struct {
	MSPName     string
	MaskedEmail string
	ExpiresAt   time.Time
}

func newOwnerInvitationService(db *sql.DB, mailer AccountMailer, publicURL string) *OwnerInvitationService {
	return &OwnerInvitationService{
		db:         db,
		mailer:     mailer,
		publicURL:  strings.TrimRight(strings.TrimSpace(publicURL), "/"),
		random:     rand.Reader,
		now:        time.Now,
		bcryptCost: bcrypt.DefaultCost,
	}
}

func (s *OwnerInvitationService) createPendingMSP(ctx context.Context, input createPendingMSPInput) (ownerInvitationDelivery, error) {
	var result ownerInvitationDelivery
	if s == nil || s.db == nil {
		return result, fmt.Errorf("identity database is unavailable")
	}
	var err error
	if input.Name, err = normalizeBoundedText(input.Name, 200); err != nil {
		return result, err
	}
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	if !mspSlugPattern.MatchString(input.Slug) {
		return result, fmt.Errorf("slug must be 1-63 lowercase letters, numbers, or hyphens")
	}
	input.Plan = strings.ToLower(strings.TrimSpace(input.Plan))
	if input.Plan == "" {
		input.Plan = "free"
	}
	if len(input.Plan) > 63 || containsControl(input.Plan) {
		return result, fmt.Errorf("plan is invalid")
	}
	emailNormalized, err := normalizeEmail(input.OwnerEmail)
	if err != nil {
		return result, err
	}
	rawToken, tokenHash, err := generateOwnerInvitationToken(s.random)
	if err != nil {
		return result, fmt.Errorf("generate owner invitation")
	}
	expiresAt := s.now().UTC().Add(ownerInvitationTTL)
	result.MSPID = uuid.NewString()
	result.InvitationID = uuid.NewString()

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("begin MSP creation")
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := authorizePlatformActor(ctx, tx, input.ActorID); err != nil {
		return ownerInvitationDelivery{}, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.role', 'platform_admin', true)`); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("establish platform security context")
	}
	var emailExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE normalized_email = $1)`, emailNormalized).Scan(&emailExists); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("validate owner identity")
	}
	if emailExists {
		return ownerInvitationDelivery{}, errInvitationEmailRegistered
	}
	var planID string
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM plans WHERE slug = $1 AND is_active = TRUE`, input.Plan).Scan(&planID); errors.Is(err, sql.ErrNoRows) {
		return ownerInvitationDelivery{}, errPlanUnavailable
	} else if err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("validate plan")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO msp_tenants (id, name, slug, plan, is_active, onboarding_status)
		VALUES ($1, $2, $3, $4, FALSE, 'pending_owner')
	`, result.MSPID, input.Name, input.Slug, input.Plan); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "slug") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ownerInvitationDelivery{}, errMSPSlugConflict
		}
		return ownerInvitationDelivery{}, fmt.Errorf("create pending MSP")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO plan_entitlements (msp_id, plan_id, status)
		VALUES ($1, $2, 'suspended')
	`, result.MSPID, planID); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("create suspended entitlement")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_invitations (
			id, msp_id, email_normalized, purpose, token_hash, created_by, expires_at
		) VALUES ($1, $2, $3, 'msp_owner_activation', $4, $5, $6)
	`, result.InvitationID, result.MSPID, emailNormalized, tokenHash, input.ActorID, expiresAt); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("create owner invitation")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO control_plane_audit (msp_id, actor_user_id, action, resource_type, resource_id, details)
		VALUES
			($1, $2, 'msp.created', 'msp', $1::uuid::text, jsonb_build_object('name', $3::text, 'slug', $4::text, 'plan', $5::text)),
			($1, $2, 'msp.owner_invitation_created', 'account_invitation', $6, '{}'::jsonb)
	`, result.MSPID, input.ActorID, input.Name, input.Slug, input.Plan, result.InvitationID); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("record MSP owner invitation audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("commit pending MSP")
	}
	committed = true
	result.DeliveryStatus = s.deliver(ctx, result.InvitationID, result.MSPID, input.Name, emailNormalized, rawToken, tokenHash, expiresAt)
	return result, nil
}

func (s *OwnerInvitationService) resend(ctx context.Context, mspID, actorID string) (ownerInvitationDelivery, error) {
	result := ownerInvitationDelivery{MSPID: mspID, InvitationID: uuid.NewString()}
	if _, err := uuid.Parse(mspID); err != nil {
		return ownerInvitationDelivery{}, errInvitationInvalid
	}
	rawToken, tokenHash, err := generateOwnerInvitationToken(s.random)
	if err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("generate owner invitation")
	}
	expiresAt := s.now().UTC().Add(ownerInvitationTTL)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("begin invitation rotation")
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := authorizePlatformActor(ctx, tx, actorID); err != nil {
		return ownerInvitationDelivery{}, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.role', 'platform_admin', true)`); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("establish platform security context")
	}
	var mspName, emailNormalized, invitationID, deliveryStatus string
	var currentExpires time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT m.name, invitation.email_normalized, invitation.id::text,
		       invitation.delivery_status, invitation.expires_at
		FROM msp_tenants m
		JOIN account_invitations invitation ON invitation.msp_id = m.id
		WHERE m.id = $1 AND m.onboarding_status = 'pending_owner' AND m.is_active = FALSE
		  AND invitation.accepted_at IS NULL AND invitation.revoked_at IS NULL
		FOR UPDATE OF invitation
	`, mspID).Scan(&mspName, &emailNormalized, &invitationID, &deliveryStatus, &currentExpires)
	if errors.Is(err, sql.ErrNoRows) {
		return ownerInvitationDelivery{}, errInvitationInvalid
	}
	if err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("lock owner invitation")
	}
	if deliveryStatus == "delivered" && currentExpires.After(s.now()) {
		return ownerInvitationDelivery{}, errInvitationAlreadyDelivered
	}
	var emailExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE normalized_email = $1)`, emailNormalized).Scan(&emailExists); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("validate owner identity")
	}
	if emailExists {
		return ownerInvitationDelivery{}, errInvitationEmailRegistered
	}
	if _, err := tx.ExecContext(ctx, `UPDATE account_invitations SET revoked_at = NOW() WHERE id = $1`, invitationID); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("revoke previous owner invitation")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_invitations (
			id, msp_id, email_normalized, purpose, token_hash, created_by, expires_at
		) VALUES ($1, $2, $3, 'msp_owner_activation', $4, $5, $6)
	`, result.InvitationID, mspID, emailNormalized, tokenHash, actorID, expiresAt); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("rotate owner invitation")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO control_plane_audit (msp_id, actor_user_id, action, resource_type, resource_id, details)
		VALUES ($1, $2, 'msp.owner_invitation_resent', 'account_invitation', $3, '{}'::jsonb)
	`, mspID, actorID, result.InvitationID); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("record invitation rotation audit")
	}
	if err := tx.Commit(); err != nil {
		return ownerInvitationDelivery{}, fmt.Errorf("commit invitation rotation")
	}
	committed = true
	result.DeliveryStatus = s.deliver(ctx, result.InvitationID, mspID, mspName, emailNormalized, rawToken, tokenHash, expiresAt)
	return result, nil
}

func (s *OwnerInvitationService) inspect(ctx context.Context, rawToken string) (inspectedOwnerInvitation, error) {
	var inspected inspectedOwnerInvitation
	tokenHash, err := digestOwnerInvitationToken(rawToken)
	if err != nil || s == nil || s.db == nil {
		return inspected, errInvitationInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return inspected, fmt.Errorf("inspect invitation")
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.invitation_hash', $1, true)`, tokenHash); err != nil {
		return inspected, fmt.Errorf("inspect invitation")
	}
	var email string
	err = tx.QueryRowContext(ctx, `
		SELECT m.name, invitation.email_normalized, invitation.expires_at
		FROM account_invitations invitation
		JOIN msp_tenants m ON m.id = invitation.msp_id
		WHERE invitation.token_hash = $1
		  AND invitation.purpose = 'msp_owner_activation'
		  AND invitation.accepted_at IS NULL
		  AND invitation.revoked_at IS NULL
		  AND invitation.expires_at > NOW()
		  AND m.onboarding_status = 'pending_owner'
		  AND m.is_active = FALSE
	`, tokenHash).Scan(&inspected.MSPName, &email, &inspected.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return inspected, errInvitationInvalid
	}
	if err != nil {
		return inspected, fmt.Errorf("inspect invitation")
	}
	inspected.MaskedEmail = maskEmail(email)
	return inspected, nil
}

func (s *OwnerInvitationService) accept(ctx context.Context, rawToken, password string) error {
	if err := validateOwnerPassword(password); err != nil {
		return err
	}
	tokenHash, err := digestOwnerInvitationToken(rawToken)
	if err != nil || s == nil || s.db == nil {
		return errInvitationInvalid
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("secure password processing failed")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin owner activation")
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.invitation_hash', $1, true)`, tokenHash); err != nil {
		return fmt.Errorf("establish invitation security context")
	}
	var invitationID, mspID, emailNormalized string
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT invitation.id::text, invitation.msp_id::text,
		       invitation.email_normalized, invitation.expires_at
		FROM account_invitations invitation
		JOIN msp_tenants m ON m.id = invitation.msp_id
		JOIN plan_entitlements entitlement ON entitlement.msp_id = m.id
		WHERE invitation.token_hash = $1
		  AND invitation.purpose = 'msp_owner_activation'
		  AND invitation.accepted_at IS NULL
		  AND invitation.revoked_at IS NULL
		  AND m.onboarding_status = 'pending_owner'
		  AND m.is_active = FALSE
		  AND entitlement.status = 'suspended'
		FOR UPDATE OF invitation
	`, tokenHash).Scan(&invitationID, &mspID, &emailNormalized, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return errInvitationInvalid
	}
	if err != nil {
		return fmt.Errorf("lock owner invitation")
	}
	if !expiresAt.After(s.now()) {
		return errInvitationInvalid
	}
	var userExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE normalized_email = $1)`, emailNormalized).Scan(&userExists); err != nil {
		return fmt.Errorf("validate owner identity")
	}
	if userExists {
		return errInvitationEmailRegistered
	}
	userID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (
			id, tenant_id, email, password_hash, role, is_active, email_verified_at
		) VALUES ($1, NULL, $2, $3, 'viewer', TRUE, NOW())
	`, userID, emailNormalized, string(passwordHash)); err != nil {
		return errInvitationEmailRegistered
	}
	if _, err := tx.ExecContext(ctx, `
		SELECT set_config('app.user_id', $1, true),
		       set_config('app.msp_id', $2, true),
		       set_config('app.permission', 'write', true)
	`, userID, mspID); err != nil {
		return fmt.Errorf("establish owner security context")
	}
	var existingOwners int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM memberships
		WHERE scope_type = 'msp' AND scope_id = $1
		  AND role = 'msp_owner' AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > NOW())
	`, mspID).Scan(&existingOwners); err != nil {
		return fmt.Errorf("validate MSP ownership")
	}
	if existingOwners != 0 {
		return errInvitationInvalid
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO memberships (user_id, role, scope_type, scope_id, created_by, status)
		VALUES ($1, 'msp_owner', 'msp', $2, $1, 'active')
	`, userID, mspID); err != nil {
		return fmt.Errorf("activate owner membership")
	}
	if affected, err := execOne(ctx, tx, `
		UPDATE msp_tenants
		SET is_active = TRUE, onboarding_status = 'active', updated_at = NOW()
		WHERE id = $1 AND is_active = FALSE AND onboarding_status = 'pending_owner'
	`, mspID); err != nil || !affected {
		return fmt.Errorf("activate MSP")
	}
	if affected, err := execOne(ctx, tx, `
		UPDATE plan_entitlements
		SET status = 'active', updated_at = NOW()
		WHERE msp_id = $1 AND status = 'suspended'
	`, mspID); err != nil || !affected {
		return fmt.Errorf("activate entitlement")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO control_plane_audit (msp_id, actor_user_id, action, resource_type, resource_id, details)
		VALUES ($1, $2, 'msp.owner_activated', 'msp', $1::uuid::text, '{}'::jsonb)
	`, mspID, userID); err != nil {
		return fmt.Errorf("record owner activation audit")
	}
	if affected, err := execOne(ctx, tx, `
		UPDATE account_invitations SET accepted_at = NOW()
		WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL
	`, invitationID); err != nil || !affected {
		return errInvitationInvalid
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit owner activation")
	}
	committed = true
	return nil
}

func authorizePlatformActor(ctx context.Context, tx *sql.Tx, actorID string) error {
	if _, err := uuid.Parse(actorID); err != nil {
		return errPlatformAuthorization
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.user_id', $1, true)`, actorID); err != nil {
		return errPlatformAuthorization
	}
	var authorized bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM users u
			JOIN memberships membership ON membership.user_id = u.id::text
			WHERE u.id = $1 AND u.is_active = TRUE AND u.email_verified_at IS NOT NULL
			  AND membership.scope_type = 'platform'
			  AND membership.scope_id = '00000000-0000-0000-0000-000000000001'
			  AND membership.role IN ('platform_owner', 'platform_admin')
			  AND membership.status = 'active'
			  AND (membership.expires_at IS NULL OR membership.expires_at > NOW())
		)
	`, actorID).Scan(&authorized); err != nil || !authorized {
		return errPlatformAuthorization
	}
	return nil
}

func (s *OwnerInvitationService) deliver(ctx context.Context, invitationID, mspID, mspName, recipient, rawToken, tokenHash string, expiresAt time.Time) string {
	status := "unconfigured"
	if s.mailer != nil && s.publicURL != "" {
		activationURL := s.publicURL + "/activate-account#" + rawToken
		if err := s.mailer.SendOwnerActivation(ctx, OwnerActivationMail{
			Recipient: recipient, MSPName: mspName, ActivationURL: activationURL,
			Token: rawToken, ExpiresAt: expiresAt,
		}); err != nil {
			status = "failed"
		} else {
			status = "delivered"
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "pending"
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.invitation_hash', $1, true)`, tokenHash); err != nil {
		return "pending"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE account_invitations
		SET delivery_status = $2,
		    delivered_at = CASE WHEN $2 = 'delivered' THEN NOW() ELSE NULL END
		WHERE id = $1 AND msp_id = $3 AND accepted_at IS NULL AND revoked_at IS NULL
	`, invitationID, status, mspID); err != nil {
		return "pending"
	}
	if err := tx.Commit(); err != nil {
		return "pending"
	}
	return status
}

func generateOwnerInvitationToken(random io.Reader) (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(random, bytes); err != nil {
		return "", "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(bytes)
	digest := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(digest[:]), nil
}

func digestOwnerInvitationToken(raw string) (string, error) {
	if raw == "" || len(raw) > 128 || containsControl(raw) {
		return "", errInvitationInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return "", errInvitationInvalid
	}
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:]), nil
}

func normalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > 320 || containsControl(trimmed) {
		return "", fmt.Errorf("owner_email is invalid")
	}
	address, err := mail.ParseAddress(trimmed)
	if err != nil || !strings.EqualFold(address.Address, trimmed) {
		return "", fmt.Errorf("owner_email is invalid")
	}
	return strings.ToLower(address.Address), nil
}

func normalizeBoundedText(raw string, maxBytes int) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxBytes || containsControl(value) {
		return "", fmt.Errorf("name is invalid")
	}
	return value, nil
}

func validateOwnerPassword(password string) error {
	if len(password) < 14 || len(password) > 72 {
		return fmt.Errorf("password must be 14-72 bytes")
	}
	if containsControl(password) {
		return fmt.Errorf("password must not contain control characters")
	}
	return nil
}

func containsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***"
	}
	local := "*"
	if parts[0] != "" {
		local = string([]rune(parts[0])[0]) + "***"
	}
	domainParts := strings.Split(parts[1], ".")
	domain := "***"
	if domainParts[0] != "" {
		domain = string([]rune(domainParts[0])[0]) + "***"
	}
	if len(domainParts) > 1 {
		domain += "." + domainParts[len(domainParts)-1]
	}
	return local + "@" + domain
}

func execOne(ctx context.Context, tx *sql.Tx, query string, args ...interface{}) (bool, error) {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func activationOrigin(publicURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("public URL is invalid")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func (s *APIServer) handleInspectOwnerInvitation(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, invitationBodyMax)
	var request struct {
		Token string `json:"token"`
	}
	if err := decodeStrictJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "invitation service unavailable"})
		return
	}
	inspected, err := newOwnerInvitationService(s.db.DB(), nil, "").inspect(r.Context(), request.Token)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invitation is invalid or unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"msp": map[string]string{
			"name": inspected.MSPName,
		},
		"masked_email": inspected.MaskedEmail,
		"expires_at":   inspected.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleAcceptOwnerInvitation(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, invitationBodyMax)
	var request struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeStrictJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "invitation service unavailable"})
		return
	}
	err := newOwnerInvitationService(s.db.DB(), nil, "").accept(r.Context(), request.Token, request.Password)
	if err != nil {
		writeOwnerInvitationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func authorizeTopLevelPlatformRequest(w http.ResponseWriter, r *http.Request) bool {
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authorization required"})
		return false
	}
	if !isPlatformGlobal(getRoles(r)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "top-level platform administrator required"})
		return false
	}
	for _, key := range []contextKey{ctxKeyMSPID, ctxKeyClientID, ctxKeySiteID} {
		if scope, _ := r.Context().Value(key).(string); scope != "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "top-level platform session required"})
			return false
		}
	}
	return true
}

func writeOwnerInvitationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errPlatformAuthorization):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "top-level platform administrator required"})
	case errors.Is(err, errInvitationEmailRegistered):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "owner email is already registered"})
	case errors.Is(err, errMSPSlugConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "msp slug already exists"})
	case errors.Is(err, errInvitationAlreadyDelivered):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "owner invitation is still valid and delivered"})
	case errors.Is(err, errInvitationInvalid):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invitation is invalid or unavailable"})
	case errors.Is(err, errPlanUnavailable):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown or inactive plan"})
	case strings.HasPrefix(err.Error(), "password must"):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case err.Error() == "owner_email is invalid" || err.Error() == "name is invalid" ||
		strings.HasPrefix(err.Error(), "slug must") || err.Error() == "plan is invalid":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "owner activation operation failed"})
	}
}
