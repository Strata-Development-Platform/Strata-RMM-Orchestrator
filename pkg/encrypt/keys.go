package encrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

var ErrKeyNotFound = errors.New("encryption key not found")

type KMSProvider string

const (
	KMSLocal  KMSProvider = "local"
	KMSAWS    KMSProvider = "aws_kms"
	KMSGCP    KMSProvider = "gcp_kms"
	KMSAzure  KMSProvider = "azure_kv"
)

type EncryptionScheme string

const (
	AES256GCM EncryptionScheme = "aes-256-gcm"
	AES256CBC EncryptionScheme = "aes-256-cbc"
	SSES3     EncryptionScheme = "sse-s3"
	SSEKMS    EncryptionScheme = "sse-kms"
)

type TenantKey struct {
	ID          string           `json:"id"`
	TenantID    string           `json:"tenant_id"`
	KeyAlias    string           `json:"key_alias"`
	KMSProvider KMSProvider      `json:"kms_type"`
	KMSKeyID    string           `json:"kms_key_id,omitempty"`
	Encryption  EncryptionScheme `json:"encryption"`
	KeyMaterial []byte           `json:"-"`
	Region      string           `json:"region,omitempty"`
	Endpoint    string           `json:"endpoint,omitempty"`
	Status      string           `json:"status"`
	CreatedAt   time.Time        `json:"created_at"`
	RotatedAt   *time.Time       `json:"rotated_at,omitempty"`
	ExpiresAt   *time.Time       `json:"expires_at,omitempty"`
}

type KeyStore struct {
	db *sql.DB
}

func NewKeyStore(db *sql.DB) *KeyStore {
	return &KeyStore{db: db}
}

func (s *KeyStore) CreateKey(ctx context.Context, tenantID string, opts CreateKeyOptions) (*TenantKey, error) {
	keyMaterial := make([]byte, 32)
	if _, err := rand.Read(keyMaterial); err != nil {
		return nil, fmt.Errorf("generate key material: %w", err)
	}

	if opts.KMSProvider == "" {
		opts.KMSProvider = KMSLocal
	}
	if opts.Encryption == "" {
		opts.Encryption = AES256GCM
	}
	if opts.KeyAlias == "" {
		opts.KeyAlias = "primary"
	}

	var key TenantKey
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO tenant_encryption_keys (tenant_id, key_alias, kms_type, encryption, key_material, region, endpoint)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, tenant_id, key_alias, kms_type, encryption, kms_key_id, region, endpoint, status, created_at, rotated_at, expires_at
	`, tenantID, opts.KeyAlias, opts.KMSProvider, opts.Encryption, keyMaterial, opts.Region, opts.Endpoint).Scan(
		&key.ID, &key.TenantID, &key.KeyAlias, &key.KMSProvider, &key.Encryption,
		&key.KMSKeyID, &key.Region, &key.Endpoint, &key.Status, &key.CreatedAt, &key.RotatedAt, &key.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create key: %w", err)
	}

	key.KeyMaterial = keyMaterial
	return &key, nil
}

func (s *KeyStore) GetActiveKey(ctx context.Context, tenantID string) (*TenantKey, error) {
	var key TenantKey
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, key_alias, kms_type, encryption, kms_key_id, key_material, region, endpoint, status, created_at, rotated_at, expires_at
		FROM tenant_encryption_keys
		WHERE tenant_id = $1 AND status = 'active'
		ORDER BY created_at DESC LIMIT 1
	`, tenantID).Scan(
		&key.ID, &key.TenantID, &key.KeyAlias, &key.KMSProvider, &key.Encryption,
		&key.KMSKeyID, &key.KeyMaterial, &key.Region, &key.Endpoint, &key.Status,
		&key.CreatedAt, &key.RotatedAt, &key.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w for tenant %s", ErrKeyNotFound, tenantID)
		}
		return nil, fmt.Errorf("get active key: %w", err)
	}
	return &key, nil
}

func (s *KeyStore) RotateKey(ctx context.Context, tenantID string) (*TenantKey, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tenant_encryption_keys SET status = 'rotating', rotated_at = NOW()
		WHERE tenant_id = $1 AND status = 'active'
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("deactivate old key: %w", err)
	}

	return s.CreateKey(ctx, tenantID, CreateKeyOptions{
		KeyAlias:    "primary",
		KMSProvider: KMSLocal,
		Encryption:  AES256GCM,
	})
}

func (s *KeyStore) RevokeKey(ctx context.Context, keyID, tenantID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tenant_encryption_keys SET status = 'compromised', updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
	`, keyID, tenantID)
	return err
}

func (s *KeyStore) ListKeys(ctx context.Context, tenantID string) ([]*TenantKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, key_alias, kms_type, encryption, kms_key_id, region, endpoint, status, created_at, rotated_at, expires_at
		FROM tenant_encryption_keys
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*TenantKey
	for rows.Next() {
		var key TenantKey
		if err := rows.Scan(
			&key.ID, &key.TenantID, &key.KeyAlias, &key.KMSProvider, &key.Encryption,
			&key.KMSKeyID, &key.Region, &key.Endpoint, &key.Status,
			&key.CreatedAt, &key.RotatedAt, &key.ExpiresAt,
		); err != nil {
			continue
		}
		keys = append(keys, &key)
	}
	return keys, nil
}

type CreateKeyOptions struct {
	KeyAlias    string
	KMSProvider KMSProvider
	Encryption  EncryptionScheme
	Region      string
	Endpoint    string
}

type Encryptor struct {
	key *TenantKey
}

func NewEncryptor(key *TenantKey) *Encryptor {
	return &Encryptor{key: key}
}

func (e *Encryptor) Encrypt(plaintext []byte) (string, error) {
	switch e.key.Encryption {
	case AES256GCM:
		return e.encryptGCM(plaintext)
	default:
		return "", fmt.Errorf("unsupported encryption scheme: %s", e.key.Encryption)
	}
}

func (e *Encryptor) Decrypt(ciphertext string) ([]byte, error) {
	switch e.key.Encryption {
	case AES256GCM:
		return e.decryptGCM(ciphertext)
	default:
		return nil, fmt.Errorf("unsupported encryption scheme: %s", e.key.Encryption)
	}
}

func (e *Encryptor) encryptGCM(plaintext []byte) (string, error) {
	key := deriveKey(e.key.KeyMaterial)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *Encryptor) decryptGCM(ciphertext string) ([]byte, error) {
	key := deriveKey(e.key.KeyMaterial)
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := data[:nonceSize]
	ct := data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func deriveKey(material []byte) []byte {
	if len(material) >= 32 {
		return material[:32]
	}
	h := sha256.Sum256(material)
	return h[:]
}

func GenerateEncryptionKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return hex.EncodeToString(key), nil
}
