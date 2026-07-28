package config

import (
	"os"
	"testing"
)

func setenv(t *testing.T, key, value string) {
	t.Helper()
	prev := os.Getenv(key)
	os.Setenv(key, value)
	t.Cleanup(func() {
		if prev == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, prev)
		}
	})
}

func TestParseRuntimeMode(t *testing.T) {
	tests := []struct {
		input string
		want  RuntimeMode
		err   bool
	}{
		{"development", ModeDevelopment, false},
		{"dev", ModeDevelopment, false},
		{"test", ModeTest, false},
		{"production", ModeProduction, false},
		{"prod", ModeProduction, false},
		{"", ModeDevelopment, false},
		{"unknown", "", true},
		{"Production", ModeProduction, false},
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

func TestValidateProductionRejectsDevSecrets(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "dev-my-secret-key-here-12345678"},
		DB:          DatabaseConfig{DSN: "postgres://localhost:5432/strata_rmm?sslmode=disable"},
		NATS:        NATSConfig{URL: "nats://localhost:4222"},
		HTTP:        HTTPConfig{APIAddr: ":8080"},
	}
	err := cfg.ValidateProduction()
	if err == nil {
		t.Fatal("expected production validation error for dev placeholder JWT")
	}
}

func TestValidateProductionRejectsShortSecret(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "short"},
		DB:          DatabaseConfig{DSN: "postgres://localhost:5432/strata_rmm?sslmode=disable"},
		NATS:        NATSConfig{URL: "nats://localhost:4222"},
		HTTP:        HTTPConfig{APIAddr: ":8080"},
	}
	err := cfg.ValidateProduction()
	if err == nil {
		t.Fatal("expected production validation error for short JWT")
	}
}

func TestValidateProductionRejectsSSLDisable(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://localhost:5432/strata_rmm?sslmode=disable"},
		NATS:        NATSConfig{URL: "nats://localhost:4222"},
		HTTP:        HTTPConfig{APIAddr: ":8080"},
	}
	err := cfg.ValidateProduction()
	if err == nil {
		t.Fatal("expected production validation error for sslmode=disable")
	}
}

func TestValidateProductionRejectsWildcardCORS(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://localhost:5432/strata_rmm"},
		NATS:        NATSConfig{URL: "nats://localhost:4222"},
		HTTP:        HTTPConfig{APIAddr: ":8080", CORSOrigins: []string{"*"}},
	}
	err := cfg.ValidateProduction()
	if err == nil {
		t.Fatal("expected production validation error for wildcard CORS")
	}
}

func TestValidateProductionRejectsDefaultPassword(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://postgres:password@localhost:5432/strata_rmm"},
		NATS:        NATSConfig{URL: "nats://localhost:4222"},
		HTTP:        HTTPConfig{APIAddr: ":8080"},
	}
	err := cfg.ValidateProduction()
	if err == nil {
		t.Fatal("expected production validation error for default password")
	}
}

func TestValidateProductionRejectsHTTPPublicURL(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://localhost:5432/strata_rmm"},
		NATS:        NATSConfig{URL: "nats://localhost:4222"},
		HTTP:        HTTPConfig{APIAddr: ":8080", PublicURL: "http://example.com"},
	}
	err := cfg.ValidateProduction()
	if err == nil {
		t.Fatal("expected production validation error for http public URL")
	}
}

func TestValidateDevelopmentPasses(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeDevelopment,
		JWT:         JWTConfig{Secret: "short"},
		DB:          DatabaseConfig{DSN: "postgres://localhost:5432/strata_rmm?sslmode=disable"},
		NATS:        NATSConfig{URL: "nats://localhost:4222"},
		HTTP:        HTTPConfig{APIAddr: ":8080"},
	}
	err := cfg.ValidateProduction()
	if err != nil {
		t.Fatalf("development should pass production validation (it's skipped): %v", err)
	}
}

func TestRedactedSummaryNoSecrets(t *testing.T) {
	cfg := &OrchestratorConfig{
		JWT: JWTConfig{Secret: "my-super-secret-key-that-is-long-enough-32"},
		DB:  DatabaseConfig{DSN: "postgres://user:secretpass@localhost:5432/db"},
	}
	summary := cfg.RedactedSummary()
	if jl, ok := summary["jwt_secret_len"].(int); ok && jl == 0 {
		t.Error("jwt_secret_len should be non-zero")
	}
	if _, ok := summary["jwt_secret"]; ok {
		t.Error("jwt_secret should not appear in redacted summary")
	}
}

func TestLoadOrchestratorConfigDefaults(t *testing.T) {
	cfg := LoadOrchestratorConfig()
	if cfg.HTTP.APIAddr == "" {
		t.Error("APIAddr should default to something")
	}
	if cfg.DB.MaxOpenConns <= 0 {
		t.Error("MaxOpenConns should be positive")
	}
	if cfg.RuntimeMode != ModeDevelopment {
		t.Error("default runtime mode should be development")
	}
}

func TestLoadOrchestratorConfigFromEnv(t *testing.T) {
	setenv(t, "STRATA_RUNTIME_MODE", "production")
	setenv(t, "NATS_URL", "nats://example.com:4222")
	setenv(t, "JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	setenv(t, "STRATA_API_ADDR", ":9090")

	cfg := LoadOrchestratorConfig()
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

func TestAbsentRequiredSettingFailsValidation(t *testing.T) {
	cfg := &OrchestratorConfig{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty config")
	}
	if err.Error() != "HTTP.APIAddr is required" {
		t.Logf("got error: %v", err)
	}
}

func TestValidateProductionIdleConnsExceedsMax(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://localhost:5432/test", MaxOpenConns: 10, MaxIdleConns: 20},
		NATS:        NATSConfig{URL: "nats://localhost:4222"},
		HTTP:        HTTPConfig{APIAddr: ":8080"},
	}
	err := cfg.ValidateProduction()
	if err == nil {
		t.Fatal("expected error for idle > max")
	}
}
