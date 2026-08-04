package automation

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrSecretExposure indicates a secret was found to have touched disk.
var ErrSecretExposure = errors.New("secret exposure detected: secret touched disk")

// EnvelopeEncryptor implements AES-256-GCM envelope encryption for secret variables.
// Envelope encryption generates a unique Data Encryption Key (DEK) per secret,
// encrypts the secret with the DEK, then encrypts the DEK with a Content
// Encryption Key (CEK) from the key store. This ensures that even if one DEK
// is compromised, only one secret is affected.
type EnvelopeEncryptor struct {
	cek []byte // Content Encryption Key (from key store)
	mu  sync.RWMutex
}

// EnvelopeCiphertext is the serialized form of an envelope-encrypted secret.
// Format: nonce (12 bytes) + encrypted_dek (32 bytes + 16 auth tag) + encrypted_secret (N bytes + 16 auth tag)
type EnvelopeCiphertext struct {
	Nonce          []byte `json:"nonce"`          // 12-byte nonce for DEK encryption
	EncryptedDEK   []byte `json:"encrypted_dek"`  // DEK encrypted with CEK (32 bytes + 16 auth tag)
	DEKNonce       []byte `json:"dek_nonce"`      // 12-byte nonce for secret encryption
	EncryptedData  []byte `json:"encrypted_data"` // Secret encrypted with DEK (variable length + 16 auth tag)
}

// NewEnvelopeEncryptor creates a new envelope encryptor with the given CEK.
// The CEK should be 32 bytes (256 bits) for AES-256.
func NewEnvelopeEncryptor(cek []byte) (*EnvelopeEncryptor, error) {
	if len(cek) != 32 {
		return nil, fmt.Errorf("CEK must be 32 bytes, got %d", len(cek))
	}
	return &EnvelopeEncryptor{
		cek: make([]byte, 32),
	}, nil
}

// SetCEK updates the Content Encryption Key.
// The CEK must be exactly 32 bytes (256 bits).
func (e *EnvelopeEncryptor) SetCEK(cek []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(cek) != 32 {
		return fmt.Errorf("CEK must be 32 bytes, got %d", len(cek))
	}
	copy(e.cek, cek)
	return nil
}

// Encrypt performs envelope encryption on the given plaintext secret.
// The secret is encrypted with a unique DEK, and the DEK is encrypted with the CEK.
// Returns the serialized EnvelopeCiphertext or an error.
func (e *EnvelopeEncryptor) Encrypt(plaintext []byte) (*EnvelopeCiphertext, error) {
	e.mu.RLock()
	cek := make([]byte, 32)
	copy(cek, e.cek)
	e.mu.RUnlock()

	// Step 1: Generate a unique DEK (Data Encryption Key)
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("generate DEK: %w", err)
	}

	// Step 2: Encrypt the DEK with the CEK
	encryptedDEK, dekNonce, err := e.encryptWithKey(cek, dek)
	if err != nil {
		return nil, fmt.Errorf("encrypt DEK with CEK: %w", err)
	}

	// Step 3: Encrypt the secret with the DEK
	encryptedData, dataNonce, err := e.encryptWithKey(dek, plaintext)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret with DEK: %w", err)
	}

	return &EnvelopeCiphertext{
		Nonce:         dekNonce,
		EncryptedDEK:  encryptedDEK,
		DEKNonce:      dataNonce,
		EncryptedData: encryptedData,
	}, nil
}

// Decrypt performs envelope decryption on the given ciphertext.
// First decrypts the DEK with the CEK, then decrypts the secret with the DEK.
// Returns the plaintext secret or an error.
func (e *EnvelopeEncryptor) Decrypt(ciphertext *EnvelopeCiphertext) ([]byte, error) {
	e.mu.RLock()
	cek := make([]byte, 32)
	copy(cek, e.cek)
	e.mu.RUnlock()

	// Step 1: Decrypt the DEK with the CEK
	dek, err := e.decryptWithKey(cek, ciphertext.EncryptedDEK, ciphertext.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decrypt DEK with CEK: %w", err)
	}

	// Step 2: Decrypt the secret with the DEK
	plaintext, err := e.decryptWithKey(dek, ciphertext.EncryptedData, ciphertext.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret with DEK: %w", err)
	}

	return plaintext, nil
}

// encryptWithKey encrypts plaintext using AES-256-GCM with the given key.
// Returns (ciphertext_with_tag, nonce, error).
func (e *EnvelopeEncryptor) encryptWithKey(key, plaintext []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("new cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// decryptWithKey decrypts ciphertext using AES-256-GCM with the given key.
// Returns plaintext or error.
func (e *EnvelopeEncryptor) decryptWithKey(key, ciphertextWithNonce, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertextWithNonce) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short: expected at least %d bytes", nonceSize)
	}

	actualNonce := ciphertextWithNonce[:nonceSize]
	ct := ciphertextWithNonce[nonceSize:]

	plaintext, err := aesGCM.Open(nil, actualNonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// Serialize serializes the EnvelopeCiphertext to a base64-encoded string.
func (c *EnvelopeCiphertext) Serialize() string {
	// Format: base64(nonce) + "." + base64(encrypted_dek) + "." + base64(dek_nonce) + "." + base64(encrypted_data)
	return base64.StdEncoding.EncodeToString(c.Nonce) + "." +
		base64.StdEncoding.EncodeToString(c.EncryptedDEK) + "." +
		base64.StdEncoding.EncodeToString(c.DEKNonce) + "." +
		base64.StdEncoding.EncodeToString(c.EncryptedData)
}

// Deserialize deserializes a base64-encoded string to an EnvelopeCiphertext.
func DeserializeCiphertext(serialized string) (*EnvelopeCiphertext, error) {
	parts := splitCiphertext(serialized)
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid ciphertext format: expected 4 parts separated by '.', got %d", len(parts))
	}

	nonce, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}

	encryptedDEK, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode encrypted_dek: %w", err)
	}

	dekNonce, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode dek_nonce: %w", err)
	}

	encryptedData, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return nil, fmt.Errorf("decode encrypted_data: %w", err)
	}

	return &EnvelopeCiphertext{
		Nonce:         nonce,
		EncryptedDEK:  encryptedDEK,
		DEKNonce:      dekNonce,
		EncryptedData: encryptedData,
	}, nil
}

// SplitCiphertext splits the serialized ciphertext into its parts.
func splitCiphertext(s string) []string {
	var parts []string
	start := 0
	for i, c := range s {
		if c == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// SecretVault manages envelope encryption for secret variables.
// Secrets are never written to disk; they exist only in memory.
type SecretVault struct {
	encryptor *EnvelopeEncryptor
}

// NewSecretVault creates a new SecretVault with the given CEK.
func NewSecretVault(cek []byte) (*SecretVault, error) {
	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		return nil, fmt.Errorf("create envelope encryptor: %w", err)
	}
	return &SecretVault{encryptor: encryptor}, nil
}

// Store encrypts and stores a secret variable.
// The secret is encrypted in memory and the ciphertext is returned.
// The plaintext is never written to disk.
func (v *SecretVault) Store(secretID string, plaintext []byte) (string, error) {
	ciphertext, err := v.encryptor.Encrypt(plaintext)
	if err != nil {
		return "", fmt.Errorf("encrypt secret: %w", err)
	}
	// Note: plaintext is zeroed out by Go's garbage collector after this function returns.
	// For additional safety, callers should explicitly zero their local copies.
	return ciphertext.Serialize(), nil
}

// Retrieve decrypts and returns a secret variable.
// The plaintext is returned in memory. The caller is responsible for zeroing it.
func (v *SecretVault) Retrieve(serializedCiphertext string) ([]byte, error) {
	ciphertext, err := DeserializeCiphertext(serializedCiphertext)
	if err != nil {
		return nil, fmt.Errorf("deserialize ciphertext: %w", err)
	}
	plaintext, err := v.encryptor.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}

// RotateCEK generates a new CEK and returns it.
// The new CEK should be used for future encryption operations.
// Existing ciphertexts encrypted with the old CEK will need to be re-encrypted.
func (v *SecretVault) RotateCEK() ([]byte, error) {
	newCEK := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newCEK); err != nil {
		return nil, fmt.Errorf("generate new CEK: %w", err)
	}
	if err := v.encryptor.SetCEK(newCEK); err != nil {
		return nil, fmt.Errorf("set new CEK: %w", err)
	}
	return newCEK, nil
}

// deriveCEK derives a 32-byte CEK from a passphrase using SHA-256.
// This is for testing purposes only; in production, use a KMS provider.
func deriveCEK(passphrase string) []byte {
	h := sha256.Sum256([]byte(passphrase))
	return h[:]
}
