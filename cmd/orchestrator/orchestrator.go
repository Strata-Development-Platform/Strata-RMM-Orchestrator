package orchestrator

import (
	"context"
	"fmt"
	"net/url"

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
	var (
		natsURL         string
		timescaleDSN    string
		apiAddr         string
		tunnelAddr      string
		storageBackend  string
		storageBucket   string
		storageRegion   string
		storageEndpoint string
	)

	cmd := &cobra.Command{
		Use:   "orchestrator",
		Short: "Start the RMM orchestrator platform services",
		Long:  `Starts the core platform services: NATS consumer, TimescaleDB ingestion, REST API, and alerting`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stage 1: Load and validate configuration
			logger.Info("loading configuration")
			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("loading configuration: %w", err)
			}
			logger.Info("runtime mode", zap.String("mode", string(cfg.RuntimeMode)))

			// Stage 2: Apply CLI flag overrides (flags take highest precedence)
			if natsURL != "" {
				cfg.NATS.URL = natsURL
			}
			if timescaleDSN != "" {
				cfg.DB.DSN = timescaleDSN
			}
			if apiAddr != "" {
				cfg.HTTP.APIAddr = apiAddr
			}
			if tunnelAddr != "" {
				cfg.HTTP.TunnelAddr = tunnelAddr
			}
			if storageBackend != "" {
				cfg.Storage.Backend = storageBackend
			}
			if storageBucket != "" {
				cfg.Storage.Bucket = storageBucket
			}
			if storageRegion != "" {
				cfg.Storage.Region = storageRegion
			}
			if storageEndpoint != "" {
				cfg.Storage.Endpoint = storageEndpoint
			}

			// Stage 3: Validate configuration
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("configuration validation failed: %w", err)
			}

			// Stage 4: Log redacted summary
			summary := cfg.RedactedSummary()
			logger.Info("configuration validated", zap.Any("summary", summary))

			// Stage 5: Verify runtime mode governance (fail closed)
			if cfg.RuntimeMode == config.ModeProduction {
				if err := cfg.ProductionValidate(); err != nil {
					return fmt.Errorf("production configuration rejected: %w", err)
				}
				if cfg.Seeding.SeedDev {
					return fmt.Errorf("production startup rejected: development seeding must not be enabled")
				}
			}

			// Stage 6: Initialize JWT
			logger.Info("initializing JWT")
			if err := auth.ValidateJWTConfig(); err != nil {
				return fmt.Errorf("JWT configuration: %w", err)
			}
			tokenGen, err := auth.NewTokenGeneratorOrFail("")
			if err != nil {
				return fmt.Errorf("token generator: %w", err)
			}

			// Stage 7: Connect NATS
			logger.Info("connecting to NATS", zap.String("url", redactURL(cfg.NATS.URL)))
			nc, err := connectNATS(cfg)
			if err != nil {
				return fmt.Errorf("connecting to NATS: %w", err)
			}
			defer nc.Close()
			logger.Info("connected to NATS")

			// Stage 8: Connect database
			logger.Info("connecting to database")
			tsdb, err := timescale.NewClient(ctx, cfg.DB.DSN)
			if err != nil {
				return fmt.Errorf("connecting to database: %w", err)
			}
			tsdb.SetPoolConfig(cfg.DB.MaxOpenConns, cfg.DB.MaxIdleConns, cfg.DB.ConnMaxLifetime)
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

			if cfg.Seeding.SeedDev && cfg.RuntimeMode != config.ModeProduction {
				if err := postgres.SeedDevTenant(tsdb.DB()); err != nil {
					return fmt.Errorf("seed development tenant: %w", err)
				}
				logger.Info("development tenant seeded")
			}

			// Stage 9: Start ingestion
			logger.Info("starting metrics ingestion")
			ingest := monitoring.NewIngestService(nc, tsdb, logger)
			if err := ingest.Start(ctx); err != nil {
				return fmt.Errorf("starting ingestion: %w", err)
			}
			defer ingest.Stop()
			logger.Info("metrics ingestion started")

			// Stage 10: Alerting
			logger.Info("starting alerting engine")
			alertStore := alerting.NewStore(tsdb.DB())
			alertNotifier := alerting.NewNotifier()
			alertEngine := alerting.NewEngine(nc, tsdb, alertStore, alertNotifier, logger)
			if err := alertEngine.Start(ctx); err != nil {
				return fmt.Errorf("starting alerting engine: %w", err)
			}
			defer alertEngine.Stop()
			logger.Info("alerting engine started")

			// Stage 11: Vulnerability engines
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

			cveSync := inventory.NewCVESyncEngine(tsdb.DB(), logger)
			cveSync.Start(ctx)
			logger.Info("CVE sync engine started")

			thirdParty := inventory.NewThirdPartyEngine(tsdb.DB(), logger)
			go thirdParty.Start(ctx)
			logger.Info("third-party patching engine started")

			// Stage 12: Storage and tunnel gateway
			updateMgr := platform.NewUpdateManager(version, "Strata-Development-Platform", "Strata-RMM-Orchestrator", cfg.HTTP.APIAddr, logger)
			releaseServer := platform.NewReleaseServer("/var/lib/strata-rmm/releases", "Strata-Development-Platform", "Strata-RMM-Orchestrator")

			api, err := platform.NewAPIServer(cfg.HTTP.APIAddr, tsdb, nc, logger, tokenGen)
			if err != nil {
				return fmt.Errorf("creating API server: %w", err)
			}
			api.WithHTTPConfig(
				cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout, cfg.HTTP.IdleTimeout,
				cfg.HTTP.MaxBodySizeBytes, cfg.HTTP.CORSOrigins, cfg.HTTP.TrustedProxies,
			)
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

			// Stage 13: Job dispatcher
			dispatcher := platform.NewDispatcher(tsdb, nc, logger)
			dispatcher.Start(ctx)
			defer dispatcher.Stop()
			logger.Info("job dispatcher started")

			// Stage 14: API server
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

	// Restore all original CLI flags
	cmd.Flags().StringVar(&natsURL, "nats-url", "", "NATS server URL (overrides NATS_URL env)")
	cmd.Flags().StringVar(&timescaleDSN, "timescale-dsn", "", "TimescaleDB DSN (overrides TIMESCALE_DSN env)")
	cmd.Flags().StringVar(&apiAddr, "api-addr", "", "API server listen address (overrides STRATA_API_ADDR env)")
	cmd.Flags().StringVar(&tunnelAddr, "tunnel-addr", "", "Tunnel gateway listen address (overrides STRATA_TUNNEL_ADDR env)")
	cmd.Flags().StringVar(&storageBackend, "storage-backend", "", "Storage backend (minio, s3, local, none) (overrides STORAGE_BACKEND env)")
	cmd.Flags().StringVar(&storageBucket, "storage-bucket", "", "Storage bucket name (overrides STORAGE_BUCKET env)")
	cmd.Flags().StringVar(&storageRegion, "storage-region", "", "Storage region (overrides STORAGE_REGION env)")
	cmd.Flags().StringVar(&storageEndpoint, "storage-endpoint", "", "Storage endpoint (overrides STORAGE_ENDPOINT env)")

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

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}
