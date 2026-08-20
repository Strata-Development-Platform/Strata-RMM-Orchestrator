package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/update"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/config"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

const defaultDockerOCIRepository = "ghcr.io/strata-development-platform/strata-rmm-orchestrator"

// NewProductCommand assembles the shipped orchestrator command with the verified
// update entrypoint. The historical NewCommand remains available to narrow unit
// tests, but its legacy update child is removed from the product command tree.
func NewProductCommand(ctx context.Context, version, commit string, logger *zap.Logger) *cobra.Command {
	cmd := NewCommand(ctx, version, commit, logger)
	for _, child := range cmd.Commands() {
		if child.Name() == "update" {
			cmd.RemoveCommand(child)
		}
	}
	cmd.AddCommand(NewVerifiedUpdateCommand(ctx, version, logger))
	return cmd
}

// NewVerifiedUpdateCommand preserves one authoritative native staging policy
// and fails closed for deployment modes that require an external privileged owner.
func NewVerifiedUpdateCommand(ctx context.Context, version string, logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check and apply orchestrator updates",
		RunE: func(cmd *cobra.Command, args []string) error {
			checkOnly, _ := cmd.Flags().GetBool("check")
			updater := update.NewOrchestratorUpdater(version, "Strata-Development-Platform", "Strata-RMM-Orchestrator")
			service := update.NewService(updater, nil)
			logger.Info("checking for updates", zap.String("current", version))
			plan, err := service.Plan(ctx, false)
			if err != nil {
				return fmt.Errorf("check failed: %w", err)
			}
			if !plan.Available || plan.Release == nil {
				logger.Info("already up to date", zap.String("version", version))
				return nil
			}
			logger.Info("update available", zap.String("current", version), zap.String("latest", plan.Release.Version))
			if checkOnly {
				return nil
			}
			logger.Info("deployment mode", zap.String("mode", plan.Mode))
			switch plan.Mode {
			case "docker":
				return fmt.Errorf("docker apply is refused inside the service container; use the verified privileged host-side docker updater")
			case "kubernetes":
				return fmt.Errorf("kubernetes automatic apply is unsupported; no generic Helm bypass is accepted")
			case "baremetal":
			default:
				return fmt.Errorf("unsupported deployment mode %q", plan.Mode)
			}

			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load runtime configuration for native update: %w", err)
			}
			db, err := timescale.NewClient(ctx, cfg.DB.DSN, cfg.DB.ReplicaDSN)
			if err != nil {
				return fmt.Errorf("connect to database for native update: %w", err)
			}
			defer db.Close()
			service = update.NewService(updater, update.NewRuntimePreflight(db.DB(), logger, update.DefaultUpgradeSnapshotDir))
			binaryPath, preflight, err := service.Stage(ctx, plan.Release)
			if err != nil {
				return fmt.Errorf("verified native preflight/stage failed: %w", err)
			}
			logger.Info("verified native update and recovery point staged; handing mutation ownership to external finalizer",
				zap.String("version", plan.Release.Version),
				zap.Int("source_schema", preflight.SourceSchemaVersion),
				zap.Int("target_schema", preflight.TargetSchemaVersion),
			)
			return updater.TriggerRestartWithSchema(binaryPath, preflight.SourceSchemaVersion, preflight.TargetSchemaVersion)
		},
	}
	cmd.Flags().Bool("check", false, "Only check for updates, don't apply")
	return cmd
}

func NewDockerUpdateHostCommand(ctx context.Context, version, commit string, logger *zap.Logger) *cobra.Command {
	var composeFile, envFile, journalFile, project, repository string

	cmd := &cobra.Command{
		Use:   "docker-update-host",
		Short: "Apply a verified Docker release from a privileged host-side utility",
		Long: "Runs the signed-manifest Docker upgrade transaction from a one-shot host-side utility. " +
			"This command is not intended for the long-running web-facing orchestrator and never grants that service Docker-socket access.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Geteuid() != 0 {
				return fmt.Errorf("docker-update-host must run as root in the privileged one-shot utility")
			}
			if _, err := os.Stat("/var/run/docker.sock"); err != nil {
				return fmt.Errorf("docker socket is unavailable to the privileged one-shot utility: %w", err)
			}
			paths, err := validateDockerHostUpdatePaths(composeFile, envFile, journalFile)
			if err != nil {
				return err
			}
			if strings.TrimSpace(project) == "" {
				return fmt.Errorf("compose project is required")
			}
			repository = strings.ToLower(strings.TrimSpace(repository))
			if repository == "" || strings.Contains(repository, "@") || strings.Contains(repository, "://") {
				return fmt.Errorf("OCI repository must be a canonical registry/repository reference")
			}

			executor := update.DockerUpgradeExecutor{
				ComposeFile:   paths.compose,
				EnvFile:       paths.env,
				JournalFile:   paths.journal,
				Project:       project,
				HealthTimeout: 2 * time.Minute,
			}
			if _, err := os.Lstat(paths.journal); err == nil {
				logger.Warn("retained docker upgrade journal found; reconciling before any new mutation", zap.String("journal", paths.journal))
				if err := executor.Reconcile(ctx); err != nil {
					return fmt.Errorf("reconcile retained docker upgrade transaction: %w", err)
				}
				return fmt.Errorf("retained docker upgrade transaction was reconciled; rerun the command to evaluate a new release")
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect docker upgrade journal: %w", err)
			}

			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load runtime configuration for docker update: %w", err)
			}
			db, err := timescale.NewClient(ctx, cfg.DB.DSN, cfg.DB.ReplicaDSN)
			if err != nil {
				return fmt.Errorf("connect to database for docker update: %w", err)
			}
			defer db.Close()

			updater := update.NewOrchestratorUpdater(version, "Strata-Development-Platform", "Strata-RMM-Orchestrator")
			service := update.NewService(updater, update.NewRuntimePreflight(db.DB(), logger, update.DefaultUpgradeBackupDir))
			plan, err := service.PlanDocker(ctx, repository, true)
			if err != nil {
				return fmt.Errorf("plan verified docker release: %w", err)
			}
			if !plan.Available || plan.Candidate == nil {
				logger.Info("docker deployment is already at the newest compatible signed release", zap.String("current", plan.CurrentVersion))
				return nil
			}
			if plan.Preflight == nil || !plan.Preflight.Pass {
				return fmt.Errorf("verified docker release is unavailable for apply because runtime preflight did not pass")
			}
			if err := updater.VerifyOCICandidate(ctx, *plan.Candidate); err != nil {
				return fmt.Errorf("verify docker candidate provenance: %w", err)
			}
			if err := executor.Apply(ctx, *plan.Candidate, *plan.Preflight, version, commit, plan.ManifestSHA256); err != nil {
				return fmt.Errorf("apply verified docker release: %w", err)
			}
			logger.Info("verified docker release applied", zap.String("version", plan.Candidate.Version), zap.String("image", plan.Candidate.Image))
			return nil
		},
	}

	cmd.Flags().StringVar(&composeFile, "compose-file", "", "absolute path to the protected production Compose file")
	cmd.Flags().StringVar(&envFile, "env-file", "", "absolute path to the protected production Compose environment file")
	cmd.Flags().StringVar(&journalFile, "journal-file", "", "absolute path to protected Docker upgrade transaction state")
	cmd.Flags().StringVar(&project, "project", "strata-rmm", "Docker Compose project name")
	cmd.Flags().StringVar(&repository, "repository", defaultDockerOCIRepository, "canonical signed OCI repository")
	_ = cmd.MarkFlagRequired("compose-file")
	_ = cmd.MarkFlagRequired("env-file")
	_ = cmd.MarkFlagRequired("journal-file")
	return cmd
}

type dockerHostUpdatePaths struct {
	compose string
	env     string
	journal string
}

func validateDockerHostUpdatePaths(composeFile, envFile, journalFile string) (dockerHostUpdatePaths, error) {
	cleanAbsolute := func(name, value string) (string, error) {
		if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return "", fmt.Errorf("%s must be a clean absolute path", name)
		}
		return value, nil
	}
	compose, err := cleanAbsolute("compose-file", composeFile)
	if err != nil {
		return dockerHostUpdatePaths{}, err
	}
	env, err := cleanAbsolute("env-file", envFile)
	if err != nil {
		return dockerHostUpdatePaths{}, err
	}
	journal, err := cleanAbsolute("journal-file", journalFile)
	if err != nil {
		return dockerHostUpdatePaths{}, err
	}
	if filepath.Dir(compose) != filepath.Dir(env) {
		return dockerHostUpdatePaths{}, fmt.Errorf("compose-file and env-file must share the protected deployment directory")
	}
	for name, path := range map[string]string{"compose-file": compose, "env-file": env} {
		info, err := os.Lstat(path)
		if err != nil {
			return dockerHostUpdatePaths{}, fmt.Errorf("inspect %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return dockerHostUpdatePaths{}, fmt.Errorf("%s must be a regular non-symlink file", name)
		}
		if name == "env-file" && info.Mode().Perm()&0077 != 0 {
			return dockerHostUpdatePaths{}, fmt.Errorf("env-file permissions are too broad")
		}
	}
	return dockerHostUpdatePaths{compose: compose, env: env, journal: journal}, nil
}
