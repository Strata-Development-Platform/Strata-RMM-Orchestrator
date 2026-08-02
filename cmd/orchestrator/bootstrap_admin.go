package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/config"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func newBootstrapAdminCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var email, passwordFile, tenantName string

	cmd := &cobra.Command{
		Use:   "bootstrap-admin",
		Short: "Create the first local platform administrator",
		Long:  "Creates the initial administrator exactly once. The password is read from a protected local file and is never accepted as a command-line argument.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			password, err := readBootstrapPassword(passwordFile)
			if err != nil {
				return err
			}
			defer clearBytes(password)

			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			db, err := timescale.NewClient(ctx, cfg.DB.DSN)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer db.Close()

			schema := postgres.NewSchemaManager(db.DB())
			if err := schema.Apply(ctx); err != nil {
				return fmt.Errorf("apply schema before bootstrap: %w", err)
			}

			userID, err := postgres.BootstrapInitialAdmin(ctx, db.DB(), postgres.BootstrapAdminInput{
				Email:      email,
				Password:   string(password),
				TenantName: tenantName,
			})
			if err != nil {
				return fmt.Errorf("bootstrap administrator: %w", err)
			}
			logger.Info("initial administrator created", zap.String("user_id", userID), zap.String("email", email))
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Initial administrator email address")
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "Protected file containing the initial administrator password")
	cmd.Flags().StringVar(&tenantName, "tenant-name", "Platform Administration", "Initial platform tenant name")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("password-file")
	return cmd
}

func readBootstrapPassword(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("password file is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat password file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("password file must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("password file must not be accessible by group or others")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open password file: %w", err)
	}
	defer func() { _ = file.Close() }()

	password, err := io.ReadAll(io.LimitReader(file, 74))
	if err != nil {
		return nil, fmt.Errorf("read password file: %w", err)
	}
	password = bytes.TrimSuffix(password, []byte("\n"))
	password = bytes.TrimSuffix(password, []byte("\r"))
	if len(password) > 72 {
		clearBytes(password)
		return nil, fmt.Errorf("administrator password must not exceed 72 bytes")
	}
	return password, nil
}

func clearBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
