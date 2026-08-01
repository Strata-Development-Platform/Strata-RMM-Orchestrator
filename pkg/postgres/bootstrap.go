package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const bootstrapLockID int64 = DefaultLockID + 2

var ErrAlreadyBootstrapped = errors.New("initial administrator already exists")

type BootstrapAdminInput struct {
	Email      string
	Password   string
	TenantName string
}

func (in BootstrapAdminInput) validate() error {
	address, err := mail.ParseAddress(strings.TrimSpace(in.Email))
	if err != nil || !strings.EqualFold(address.Address, strings.TrimSpace(in.Email)) {
		return fmt.Errorf("administrator email is invalid")
	}
	if len(in.Password) < 14 {
		return fmt.Errorf("administrator password must be at least 14 characters")
	}
	if len(in.Password) > 72 {
		return fmt.Errorf("administrator password must not exceed 72 bytes")
	}
	if strings.TrimSpace(in.TenantName) == "" {
		return fmt.Errorf("platform tenant name is required")
	}
	return nil
}

// BootstrapInitialAdmin creates the one and only first local administrator.
// It is serialized by a transaction-scoped advisory lock and fails closed after
// any user exists. It must only be called through the local bootstrap command.
func BootstrapInitialAdmin(ctx context.Context, db *sql.DB, in BootstrapAdminInput) (string, error) {
	if db == nil {
		return "", fmt.Errorf("database is required")
	}
	if err := in.validate(); err != nil {
		return "", err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash administrator password: %w", err)
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", fmt.Errorf("begin bootstrap transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", bootstrapLockID); err != nil {
		return "", fmt.Errorf("acquire bootstrap lock: %w", err)
	}

	var userCount int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		return "", fmt.Errorf("check existing users: %w", err)
	}
	if userCount != 0 {
		return "", ErrAlreadyBootstrapped
	}

	var tenantID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO tenants (name, slug, plan)
		VALUES ($1, 'platform', 'enterprise')
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`, strings.TrimSpace(in.TenantName)).Scan(&tenantID)
	if err != nil {
		return "", fmt.Errorf("create platform tenant: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		SELECT
			set_config('app.tenant_id', $1, true),
			set_config('app.msp_id', $1, true),
			set_config('app.role', 'platform_admin', true)
	`, tenantID); err != nil {
		return "", fmt.Errorf("set bootstrap security context: %w", err)
	}

	var userID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (tenant_id, email, password_hash, role, email_verified_at)
		VALUES ($1, lower($2), $3, 'admin', NOW())
		RETURNING id::text
	`, tenantID, strings.TrimSpace(in.Email), string(passwordHash)).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("create initial administrator: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO memberships (
			user_id, role, scope_type, scope_id, created_by, status
		)
		VALUES (
			$1, 'platform_owner', 'platform',
			'00000000-0000-0000-0000-000000000001', $1, 'active'
		)
		ON CONFLICT (user_id, scope_type, scope_id, role)
			WHERE status = 'active'
		DO NOTHING
	`, userID); err != nil {
		return "", fmt.Errorf("grant initial platform owner membership: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_log (tenant_id, user_id, action, resource, details)
		VALUES ($1, $2, 'platform.bootstrap_admin', 'user', '{"source":"local-installer"}'::jsonb)
	`, tenantID, userID); err != nil {
		return "", fmt.Errorf("record bootstrap audit event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit bootstrap transaction: %w", err)
	}
	committed = true
	return userID, nil
}
