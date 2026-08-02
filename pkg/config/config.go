package config

import (
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
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
	URL           string
	AdvertiseURLs []string
	Token         string
	TLSEnabled    bool
	TLSCertFile   string
	TLSKeyFile    string
	TLSCAFile     string
	ReconnectWait time.Duration
	MaxReconnects int
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
	Secret string
	// Issuer, Audience, TokenDuration — deferred to Phase 8G.
	// Hardcoded in pkg/auth/jwt.go for Phase 8A.
}

type HTTPConfig struct {
	APIAddr     string
	TunnelAddr  string
	PublicURL   string
	CORSOrigins []string
	// TrustedProxies — deferred to a later phase.
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	MaxBodySizeBytes int64
}

type ObservabilityConfig struct {
	MetricsToken string
}

type SMTPConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	ImplicitTLS bool
}

type AlertDeliveryConfig struct {
	SlackURL        string
	TeamsURL        string
	WebhookURL      string
	PagerDutyKey    string
	EmailRecipients []string
}

func (a AlertDeliveryConfig) validate(smtp SMTPConfig) error {
	var errs []string
	for name, raw := range map[string]string{"Slack": a.SlackURL, "Teams": a.TeamsURL, "Webhook": a.WebhookURL} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			errs = append(errs, name+" URL must be absolute HTTPS without credentials")
		}
	}
	if len(a.EmailRecipients) > 0 && !smtp.Configured() {
		errs = append(errs, "email recipients require SMTP configuration")
	}
	for _, recipient := range a.EmailRecipients {
		parsed, err := mail.ParseAddress(recipient)
		if err != nil || parsed.Address == "" {
			errs = append(errs, "email recipient is invalid")
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (s SMTPConfig) Configured() bool {
	return s.Host != "" || s.Port != 0 || s.Username != "" || s.Password != "" || s.FromAddress != "" || s.ImplicitTLS
}

func (s SMTPConfig) validate() error {
	if !s.Configured() {
		return nil
	}
	if strings.TrimSpace(s.Host) == "" {
		return fmt.Errorf("host is required when SMTP delivery is configured")
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("port must be from 1 to 65535")
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(s.FromAddress)); err != nil {
		return fmt.Errorf("from address is invalid")
	}
	if (s.Username == "") != (s.Password == "") {
		return fmt.Errorf("username and password must be configured together")
	}
	return nil
}

type SeedingConfig struct {
	SeedDev       bool
	DevAdminEmail string
	DevAdminPwd   string
}

type BackupConfig struct {
	Enabled                  bool
	EnvironmentID            string
	DatabaseType             string
	BackupDirectory          string
	EncryptionScheme         string
	ExternalBackupBucket     string
	ExternalBackupRegion     string
	ExternalBackupEndpoint   string
	ExternalBackupAccessKey  string
	ExternalBackupSecretKey  string
	RepositoryType           string
	KeyProviderPath          string
	RecoveryStorageBackend   string
	RecoveryStorageBucket    string
	RecoveryStorageRegion    string
	RecoveryStorageEndpoint  string
	RecoveryStorageAccessKey string
	RecoveryStorageSecretKey string
	RecoveryStorageUseSSL    bool
	RecoveryNATSURL          string
	RecoveryNATSToken        string
	RecoveryNATSTLSCAFile    string
	RecoveryNATSTLSCertFile  string
	RecoveryNATSTLSKeyFile   string
}

func (b *BackupConfig) validate() error {
	// F8C-13: Only authenticated encryption allowed — AES-256-GCM exclusively.
	if b.EncryptionScheme != "" && b.EncryptionScheme != "aes-256-gcm" {
		return fmt.Errorf("unsupported encryption scheme: %s (only aes-256-gcm is allowed)", b.EncryptionScheme)
	}
	if b.DatabaseType != "" && b.DatabaseType != "postgresql" && b.DatabaseType != "timescaledb" {
		return fmt.Errorf("unsupported database type: %s", b.DatabaseType)
	}
	if b.Enabled {
		if b.EnvironmentID == "" {
			return fmt.Errorf("STRATA_BACKUP_ENVIRONMENT_ID is required when backups are enabled")
		}
		if b.KeyProviderPath == "" {
			return fmt.Errorf("STRATA_BACKUP_KEY_PROVIDER_PATH is required when backups are enabled")
		}
		switch b.RepositoryType {
		case "filesystem":
			if b.BackupDirectory == "" {
				return fmt.Errorf("STRATA_BACKUP_DIRECTORY is required for filesystem backup repository")
			}
		case "s3":
			if b.ExternalBackupBucket == "" || b.ExternalBackupRegion == "" ||
				b.ExternalBackupAccessKey == "" || b.ExternalBackupSecretKey == "" {
				return fmt.Errorf("S3 backup repository bucket, region, access key, and secret key are required")
			}
		default:
			return fmt.Errorf("unsupported backup repository type: %s", b.RepositoryType)
		}
	}
	return nil
}

type OrchestratorConfig struct {
	RuntimeMode   RuntimeMode
	NATS          NATSConfig
	DB            DatabaseConfig
	Storage       StorageConfig
	JWT           JWTConfig
	HTTP          HTTPConfig
	Observability ObservabilityConfig
	SMTP          SMTPConfig
	AlertDelivery AlertDeliveryConfig
	Seeding       SeedingConfig
	Backup        BackupConfig
	Version       string
	Commit        string
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
	if c.Observability.MetricsToken != "" && len(c.Observability.MetricsToken) < 32 {
		errs = append(errs, "Observability.MetricsToken: must be at least 32 characters when set")
	}

	check("Storage", c.Storage.validate())
	check("JWT.Secret", c.JWT.validate())
	check("Seeding", c.Seeding.validate(c.RuntimeMode))
	check("Backup", c.Backup.validate())
	check("SMTP", c.SMTP.validate())
	check("AlertDelivery", c.AlertDelivery.validate(c.SMTP))
	if c.SMTP.Configured() && c.HTTP.PublicURL == "" {
		errs = append(errs, "HTTP.PublicURL: required when SMTP delivery is configured")
	} else if c.SMTP.Configured() {
		publicURL, err := url.Parse(c.HTTP.PublicURL)
		if err != nil || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" ||
			publicURL.Fragment != "" || (publicURL.Path != "" && publicURL.Path != "/") ||
			(publicURL.Scheme != "http" && publicURL.Scheme != "https") {
			errs = append(errs, "HTTP.PublicURL: must be an absolute HTTP(S) URL without credentials for SMTP delivery")
		}
	}

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
	if c.HTTP.TunnelAddr != "" {
		errs = append(errs, "HTTP.TunnelAddr: raw tunnel gateway is not production-safe; authenticated TLS transport is required")
	}
	if c.Observability.MetricsToken == "" {
		errs = append(errs, "Observability.MetricsToken: required in production")
	}

	if !c.NATS.TLSEnabled {
		errs = append(errs, "NATS: TLS is required in production; a token does not encrypt transport")
	}
	if c.NATS.TLSEnabled && c.NATS.TLSCAFile == "" {
		errs = append(errs, "NATS: TLS CA file is required in production")
	}
	if (c.NATS.TLSCertFile == "") != (c.NATS.TLSKeyFile == "") {
		errs = append(errs, "NATS: client certificate and key must be configured together")
	}
	if u, err := url.Parse(c.NATS.URL); err == nil && u.Scheme == "nats" {
		errs = append(errs, "NATS.URL: plaintext nats scheme not allowed in production; use tls or nats+tls")
	}
	if c.NATS.Token == "" && c.NATS.TLSCertFile == "" {
		errs = append(errs, "NATS: token authentication or an mTLS client certificate is required in production")
	}
	if len(c.NATS.AdvertiseURLs) == 0 {
		errs = append(errs, "NATS.AdvertiseURLs: at least one agent-reachable URL is required in production")
	}
	for _, raw := range c.NATS.AdvertiseURLs {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "tls" && u.Scheme != "nats+tls") {
			errs = append(errs, "NATS.AdvertiseURLs: production agent URLs must be absolute tls or nats+tls URLs without credentials")
			break
		}
		host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
		if host == "localhost" || host == "host.docker.internal" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback() {
			errs = append(errs, "NATS.AdvertiseURLs: production agent URLs must not use a loopback or container-local host")
			break
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
		JWT: JWTConfig{},
	}

	var errs []string

	if modeRaw := os.Getenv("STRATA_RUNTIME_MODE"); modeRaw != "" {
		mode, err := ParseRuntimeMode(modeRaw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("STRATA_RUNTIME_MODE: %v", err))
		} else {
			cfg.RuntimeMode = mode
		}
	} else {
		cfg.RuntimeMode = ModeDevelopment
	}

	if dsn, err := resolveDSN(); err != nil {
		errs = append(errs, fmt.Sprintf("database DSN: %v", err))
	} else {
		cfg.DB.DSN = dsn
	}
	cfg.NATS.URL = envStr("NATS_URL", cfg.NATS.URL)
	if raw := strings.TrimSpace(os.Getenv("NATS_ADVERTISE_URLS")); raw != "" {
		for _, candidate := range strings.Split(raw, ",") {
			if candidate = strings.TrimSpace(candidate); candidate != "" {
				cfg.NATS.AdvertiseURLs = append(cfg.NATS.AdvertiseURLs, candidate)
			}
		}
	}
	if value, err := secretEnv("NATS_TOKEN"); err != nil {
		errs = append(errs, fmt.Sprintf("NATS_TOKEN: %v", err))
	} else {
		cfg.NATS.Token = value
	}

	if v, err := envBoolStrict("NATS_TLS_ENABLED"); err != nil {
		errs = append(errs, fmt.Sprintf("NATS_TLS_ENABLED: %v", err))
	} else {
		cfg.NATS.TLSEnabled = v
	}
	cfg.NATS.TLSCertFile = envStr("NATS_TLS_CERT", "")
	cfg.NATS.TLSKeyFile = envStr("NATS_TLS_KEY", "")
	cfg.NATS.TLSCAFile = envStr("NATS_TLS_CA", "")

	if v, err := envDurationStrict("NATS_RECONNECT_WAIT", cfg.NATS.ReconnectWait); err != nil {
		errs = append(errs, fmt.Sprintf("NATS_RECONNECT_WAIT: %v", err))
	} else {
		cfg.NATS.ReconnectWait = v
	}
	if v, err := envIntStrict("NATS_MAX_RECONNECTS", cfg.NATS.MaxReconnects); err != nil {
		errs = append(errs, fmt.Sprintf("NATS_MAX_RECONNECTS: %v", err))
	} else {
		cfg.NATS.MaxReconnects = v
	}

	if v, err := envIntStrict("DB_MAX_OPEN_CONNS", cfg.DB.MaxOpenConns); err != nil {
		errs = append(errs, fmt.Sprintf("DB_MAX_OPEN_CONNS: %v", err))
	} else {
		cfg.DB.MaxOpenConns = v
	}
	if v, err := envIntStrict("DB_MAX_IDLE_CONNS", cfg.DB.MaxIdleConns); err != nil {
		errs = append(errs, fmt.Sprintf("DB_MAX_IDLE_CONNS: %v", err))
	} else {
		cfg.DB.MaxIdleConns = v
		if v < 0 {
			errs = append(errs, "DB_MAX_IDLE_CONNS: must be non-negative")
		}
	}
	if v, err := envDurationStrict("DB_CONN_MAX_LIFETIME", cfg.DB.ConnMaxLifetime); err != nil {
		errs = append(errs, fmt.Sprintf("DB_CONN_MAX_LIFETIME: %v", err))
	} else {
		cfg.DB.ConnMaxLifetime = v
	}

	cfg.Storage.Backend = envStr("STORAGE_BACKEND", "local")
	cfg.Storage.Bucket = envStr("STORAGE_BUCKET", "strata-recordings")
	cfg.Storage.Region = os.Getenv("STORAGE_REGION")
	cfg.Storage.Endpoint = os.Getenv("STORAGE_ENDPOINT")
	if value, err := secretEnv("STORAGE_ACCESS_KEY"); err != nil {
		errs = append(errs, fmt.Sprintf("STORAGE_ACCESS_KEY: %v", err))
	} else {
		cfg.Storage.AccessKey = value
	}
	if value, err := secretEnv("STORAGE_SECRET_KEY"); err != nil {
		errs = append(errs, fmt.Sprintf("STORAGE_SECRET_KEY: %v", err))
	} else {
		cfg.Storage.SecretKey = value
	}
	if v, err := envBoolStrict("STORAGE_USE_SSL"); err != nil {
		errs = append(errs, fmt.Sprintf("STORAGE_USE_SSL: %v", err))
	} else {
		cfg.Storage.UseSSL = v
	}
	cfg.Storage.KMSKeyID = os.Getenv("STORAGE_KMS_KEY_ID")

	if value, err := secretEnv("JWT_SECRET"); err != nil {
		errs = append(errs, fmt.Sprintf("JWT_SECRET: %v", err))
	} else {
		cfg.JWT.Secret = value
	}
	if value, err := secretEnv("STRATA_METRICS_TOKEN"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_METRICS_TOKEN: %v", err))
	} else {
		cfg.Observability.MetricsToken = value
	}
	cfg.SMTP.Host = strings.TrimSpace(os.Getenv("STRATA_SMTP_HOST"))
	if value, err := secretEnv("STRATA_ALERT_SLACK_URL"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_ALERT_SLACK_URL: %v", err))
	} else {
		cfg.AlertDelivery.SlackURL = value
	}
	if value, err := secretEnv("STRATA_ALERT_TEAMS_URL"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_ALERT_TEAMS_URL: %v", err))
	} else {
		cfg.AlertDelivery.TeamsURL = value
	}
	if value, err := secretEnv("STRATA_ALERT_WEBHOOK_URL"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_ALERT_WEBHOOK_URL: %v", err))
	} else {
		cfg.AlertDelivery.WebhookURL = value
	}
	if value, err := secretEnv("STRATA_ALERT_PAGERDUTY_KEY"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_ALERT_PAGERDUTY_KEY: %v", err))
	} else {
		cfg.AlertDelivery.PagerDutyKey = value
	}
	cfg.AlertDelivery.EmailRecipients = splitTrim(os.Getenv("STRATA_ALERT_EMAIL_RECIPIENTS"), ",")
	if value, err := secretEnv("STRATA_SMTP_USERNAME"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_SMTP_USERNAME: %v", err))
	} else {
		cfg.SMTP.Username = value
	}
	if value, err := secretEnv("STRATA_SMTP_PASSWORD"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_SMTP_PASSWORD: %v", err))
	} else {
		cfg.SMTP.Password = value
	}
	cfg.SMTP.FromAddress = strings.TrimSpace(os.Getenv("STRATA_SMTP_FROM"))
	if v, err := envIntStrict("STRATA_SMTP_PORT", 0); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_SMTP_PORT: %v", err))
	} else {
		cfg.SMTP.Port = v
	}
	if v, err := envBoolStrict("STRATA_SMTP_IMPLICIT_TLS"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_SMTP_IMPLICIT_TLS: %v", err))
	} else {
		cfg.SMTP.ImplicitTLS = v
	}

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
	if v, err := envDurationStrict("HTTP_READ_TIMEOUT", cfg.HTTP.ReadTimeout); err != nil {
		errs = append(errs, fmt.Sprintf("HTTP_READ_TIMEOUT: %v", err))
	} else {
		cfg.HTTP.ReadTimeout = v
	}
	if v, err := envDurationStrict("HTTP_WRITE_TIMEOUT", cfg.HTTP.WriteTimeout); err != nil {
		errs = append(errs, fmt.Sprintf("HTTP_WRITE_TIMEOUT: %v", err))
	} else {
		cfg.HTTP.WriteTimeout = v
	}
	if v, err := envDurationStrict("HTTP_IDLE_TIMEOUT", cfg.HTTP.IdleTimeout); err != nil {
		errs = append(errs, fmt.Sprintf("HTTP_IDLE_TIMEOUT: %v", err))
	} else {
		cfg.HTTP.IdleTimeout = v
	}
	if v, err := envInt64Strict("HTTP_MAX_BODY_SIZE", cfg.HTTP.MaxBodySizeBytes); err != nil {
		errs = append(errs, fmt.Sprintf("HTTP_MAX_BODY_SIZE: %v", err))
	} else {
		cfg.HTTP.MaxBodySizeBytes = v
	}

	if v, err := envBoolStrict("STRATA_SEED_DEV"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_SEED_DEV: %v", err))
	} else {
		cfg.Seeding.SeedDev = v
	}
	cfg.Seeding.DevAdminEmail = os.Getenv("STRATA_DEV_ADMIN_EMAIL")
	cfg.Seeding.DevAdminPwd = os.Getenv("STRATA_DEV_ADMIN_PASSWORD_HASH")

	// Backup configuration
	if v, err := envBoolStrict("STRATA_BACKUP_ENABLED"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_BACKUP_ENABLED: %v", err))
	} else {
		cfg.Backup.Enabled = v
	}
	cfg.Backup.EnvironmentID = os.Getenv("STRATA_BACKUP_ENVIRONMENT_ID")
	cfg.Backup.DatabaseType = envStr("STRATA_BACKUP_DATABASE_TYPE", "timescaledb")
	cfg.Backup.BackupDirectory = os.Getenv("STRATA_BACKUP_DIRECTORY")
	cfg.Backup.EncryptionScheme = envStr("STRATA_BACKUP_ENCRYPTION_SCHEME", "aes-256-gcm")
	cfg.Backup.ExternalBackupBucket = os.Getenv("STRATA_BACKUP_EXTERNAL_BUCKET")
	cfg.Backup.ExternalBackupRegion = os.Getenv("STRATA_BACKUP_EXTERNAL_REGION")
	cfg.Backup.ExternalBackupEndpoint = os.Getenv("STRATA_BACKUP_EXTERNAL_ENDPOINT")
	cfg.Backup.ExternalBackupAccessKey = os.Getenv("STRATA_BACKUP_EXTERNAL_ACCESS_KEY")
	cfg.Backup.ExternalBackupSecretKey = os.Getenv("STRATA_BACKUP_EXTERNAL_SECRET_KEY")
	cfg.Backup.RepositoryType = envStr("STRATA_BACKUP_REPOSITORY_TYPE", "filesystem")
	cfg.Backup.KeyProviderPath = os.Getenv("STRATA_BACKUP_KEY_PROVIDER_PATH")
	cfg.Backup.RecoveryStorageBackend = os.Getenv("STRATA_RECOVERY_STORAGE_BACKEND")
	cfg.Backup.RecoveryStorageBucket = os.Getenv("STRATA_RECOVERY_STORAGE_BUCKET")
	cfg.Backup.RecoveryStorageRegion = os.Getenv("STRATA_RECOVERY_STORAGE_REGION")
	cfg.Backup.RecoveryStorageEndpoint = os.Getenv("STRATA_RECOVERY_STORAGE_ENDPOINT")
	cfg.Backup.RecoveryStorageAccessKey = os.Getenv("STRATA_RECOVERY_STORAGE_ACCESS_KEY")
	cfg.Backup.RecoveryStorageSecretKey = os.Getenv("STRATA_RECOVERY_STORAGE_SECRET_KEY")
	cfg.Backup.RecoveryNATSURL = os.Getenv("STRATA_RECOVERY_NATS_URL")
	cfg.Backup.RecoveryNATSToken = os.Getenv("STRATA_RECOVERY_NATS_TOKEN")
	cfg.Backup.RecoveryNATSTLSCAFile = os.Getenv("STRATA_RECOVERY_NATS_TLS_CA")
	cfg.Backup.RecoveryNATSTLSCertFile = os.Getenv("STRATA_RECOVERY_NATS_TLS_CERT")
	cfg.Backup.RecoveryNATSTLSKeyFile = os.Getenv("STRATA_RECOVERY_NATS_TLS_KEY")
	if v, err := envBoolStrict("STRATA_RECOVERY_STORAGE_USE_SSL"); err != nil {
		errs = append(errs, fmt.Sprintf("STRATA_RECOVERY_STORAGE_USE_SSL: %v", err))
	} else {
		cfg.Backup.RecoveryStorageUseSSL = v
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("configuration load errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return cfg, nil
}

func resolveDSN() (string, error) {
	for _, key := range []string{"TIMESCALE_DSN", "STRATA_DB_DSN", "DATABASE_URL"} {
		value, err := secretEnv(key)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
	}
	return "postgres://localhost:5432/strata_rmm?sslmode=disable", nil
}

func secretEnv(key string) (string, error) {
	direct := os.Getenv(key)
	filePath := strings.TrimSpace(os.Getenv(key + "_FILE"))
	if direct != "" && filePath != "" {
		return "", fmt.Errorf("%s and %s_FILE are mutually exclusive", key, key)
	}
	if filePath == "" {
		return direct, nil
	}
	if !filepath.IsAbs(filePath) || filepath.Clean(filePath) != filePath {
		return "", fmt.Errorf("secret file path must be absolute and canonical")
	}

	// The operator explicitly supplies this absolute configuration path. The
	// validation above rejects relative and non-canonical traversal forms.
	file, err := os.Open(filePath) // #nosec G703 -- validated operator configuration path
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat secret file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("secret file must be a regular file")
	}
	if info.Size() > 16<<10 {
		return "", fmt.Errorf("secret file exceeds 16 KiB")
	}
	contents, err := io.ReadAll(io.LimitReader(file, (16<<10)+1))
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	if len(contents) > 16<<10 {
		return "", fmt.Errorf("secret file exceeds 16 KiB")
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(contents), "\n"), "\r")
	if value == "" {
		return "", fmt.Errorf("secret file is empty")
	}
	return value, nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBoolStrict(key string) (bool, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return false, nil
	}
	switch strings.ToLower(v) {
	case "true", "t", "yes", "1":
		return true, nil
	case "false", "f", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", v)
	}
}

func envIntStrict(key string, def int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", v, err)
	}
	return n, nil
}

func envInt64Strict(key string, def int64) (int64, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", v, err)
	}
	return n, nil
}

func envDurationStrict(key string, def time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", v, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid duration %q: must be positive", v)
	}
	return d, nil
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
		"runtime_mode":             string(c.RuntimeMode),
		"nats_url":                 redactURL(c.NATS.URL),
		"nats_tls":                 c.NATS.TLSEnabled,
		"db_dsn":                   redactDSN(c.DB.DSN),
		"db_pool_max":              c.DB.MaxOpenConns,
		"db_pool_idle":             c.DB.MaxIdleConns,
		"storage_type":             c.Storage.Backend,
		"storage_bucket":           c.Storage.Bucket,
		"api_addr":                 c.HTTP.APIAddr,
		"tunnel_addr":              c.HTTP.TunnelAddr,
		"public_url":               c.HTTP.PublicURL,
		"cors_origins":             c.HTTP.CORSOrigins,
		"jwt_configured":           c.JWT.Secret != "",
		"metrics_token_configured": c.Observability.MetricsToken != "",
		"smtp_configured":          c.SMTP.Configured(),
		"alert_delivery_channels":  len(c.AlertDelivery.EmailRecipients) + boolCount(c.AlertDelivery.SlackURL != "") + boolCount(c.AlertDelivery.TeamsURL != "") + boolCount(c.AlertDelivery.WebhookURL != "") + boolCount(c.AlertDelivery.PagerDutyKey != ""),
		"seed_dev":                 c.Seeding.SeedDev,
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
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
	if !strings.Contains(dsn, "://") {
		fields := strings.Fields(dsn)
		for index, field := range fields {
			key, _, found := strings.Cut(field, "=")
			if !found {
				continue
			}
			lower := strings.ToLower(key)
			if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "key") {
				fields[index] = key + "=***"
			}
		}
		return strings.Join(fields, " ")
	}
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

// RedactDSN removes credentials from PostgreSQL connection strings before
// logging or returning errors.
func RedactDSN(dsn string) string {
	return redactDSN(dsn)
}
