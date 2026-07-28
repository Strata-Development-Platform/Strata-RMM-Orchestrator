package config

import (
	"os"
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
	for _, v := range []string{"false", "", "0", "maybe", "truthy"} {
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

func TestEnvDurationLoaderInvalidDefaults(t *testing.T) {
	setenv(t, "HTTP_READ_TIMEOUT", "tomorrow")
	cfg, err := LoadOrchestratorConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTP.ReadTimeout <= 0 {
		t.Error("ReadTimeout should use default when env is invalid")
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

func TestEnvIntLoaderInvalidKeepsDefault(t *testing.T) {
	setenv(t, "DB_MAX_OPEN_CONNS", "abc")
	cfg, err := LoadOrchestratorConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DB.MaxOpenConns != 25 {
		t.Errorf("MaxOpenConns should use default when env is invalid, got %d", cfg.DB.MaxOpenConns)
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
