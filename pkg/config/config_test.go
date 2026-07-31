package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseRuntimeMode(t *testing.T) {
	tests := []struct {
		input string
		want  RuntimeMode
		err   bool
	}{
		{"development", ModeDevelopment, false},
		{"dev", ModeDevelopment, false},
		{"Development", ModeDevelopment, false},
		{"  development  ", ModeDevelopment, false},
		{"test", ModeTest, false},
		{"production", ModeProduction, false},
		{"prod", ModeProduction, false},
		{"Production", ModeProduction, false},
		{"", ModeDevelopment, false},
		{"unknown", "", true},
		{"produciton", "", true},
		{"devv", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseRuntimeMode(tt.input)
			if tt.err && err == nil {
				t.Fatal("expected error")
			}
			if !tt.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.err && got != tt.want {
				t.Errorf("ParseRuntimeMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoaderReturnsErrorForMisspelledMode(t *testing.T) {
	setenv(t, "STRATA_RUNTIME_MODE", "produciton")
	_, err := LoadOrchestratorConfig()
	if err == nil {
		t.Fatal("expected error for misspelled runtime mode")
	}
	if !contains(err.Error(), "STRATA_RUNTIME_MODE") {
		t.Errorf("expected error to name STRATA_RUNTIME_MODE, got: %v", err)
	}
}

func TestLoaderDefaultsToDevelopment(t *testing.T) {
	cfg, err := LoadOrchestratorConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RuntimeMode != ModeDevelopment {
		t.Errorf("default runtime mode should be development, got %q", cfg.RuntimeMode)
	}
}

func TestLoaderAcceptsValidModes(t *testing.T) {
	for _, mode := range []string{"development", "dev", "test", "production", "prod"} {
		setenv(t, "STRATA_RUNTIME_MODE", mode)
		cfg, err := LoadOrchestratorConfig()
		if err != nil {
			t.Fatalf("unexpected error for mode %q: %v", mode, err)
		}
		if cfg.RuntimeMode.String() == "" {
			t.Errorf("mode %q: runtime mode should be set", mode)
		}
	}
}

func TestLoaderFromEnv(t *testing.T) {
	setenv(t, "STRATA_RUNTIME_MODE", "production")
	setenv(t, "NATS_URL", "nats://example.com:4222")
	setenv(t, "JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	setenv(t, "STRATA_API_ADDR", ":9090")

	cfg, err := LoadOrchestratorConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RuntimeMode != ModeProduction {
		t.Errorf("runtime mode = %q, want production", cfg.RuntimeMode)
	}
	if cfg.NATS.URL != "nats://example.com:4222" {
		t.Errorf("NATS URL = %q", cfg.NATS.URL)
	}
	if cfg.HTTP.APIAddr != ":9090" {
		t.Errorf("API addr = %q", cfg.HTTP.APIAddr)
	}
}

func TestMetricsTokenValidationAndRedaction(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeTest,
		NATS: NATSConfig{
			URL:           "nats://localhost:4222",
			ReconnectWait: time.Second,
		},
		DB: DatabaseConfig{
			DSN:             "postgres://localhost/test?sslmode=disable",
			MaxOpenConns:    2,
			MaxIdleConns:    1,
			ConnMaxLifetime: time.Minute,
		},
		JWT: JWTConfig{Secret: "0123456789abcdef0123456789abcdef"},
		HTTP: HTTPConfig{
			APIAddr:          ":8080",
			ReadTimeout:      time.Second,
			WriteTimeout:     time.Second,
			MaxBodySizeBytes: 1024,
		},
	}
	cfg.Observability.MetricsToken = "short"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "MetricsToken") {
		t.Fatalf("expected short metrics token rejection, got %v", err)
	}

	cfg.Observability.MetricsToken = "0123456789abcdef0123456789abcdef"
	summary := cfg.RedactedSummary()
	if summary["metrics_token_configured"] != true {
		t.Fatal("redacted summary should report metrics token presence")
	}
	if strings.Contains(fmt.Sprint(summary), cfg.Observability.MetricsToken) {
		t.Fatal("metrics token leaked in redacted summary")
	}
}

func TestLoadMetricsTokenFromEnvironment(t *testing.T) {
	const token = "abcdef0123456789abcdef0123456789"
	setenv(t, "STRATA_METRICS_TOKEN", token)
	cfg, err := LoadOrchestratorConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Observability.MetricsToken != token {
		t.Fatal("metrics token did not reach observability configuration")
	}
}

func TestProductionRequiresMetricsToken(t *testing.T) {
	cfg := productionConfig(NATSConfig{
		URL: "tls://nats.example.com:4222", Token: "validtok", TLSEnabled: true,
		TLSCAFile: "ca.pem", ReconnectWait: 5, MaxReconnects: -1,
	})
	cfg.Observability.MetricsToken = ""
	err := cfg.ProductionValidate()
	if err == nil || !strings.Contains(err.Error(), "MetricsToken") {
		t.Fatalf("expected missing metrics token rejection, got %v", err)
	}
}

func TestProductionRejectsUnauthenticatedRawTunnelGateway(t *testing.T) {
	cfg := productionConfig(NATSConfig{
		URL: "tls://nats.example.com:4222", Token: "validtok", TLSEnabled: true,
		TLSCAFile: "ca.pem", AdvertiseURLs: []string{"tls://agents.example.com:4222"},
		ReconnectWait: 5, MaxReconnects: -1,
	})
	cfg.HTTP.TunnelAddr = ":9091"
	err := cfg.ProductionValidate()
	if err == nil || !strings.Contains(err.Error(), "raw tunnel gateway") {
		t.Fatalf("expected unsafe tunnel rejection, got %v", err)
	}
}

func TestProductionRejectsLoopbackAdvertisedNATSURL(t *testing.T) {
	cfg := productionConfig(NATSConfig{
		URL: "tls://nats.example.com:4222", Token: "validtok", TLSEnabled: true,
		TLSCAFile: "ca.pem", AdvertiseURLs: []string{"tls://127.0.0.1:4222"},
		ReconnectWait: 5, MaxReconnects: -1,
	})
	err := cfg.ProductionValidate()
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback advertised NATS URL rejection, got %v", err)
	}
}

func TestValidateProductionRejectsDevPlaceholder(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "dev-xxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("base config should validate: %v", err)
	}
	err = cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "development placeholder") {
		t.Fatalf("expected development placeholder rejection, got: %v", err)
	}
}

func TestValidateProductionRejectsShortSecret(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "short"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.Validate()
	if err == nil || !contains(err.Error(), "32 characters") {
		t.Fatalf("expected 32-char error, got: %v", err)
	}
}

func TestValidateProductionRejectsSSLDisable(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d?sslmode=disable", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "sslmode=disable") {
		t.Fatalf("expected sslmode error, got: %v", err)
	}
}

func TestValidateProductionRejectsWildcardCORS(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000, CORSOrigins: []string{"*"}},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "wildcard") {
		t.Fatalf("expected wildcard error, got: %v", err)
	}
}

func TestValidateProductionRejectsHTTPPublicURL(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000, PublicURL: "http://example.com"},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "https") {
		t.Fatalf("expected https error, got: %v", err)
	}
}

func TestValidateProductionRejectsDefaultPassword(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://postgres:password@h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "default password") {
		t.Fatalf("expected default password error, got: %v", err)
	}
}

func TestValidateProductionRejectsIdleOverMax(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 10, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.Validate()
	if err == nil || !contains(err.Error(), "MaxIdleConns") {
		t.Fatalf("expected MaxIdleConns error, got: %v", err)
	}
}

func TestValidateDevelopmentPassesWithDevDefaults(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeDevelopment,
		JWT:         JWTConfig{Secret: "test-secret-key-long-enough-32chars!!"},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		DB:          DatabaseConfig{DSN: "postgres://localhost/d?sslmode=disable", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("development config should validate: %v", err)
	}
}

func TestValidateRejectsEmptyNATSURL(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeDevelopment,
		JWT:         JWTConfig{Secret: "test-secret-key-long-enough-32chars!!"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.Validate()
	if err == nil || !contains(err.Error(), "NATS.URL") {
		t.Fatalf("expected NATS.URL error, got: %v", err)
	}
}

func TestValidateRejectsInvalidNATSScheme(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeDevelopment,
		JWT:         JWTConfig{Secret: "test-secret-key-long-enough-32chars!!"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "http://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.Validate()
	if err == nil || !contains(err.Error(), "scheme") {
		t.Fatalf("expected scheme error, got: %v", err)
	}
}

func TestRedactedSummaryNoSecrets(t *testing.T) {
	cfg := &OrchestratorConfig{
		JWT: JWTConfig{Secret: "my-super-secret-key-that-is-long-enough-32"},
		DB:  DatabaseConfig{DSN: "postgres://user:secretpass@localhost:5432/db"},
	}
	summary := cfg.RedactedSummary()
	if _, ok := summary["jwt_secret"]; ok {
		t.Error("jwt_secret should not appear in redacted summary")
	}
	if jl, ok := summary["jwt_configured"].(bool); !ok || !jl {
		t.Error("jwt_configured should be true")
	}
}

func TestValidateForEmptyDSN(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeDevelopment,
		JWT:         JWTConfig{Secret: "test-secret-key-long-enough-32chars!!"},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.Validate()
	if err == nil || !contains(err.Error(), "DB.DSN") {
		t.Fatalf("expected DB.DSN error, got: %v", err)
	}
}

func TestEnvBoolLoader(t *testing.T) {
	for _, v := range []string{"true", "True", "yes", "YES", "1", "t", "T"} {
		setenv(t, "NATS_TLS_ENABLED", v)
		cfg, err := LoadOrchestratorConfig()
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", v, err)
		}
		if !cfg.NATS.TLSEnabled {
			t.Errorf("NATS_TLS_ENABLED=%q should be true", v)
		}
	}
	for _, v := range []string{"false", "", "0"} {
		setenv(t, "NATS_TLS_ENABLED", v)
		cfg, err := LoadOrchestratorConfig()
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", v, err)
		}
		if cfg.NATS.TLSEnabled {
			t.Errorf("NATS_TLS_ENABLED=%q should be false", v)
		}
	}
}

func TestEnvDurationLoader(t *testing.T) {
	setenv(t, "HTTP_READ_TIMEOUT", "5s")
	cfg, err := LoadOrchestratorConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTP.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout = %v, want 5s", cfg.HTTP.ReadTimeout)
	}
}

func TestEnvIntLoader(t *testing.T) {
	setenv(t, "DB_MAX_OPEN_CONNS", "42")
	cfg, err := LoadOrchestratorConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DB.MaxOpenConns != 42 {
		t.Errorf("MaxOpenConns = %d, want 42", cfg.DB.MaxOpenConns)
	}
}

func TestLoadOrchestratorConfigMalformedEnv(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"STRATA_SEED_DEV", "truthy"},
		{"NATS_TLS_ENABLED", "enabled"},
		{"STORAGE_USE_SSL", "maybe"},
		{"DB_MAX_OPEN_CONNS", "abc"},
		{"DB_MAX_IDLE_CONNS", "-1"},
		{"DB_CONN_MAX_LIFETIME", "tomorrow"},
		{"NATS_RECONNECT_WAIT", "-5s"},
		{"NATS_MAX_RECONNECTS", "abc"},
		{"HTTP_READ_TIMEOUT", "tomorrow"},
		{"HTTP_WRITE_TIMEOUT", "0s"},
		{"HTTP_IDLE_TIMEOUT", "-1s"},
		{"HTTP_MAX_BODY_SIZE", "large"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			setenv(t, tt.key, tt.value)
			_, err := LoadOrchestratorConfig()
			if err == nil {
				t.Errorf("expected error for %s=%s", tt.key, tt.value)
			}
		})
	}
}

func TestValidateRejectsMissingPublicURLInProduction(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "PublicURL") {
		t.Fatalf("expected PublicURL error, got: %v", err)
	}
}

func TestValidateRejectsSeedDevInProduction(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5, MaxReconnects: -1},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
		Seeding:     SeedingConfig{SeedDev: true},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "SeedDev") {
		t.Fatalf("expected SeedDev error, got: %v", err)
	}
}

func productionConfig(natsCfg NATSConfig) *OrchestratorConfig {
	if len(natsCfg.AdvertiseURLs) == 0 {
		natsCfg.AdvertiseURLs = []string{"tls://nats.example.com:4222"}
	}
	return &OrchestratorConfig{
		RuntimeMode:   ModeProduction,
		JWT:           JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:            DatabaseConfig{DSN: "postgres://user:strong-secret@db:5432/strata?sslmode=require", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 10},
		NATS:          natsCfg,
		HTTP:          HTTPConfig{APIAddr: ":8080", PublicURL: "https://rmm.example.com", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
		Observability: ObservabilityConfig{MetricsToken: "0123456789abcdef0123456789abcdef"},
	}
}

func TestLoadAgentNATSAdvertiseURLs(t *testing.T) {
	setenv(t, "NATS_ADVERTISE_URLS", " tls://nats-a.example.com:4222, nats+tls://nats-b.example.com:4222 ")
	cfg, err := LoadOrchestratorConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.NATS.AdvertiseURLs) != 2 || cfg.NATS.AdvertiseURLs[0] != "tls://nats-a.example.com:4222" {
		t.Fatalf("NATS advertise URLs = %#v", cfg.NATS.AdvertiseURLs)
	}
}

func TestProductionRejectsMissingAgentNATSAdvertiseURL(t *testing.T) {
	cfg := productionConfig(NATSConfig{URL: "tls://nats.internal:4222", Token: "validtok", TLSEnabled: true, TLSCAFile: "ca.pem"})
	cfg.NATS.AdvertiseURLs = nil
	if err := cfg.ProductionValidate(); err == nil || !strings.Contains(err.Error(), "AdvertiseURLs") {
		t.Fatalf("ProductionValidate() error = %v, want agent advertise URL failure", err)
	}
}

func TestProductionNATSRejectsPlaintextEvenWithToken(t *testing.T) {
	cfg := productionConfig(NATSConfig{URL: "nats://example.com:4222", Token: "validtok", ReconnectWait: 5, MaxReconnects: -1})
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "TLS is required") || !contains(err.Error(), "plaintext nats scheme") {
		t.Fatalf("expected plaintext transport rejection, got: %v", err)
	}
}

func TestProductionNATSRejectsMissingCA(t *testing.T) {
	cfg := productionConfig(NATSConfig{URL: "tls://nats.example.com:4222", Token: "validtok", TLSEnabled: true, ReconnectWait: 5, MaxReconnects: -1})
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "TLS CA file") {
		t.Fatalf("expected missing CA rejection, got: %v", err)
	}
}

func TestProductionNATSRejectsPartialMTLSIdentity(t *testing.T) {
	cfg := productionConfig(NATSConfig{URL: "tls://nats.example.com:4222", TLSEnabled: true, TLSCAFile: "ca.pem", TLSCertFile: "client.pem", ReconnectWait: 5, MaxReconnects: -1})
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "certificate and key") {
		t.Fatalf("expected partial mTLS identity rejection, got: %v", err)
	}
}

func TestProductionNATSWithTLSAndTokenAccepted(t *testing.T) {
	cfg := productionConfig(NATSConfig{URL: "tls://nats.example.com:4222", Token: "validtok", TLSEnabled: true, TLSCAFile: "ca.pem", ReconnectWait: 5, MaxReconnects: -1})
	if err := cfg.ProductionValidate(); err != nil {
		t.Fatalf("expected CA-validated TLS with token authentication to pass: %v", err)
	}
}

func TestProductionNATSWithMTLSAccepted(t *testing.T) {
	cfg := productionConfig(NATSConfig{URL: "tls://nats.example.com:4222", TLSEnabled: true, TLSCAFile: "ca.pem", TLSCertFile: "client.pem", TLSKeyFile: "client-key.pem", ReconnectWait: 5, MaxReconnects: -1})
	if err := cfg.ProductionValidate(); err != nil {
		t.Fatalf("expected CA-validated mTLS to pass: %v", err)
	}
}

func TestRedactDSN(t *testing.T) {
	r := redactDSN("postgres://user:secretpass@localhost:5432/db")
	if contains(r, "secretpass") {
		t.Error("redacted DSN should not contain password")
	}
}

func TestRedactURL(t *testing.T) {
	r := redactURL("nats://token:supersecret@localhost:4222")
	if contains(r, "supersecret") {
		t.Error("redacted URL should not contain secret")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func setenv(t *testing.T, key, value string) {
	t.Helper()
	prev := os.Getenv(key)
	_ = os.Setenv(key, value)
	t.Cleanup(func() {
		if prev == "" {
			_ = os.Unsetenv(key)
		} else {
			_ = os.Setenv(key, prev)
		}
	})
}

func TestSecretEnvReadsFileWithoutLoggingValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("file-secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SECRET", "")
	t.Setenv("TEST_SECRET_FILE", path)

	got, err := secretEnv("TEST_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if got != "file-secret-value" {
		t.Fatal("secret file value was not loaded")
	}
}

func TestSecretEnvRejectsAmbiguousSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("file-secret-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SECRET", "environment-secret")
	t.Setenv("TEST_SECRET_FILE", path)

	if _, err := secretEnv("TEST_SECRET"); err == nil {
		t.Fatal("expected direct and file secret sources to be rejected")
	}
}
