package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type RuntimeMode string

const (
	ModeDevelopment RuntimeMode = "development"
	ModeTest        RuntimeMode = "test"
	ModeProduction  RuntimeMode = "production"
)

func ParseRuntimeMode(s string) (RuntimeMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "development", "dev":
		return ModeDevelopment, nil
	case "test":
		return ModeTest, nil
	case "production", "prod":
		return ModeProduction, nil
	case "":
		return ModeDevelopment, nil
	default:
		return "", fmt.Errorf("unknown runtime mode %q; valid values: development, test, production", s)
	}
}

type OrchestratorConfig struct {
	RuntimeMode RuntimeMode

	NATS    NATSConfig
	DB      DatabaseConfig
	Storage StorageConfig
	JWT     JWTConfig
	HTTP    HTTPConfig
	Seeding SeedingConfig
}

type NATSConfig struct {
	URL             string
	Token           string
	TLSEnabled      bool
	TLSCertFile     string
	TLSKeyFile      string
	TLSCAFile       string
	ReconnectWait   time.Duration
	MaxReconnects   int
}

type DatabaseConfig struct {
	DSN           string
	MaxOpenConns  int
	MaxIdleConns  int
	ConnMaxLifetime time.Duration
	SSLMode       string
}

type StorageConfig struct {
	Backend  string
	Bucket   string
	Region   string
	Endpoint string
	AccessKey string
	SecretKey string
	UseSSL    bool
	KMSKeyID  string
}

type JWTConfig struct {
	Secret        string
	Issuer        string
	Audience      string
	TokenDuration time.Duration
}

type HTTPConfig struct {
	APIAddr          string
	TunnelAddr       string
	PublicURL        string
	CORSOrigins      []string
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	MaxBodySizeBytes int64
	TrustedProxies   []string
}

type SeedingConfig struct {
	SeedDev      bool
	DevAdminEmail string
	DevAdminPwdHash string
}

func (c *OrchestratorConfig) Validate() error {
	if c.HTTP.APIAddr == "" {
		return fmt.Errorf("HTTP.APIAddr is required")
	}
	if c.NATS.URL == "" {
		return fmt.Errorf("NATS.URL is required")
	}
	if c.DB.DSN == "" {
		return fmt.Errorf("DB.DSN is required")
	}
	if c.JWT.Secret == "" {
		return fmt.Errorf("JWT.Secret is required")
	}
	return nil
}

func (c *OrchestratorConfig) ValidateProduction() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.RuntimeMode != ModeProduction {
		return nil
	}

	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("JWT.Secret must be at least 32 characters in production")
	}
	if c.JWT.Secret == "strata-rmm-dev-secret" || strings.HasPrefix(c.JWT.Secret, "dev-") {
		return fmt.Errorf("JWT.Secret contains a development placeholder")
	}

	if strings.Contains(c.DB.DSN, "sslmode=disable") {
		return fmt.Errorf("DB.DSN must not use sslmode=disable in production without explicit policy override")
	}
	u, err := url.Parse(c.DB.DSN)
	if err == nil && u.User != nil {
		if pwd, _ := u.User.Password(); pwd == "strata" || pwd == "password" || pwd == "postgres" {
			return fmt.Errorf("DB.DSN contains a known default password")
		}
	}

	if c.HTTP.PublicURL != "" {
		u, err := url.Parse(c.HTTP.PublicURL)
		if err != nil {
			return fmt.Errorf("HTTP.PublicURL is not a valid URL")
		}
		if u.Scheme != "https" {
			return fmt.Errorf("HTTP.PublicURL must use https in production")
		}
	}

	for _, origin := range c.HTTP.CORSOrigins {
		if origin == "*" {
			return fmt.Errorf("HTTP.CORSOrigins must not contain wildcard origin in production")
		}
	}

	if c.DB.MaxOpenConns <= 0 {
		c.DB.MaxOpenConns = 25
	}
	if c.DB.MaxIdleConns <= 0 {
		c.DB.MaxIdleConns = 5
	}
	if c.DB.MaxIdleConns > c.DB.MaxOpenConns {
		return fmt.Errorf("DB.MaxIdleConns (%d) must not exceed DB.MaxOpenConns (%d)", c.DB.MaxIdleConns, c.DB.MaxOpenConns)
	}

	if c.HTTP.ReadTimeout <= 0 {
		c.HTTP.ReadTimeout = 10 * time.Second
	}
	if c.HTTP.WriteTimeout <= 0 {
		c.HTTP.WriteTimeout = 10 * time.Second
	}
	if c.HTTP.MaxBodySizeBytes <= 0 {
		c.HTTP.MaxBodySizeBytes = 10 << 20
	}

	return nil
}

func (c *OrchestratorConfig) RedactedSummary() map[string]interface{} {
	return map[string]interface{}{
		"runtime_mode":  string(c.RuntimeMode),
		"nats_url":      redactURL(c.NATS.URL),
		"db_dsn":        redactDSN(c.DB.DSN),
		"storage_type":  c.Storage.Backend,
		"storage_bucket": c.Storage.Bucket,
		"api_addr":      c.HTTP.APIAddr,
		"tunnel_addr":   c.HTTP.TunnelAddr,
		"public_url":    c.HTTP.PublicURL,
		"jwt_configured": c.JWT.Secret != "",
		"jwt_secret_len": len(c.JWT.Secret),
		"cors_origins":  c.HTTP.CORSOrigins,
		"seed_dev":      c.Seeding.SeedDev,
	}
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}

func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "<invalid>"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	q := u.Query()
	for k := range q {
		if strings.Contains(strings.ToLower(k), "password") || strings.Contains(strings.ToLower(k), "secret") || strings.Contains(strings.ToLower(k), "key") {
			q.Set(k, "***")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

type Option func(*OrchestratorConfig)

func LoadOrchestratorConfig(opts ...Option) *OrchestratorConfig {
	cfg := &OrchestratorConfig{
		HTTP: HTTPConfig{
			APIAddr:          ":8080",
			MaxBodySizeBytes: 10 << 20,
			ReadTimeout:      10 * time.Second,
			WriteTimeout:     10 * time.Second,
			IdleTimeout:      60 * time.Second,
		},
		DB: DatabaseConfig{
			DSN:            envOr("TIMESCALE_DSN", envOr("STRATA_DB_DSN", envOr("DATABASE_URL", "postgres://localhost:5432/strata_rmm?sslmode=disable"))),
			MaxOpenConns:  25,
			MaxIdleConns:  5,
			ConnMaxLifetime: 5 * time.Minute,
		},
		NATS: NATSConfig{
			URL:           envOr("NATS_URL", "nats://localhost:4222"),
			ReconnectWait: 5 * time.Second,
			MaxReconnects: -1,
		},
		Storage: StorageConfig{
			Backend:  envOr("STORAGE_BACKEND", "local"),
			Bucket:   envOr("STORAGE_BUCKET", "strata-recordings"),
			Region:   envOr("STORAGE_REGION", ""),
			Endpoint: envOr("STORAGE_ENDPOINT", ""),
			AccessKey: envOr("STORAGE_ACCESS_KEY", ""),
			SecretKey: envOr("STORAGE_SECRET_KEY", ""),
			UseSSL:    envBool("STORAGE_USE_SSL"),
			KMSKeyID:  envOr("STORAGE_KMS_KEY_ID", ""),
		},
		JWT: JWTConfig{
			Secret:        envOr("JWT_SECRET", ""),
			Issuer:        "strata-rmm",
			Audience:      "strata-rmm-api",
			TokenDuration: 24 * time.Hour,
		},
		Seeding: SeedingConfig{
			SeedDev:        envBool("STRATA_SEED_DEV"),
			DevAdminEmail:  envOr("STRATA_DEV_ADMIN_EMAIL", ""),
			DevAdminPwdHash: envOr("STRATA_DEV_ADMIN_PASSWORD_HASH", ""),
		},
	}

	if mode, err := ParseRuntimeMode(envOr("STRATA_RUNTIME_MODE", "development")); err == nil {
		cfg.RuntimeMode = mode
	}
	if pi := envInt("DB_MAX_OPEN_CONNS", 0); pi > 0 {
		cfg.DB.MaxOpenConns = pi
	}
	if pi := envInt("DB_MAX_IDLE_CONNS", 0); pi > 0 {
		cfg.DB.MaxIdleConns = pi
	}
	if v := envOr("STRATA_API_ADDR", ""); v != "" {
		cfg.HTTP.APIAddr = v
	} else if cfg.HTTP.APIAddr == "" {
		cfg.HTTP.APIAddr = envOr("API_ADDR", ":8080")
	}
	if v := envOr("STRATA_TUNNEL_ADDR", ""); v != "" {
		cfg.HTTP.TunnelAddr = v
	} else if cfg.HTTP.TunnelAddr == "" {
		cfg.HTTP.TunnelAddr = envOr("TUNNEL_ADDR", ":8443")
	}
	if v := envOr("STRATA_PUBLIC_URL", ""); v != "" {
		cfg.HTTP.PublicURL = v
	}

	if v := envOr("NATS_URL", ""); v != "" {
		cfg.NATS.URL = v
	}
	if v := envOr("NATS_TOKEN", ""); v != "" {
		cfg.NATS.Token = v
	}

	for _, o := range opts {
		o(cfg)
	}

	corsStr := envOr("CORS_ORIGINS", "")
	if corsStr != "" {
		cfg.HTTP.CORSOrigins = strings.Split(corsStr, ",")
	}

	return cfg
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	return strings.ToLower(os.Getenv(key)) == "true"
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
