package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/backup"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/config"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/recovery"
)

// newBackupCommand creates the backup subcommand.
func newBackupCommand(ctx context.Context, logger *zap.Logger, version, commit string) *cobra.Command {
	var (
		databaseType string
		dryRun       bool
		timeout      string
	)

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create encrypted backups of Strata RMM services",
		Long: `Creates encrypted backups of PostgreSQL/TimescaleDB, NATS JetStream, and object storage.

The backup process:
  1. Acquires environment-scoped advisory lock to prevent concurrent operations
  2. Quiesces all services (mutation gate, dispatcher, durable jobs)
  3. Performs encrypted database backup via pg_dump (custom format)
  4. Performs JetStream stream/consumer/message backup
  5. Performs object storage backup with actual byte preservation
  6. Verifies integrity of all components (SHA-256)
  7. Publishes to external artifact repository
  8. Records backup metadata and RPO metrics

All backup data is encrypted using AES-256-GCM with per-artifact nonce.
Integrity is verified via SHA-256 checksums.

HOST-LEVEL: This command requires direct host access (no network authentication).
Run on the orchestrator host or via secure shell.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg.Version = version
			cfg.Commit = commit

			if databaseType == "" {
				databaseType = "timescaledb"
			}

			if databaseType != "postgresql" && databaseType != "timescaledb" {
				return fmt.Errorf("unsupported database type: %s", databaseType)
			}

			_, err = time.ParseDuration(timeout)
			if err != nil {
				return fmt.Errorf("invalid timeout: %w", err)
			}

			runtime, err := buildRecoveryRuntime(cmd.Context(), cfg, "")
			if err != nil {
				return fmt.Errorf("backup preflight failed: %w", err)
			}
			defer runtime.close()

			logger.Info("starting backup",
				zap.String("database_type", databaseType),
				zap.Bool("dry_run", dryRun),
				zap.String("timeout", timeout),
			)

			if dryRun {
				fmt.Println("Backup preflight passed; no backup was created")
				return nil
			}
			operationCtx, cancel := context.WithTimeout(cmd.Context(), mustDuration(timeout))
			defer cancel()
			manifest, err := runtime.engine.Backup(operationCtx)
			if err != nil {
				return fmt.Errorf("backup failed: %w", err)
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(manifest)
		},
	}

	cmd.Flags().StringVar(&databaseType, "database-type", "timescaledb", "Database type: postgresql or timescaledb")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate configuration without executing backup operations")
	cmd.Flags().StringVar(&timeout, "timeout", "2h", "Maximum backup duration (e.g., 30m, 2h)")

	return cmd
}

// newRecoveryCommand creates the recovery subcommand.
func newRecoveryCommand(ctx context.Context, logger *zap.Logger, version, commit string) *cobra.Command {
	var (
		backupID  string
		targetDSN string
		dryRun    bool
		timeout   string
		operation string
		confirm   bool
	)

	cmd := &cobra.Command{
		Use:   "recovery [operation]",
		Short: "Execute disaster recovery operations",
		Long: `Execute disaster recovery operations. Supported operations:

  preflight  - Validate recovery prerequisites (repository, keys, artifact integrity)
  restore    - Restore from a backup set into a target database
  status     - Show current recovery state and available backup sets
  verify     - Verify integrity of an existing backup set
  key-init   - Create the first active recovery key in the configured provider

REQUIREMENTS FOR RESTORE:
  - --backup-id: Required. Specifies the backup set to restore from.
  - --target-dsn: Required. Connection string for the clean target database.
  - --confirm: Required for restore. Pass --confirm to acknowledge destructive action.
  - --dry-run: Validates without executing changes.

SAFETY:
  - Advisory lock prevents concurrent recovery operations.
  - Source-equals-target is always rejected (no in-place restore).
  - Artifact integrity is verified before any mutation.
  - Post-restore verification queries the target database.
   - Mandatory safety checks cannot be bypassed.

HOST-LEVEL: This command requires direct host access.
Run on the orchestrator host or via secure shell.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg.Version = version
			cfg.Commit = commit

			if len(args) > 0 {
				operation = args[0]
			}

			if operation == "" {
				return fmt.Errorf("operation required: preflight, restore, status, or verify\n%s", cmd.UsageString())
			}

			timeoutDur, err := time.ParseDuration(timeout)
			if err != nil {
				return fmt.Errorf("invalid timeout: %w", err)
			}

			// Validate operation-specific requirements
			switch operation {
			case "preflight":
				return runPreflight(cfg, dryRun, timeoutDur, logger)
			case "restore":
				return runRestore(cfg, backupID, targetDSN, dryRun, timeoutDur, confirm, logger)
			case "status":
				return runStatus(cfg, dryRun, logger)
			case "verify":
				return runVerify(cfg, backupID, dryRun, logger)
			case "key-init":
				return runKeyInit(cfg, logger)
			default:
				return fmt.Errorf("unknown operation: %s (valid: preflight, restore, status, verify, key-init)", operation)
			}
		},
	}

	cmd.Flags().StringVarP(&backupID, "backup-id", "b", "", "Backup set ID to restore from or verify")
	cmd.Flags().StringVar(&targetDSN, "target-dsn", "", "Target database connection string (required for restore)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate prerequisites without executing changes")
	cmd.Flags().StringVar(&timeout, "timeout", "4h", "Maximum recovery duration (default: 4h)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Acknowledge destructive restore operation (required)")

	// Restore operation requires --backup-id and --target-dsn
	_ = cmd.RegisterFlagCompletionFunc("operation", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"preflight", "restore", "status", "verify", "key-init"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func runPreflight(cfg *config.OrchestratorConfig, dryRun bool, timeout time.Duration, logger *zap.Logger) error {
	logger.Info("running preflight checks", zap.Bool("dry_run", dryRun))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	runtime, err := buildRecoveryRuntime(ctx, cfg, "")
	if err != nil {
		return fmt.Errorf("preflight failed: %w", err)
	}
	defer runtime.close()
	if _, err := runtime.repo.ListBackupSets(ctx); err != nil {
		return fmt.Errorf("preflight repository access failed: %w", err)
	}
	fmt.Println("Preflight validation passed")
	return nil
}

func runRestore(cfg *config.OrchestratorConfig, backupID, targetDSN string, dryRun bool, timeout time.Duration, confirm bool, logger *zap.Logger) error {
	// Validate required fields
	if backupID == "" {
		return fmt.Errorf("--backup-id is required for restore")
	}
	if targetDSN == "" {
		return fmt.Errorf("--target-dsn is required for restore")
	}

	// Reject destructive action without confirmation
	if !confirm && !dryRun {
		return fmt.Errorf("destructive operation: pass --confirm to acknowledge. Backup set %s will be restored into the target database", backupID)
	}

	// Reject if target equals source
	if cfg.DB.DSN == targetDSN {
		return fmt.Errorf("target DSN must not equal source DSN (no in-place restore allowed)")
	}

	logger.Info("starting restore",
		zap.String("backup_id", backupID),
		zap.String("target_dsn_redacted", redactDSN(targetDSN)),
		zap.Bool("dry_run", dryRun),
	)

	fmt.Printf("Restore operation: backup %s -> target database\n", backupID)
	fmt.Printf("  Dry run: %v\n", dryRun)
	fmt.Printf("  Timeout: %s\n", timeout)
	fmt.Printf("  Confirmed: %v\n", confirm)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	runtime, err := buildRecoveryRuntime(ctx, cfg, targetDSN)
	if err != nil {
		return fmt.Errorf("restore preflight failed: %w", err)
	}
	defer runtime.close()
	if err := runtime.engine.Verify(ctx, backupID); err != nil {
		return fmt.Errorf("restore preflight integrity verification failed: %w", err)
	}
	if dryRun {
		fmt.Println("Restore preflight passed; no target was mutated")
		return nil
	}
	if err := runtime.engine.Restore(ctx, backupID); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}
	fmt.Println("Restore completed and target components verified")
	return nil
}

func runStatus(cfg *config.OrchestratorConfig, dryRun bool, logger *zap.Logger) error {
	logger.Info("checking recovery status", zap.Bool("dry_run", dryRun))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repo, err := buildArtifactRepository(ctx, cfg)
	if err != nil {
		return fmt.Errorf("status preflight failed: %w", err)
	}
	sets, err := repo.ListBackupSets(ctx)
	if err != nil {
		return fmt.Errorf("list backup sets: %w", err)
	}
	data, err := json.MarshalIndent(sets, "", "  ")
	if err != nil {
		return fmt.Errorf("encode backup sets: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func runVerify(cfg *config.OrchestratorConfig, backupID string, dryRun bool, logger *zap.Logger) error {
	if backupID == "" {
		return fmt.Errorf("--backup-id is required for verify")
	}
	if cfg.Backup.KeyProviderPath == "" {
		return fmt.Errorf("STRATA_BACKUP_KEY_PROVIDER_PATH is required")
	}

	logger.Info("verifying backup integrity",
		zap.String("backup_id", backupID),
		zap.Bool("dry_run", dryRun),
	)

	fmt.Printf("Verify operation: backup set %s\n", backupID)
	fmt.Printf("  Dry run: %v\n", dryRun)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	repo, err := buildArtifactRepository(ctx, cfg)
	if err != nil {
		return fmt.Errorf("verify preflight failed: %w", err)
	}
	keys, err := recovery.NewFileKeyProvider(cfg.Backup.KeyProviderPath)
	if err != nil {
		return fmt.Errorf("verify key provider failed: %w", err)
	}
	if err := backup.VerifyBackupSet(ctx, repo, keys, backupID); err != nil {
		return fmt.Errorf("backup verification failed: %w", err)
	}
	fmt.Println("Backup verification passed")
	return nil
}

func runKeyInit(cfg *config.OrchestratorConfig, logger *zap.Logger) error {
	if cfg.Backup.KeyProviderPath == "" {
		return fmt.Errorf("STRATA_BACKUP_KEY_PROVIDER_PATH is required")
	}
	if cfg.Backup.EnvironmentID == "" {
		return fmt.Errorf("STRATA_BACKUP_ENVIRONMENT_ID is required")
	}
	provider, err := recovery.NewFileKeyProvider(cfg.Backup.KeyProviderPath)
	if err != nil {
		return fmt.Errorf("initialize recovery key provider: %w", err)
	}
	if current, err := provider.CurrentKey(context.Background()); err == nil {
		return fmt.Errorf("an active recovery key already exists: %s", current.ID)
	} else if !recovery.IsKeyNotFound(err) {
		return fmt.Errorf("inspect recovery key provider: %w", err)
	}
	key, err := provider.RotateKey(context.Background(), cfg.Backup.EnvironmentID+"-recovery")
	if err != nil {
		return fmt.Errorf("create recovery key: %w", err)
	}
	logger.Info("recovery key initialized", zap.String("key_id", key.ID), zap.String("provider", provider.ProviderName()))
	fmt.Printf("Recovery key initialized: %s\n", key.ID)
	return nil
}

// redactDSN removes password and credentials from a DSN for logging.
func redactDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	return config.RedactDSN(dsn)
}

func mustDuration(value string) time.Duration {
	duration, _ := time.ParseDuration(value)
	return duration
}
