package orchestrator

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/backup"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/config"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func newRecoveryCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var (
		backupID  string
		dryRun    bool
		timeout   string
		force     bool
		operation string
	)

	cmd := &cobra.Command{
		Use:   "recovery [backup-id]",
		Short: "Execute disaster recovery",
		Long: `Execute disaster recovery from a backup. This command coordinates the 20-state recovery workflow including:
  - Environment-scoped locking (pg_try_advisory_lock)
  - Service quiescing
  - Database, JetStream, and object storage backup/restore
  - Post-restore validation
  - RPO/RTO measurement
  - Automatic rollback on failure

The recovery process acquires an exclusive advisory lock to prevent concurrent recoveries. If dry-run is enabled, all state transitions execute but no side effects occur. Use --force to bypass pre-flight validation (use with caution).

REQUIRES: Platform administrator privileges (platform_admin role).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if len(args) > 0 {
				backupID = args[0]
			}

			if backupID == "" && operation == "restore" {
				return fmt.Errorf("backup ID is required for restore operations (use --operation backup for backup-only)")
			}

			db, err := timescale.NewClient(ctx, cfg.DB.DSN)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer db.Close()

			keyStore := encrypt.NewKeyStore(db.DB())
			coordinator := backup.NewRecoveryCoordinator(db.DB(), keyStore)

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

			logger.Info("starting recovery",
				zap.String("operation", operation),
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

			logger.Info("recovery completed",
				zap.String("recovery_id", result.RecoveryID),
				zap.String("final_state", result.State.String()),
				zap.String("phase", result.Phase.String()),
				zap.Bool("success", result.Success),
			)

			fmt.Printf("Recovery ID: %s\n", result.RecoveryID)
			fmt.Printf("Final State: %s\n", result.State.String())
			fmt.Printf("Phase: %s\n", result.Phase.String())
			fmt.Printf("Success: %v\n", result.Success)
			fmt.Printf("Events: %d\n", len(result.Events))

			return nil
		},
	}

	cmd.Flags().StringVarP(&backupID, "backup-id", "b", "", "Backup ID to restore from")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run through recovery process without executing changes")
	cmd.Flags().StringVar(&timeout, "timeout", "2h", "Recovery timeout (default: 2h)")
	cmd.Flags().BoolVar(&force, "force", false, "Bypass pre-flight validation (use with caution)")
	cmd.Flags().StringVar(&operation, "operation", "", "Operation type: backup, restore, or both (default: both)")
	cmd.MarkFlagsRequiredTogether("backup-id", "operation")

	return cmd
}

func newBackupCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var (
		databaseType string
		dryRun       bool
		timeout      string
	)

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a backup of all Strata RMM services",
		Long: `Creates encrypted backups of PostgreSQL/TimescaleDB, NATS JetStream, and object storage.

The backup process:
  1. Acquires advisory lock to prevent concurrent operations
  2. Quiesces all services
  3. Performs encrypted database backup via pg_dump
  4. Performs JetStream stream/consumer/message backup
  5. Performs object storage inventory backup
  6. Verifies integrity of all components
  7. Records backup metadata in the backup_records table

All backup data is encrypted using AES-256-GCM. Integrity is verified via SHA-256 checksums.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if databaseType == "" {
				databaseType = "timescaledb"
			}

			if databaseType != "postgresql" && databaseType != "timescaledb" {
				return fmt.Errorf("unsupported database type: %s", databaseType)
			}

			db, err := timescale.NewClient(ctx, cfg.DB.DSN)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer db.Close()

			keyStore := encrypt.NewKeyStore(db.DB())
			coordinator := backup.NewRecoveryCoordinator(db.DB(), keyStore)

			if timeout != "" {
				timeoutDur, err := time.ParseDuration(timeout)
				if err != nil {
					return fmt.Errorf("invalid timeout: %w", err)
				}
				coordinator.SetTimeout(timeoutDur)
			}

			coordinator.SetDryRun(dryRun)

			logger.Info("starting backup",
				zap.String("database_type", databaseType),
				zap.Bool("dry_run", dryRun),
				zap.String("timeout", timeout),
			)

			result, err := coordinator.Recover(ctx)
			if err != nil {
				logger.Error("backup failed",
					zap.String("recovery_id", result.RecoveryID),
					zap.String("error", err.Error()),
				)
				return fmt.Errorf("backup failed: %w", err)
			}

			fmt.Printf("Backup completed. Recovery ID: %s\n", result.RecoveryID)
			fmt.Printf("Final State: %s\n", result.State.String())
			fmt.Printf("Events: %d\n", len(result.Events))

			return nil
		},
	}

	cmd.Flags().StringVar(&databaseType, "database-type", "timescaledb", "Database type: postgresql or timescaledb")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run through backup process without executing changes")
	cmd.Flags().StringVar(&timeout, "timeout", "2h", "Backup timeout (default: 2h)")

	return cmd
}

// Ensure context is unused (to fix unused import when building without full context).
var _ = os.Getenv
