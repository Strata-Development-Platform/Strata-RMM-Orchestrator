package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"crypto/subtle"
	"fmt"
	"os"
	"time"
)

const (
	issuer     = "strata-rmm"
	audience   = "strata-rmm-api"
	minSecretLen = 32
)

func jwtSecret() string {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return s
	}
	return ""
}

func ValidateJWTConfig() error {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return fmt.Errorf("JWT_SECRET environment variable is required")
	}
	if len(secret) < minSecretLen {
		return fmt.Errorf("JWT_SECRET must be at least %d characters", minSecretLen)
	}
	return nil
}

type Claims struct {
	Subject    string   `json:"sub"`
	TokenID    string   `json:"jti"`
	Issuer     string   `json:"iss"`
	Audience   string   `json:"aud"`
	TokenUse   string   `json:"token_use"`
	ExpiresAt  int64    `json:"exp"`
	IssuedAt   int64    `json:"iat"`
	NotBefore  int64    `json:"nbf,omitempty"`

	TenantID   string   `json:"tid"`
	MSPID      string   `json:"mid"`
	ClientID   string   `json:"cid"`
	SiteID     string   `json:"sid"`
	AgentID    string   `json:"aid"`
	Roles      []string `json:"roles"`
}

func generateTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type TokenGenerator struct {
	secret []byte
}

func NewTokenGenerator(secret string) *TokenGenerator {
	if secret == "" {
		secret = jwtSecret()
	}
	if secret == "" {
		return &TokenGenerator{secret: []byte("dev-fallback-not-for-production")}
	}
	return &TokenGenerator{secret: []byte(secret)}
}

func NewTokenGeneratorOrFail(secret string) (*TokenGenerator, error) {
	if secret == "" {
		secret = jwtSecret()
	}
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is not configured")
	}
	if len(secret) < minSecretLen {
		return nil, fmt.Errorf("JWT_SECRET must be at least %d characters", minSecretLen)
	}
	return &TokenGenerator{secret: []byte(secret)}, nil
}

func (g *TokenGenerator) GenerateAgentToken(tenantID, agentID string, ttl time.Duration) (string, error) {
	tokenID, err := generateTokenID()
	if err != nil {
		return "", fmt.Errorf("generating token id: %w", err)
	}
	now := time.Now()
	claims := Claims{
		Subject:   agentID,
		TokenID:   tokenID,
		Issuer:    issuer,
		Audience:  audience,
		TokenUse:  "agent",
		TenantID:  tenantID,
		AgentID:   agentID,
		Roles:     []string{"agent"},
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	}
	return g.encode(claims)
}

func (g *TokenGenerator) GenerateUserToken(userID, tenantID, mspID, clientID, siteID string, roles []string, ttl time.Duration) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("userID is required")
	}
	if len(roles) == 0 {
		return "", fmt.Errorf("at least one role is required")
	}
	tokenID, err := generateTokenID()
	if err != nil {
		return "", fmt.Errorf("generating token id: %w", err)
	}
	now := time.Now()
	claims := Claims{
		Subject:   userID,
		TokenID:   tokenID,
		Issuer:    issuer,
		Audience:  audience,
		TokenUse:  "user",
		TenantID:  tenantID,
		MSPID:     mspID,
		ClientID:  clientID,
		SiteID:    siteID,
		Roles:     roles,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	}
	return g.encode(claims)
}

func (g *TokenGenerator) Validate(token string) (*Claims, error) {
	parts := tokenize(token, '.')
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Reject algorithm confusion: header must be valid JSON with alg=HS256
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decoding header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("unmarshaling header: %w", err)
	}
	if header.Alg != "HS256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Alg)
	}

	expectedSig := g.sign(parts[0] + "." + parts[1])
	if subtle.ConstantTimeCompare([]byte(expectedSig), []byte(parts[2])) == 0 {
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

	now := time.Now().Unix()

	// Require mandatory claims
	if claims.Subject == "" {
		return nil, fmt.Errorf("missing required claim: sub")
	}
	if claims.TokenID == "" {
		return nil, fmt.Errorf("missing required claim: jti")
	}
	if claims.Issuer == "" {
		return nil, fmt.Errorf("missing required claim: iss")
	}
	if claims.Audience == "" {
		return nil, fmt.Errorf("missing required claim: aud")
	}
	if claims.TokenUse == "" {
		return nil, fmt.Errorf("missing required claim: token_use")
	}
	if claims.ExpiresAt == 0 {
		return nil, fmt.Errorf("missing required claim: exp")
	}
	if claims.IssuedAt == 0 {
		return nil, fmt.Errorf("missing required claim: iat")
	}

	// Validate issuer
	if claims.Issuer != issuer {
		return nil, fmt.Errorf("invalid issuer: %s", claims.Issuer)
	}

	// Validate audience
	if claims.Audience != audience {
		return nil, fmt.Errorf("invalid audience: %s", claims.Audience)
	}

	// Validate token use
	if claims.TokenUse != "user" && claims.TokenUse != "agent" {
		return nil, fmt.Errorf("unsupported token_use: %s", claims.TokenUse)
	}

	// Validate timestamps
	if now > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	if claims.NotBefore > 0 && now < claims.NotBefore {
		return nil, fmt.Errorf("token not yet valid")
	}

	// Reject tokens issued far in the future (clock drift tolerance: 5 minutes)
	maxIatSkew := int64(300)
	if claims.IssuedAt > now+maxIatSkew {
		return nil, fmt.Errorf("token issued in the future")
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
	if secret == "" {
		secret = jwtSecret()
	}
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
