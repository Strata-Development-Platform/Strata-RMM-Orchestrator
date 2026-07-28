package config

import (
	"os"
	"testing"
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
		{"prod ", ModeProduction, false},
		{" production ", ModeProduction, false},
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

func TestParseBool(t *testing.T) {
	tests := []struct {
		input string
		want  bool
		err   bool
	}{
		{"true", true, false}, {"True", true, false}, {"TRUE", true, false},
		{"t", true, false}, {"T", true, false},
		{"yes", true, false}, {"YES", true, false},
		{"1", true, false},
		{"false", false, false}, {"False", false, false},
		{"f", false, false}, {"F", false, false},
		{"no", false, false}, {"NO", false, false},
		{"0", false, false}, {"", false, false},
		{"truthy", false, true}, {"ye", false, true},
		{"2", false, true}, {"-1", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseBool(tt.input)
			if tt.err && err == nil {
				t.Fatal("expected error")
			}
			if !tt.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.err && got != tt.want {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		err   bool
	}{
		{"42", 42, false}, {"0", 0, false}, {"-5", -5, false},
		{"", 0, true}, {"abc", 0, true}, {"12.5", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseInt(tt.input, 64)
			if tt.err && err == nil {
				t.Fatal("expected error")
			}
			if !tt.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.err && got != tt.want {
				t.Errorf("parseInt(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  bool
		err   bool
	}{
		{"5s", true, false}, {"10m", true, false}, {"1h", true, false},
		{"", false, true}, {"abc", false, true}, {"-5s", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.err && err == nil {
				t.Fatal("expected error")
			}
			if !tt.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.err && (got <= 0) != !tt.want {
				t.Errorf("parseDuration(%q) = %v", tt.input, got)
			}
		})
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		input string
		err   bool
	}{
		{"nats://localhost:4222", false},
		{"postgres://user:pass@localhost:5432/db", false},
		{"https://example.com", false},
		{"", true}, {"not-a-url", true}, {":invalid", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseURL(tt.input)
			if tt.err && err == nil {
				t.Fatal("expected error")
			}
			if !tt.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateProductionRejectsDevSecrets(t *testing.T) {
	cfg := &OrchestratorConfig{RuntimeMode: ModeDevelopment}
	err := cfg.ProductionValidate()
	if err != nil {
		t.Fatalf("development should not validate production: %v", err)
	}

	// Use a 32-char dev-prefixed secret to test the prefix check specifically
	cfg = &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "dev-xxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err = cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "development placeholder") {
		t.Fatalf("expected dev- rejection, got: %v", err)
	}
}

func TestValidateProductionRejectsShortSecret(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "short"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "32 characters") {
		t.Fatalf("expected 32-char error, got: %v", err)
	}
}

func TestValidateProductionRejectsSSLDisable(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d?sslmode=disable", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
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
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
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
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
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
		DB:          DatabaseConfig{DSN: "postgres://postgres:password@h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "default password") {
		t.Fatalf("expected default password error, got: %v", err)
	}
}

func TestValidateProductionRejectsSeedDev(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
		Seeding:     SeedingConfig{SeedDev: true},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "SeedDev") {
		t.Fatalf("expected SeedDev error, got: %v", err)
	}
}

func TestValidateProductionRejectsIdleOverMax(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 10, ConnMaxLifetime: 60},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "MaxIdleConns") {
		t.Fatalf("expected IdleConns error, got: %v", err)
	}
}

func TestValidateDevelopmentPassesWithDevDefaults(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeDevelopment,
		JWT:         JWTConfig{Secret: "short"},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
		DB:          DatabaseConfig{DSN: "postgres://localhost/d?sslmode=disable", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("development config should validate: %v", err)
	}
}

func TestValidateProductionRejectsLocalStorage(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeProduction,
		JWT:         JWTConfig{Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
		Storage:     StorageConfig{Backend: "local", Bucket: "b"},
	}
	err := cfg.ProductionValidate()
	if err == nil || !contains(err.Error(), "local backend") {
		t.Fatalf("expected local backend error, got: %v", err)
	}
}

func TestValidateRequiresNATSScheme(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeDevelopment,
		JWT:         JWTConfig{Secret: "test"},
		NATS:        NATSConfig{URL: "http://localhost:4222", ReconnectWait: 5},
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
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
	if jl, ok := summary["jwt_secret_len"].(int); !ok || jl == 0 {
		t.Error("jwt_secret_len should be present and non-zero")
	}
}

func TestLoadOrchestratorConfigDefaults(t *testing.T) {
	cfg := LoadOrchestratorConfig()
	if cfg.HTTP.APIAddr == "" {
		t.Error("APIAddr should have a default")
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

func TestAbsentNATSURLFails(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeDevelopment,
		DB:          DatabaseConfig{DSN: "postgres://h:1/d", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
		JWT:         JWTConfig{Secret: "test"},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
	}
	err := cfg.Validate()
	if err == nil || !contains(err.Error(), "NATS.URL") {
		t.Fatalf("expected NATS.URL error, got: %v", err)
	}
}

func TestValidateEmptyDNSFails(t *testing.T) {
	cfg := &OrchestratorConfig{
		RuntimeMode: ModeDevelopment,
		NATS:        NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 5},
		JWT:         JWTConfig{Secret: "test"},
		HTTP:        HTTPConfig{APIAddr: ":8080", ReadTimeout: 5, WriteTimeout: 5, MaxBodySizeBytes: 1000},
		DB:          DatabaseConfig{DSN: "postgres:///", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: 60},
	}
	err := cfg.Validate()
	if err == nil || !contains(err.Error(), "missing host") {
		t.Fatalf("expected missing host error, got: %v", err)
	}
}

func TestNATSValidateRejectsInvalidScheme(t *testing.T) {
	nc := NATSConfig{URL: "http://localhost:4222", ReconnectWait: 5}
	err := nc.Validate(ModeDevelopment)
	if err == nil || !contains(err.Error(), "scheme") {
		t.Fatalf("expected scheme error, got: %v", err)
	}
}

func TestNATSValidateRequiresReconnectWait(t *testing.T) {
	nc := NATSConfig{URL: "nats://localhost:4222", ReconnectWait: 0}
	err := nc.Validate(ModeDevelopment)
	if err == nil || !contains(err.Error(), "positive") {
		t.Fatalf("expected positive error, got: %v", err)
	}
}

func TestStorageValidateProductionRejectsMissingCreds(t *testing.T) {
	sc := StorageConfig{Backend: "s3", Bucket: "b", AccessKey: "", SecretKey: ""}
	err := sc.Validate(ModeProduction)
	if err == nil || !contains(err.Error(), "access key") {
		t.Fatalf("expected access key error, got: %v", err)
	}
}

func TestStorageValidateProductionRejectsLocal(t *testing.T) {
	sc := StorageConfig{Backend: "local", Bucket: "b"}
	err := sc.Validate(ModeProduction)
	if err == nil || !contains(err.Error(), "local backend") {
		t.Fatalf("expected local backend error, got: %v", err)
	}
}

func TestHTTPValidateProductionRejectsEmptyTimeouts(t *testing.T) {
	hc := HTTPConfig{APIAddr: ":8080", ReadTimeout: 0, WriteTimeout: 0, MaxBodySizeBytes: 0}
	err := hc.Validate(ModeProduction)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestSeedingValidateRejectsSeedInProduction(t *testing.T) {
	sc := SeedingConfig{SeedDev: true}
	err := sc.Validate(ModeProduction)
	if err == nil || !contains(err.Error(), "SeedDev") {
		t.Fatalf("expected SeedDev error, got: %v", err)
	}
}

func TestSeedingValidateAllowsInDevelopment(t *testing.T) {
	sc := SeedingConfig{SeedDev: true}
	err := sc.Validate(ModeDevelopment)
	if err != nil {
		t.Fatalf("SeedDev should be allowed in development: %v", err)
	}
}

func TestJWTValidateProductionRejectsDevPrefix(t *testing.T) {
	jc := JWTConfig{Secret: "dev-xxxxxxxxxxxxxxxxxxxxxxxxxxxx"}
	err := jc.Validate(ModeProduction)
	if err == nil || !contains(err.Error(), "placeholder") {
		t.Fatalf("expected placeholder rejection, got: %v", err)
	}
}

func TestJWTValidateProductionRejectsTestPrefix(t *testing.T) {
	jc := JWTConfig{Secret: "test-xxxxxxxxxxxxxxxxxxxxxxxxxxx"}
	err := jc.Validate(ModeProduction)
	if err == nil || !contains(err.Error(), "placeholder") {
		t.Fatalf("expected placeholder rejection, got: %v", err)
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
	return len(s) >= len(substr) && containsString(s, substr)
}

func containsString(s, substr string) bool {
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
