package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type Claims struct {
	TenantID   string   `json:"tid"`
	AgentID    string   `json:"aid"`
	Roles      []string `json:"roles"`
	ExpiresAt  int64    `json:"exp"`
	IssuedAt   int64    `json:"iat"`
}

type TokenGenerator struct {
	secret []byte
}

func NewTokenGenerator(secret string) *TokenGenerator {
	return &TokenGenerator{secret: []byte(secret)}
}

func (g *TokenGenerator) GenerateAgentToken(tenantID, agentID string, ttl time.Duration) (string, error) {
	claims := Claims{
		TenantID:  tenantID,
		AgentID:   agentID,
		Roles:     []string{"agent"},
		ExpiresAt: time.Now().Add(ttl).Unix(),
		IssuedAt:  time.Now().Unix(),
	}
	return g.encode(claims)
}

func (g *TokenGenerator) GenerateUserToken(tenantID string, roles []string, ttl time.Duration) (string, error) {
	claims := Claims{
		TenantID:  tenantID,
		Roles:     roles,
		ExpiresAt: time.Now().Add(ttl).Unix(),
		IssuedAt:  time.Now().Unix(),
	}
	return g.encode(claims)
}

func (g *TokenGenerator) Validate(token string) (*Claims, error) {
	parts := tokenize(token, '.')
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	sig := g.sign(parts[0] + "." + parts[1])
	if sig != parts[2] {
		return nil, fmt.Errorf("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding payload: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshaling claims: %w", err)
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func (g *TokenGenerator) encode(claims Claims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshaling claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)

	sig := g.sign(header + "." + payload)

	return header + "." + payload + "." + sig, nil
}

func (g *TokenGenerator) sign(data string) string {
	mac := hmac.New(sha256.New, g.secret)
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type EnrollmentToken struct {
	Token     string    `json:"token"`
	TenantID  string    `json:"tenant_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

type EnrollmentManager struct {
	generator *TokenGenerator
	tokens    map[string]*EnrollmentToken
}

func NewEnrollmentManager(secret string) *EnrollmentManager {
	return &EnrollmentManager{
		generator: NewTokenGenerator(secret),
		tokens:    make(map[string]*EnrollmentToken),
	}
}

func (m *EnrollmentManager) CreateEnrollmentToken(tenantID string, ttl time.Duration) (*EnrollmentToken, error) {
	token := fmt.Sprintf("enr_%s_%d", tenantID, time.Now().UnixNano())
	et := &EnrollmentToken{
		Token:     token,
		TenantID:  tenantID,
		ExpiresAt: time.Now().Add(ttl),
	}
	m.tokens[token] = et
	return et, nil
}

func (m *EnrollmentManager) ValidateEnrollmentToken(token string) (string, error) {
	et, ok := m.tokens[token]
	if !ok {
		return "", fmt.Errorf("token not found")
	}
	if et.Used {
		return "", fmt.Errorf("token already used")
	}
	if time.Now().After(et.ExpiresAt) {
		return "", fmt.Errorf("token expired")
	}
	et.Used = true
	return et.TenantID, nil
}

func tokenize(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
