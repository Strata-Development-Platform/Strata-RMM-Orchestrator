package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type TOTPManager struct{}

func NewTOTPManager() *TOTPManager {
	return &TOTPManager{}
}

func (m *TOTPManager) GenerateSecret() (string, error) {
	key := make([]byte, 20)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key), nil
}

func (m *TOTPManager) ProvisioningURI(secret, email, issuer string) string {
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", issuer)
	params.Set("algorithm", "SHA1")
	params.Set("digits", "6")
	params.Set("period", "30")
	return fmt.Sprintf("otpauth://totp/%s:%s?%s",
		url.PathEscape(issuer),
		url.PathEscape(email),
		params.Encode(),
	)
}

func (m *TOTPManager) GenerateCode(secret string, t time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	counter := uint64(t.Unix()) / 30
	return m.generateCode(key, counter), nil
}

func (m *TOTPManager) ValidateCode(secret, code string, t time.Time) (bool, error) {
	// Allow ±1 window (30s skew tolerance)
	for offset := -1; offset <= 1; offset++ {
		adjusted := t.Add(time.Duration(offset*30) * time.Second)
		expected, err := m.GenerateCode(secret, adjusted)
		if err != nil {
			return false, err
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true, nil
		}
	}
	return false, nil
}

func (m *TOTPManager) generateCode(key []byte, counter uint64) string {
	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(counterBytes)
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code := truncated % 1000000

	return fmt.Sprintf("%06d", code)
}

func (m *TOTPManager) ValidateCodeStrict(secret, code string, t time.Time, window int) (bool, error) {
	for offset := -window; offset <= window; offset++ {
		adjusted := t.Add(time.Duration(offset*30) * time.Second)
		expected, err := m.GenerateCode(secret, adjusted)
		if err != nil {
			return false, err
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true, nil
		}
	}
	return false, nil
}
