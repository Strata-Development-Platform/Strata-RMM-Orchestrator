package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/config"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/recovery"
)

func TestRecoveryRedactDSN(t *testing.T) {
	for _, dsn := range []string{
		"postgres://operator:super-secret@db.internal/strata?sslmode=require",
		"host=db.internal user=operator password=super-secret dbname=strata",
	} {
		redacted := redactDSN(dsn)
		require.NotContains(t, redacted, "super-secret")
	}
}

func TestRestoreRequiresSafetyInputsBeforeInfrastructure(t *testing.T) {
	cfg := &config.OrchestratorConfig{}
	logger := zap.NewNop()
	require.ErrorContains(t, runRestore(cfg, "", "", false, 0, false, logger), "--backup-id")
	require.ErrorContains(t, runRestore(cfg, "backup", "", false, 0, false, logger), "--target-dsn")
	require.ErrorContains(t, runRestore(cfg, "backup", "target", false, 0, false, logger), "--confirm")
}

func TestValidateRecoveryTransportProduction(t *testing.T) {
	secure := config.NATSConfig{
		URL:        "tls://nats.internal:4222",
		Token:      "token",
		TLSEnabled: true,
		TLSCAFile:  "/run/secrets/nats-ca.pem",
	}
	require.NoError(t, validateRecoveryTransport(config.ModeProduction, secure, "postgres://db/strata?sslmode=verify-full"))

	insecure := secure
	insecure.URL = "nats://nats.internal:4222"
	require.ErrorContains(t, validateRecoveryTransport(config.ModeProduction, insecure, "postgres://db/strata?sslmode=verify-full"), "must use")

	insecure = secure
	insecure.Token = ""
	require.ErrorContains(t, validateRecoveryTransport(config.ModeProduction, insecure, "postgres://db/strata?sslmode=verify-full"), "authentication")
	require.ErrorContains(t, validateRecoveryTransport(config.ModeProduction, secure, "postgres://db/strata?sslmode=disable"), "must not disable TLS")
}

func TestRecoveryKeyInitCreatesOneActiveKey(t *testing.T) {
	root := t.TempDir()
	cfg := &config.OrchestratorConfig{
		Backup: config.BackupConfig{
			EnvironmentID:   "test-environment",
			KeyProviderPath: filepath.Join(root, "keys"),
		},
	}
	require.NoError(t, runKeyInit(cfg, zap.NewNop()))
	require.Error(t, runKeyInit(cfg, zap.NewNop()), "second initialization must not rotate the active key")
	provider, err := recovery.NewFileKeyProvider(cfg.Backup.KeyProviderPath)
	require.NoError(t, err)
	keys, err := provider.ListKeys(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.True(t, keys[0].Active)
	keyData, err := os.ReadFile(filepath.Join(cfg.Backup.KeyProviderPath, keys[0].ID+".key"))
	require.NoError(t, err)
	require.Len(t, keyData, 32)
	info, err := os.Stat(filepath.Join(cfg.Backup.KeyProviderPath, keys[0].ID+".key"))
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0077, "key material must not be group/world accessible")
	require.False(t, strings.Contains(string(keyData), "test-environment"))
}
