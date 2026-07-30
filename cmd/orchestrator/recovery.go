package orchestrator

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/backup"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/config"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func newRecoveryCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var (
		backupID    string
		dryRun      bool
		timeout     string
		force       bool
		bearerToken string
	)

	cmd := &cobra.Command{
		Use:   "recover [backup-id]",
		Short: "Execute disaster recovery",
		Long: `Execute disaster recovery from a backup. This command coordinates the 20-state recovery workflow including:
		- Environment-scoped locking (pg_try_advisory_lock)
		- Service quiescing
		- Database, JetStream, and object storage restore
		- Post-restore validation
		- RPO/RTO measurement
		- Automatic rollback on failure

		The recovery process acquires an exclusive advisory lock to prevent concurrent recoveries. If dry-run is enabled, all state transitions execute but no side effects occur. Use --force to bypass pre-flight validation (use with caution).

		REQUIRES: Platform administrator privileges (platform_admin role).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate JWT configuration first
			if err := auth.ValidateJWTConfig(); err != nil {
				return fmt.Errorf("JWT configuration required: %w", err)
			}

			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Validate bearer token for platform administrator
			if bearerToken == "" {
				return fmt.Errorf("bearer token is required (use --bearer-token)")
			}

			tokenGenerator := auth.NewTokenGenerator(os.Getenv("JWT_SECRET"))
			claims, err := tokenGenerator.Validate(bearerToken)
			if err != nil {
				return fmt.Errorf("invalid bearer token: %w", err)
			}

			// Check for platform_admin role
			hasPlatformAdmin := false
			for _, role := range claims.Roles {
				if role == "platform_admin" {
					hasPlatformAdmin = true
					break
				}
			}
			if !hasPlatformAdmin {
				return fmt.Errorf("recovery requires platform_admin role. Current roles: %v", claims.Roles)
			}

			if len(args) > 0 {
				backupID = args[0]
			}

			if backupID == "" && !dryRun {
				return fmt.Errorf("backup ID is required for non-dry-run recovery")
			}

			// Connect to database
			db, err := timescale.NewClient(ctx, cfg.DB.DSN)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer db.Close()

		// Initialize encryptor
		keyStore := encrypt.NewKeyStore(db.DB())

			// Create recovery coordinator
			coordinator := backup.NewRecoveryCoordinator(db.DB(), keyStore)

			// Configure coordinator
			if timeout != "" {
				timeoutDur, err := time.ParseDuration(timeout)
				if err != nil {
					return fmt.Errorf("invalid timeout: %w", err)
				}
				coordinator.SetTimeout(timeoutDur)
			}

			coordinator.SetDryRun(dryRun)
			if backupID != "" {
				coordinator.SetBackupID(backupID)
			}

			// Initialize stores if needed
			if !dryRun {
				databaseStore := backup.NewBackupStore(db.DB(), keyStore, "")
				coordinator.SetStores(databaseStore, nil, nil)
			}

			// Execute recovery
			logger.Info("starting recovery",
				zap.String("backup_id", backupID),
				zap.Bool("dry_run", dryRun),
				zap.String("timeout", timeout),
				zap.Bool("force", force),
			)

			result, err := coordinator.Recover(ctx)
			if err != nil {
				logger.Error("recovery failed",
					zap.String("recovery_id", result.RecoveryID),
					zap.String("error", err.Error()),
					zap.String("final_state", result.State.String()),
				)
				return fmt.Errorf("recovery failed: %w", err)
			}

			logger.Info("recovery completed successfully",
				zap.String("recovery_id", result.RecoveryID),
				zap.String("final_state", result.State.String()),
				zap.Bool("success", result.Success),
			)

			// Print RPO/RTO metrics
			if !dryRun {
				logger.Info("RPO metrics",
					zap.Duration("data_loss_window", result.RPO.DataLossWindow),
					zap.Time("last_backup_time", result.RPO.LastBackupTime),
					zap.Duration("max_acceptable_rpo", result.RPO.MaxAcceptableRPO),
				)

				logger.Info("RTO metrics",
					zap.Duration("total_recovery_time", result.RTO.TotalRecoveryTime),
					zap.Duration("recovery_start", result.RTO.RecoveryStartTime.Sub(result.RTO.RecoveryEndTime)),
				)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&backupID, "backup-id", "b", "", "Backup ID to restore from")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run through recovery process without executing changes")
	cmd.Flags().StringVar(&timeout, "timeout", "2h", "Recovery timeout (default: 2h)")
	cmd.Flags().BoolVar(&force, "force", false, "Bypass pre-flight validation (use with caution)")
	cmd.Flags().StringVar(&bearerToken, "bearer-token", "", "JWT bearer token for platform administrator authentication")

	return cmd
}
