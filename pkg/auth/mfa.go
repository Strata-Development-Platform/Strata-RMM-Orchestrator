package auth

import (
	"database/sql"
	"fmt"
	"time"
)

type MFAStore struct {
	db *sql.DB
}

type MFASecret struct {
	UserID    string    `json:"user_id"`
	TenantID  string    `json:"tenant_id"`
	Secret    string    `json:"-"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func NewMFAStore(db *sql.DB) *MFAStore {
	return &MFAStore{db: db}
}

func (s *MFAStore) Create(userID, tenantID, secret string) error {
	_, err := s.db.Exec(`
		INSERT INTO mfa_secrets (user_id, tenant_id, secret, enabled)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (user_id) DO UPDATE SET
			secret = EXCLUDED.secret,
			enabled = true,
			updated_at = NOW()
	`, userID, tenantID, secret)
	if err != nil {
		return fmt.Errorf("create mfa secret: %w", err)
	}
	return nil
}

func (s *MFAStore) GetByUserID(userID string) (*MFASecret, error) {
	var m MFASecret
	err := s.db.QueryRow(`
		SELECT user_id, tenant_id, secret, enabled, created_at
		FROM mfa_secrets WHERE user_id = $1
	`, userID).Scan(&m.UserID, &m.TenantID, &m.Secret, &m.Enabled, &m.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("mfa not configured for user %s", userID)
		}
		return nil, fmt.Errorf("get mfa secret: %w", err)
	}
	return &m, nil
}

func (s *MFAStore) Disable(userID string) error {
	_, err := s.db.Exec(`UPDATE mfa_secrets SET enabled = false, updated_at = NOW() WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("disable mfa: %w", err)
	}
	return nil
}

func (s *MFAStore) IsEnabled(userID string) (bool, error) {
	var enabled bool
	err := s.db.QueryRow(`SELECT enabled FROM mfa_secrets WHERE user_id = $1`, userID).Scan(&enabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("check mfa enabled: %w", err)
	}
	return enabled, nil
}
