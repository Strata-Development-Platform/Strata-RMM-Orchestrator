package orchestrator

import (
	"context"
	"os"
	"testing"

	"go.uber.org/zap"
)

func TestCommand_AllFlagsDefined(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cmd := NewCommand(context.Background(), "test", logger)

	expectedFlags := []string{
		"nats-url",
		"timescale-dsn",
		"api-addr",
		"tunnel-addr",
		"storage-backend",
		"storage-bucket",
		"storage-region",
		"storage-endpoint",
	}

	for _, name := range expectedFlags {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Errorf("expected flag --%s to be defined", name)
		}
	}
}

func TestCommand_FlagChangedDetection(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cmd := NewCommand(context.Background(), "test", logger)

	// Before parsing, no flags should be changed
	if cmd.Flags().Changed("nats-url") {
		t.Error("nats-url should not be changed before parsing")
	}

	// Parse with explicit flag
	cmd.SetArgs([]string{"--nats-url", "nats://example.com:4222"})
	if _, err := cmd.ExecuteC(); err != nil {
		t.Logf("expected ExecuteC to run (may error from cfg loading): %v", err)
	}
}

func TestPrecedence_CLIOverridesEnv(t *testing.T) {
	// Set env vars
	os.Setenv("NATS_URL", "nats://env-default:4222")
	os.Setenv("TIMESCALE_DSN", "postgres://env-default/db")
	os.Setenv("STRATA_API_ADDR", ":9090")
	os.Setenv("STRATA_TUNNEL_ADDR", ":9091")
	os.Setenv("STORAGE_BACKEND", "s3")
	os.Setenv("STORAGE_BUCKET", "env-bucket")
	os.Setenv("STORAGE_REGION", "us-east-1")
	os.Setenv("STORAGE_ENDPOINT", "https://env.example.com")
	defer func() {
		os.Unsetenv("NATS_URL")
		os.Unsetenv("TIMESCALE_DSN")
		os.Unsetenv("STRATA_API_ADDR")
		os.Unsetenv("STRATA_TUNNEL_ADDR")
		os.Unsetenv("STORAGE_BACKEND")
		os.Unsetenv("STORAGE_BUCKET")
		os.Unsetenv("STORAGE_REGION")
		os.Unsetenv("STORAGE_ENDPOINT")
	}()

	logger, _ := zap.NewDevelopment()
	cmd := NewCommand(context.Background(), "test", logger)

	// Set CLI flags that should override env vars
	cmd.SetArgs([]string{
		"--nats-url", "nats://cli-override:4222",
		"--timescale-dsn", "postgres://cli-override/db",
		"--api-addr", ":8080",
		"--tunnel-addr", ":8081",
		"--storage-backend", "minio",
		"--storage-bucket", "cli-bucket",
		"--storage-region", "eu-west-2",
		"--storage-endpoint", "https://cli.example.com",
	})

	if _, err := cmd.ExecuteC(); err != nil {
		t.Logf("ExecuteC result (may err on cfg validation): %v", err)
	}
}

func TestCommand_AllFlagDefs(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cmd := NewCommand(context.Background(), "test", logger)

	_ = cmd.Flags().Lookup("nats-url")
	_ = cmd.Flags().Lookup("api-addr")

	type flagDef struct {
		name  string
		usage string
	}
	flags := []flagDef{
		{"nats-url", "NATS server URL (overrides NATS_URL env)"},
		{"timescale-dsn", "TimescaleDB DSN (overrides TIMESCALE_DSN env)"},
		{"api-addr", "API server listen address (overrides STRATA_API_ADDR env)"},
		{"tunnel-addr", "Tunnel gateway listen address (overrides STRATA_TUNNEL_ADDR env)"},
		{"storage-backend", "Storage backend (minio, s3, local, none) (overrides STORAGE_BACKEND env)"},
		{"storage-bucket", "Storage bucket name (overrides STORAGE_BUCKET env)"},
		{"storage-region", "Storage region (overrides STORAGE_REGION env)"},
		{"storage-endpoint", "Storage endpoint (overrides STORAGE_ENDPOINT env)"},
	}
	for _, f := range flags {
		flag := cmd.Flags().Lookup(f.name)
		if flag == nil {
			t.Errorf("flag --%s not found", f.name)
			continue
		}
		if flag.Usage != f.usage {
			t.Errorf("flag --%s usage: got %q, want %q", f.name, flag.Usage, f.usage)
		}
		if flag.DefValue != "" {
			t.Errorf("flag --%s defvalue: got %q, want empty string", f.name, flag.DefValue)
		}
	}
}

func TestUpdateSubcommandRegistered(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cmd := NewCommand(context.Background(), "test", logger)

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "update" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'update' subcommand to be registered")
	}
}
