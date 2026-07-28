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

func (m RuntimeMode) String() string {
	return string(m)
}

func ParseRuntimeMode(s string) (RuntimeMode, error) {
	cleaned := strings.TrimSpace(s)
	switch strings.ToLower(cleaned) {
	case "development", "dev":
		return ModeDevelopment, nil
	case "test":
		return ModeTest, nil
	case "production", "prod":
		return ModeProduction, nil
	case "":
		return ModeDevelopment, nil
	default:
		return "", fmt.Errorf("unknown runtime mode %q: valid values are development, test, production", s)
	}
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
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
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
	TrustedProxies   []string
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	MaxBodySizeBytes int64
}

type SeedingConfig struct {
	SeedDev       bool
	DevAdminEmail string
	DevAdminPwd   string
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
	check := func(label string, err error) {
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", label, err))
		}
	}

	if c.RuntimeMode == "" {
		errs = append(errs, "RuntimeMode: not set")
	}
	if c.JWT.Secret == "" {
		errs = append(errs, "JWT.Secret: required")
	}
	if c.DB.DSN == "" {
		errs = append(errs, "DB.DSN: required")
	} else {
		check("DB.DSN", validateDSN(c.DB.DSN))
		if c.DB.MaxOpenConns <= 0 {
			errs = append(errs, "DB.MaxOpenConns: must be positive")
		}
		if c.DB.MaxIdleConns < 0 {
			errs = append(errs, "DB.MaxIdleConns: must be non-negative")
		}
		if c.DB.MaxIdleConns > c.DB.MaxOpenConns {
			errs = append(errs, fmt.Sprintf("DB.MaxIdleConns (%d) > DB.MaxOpenConns (%d)", c.DB.MaxIdleConns, c.DB.MaxOpenConns))
		}
		if c.DB.ConnMaxLifetime <= 0 {
			errs = append(errs, "DB.ConnMaxLifetime: must be positive")
		}
	}
	if c.NATS.URL == "" {
		errs = append(errs, "NATS.URL: required")
	} else {
		check("NATS.URL", validateURL(c.NATS.URL, []string{"nats", "nats+tls", "tls"}))
		if c.NATS.ReconnectWait <= 0 {
			errs = append(errs, "NATS.ReconnectWait: must be positive")
		}
	}
	if c.HTTP.APIAddr == "" {
		errs = append(errs, "HTTP.APIAddr: required")
	}
	if c.HTTP.ReadTimeout <= 0 {
		errs = append(errs, "HTTP.ReadTimeout: must be positive")
	}
	if c.HTTP.WriteTimeout <= 0 {
		errs = append(errs, "HTTP.WriteTimeout: must be positive")
	}
	if c.HTTP.MaxBodySizeBytes <= 0 {
		errs = append(errs, "HTTP.MaxBodySizeBytes: must be positive")
	}

	check("Storage", c.Storage.validate())
	check("JWT.Secret", c.JWT.validate())
	check("Seeding", c.Seeding.validate(c.RuntimeMode))

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func validateURL(raw string, schemes []string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme == "" {
		return fmt.Errorf("missing scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	allowed := false
	for _, s := range schemes {
		if u.Scheme == s {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("unsupported scheme %q (valid: %s)", u.Scheme, strings.Join(schemes, ", "))
	}
	return nil
}

func validateDSN(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid DSN: %w", err)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return fmt.Errorf("missing database name")
	}
	return nil
}

func (s *StorageConfig) validate() error {
	if s.Backend == "" || s.Backend == "none" {
		return nil
	}
	if s.Bucket == "" {
		return fmt.Errorf("bucket required for backend %q", s.Backend)
	}
	return nil
}

func (j *JWTConfig) validate() error {
	if j.Secret == "" {
		return nil
	}
	if len(j.Secret) < 32 {
		return fmt.Errorf("must be at least 32 characters")
	}
	return nil
}

func (s *SeedingConfig) validate(mode RuntimeMode) error {
	if mode == ModeProduction && s.SeedDev {
		return fmt.Errorf("SeedDev must not be enabled in production")
	}
	return nil
}

func (c *OrchestratorConfig) ProductionValidate() error {
	if c.RuntimeMode != ModeProduction {
		return nil
	}
	var errs []string

	if c.HTTP.PublicURL == "" {
		errs = append(errs, "HTTP.PublicURL: required in production")
	} else {
		u, err := url.Parse(c.HTTP.PublicURL)
		if err != nil {
			errs = append(errs, fmt.Sprintf("HTTP.PublicURL: invalid: %v", err))
		} else {
			if u.Scheme != "https" {
				errs = append(errs, "HTTP.PublicURL: must use https")
			}
			if u.User != nil {
				errs = append(errs, "HTTP.PublicURL: must not contain credentials")
			}
			if u.Host == "" {
				errs = append(errs, "HTTP.PublicURL: missing host")
			}
		}
	}
	for _, origin := range c.HTTP.CORSOrigins {
		if origin == "*" {
			errs = append(errs, "HTTP.CORSOrigins: wildcard not allowed in production")
		}
	}

	if c.NATS.TLSEnabled {
		if c.NATS.TLSCertFile == "" || c.NATS.TLSKeyFile == "" {
			errs = append(errs, "NATS: TLS enabled but cert or key file missing")
		}
	}
	if strings.Contains(c.DB.DSN, "sslmode=disable") {
		errs = append(errs, "DB.DSN: sslmode=disable not allowed in production")
	}
	u, _ := url.Parse(c.DB.DSN)
	if u != nil && u.User != nil {
		if pwd, _ := u.User.Password(); pwd == "password" || pwd == "postgres" || pwd == "strata" {
			errs = append(errs, "DB.DSN: contains a known default password")
		}
	}

	if c.JWT.Secret != "" && (strings.HasPrefix(c.JWT.Secret, "dev-") || strings.HasPrefix(c.JWT.Secret, "test-")) {
		errs = append(errs, "JWT.Secret: contains a development placeholder (dev- or test- prefix)")
	}
	if err := c.Seeding.validate(c.RuntimeMode); err != nil {
		errs = append(errs, fmt.Sprintf("Seeding: %v", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("production configuration rejected:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func LoadOrchestratorConfig() (*OrchestratorConfig, error) {
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
			URL:           "nats://localhost:4222",
			ReconnectWait: 5 * time.Second,
			MaxReconnects: -1,
		},
		JWT: JWTConfig{
			Issuer:        "strata-rmm",
			Audience:      "strata-rmm-api",
			TokenDuration: 24 * time.Hour,
		},
	}

	if modeRaw := os.Getenv("STRATA_RUNTIME_MODE"); modeRaw != "" {
		mode, err := ParseRuntimeMode(modeRaw)
		if err != nil {
			return nil, fmt.Errorf("STRATA_RUNTIME_MODE: %w", err)
		}
		cfg.RuntimeMode = mode
	} else {
		cfg.RuntimeMode = ModeDevelopment
	}

	cfg.DB.DSN = resolveDSN()
	cfg.NATS.URL = envStr("NATS_URL", cfg.NATS.URL)
	cfg.NATS.Token = os.Getenv("NATS_TOKEN")
	cfg.NATS.TLSEnabled = envBool("NATS_TLS_ENABLED")
	cfg.NATS.TLSCertFile = envStr("NATS_TLS_CERT", "")
	cfg.NATS.TLSKeyFile = envStr("NATS_TLS_KEY", "")
	cfg.NATS.TLSCAFile = envStr("NATS_TLS_CA", "")
	cfg.NATS.ReconnectWait = envDuration("NATS_RECONNECT_WAIT", cfg.NATS.ReconnectWait)
	if v := envInt("NATS_MAX_RECONNECTS"); v != nil {
		cfg.NATS.MaxReconnects = *v
	}

	cfg.DB.MaxOpenConns = envIntDef("DB_MAX_OPEN_CONNS", cfg.DB.MaxOpenConns)
	cfg.DB.MaxIdleConns = envIntDef("DB_MAX_IDLE_CONNS", cfg.DB.MaxIdleConns)
	cfg.DB.ConnMaxLifetime = envDuration("DB_CONN_MAX_LIFETIME", cfg.DB.ConnMaxLifetime)

	cfg.Storage.Backend = envStr("STORAGE_BACKEND", "local")
	cfg.Storage.Bucket = envStr("STORAGE_BUCKET", "strata-recordings")
	cfg.Storage.Region = os.Getenv("STORAGE_REGION")
	cfg.Storage.Endpoint = os.Getenv("STORAGE_ENDPOINT")
	cfg.Storage.AccessKey = os.Getenv("STORAGE_ACCESS_KEY")
	cfg.Storage.SecretKey = os.Getenv("STORAGE_SECRET_KEY")
	cfg.Storage.UseSSL = envBool("STORAGE_USE_SSL")
	cfg.Storage.KMSKeyID = os.Getenv("STORAGE_KMS_KEY_ID")

	cfg.JWT.Secret = os.Getenv("JWT_SECRET")
	if v := os.Getenv("JWT_ISSUER"); v != "" {
		cfg.JWT.Issuer = v
	}
	if v := os.Getenv("JWT_AUDIENCE"); v != "" {
		cfg.JWT.Audience = v
	}
	cfg.JWT.TokenDuration = envDuration("JWT_TOKEN_DURATION", cfg.JWT.TokenDuration)

	if v := envStr("STRATA_API_ADDR", ""); v != "" {
		cfg.HTTP.APIAddr = v
	} else if v := os.Getenv("API_ADDR"); v != "" {
		cfg.HTTP.APIAddr = v
	}
	if v := envStr("STRATA_TUNNEL_ADDR", ""); v != "" {
		cfg.HTTP.TunnelAddr = v
	} else if v := os.Getenv("TUNNEL_ADDR"); v != "" {
		cfg.HTTP.TunnelAddr = v
	}
	cfg.HTTP.PublicURL = os.Getenv("STRATA_PUBLIC_URL")
	if corsStr := os.Getenv("CORS_ORIGINS"); corsStr != "" {
		cfg.HTTP.CORSOrigins = splitTrim(corsStr, ",")
	}
	if proxyStr := os.Getenv("TRUSTED_PROXIES"); proxyStr != "" {
		cfg.HTTP.TrustedProxies = splitTrim(proxyStr, ",")
	}
	cfg.HTTP.ReadTimeout = envDuration("HTTP_READ_TIMEOUT", cfg.HTTP.ReadTimeout)
	cfg.HTTP.WriteTimeout = envDuration("HTTP_WRITE_TIMEOUT", cfg.HTTP.WriteTimeout)
	cfg.HTTP.IdleTimeout = envDuration("HTTP_IDLE_TIMEOUT", cfg.HTTP.IdleTimeout)
	if v := envInt64("HTTP_MAX_BODY_SIZE", cfg.HTTP.MaxBodySizeBytes); v != nil {
		cfg.HTTP.MaxBodySizeBytes = *v
	}

	cfg.Seeding.SeedDev = envBool("STRATA_SEED_DEV")
	cfg.Seeding.DevAdminEmail = os.Getenv("STRATA_DEV_ADMIN_EMAIL")
	cfg.Seeding.DevAdminPwd = os.Getenv("STRATA_DEV_ADMIN_PASSWORD_HASH")

	return cfg, nil
}

func resolveDSN() string {
	if v := os.Getenv("TIMESCALE_DSN"); v != "" {
		return v
	}
	if v := os.Getenv("STRATA_DB_DSN"); v != "" {
		return v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://localhost:5432/strata_rmm?sslmode=disable"
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "t", "yes", "1":
		return true
	default:
		return false
	}
}

func envInt(key string) *int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

func envIntDef(key string, def int) int {
	p := envInt(key)
	if p == nil {
		return def
	}
	return *p
}

func envInt64(key string, def int64) *int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	if d <= 0 {
		return def
	}
	return d
}

func splitTrim(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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
