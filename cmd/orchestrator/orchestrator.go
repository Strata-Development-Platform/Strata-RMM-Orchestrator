package orchestrator

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/alerting"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/inventory"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/monitoring"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/platform"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/remote"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/reporting"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/update"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/config"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func NewCommand(ctx context.Context, version string, logger *zap.Logger) *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "orchestrator",
		Short: "Start the RMM orchestrator platform services",
		Long:  `Starts the core platform services: NATS consumer, TimescaleDB ingestion, REST API, and alerting`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stage 1: load and validate configuration
			logger.Info("loading configuration")
			cfg := config.LoadOrchestratorConfig()
			if cfg.RuntimeMode == "" {
				cfg.RuntimeMode = config.ModeDevelopment
			}
			logger.Info("runtime mode", zap.String("mode", string(cfg.RuntimeMode)))

			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("configuration validation failed: %w", err)
			}
			if cfg.RuntimeMode == config.ModeProduction {
				if err := cfg.ValidateProduction(); err != nil {
					return fmt.Errorf("production configuration validation failed: %w", err)
				}
			}
			summary := cfg.RedactedSummary()
			logger.Info("configuration loaded", zap.Any("summary", summary))

			// Stage 2: initialize JWT
			logger.Info("validating JWT configuration")
			if err := auth.ValidateJWTConfig(); err != nil {
				return fmt.Errorf("JWT configuration: %w", err)
			}

			// Stage 3: connect to NATS
			logger.Info("connecting to NATS", zap.String("url", redactNATSURL(cfg.NATS.URL)))
			nc, err := connectNATS(cfg)
			if err != nil {
				return fmt.Errorf("connecting to NATS: %w", err)
			}
			defer nc.Close()
			logger.Info("connected to NATS")

			// Stage 4: connect to database
			logger.Info("connecting to database")
			tsdb, err := timescale.NewClient(ctx, cfg.DB.DSN)
			if err != nil {
				return fmt.Errorf("connecting to TimescaleDB: %w", err)
			}
			defer tsdb.Close()
			logger.Info("connected to database")

			if err := tsdb.ApplyMigrations(ctx); err != nil {
				return fmt.Errorf("applying TimescaleDB migrations: %w", err)
			}
			logger.Info("TimescaleDB migrations applied")

			sm := postgres.NewSchemaManager(tsdb.DB())
			if err := sm.Apply(); err != nil {
				return fmt.Errorf("applying relational schema: %w", err)
			}
			logger.Info("relational schema migrations applied")

			if cfg.Seeding.SeedDev {
				if err := postgres.SeedDevTenant(tsdb.DB()); err != nil {
					return fmt.Errorf("seed development tenant: %w", err)
				}
				logger.Info("development tenant seeded")
			}

			// Stage 5: start ingestion
			logger.Info("starting metrics ingestion")
			ingest := monitoring.NewIngestService(nc, tsdb, logger)
			if err := ingest.Start(ctx); err != nil {
				return fmt.Errorf("starting ingestion: %w", err)
			}
			defer ingest.Stop()
			logger.Info("metrics ingestion started")

			// Stage 6: alerting
			logger.Info("starting alerting engine")
			alertStore := alerting.NewStore(tsdb.DB())
			alertNotifier := alerting.NewNotifier()
			alertEngine := alerting.NewEngine(nc, tsdb, alertStore, alertNotifier, logger)
			if err := alertEngine.Start(ctx); err != nil {
				return fmt.Errorf("starting alerting engine: %w", err)
			}
			defer alertEngine.Stop()
			logger.Info("alerting engine started")

			// Stage 7: vulnerability engine
			vulnEngine := inventory.NewVulnerabilityEngine(tsdb.DB(), logger).
				WithAlertCallback(func(tenantID, deviceID, cveID, pkg, severity, currentVer, fixedVer string) {
					alertEngine.FireCVEAlert(tenantID, deviceID, cveID, pkg, severity, currentVer, fixedVer)
				}).
				WithResolveCallback(func(deviceID, cveID string) {
					alertEngine.ResolveCVEAlert(deviceID, cveID)
				})
			if err := vulnEngine.Start(ctx); err != nil {
				logger.Warn("starting vulnerability engine", zap.Error(err))
			} else {
				logger.Info("vulnerability engine started")
			}

			// Stage 8: CVE sync and third-party
			cveSync := inventory.NewCVESyncEngine(tsdb.DB(), logger)
			cveSync.Start(ctx)
			logger.Info("CVE sync engine started")

			thirdParty := inventory.NewThirdPartyEngine(tsdb.DB(), logger)
			go thirdParty.Start(ctx)
			logger.Info("third-party patching engine started")

			// Stage 9: storage and tunnel gateway
			updateMgr := platform.NewUpdateManager(version, "Strata-Development-Platform", "Strata-RMM-Orchestrator", cfg.HTTP.APIAddr, logger)
			releaseServer := platform.NewReleaseServer("/var/lib/strata-rmm/releases", "Strata-Development-Platform", "Strata-RMM-Orchestrator")

			tokenGen, err := auth.NewTokenGeneratorOrFail("")
			if err != nil {
				return fmt.Errorf("token generator: %w", err)
			}

			api, err := platform.NewAPIServer(cfg.HTTP.APIAddr, tsdb, nc, logger, tokenGen)
			if err != nil {
				return fmt.Errorf("creating API server: %w", err)
			}
			api.WithReleaseServer(releaseServer).
				WithAlertEngine(alertEngine).
				WithVulnEngine(vulnEngine).
				WithCVESyncEngine(cveSync).
				WithThirdPartyEngine(thirdParty).
				WithUpdateManager(updateMgr)

			if cfg.Storage.Backend != "" && cfg.Storage.Backend != "none" {
				storeCfg := storage.Config{
					Type:      cfg.Storage.Backend,
					Bucket:    cfg.Storage.Bucket,
					Region:    cfg.Storage.Region,
					Endpoint:  cfg.Storage.Endpoint,
					AccessKey: cfg.Storage.AccessKey,
					SecretKey: cfg.Storage.SecretKey,
					UseSSL:    cfg.Storage.UseSSL,
					KMSKeyID:  cfg.Storage.KMSKeyID,
				}

				sb, err := storage.NewBackend(ctx, storeCfg)
				if err != nil {
					logger.Warn("storage backend init (continuing without recordings)", zap.Error(err))
				} else {
					logger.Info("storage backend initialized", zap.String("type", cfg.Storage.Backend), zap.String("bucket", cfg.Storage.Bucket))
					recStore := remote.NewRecordingStore(tsdb.DB())
					recorder := remote.NewRecorder(sb, logger)

					gw := remote.NewGateway(nc, cfg.HTTP.TunnelAddr, logger).
						WithRecorder(recorder).
						WithRecordingStore(recStore)
					if err := gw.Start(ctx); err != nil {
						return fmt.Errorf("starting tunnel gateway: %w", err)
					}
					logger.Info("tunnel gateway started", zap.String("addr", cfg.HTTP.TunnelAddr))

					cleanup := remote.NewCleanupJob(recStore, sb, logger)
					cleanup.Start(ctx)

					reportEngine := reporting.NewReportEngine(tsdb.DB(), logger, sb, cfg.Storage.Bucket)
					go reportEngine.Start(ctx)
					logger.Info("report engine started")

					api = api.WithRecordingStore(recStore).WithStorageBackend(sb)
				}
			}

			// Stage 10: job dispatcher
			dispatcher := platform.NewDispatcher(tsdb, nc, logger)
			dispatcher.Start(ctx)
			defer dispatcher.Stop()
			logger.Info("job dispatcher started")

			// Stage 11: API server
			if err := api.Start(ctx); err != nil {
				return fmt.Errorf("starting API server: %w", err)
			}
			defer api.Stop(ctx)
			logger.Info("orchestrator ready", zap.String("addr", cfg.HTTP.APIAddr))

			logger.Info("orchestrator running, waiting for signals")
			<-ctx.Done()
			logger.Info("shutting down orchestrator")
			return nil
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "", "Path to orchestrator config file")
	cmd.AddCommand(NewUpdateCommand(ctx, version, logger))
	return cmd
}

func NewUpdateCommand(ctx context.Context, version string, logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check and apply orchestrator updates",
		RunE: func(cmd *cobra.Command, args []string) error {
			checkOnly, _ := cmd.Flags().GetBool("check")
			updater := update.NewOrchestratorUpdater(version, "Strata-Development-Platform", "Strata-RMM-Orchestrator")
			logger.Info("checking for updates", zap.String("current", version))
			release, err := updater.Check(ctx)
			if err != nil {
				return fmt.Errorf("check failed: %w", err)
			}
			if release == nil {
				logger.Info("already up to date", zap.String("version", version))
				return nil
			}
			logger.Info("update available", zap.String("current", version), zap.String("latest", release.Version))
			if checkOnly {
				return nil
			}

			mode := updater.DetectMode()
			logger.Info("deployment mode", zap.String("mode", mode))
			switch mode {
			case "docker":
				logger.Info("run: docker compose pull && docker compose up -d")
				return nil
			case "kubernetes":
				logger.Info("run: helm upgrade strata-rmm ...")
				return nil
			}

			binaryPath, err := updater.Download(ctx, release)
			if err != nil {
				return fmt.Errorf("download failed: %w", err)
			}
			if err := updater.Apply(binaryPath); err != nil {
				updater.Rollback()
				return fmt.Errorf("apply failed: %w", err)
			}
			healthURL := fmt.Sprintf("http://localhost:%s/health", "8080")
			if err := updater.Verify(ctx, healthURL); err != nil {
				updater.Rollback()
				return fmt.Errorf("verification failed, rolled back: %w", err)
			}
			updater.Cleanup()
			logger.Info("update successful, restarting")
			return updater.TriggerRestart()
		},
	}
	cmd.Flags().Bool("check", false, "Only check for updates, don't apply")
	return cmd
}

func connectNATS(cfg *config.OrchestratorConfig) (*nats.Conn, error) {
	natsOpts := []nats.Option{
		nats.Name("StrataRMM-Orchestrator"),
		nats.ReconnectWait(cfg.NATS.ReconnectWait),
		nats.MaxReconnects(cfg.NATS.MaxReconnects),
		nats.RetryOnFailedConnect(true),
	}
	if cfg.NATS.Token != "" {
		natsOpts = append(natsOpts, nats.Token(cfg.NATS.Token))
	}
	if cfg.NATS.TLSEnabled {
		natsOpts = append(natsOpts, nats.Secure(nil))
	}
	return nats.Connect(cfg.NATS.URL, natsOpts...)
}

func redactNATSURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}


