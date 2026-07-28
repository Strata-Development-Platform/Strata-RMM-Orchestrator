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
	cleaned := strings.TrimSpace(s)
	if cleaned == "" {
		return ModeDevelopment, nil
	}
	switch strings.ToLower(cleaned) {
	case "development", "dev":
		return ModeDevelopment, nil
	case "test":
		return ModeTest, nil
	case "production", "prod":
		return ModeProduction, nil
	default:
		return "", fmt.Errorf("unknown runtime mode %q: valid values are development, test, production", cleaned)
	}
}

type TypedValue struct {
	Present   bool
	Value     interface{}
	Error     error
	FieldName string
}

func parseBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "t", "yes", "1":
		return true, nil
	case "false", "f", "no", "0", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q", raw)
	}
}

func parseInt(raw string, bitSize int) (int64, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return 0, fmt.Errorf("empty value")
	}
	v, err := strconv.ParseInt(cleaned, 10, bitSize)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", cleaned, err)
	}
	return v, nil
}

func parseDuration(raw string) (time.Duration, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return 0, fmt.Errorf("empty duration")
	}
	d, err := time.ParseDuration(cleaned)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", cleaned, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("negative duration %q", cleaned)
	}
	return d, nil
}

func parseURL(raw string) (*url.URL, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return nil, fmt.Errorf("empty URL")
	}
	u, err := url.Parse(cleaned)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", cleaned, err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("URL missing scheme: %q", cleaned)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("URL missing host: %q", cleaned)
	}
	return u, nil
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

func (n *NATSConfig) Validate(mode RuntimeMode) error {
	if n.URL == "" {
		return fmt.Errorf("NATS.URL is required")
	}
	u, err := parseURL(n.URL)
	if err != nil {
		return fmt.Errorf("NATS.URL: %w", err)
	}
	if u.Scheme != "nats" && u.Scheme != "nats+tls" && u.Scheme != "tls" {
		return fmt.Errorf("NATS.URL: unsupported scheme %q (use nats, nats+tls, or tls)", u.Scheme)
	}
	if mode == ModeProduction && n.TLSEnabled && (n.TLSCertFile == "" || n.TLSKeyFile == "") {
		return fmt.Errorf("NATS.TLS: cert and key files required when TLS is enabled")
	}
	if n.ReconnectWait <= 0 {
		return fmt.Errorf("NATS.ReconnectWait must be positive")
	}
	return nil
}

type DatabaseConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func (d *DatabaseConfig) Validate(mode RuntimeMode) error {
	if d.DSN == "" {
		return fmt.Errorf("DB.DSN is required")
	}
	u, err := url.Parse(d.DSN)
	if err != nil {
		return fmt.Errorf("DB.DSN: %w", err)
	}
	if u.Host == "" {
		return fmt.Errorf("DB.DSN: missing host")
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return fmt.Errorf("DB.DSN: missing database name")
	}
	if mode == ModeProduction {
		q := u.Query()
		if q.Get("sslmode") == "disable" {
			return fmt.Errorf("DB.DSN: sslmode=disable is not allowed in production without explicit policy override")
		}
		if u.User != nil {
			pwd, _ := u.User.Password()
			if pwd == "password" || pwd == "postgres" || pwd == "strata" {
				return fmt.Errorf("DB.DSN: contains a known default password")
			}
		}
	}
	if d.MaxOpenConns <= 0 {
		return fmt.Errorf("DB.MaxOpenConns must be positive")
	}
	if d.MaxIdleConns < 0 {
		return fmt.Errorf("DB.MaxIdleConns must be non-negative")
	}
	if d.MaxIdleConns > d.MaxOpenConns {
		return fmt.Errorf("DB.MaxIdleConns (%d) must not exceed DB.MaxOpenConns (%d)", d.MaxIdleConns, d.MaxOpenConns)
	}
	if d.ConnMaxLifetime <= 0 {
		return fmt.Errorf("DB.ConnMaxLifetime must be positive")
	}
	return nil
}

type StorageConfig struct {
	Backend   string
	Bucket    string
	Region    string
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	KMSKeyID  string
}

func (s *StorageConfig) Validate(mode RuntimeMode) error {
	if s.Backend == "" || s.Backend == "none" {
		return nil
	}
	if s.Bucket == "" {
		return fmt.Errorf("Storage.Bucket is required for backend %q", s.Backend)
	}
	if mode == ModeProduction {
		switch s.Backend {
		case "minio":
			if s.Endpoint == "" {
				return fmt.Errorf("Storage.Endpoint is required for MinIO backend")
			}
			if s.AccessKey == "" || s.SecretKey == "" {
				return fmt.Errorf("storage: access key and secret key required for MinIO backend")
			}
		case "s3":
			if s.AccessKey == "" || s.SecretKey == "" {
				return fmt.Errorf("storage: access key and secret key required for S3 backend")
			}
		case "local":
			return fmt.Errorf("storage: local backend is not allowed in production")
		}
	}
	return nil
}

type JWTConfig struct {
	Secret        string
	Issuer        string
	Audience      string
	TokenDuration time.Duration
}

func (j *JWTConfig) Validate(mode RuntimeMode) error {
	if j.Secret == "" {
		return fmt.Errorf("JWT.Secret is required")
	}
	if mode == ModeProduction {
		if len(j.Secret) < 32 {
			return fmt.Errorf("JWT.Secret must be at least 32 characters")
		}
		if j.Secret == "strata-rmm-dev-secret" || strings.HasPrefix(j.Secret, "dev-") || strings.HasPrefix(j.Secret, "test-") {
			return fmt.Errorf("JWT.Secret contains a development placeholder")
		}
	}
	return nil
}

type HTTPConfig struct {
	APIAddr          string
	TunnelAddr       string
	PublicURL        string
	CORSOrigins      []string
	TrustedProxies   []string
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	MaxBodySizeBytes int64
}

func (h *HTTPConfig) Validate(mode RuntimeMode) error {
	if h.APIAddr == "" {
		return fmt.Errorf("HTTP.APIAddr is required")
	}
	if mode == ModeProduction {
		if h.PublicURL != "" {
			u, err := parseURL(h.PublicURL)
			if err != nil {
				return fmt.Errorf("HTTP.PublicURL: %w", err)
			}
			if u.Scheme != "https" {
				return fmt.Errorf("HTTP.PublicURL must use https scheme")
			}
			if u.Host == "" {
				return fmt.Errorf("HTTP.PublicURL missing host")
			}
		}
		for _, origin := range h.CORSOrigins {
			if origin == "*" {
				return fmt.Errorf("HTTP.CORSOrigins: wildcard origin not allowed in production")
			}
		}
	}
	if h.ReadTimeout <= 0 {
		return fmt.Errorf("HTTP.ReadTimeout must be positive")
	}
	if h.WriteTimeout <= 0 {
		return fmt.Errorf("HTTP.WriteTimeout must be positive")
	}
	if h.MaxBodySizeBytes <= 0 {
		return fmt.Errorf("HTTP.MaxBodySizeBytes must be positive")
	}
	return nil
}

type SeedingConfig struct {
	SeedDev        bool
	DevAdminEmail  string
	DevAdminPwd    string
}

func (s *SeedingConfig) Validate(mode RuntimeMode) error {
	if mode == ModeProduction && s.SeedDev {
		return fmt.Errorf("Seeding.SeedDev must not be enabled in production")
	}
	return nil
}

type OrchestratorConfig struct {
	RuntimeMode RuntimeMode
	NATS        NATSConfig
	DB          DatabaseConfig
	Storage     StorageConfig
	JWT         JWTConfig
	HTTP        HTTPConfig
	Seeding     SeedingConfig
}

func (c *OrchestratorConfig) Validate() error {
	var errs []string
	if err := c.NATS.Validate(c.RuntimeMode); err != nil {
		errs = append(errs, err.Error())
	}
	if err := c.DB.Validate(c.RuntimeMode); err != nil {
		errs = append(errs, err.Error())
	}
	if err := c.Storage.Validate(c.RuntimeMode); err != nil {
		errs = append(errs, err.Error())
	}
	if err := c.JWT.Validate(c.RuntimeMode); err != nil {
		errs = append(errs, err.Error())
	}
	if err := c.HTTP.Validate(c.RuntimeMode); err != nil {
		errs = append(errs, err.Error())
	}
	if err := c.Seeding.Validate(c.RuntimeMode); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func (c *OrchestratorConfig) ProductionValidate() error {
	if c.RuntimeMode != ModeProduction {
		return nil
	}
	return c.Validate()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func LoadOrchestratorConfig() *OrchestratorConfig {
	cfg := &OrchestratorConfig{
		HTTP: HTTPConfig{
			APIAddr:          ":8080",
			ReadTimeout:      10 * time.Second,
			WriteTimeout:     10 * time.Second,
			IdleTimeout:      60 * time.Second,
			MaxBodySizeBytes: 10 << 20,
		},
		DB: DatabaseConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		},
		NATS: NATSConfig{
			URL:           envOr("NATS_URL", "nats://localhost:4222"),
			ReconnectWait: 5 * time.Second,
			MaxReconnects: -1,
		},
		JWT: JWTConfig{
			Issuer:        "strata-rmm",
			Audience:      "strata-rmm-api",
			TokenDuration: 24 * time.Hour,
		},
	}

	// Runtime mode
	if modeRaw := os.Getenv("STRATA_RUNTIME_MODE"); modeRaw != "" {
		if mode, err := ParseRuntimeMode(modeRaw); err == nil {
			cfg.RuntimeMode = mode
		}
	}
	if cfg.RuntimeMode == "" {
		cfg.RuntimeMode = ModeDevelopment
	}

	// Database DSN — multiple aliases
	cfg.DB.DSN = envOr("TIMESCALE_DSN", envOr("STRATA_DB_DSN", envOr("DATABASE_URL", "postgres://localhost:5432/strata_rmm?sslmode=disable")))

	if pi := envInt("DB_MAX_OPEN_CONNS", 0); pi > 0 {
		cfg.DB.MaxOpenConns = pi
	}
	if pi := envInt("DB_MAX_IDLE_CONNS", 0); pi >= 0 {
		cfg.DB.MaxIdleConns = pi
	}
	if d := envDuration("DB_CONN_MAX_LIFETIME"); d > 0 {
		cfg.DB.ConnMaxLifetime = d
	}

	// NATS
	if v := envOr("NATS_URL", ""); v != "" {
		cfg.NATS.URL = v
	}
	cfg.NATS.Token = envOr("NATS_TOKEN", "")
	cfg.NATS.TLSEnabled = envBool("NATS_TLS_ENABLED")
	cfg.NATS.TLSCertFile = envOr("NATS_TLS_CERT", "")
	cfg.NATS.TLSKeyFile = envOr("NATS_TLS_KEY", "")
	cfg.NATS.TLSCAFile = envOr("NATS_TLS_CA", "")
	if d := envDuration("NATS_RECONNECT_WAIT"); d > 0 {
		cfg.NATS.ReconnectWait = d
	}
	if pi := envInt("NATS_MAX_RECONNECTS", 0); pi != 0 {
		cfg.NATS.MaxReconnects = pi
	}

	// Storage
	cfg.Storage.Backend = envOr("STORAGE_BACKEND", "local")
	cfg.Storage.Bucket = envOr("STORAGE_BUCKET", "strata-recordings")
	cfg.Storage.Region = envOr("STORAGE_REGION", "")
	cfg.Storage.Endpoint = envOr("STORAGE_ENDPOINT", "")
	cfg.Storage.AccessKey = envOr("STORAGE_ACCESS_KEY", "")
	cfg.Storage.SecretKey = envOr("STORAGE_SECRET_KEY", "")
	cfg.Storage.UseSSL = envBool("STORAGE_USE_SSL")
	cfg.Storage.KMSKeyID = envOr("STORAGE_KMS_KEY_ID", "")

	// JWT
	cfg.JWT.Secret = envOr("JWT_SECRET", "")
	if v := envOr("JWT_ISSUER", ""); v != "" {
		cfg.JWT.Issuer = v
	}
	if v := envOr("JWT_AUDIENCE", ""); v != "" {
		cfg.JWT.Audience = v
	}
	if d := envDuration("JWT_TOKEN_DURATION"); d > 0 {
		cfg.JWT.TokenDuration = d
	}

	// HTTP
	if v := envOr("STRATA_API_ADDR", envOr("API_ADDR", "")); v != "" {
		cfg.HTTP.APIAddr = v
	}
	if v := envOr("STRATA_TUNNEL_ADDR", envOr("TUNNEL_ADDR", "")); v != "" {
		cfg.HTTP.TunnelAddr = v
	}
	cfg.HTTP.PublicURL = envOr("STRATA_PUBLIC_URL", "")
	if corsStr := envOr("CORS_ORIGINS", ""); corsStr != "" {
		cfg.HTTP.CORSOrigins = strings.Split(corsStr, ",")
	}
	if proxyStr := envOr("TRUSTED_PROXIES", ""); proxyStr != "" {
		cfg.HTTP.TrustedProxies = strings.Split(proxyStr, ",")
	}
	if d := envDuration("HTTP_READ_TIMEOUT"); d > 0 {
		cfg.HTTP.ReadTimeout = d
	}
	if d := envDuration("HTTP_WRITE_TIMEOUT"); d > 0 {
		cfg.HTTP.WriteTimeout = d
	}
	if d := envDuration("HTTP_IDLE_TIMEOUT"); d > 0 {
		cfg.HTTP.IdleTimeout = d
	}
	if pi := envInt("HTTP_MAX_BODY_SIZE", 0); pi > 0 {
		cfg.HTTP.MaxBodySizeBytes = int64(pi)
	}

	// Seeding
	cfg.Seeding.SeedDev = envBool("STRATA_SEED_DEV")
	cfg.Seeding.DevAdminEmail = envOr("STRATA_DEV_ADMIN_EMAIL", "")
	cfg.Seeding.DevAdminPwd = envOr("STRATA_DEV_ADMIN_PASSWORD_HASH", "")

	return cfg
}

func (c *OrchestratorConfig) RedactedSummary() map[string]interface{} {
	return map[string]interface{}{
		"runtime_mode":    string(c.RuntimeMode),
		"nats_url":        redactURL(c.NATS.URL),
		"nats_tls":        c.NATS.TLSEnabled,
		"db_dsn":          redactDSN(c.DB.DSN),
		"db_pool_max":     c.DB.MaxOpenConns,
		"db_pool_idle":    c.DB.MaxIdleConns,
		"storage_type":    c.Storage.Backend,
		"storage_bucket":  c.Storage.Bucket,
		"api_addr":        c.HTTP.APIAddr,
		"tunnel_addr":     c.HTTP.TunnelAddr,
		"public_url":      c.HTTP.PublicURL,
		"cors_origins":    c.HTTP.CORSOrigins,
		"jwt_configured":  c.JWT.Secret != "",
		"jwt_secret_len":  len(c.JWT.Secret),
		"seed_dev":        c.Seeding.SeedDev,
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
		lc := strings.ToLower(k)
		if strings.Contains(lc, "password") || strings.Contains(lc, "secret") || strings.Contains(lc, "key") {
			q.Set(k, "***")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func envBool(key string) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "true", "t", "yes", "1":
		return true
	default:
		return false
	}
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func envDuration(key string) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return 0
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	return d
}
