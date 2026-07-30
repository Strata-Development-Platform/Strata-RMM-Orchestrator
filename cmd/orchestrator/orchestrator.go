package orchestrator

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"time"

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

type StorageMode string

const (
	StorageDisabled StorageMode = "disabled"
	StorageRequired StorageMode = "required"
)

func NewCommand(ctx context.Context, version, commit string, logger *zap.Logger) *cobra.Command {
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
			var startupStage int32
			advanceStage := func(stage int32) {
				atomic.StoreInt32(&startupStage, stage)
				logger.Info("startup stage", zap.Int32("stage", stage))
			}

			// Stage 1: Load and validate configuration
			advanceStage(1)
			logger.Info("loading configuration")
			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("stage %d: loading configuration: %w", atomic.LoadInt32(&startupStage), err)
			}
			logger.Info("runtime mode", zap.String("mode", string(cfg.RuntimeMode)))

			// Stage 2: Apply CLI flag overrides (flags take highest precedence)
			// Precedence: CLI flag > canonical env var > legacy alias > default
			if cmd.Flags().Changed("nats-url") {
				cfg.NATS.URL = natsURL
			}
			if cmd.Flags().Changed("timescale-dsn") {
				cfg.DB.DSN = timescaleDSN
			}
			if cmd.Flags().Changed("api-addr") {
				cfg.HTTP.APIAddr = apiAddr
			}
			if cmd.Flags().Changed("tunnel-addr") {
				cfg.HTTP.TunnelAddr = tunnelAddr
			}
			if cmd.Flags().Changed("storage-backend") {
				cfg.Storage.Backend = storageBackend
			}
			if cmd.Flags().Changed("storage-bucket") {
				cfg.Storage.Bucket = storageBucket
			}
			if cmd.Flags().Changed("storage-region") {
				cfg.Storage.Region = storageRegion
			}
			if cmd.Flags().Changed("storage-endpoint") {
				cfg.Storage.Endpoint = storageEndpoint
			}

			// Stage 3: Validate configuration
			advanceStage(3)
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("stage %d: configuration validation failed: %w", atomic.LoadInt32(&startupStage), err)
			}

			// Stage 4: Log redacted summary
			advanceStage(4)
			summary := cfg.RedactedSummary()
			logger.Info("configuration validated", zap.Any("summary", summary))

			// Stage 5: Verify runtime mode governance (fail closed)
			advanceStage(5)
			if cfg.RuntimeMode == config.ModeProduction {
				if err := cfg.ProductionValidate(); err != nil {
					return fmt.Errorf("stage %d: production configuration rejected: %w", atomic.LoadInt32(&startupStage), err)
				}
				if cfg.Seeding.SeedDev {
					return fmt.Errorf("stage %d: production startup rejected: development seeding must not be enabled", atomic.LoadInt32(&startupStage))
				}
			}

			// Stage 6: Initialize JWT
			advanceStage(6)
			logger.Info("initializing JWT")
			if err := auth.ValidateJWTConfig(); err != nil {
				return fmt.Errorf("stage %d: JWT configuration: %w", atomic.LoadInt32(&startupStage), err)
			}
			tokenGen, err := auth.NewTokenGeneratorOrFail("")
			if err != nil {
				return fmt.Errorf("stage %d: token generator: %w", atomic.LoadInt32(&startupStage), err)
			}

			// Stage 7: Connect NATS
			advanceStage(7)
			logger.Info("connecting to NATS", zap.String("url", redactURL(cfg.NATS.URL)))
			nc, err := connectNATS(cfg)
			if err != nil {
				return fmt.Errorf("stage %d: connecting to NATS: %w", atomic.LoadInt32(&startupStage), err)
			}
			defer nc.Close()
			logger.Info("connected to NATS")

			// Stage 8: Connect database and apply migrations
			advanceStage(8)
			logger.Info("connecting to database")
			tsdb, err := timescale.NewClient(ctx, cfg.DB.DSN)
			if err != nil {
				return fmt.Errorf("stage %d: connecting to database: %w", atomic.LoadInt32(&startupStage), err)
			}
			tsdb.SetPoolConfig(cfg.DB.MaxOpenConns, cfg.DB.MaxIdleConns, cfg.DB.ConnMaxLifetime)
			defer tsdb.Close()
			logger.Info("connected to database")

			if err := tsdb.ApplyMigrations(ctx); err != nil {
				return fmt.Errorf("stage %d: applying TimescaleDB migrations: %w", atomic.LoadInt32(&startupStage), err)
			}
			logger.Info("TimescaleDB migrations applied")

			sm := postgres.NewSchemaManager(tsdb.DB())
			if err := sm.Apply(ctx); err != nil {
				return fmt.Errorf("stage %d: applying relational schema: %w", atomic.LoadInt32(&startupStage), err)
			}
			logger.Info("relational schema migrations applied")

			if cfg.Seeding.SeedDev && cfg.RuntimeMode != config.ModeProduction {
				if err := postgres.SeedDevTenant(tsdb.DB()); err != nil {
					return fmt.Errorf("stage %d: seed development tenant: %w", atomic.LoadInt32(&startupStage), err)
				}
				logger.Info("development tenant seeded")
			}

			// Stage 9: Start ingestion
			advanceStage(9)
			logger.Info("starting metrics ingestion")
			ingest := monitoring.NewIngestService(nc, tsdb, logger)
			if err := ingest.Start(ctx); err != nil {
				return fmt.Errorf("stage %d: starting ingestion: %w", atomic.LoadInt32(&startupStage), err)
			}
			defer ingest.Stop()
			logger.Info("metrics ingestion started")

			// Stage 10: Alerting
			advanceStage(10)
			logger.Info("starting alerting engine")
			alertStore := alerting.NewStore(tsdb.DB())
			alertNotifier := alerting.NewNotifier()
			alertEngine := alerting.NewEngine(nc, tsdb, alertStore, alertNotifier, logger)
			if err := alertEngine.Start(ctx); err != nil {
				return fmt.Errorf("stage %d: starting alerting engine: %w", atomic.LoadInt32(&startupStage), err)
			}
			defer alertEngine.Stop()
			logger.Info("alerting engine started")

			// Stage 11: Vulnerability engines
			advanceStage(11)
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

			// Stage 12: API server and storage policy
			advanceStage(12)
			updateMgr := platform.NewUpdateManager(version, "Strata-Development-Platform", "Strata-RMM-Orchestrator", cfg.HTTP.APIAddr, logger)
			releaseServer := platform.NewReleaseServer("/var/lib/strata-rmm/releases", "Strata-Development-Platform", "Strata-RMM-Orchestrator")

			api, err := platform.NewAPIServer(cfg.HTTP.APIAddr, tsdb, nc, logger, tokenGen)
			if err != nil {
				return fmt.Errorf("stage %d: creating API server: %w", atomic.LoadInt32(&startupStage), err)
			}
			api.WithVersion(version, commit).
				WithHTTPConfig(
				cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout, cfg.HTTP.IdleTimeout,
				cfg.HTTP.MaxBodySizeBytes, cfg.HTTP.CORSOrigins,
			)
			deploymentCtrl := platform.NewDeploymentController()

			api.WithReleaseServer(releaseServer).
				WithAlertEngine(alertEngine).
				WithVulnEngine(vulnEngine).
				WithCVESyncEngine(cveSync).
				WithThirdPartyEngine(thirdParty).
				WithUpdateManager(updateMgr).
				WithDeploymentController(deploymentCtrl)

			updateMgr.WithDeploymentController(deploymentCtrl)

			api.RegisterHealth("deployment", func(ctx context.Context) error {
				if deploymentCtrl.GetState() == platform.DeploymentStateFailed {
					return fmt.Errorf("deployment in failed state")
				}
				return nil
			})

			api.RegisterHealth("db", func(ctx context.Context) error {
				pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				return tsdb.DB().PingContext(pingCtx)
			})
			api.RegisterHealth("nats", func(ctx context.Context) error {
				if !nc.IsConnected() {
					return fmt.Errorf("NATS not connected")
				}
				return nil
			})
			api.RegisterHealth("migrations", func(ctx context.Context) error {
				if !api.MigrationsComplete() {
					return fmt.Errorf("migrations not complete")
				}
				return nil
			})
			api.RegisterHealth("jetstream", platform.JetStreamHealthCheck(nc))
			api.SetMigrationsComplete(true)

			if cfg.Storage.Backend == "" || cfg.Storage.Backend == "none" {
				logger.Info("storage disabled — recording/report features unavailable")
			} else {
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
					return fmt.Errorf("stage %d: storage backend initialization failed: %w", atomic.LoadInt32(&startupStage), err)
				}
				logger.Info("storage backend initialized", zap.String("type", cfg.Storage.Backend), zap.String("bucket", cfg.Storage.Bucket))
				recStore := remote.NewRecordingStore(tsdb.DB())
				recorder := remote.NewRecorder(sb, logger)
				gw := remote.NewGateway(nc, cfg.HTTP.TunnelAddr, logger).
					WithRecorder(recorder).
					WithRecordingStore(recStore)
				if err := gw.Start(ctx); err != nil {
					return fmt.Errorf("stage %d: starting tunnel gateway: %w", atomic.LoadInt32(&startupStage), err)
				}
				logger.Info("tunnel gateway started", zap.String("addr", cfg.HTTP.TunnelAddr))
				cleanup := remote.NewCleanupJob(recStore, sb, logger)
				cleanup.Start(ctx)
				reportEngine := reporting.NewReportEngine(tsdb.DB(), logger, sb, cfg.Storage.Bucket)
				go reportEngine.Start(ctx)
				logger.Info("report engine started")
				api = api.WithRecordingStore(recStore).WithStorageBackend(sb)
				api.RegisterHealth("storage", func(ctx context.Context) error {
					_, err := sb.Stat(ctx, "health-check")
					if err != nil && err != storage.ErrNotFound {
						return fmt.Errorf("storage backend unreachable: %w", err)
					}
					return nil
				})
				api.RegisterHealth("jetstream", platform.JetStreamHealthCheck(nc))
			}

			// Stage 13: Job dispatcher
			advanceStage(13)
			dispatcher := platform.NewDispatcher(tsdb, nc, logger)
			dispatcher.Start(ctx)
			defer dispatcher.Stop()
			api.RegisterHealth("dispatcher", dispatcher.Healthy)
			logger.Info("job dispatcher started")

			// Stage 14: API server
			advanceStage(14)
			if err := api.Start(ctx); err != nil {
				return fmt.Errorf("stage %d: starting API server: %w", atomic.LoadInt32(&startupStage), err)
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
	cmd.AddCommand(newPreflightCommand(logger))
	cmd.AddCommand(newUpgradeCommand(ctx, logger))
	cmd.AddCommand(newRollbackCommand(ctx, logger))
	cmd.AddCommand(newRecoveryCommand(ctx, logger))
	cmd.AddCommand(newBackupCommand(ctx, logger))
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

func newPreflightCommand(logger *zap.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "preflight",
		Short: "Validate deployment prerequisites",
		Long:  "Validates configuration, database, NATS, JetStream, storage, secrets, disk, and migration state. Exits with structured JSON on stdout.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration
			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				logger.Error("failed to load config", zap.Error(err))
				fmt.Printf(`{"results":[{"name":"config","status":"fail","message":"failed to load config","error":"%s"}],"warnings":0,"failures":1,"pass":false,"timestamp":"%s"}`+"\n",
					postgres.RedactCredentials(err.Error()), time.Now().UTC().Format(time.RFC3339))
				return fmt.Errorf("preflight checks failed")
			}

			// Validate configuration
			if err := cfg.Validate(); err != nil {
				logger.Error("config validation failed", zap.Error(err))
				fmt.Printf(`{"results":[{"name":"config","status":"fail","message":"validation failed","error":"%s"}],"warnings":0,"failures":1,"pass":false,"timestamp":"%s"}`+"\n",
					postgres.RedactCredentials(err.Error()), time.Now().UTC().Format(time.RFC3339))
				return fmt.Errorf("preflight checks failed")
			}

			// Build orchestrator config subset for preflight
			orchCfg := &postgres.OrchestratorConfig{
				NATSURL:         cfg.NATS.URL,
				NATSToken:       cfg.NATS.Token,
				NATSTLSEnabled:  cfg.NATS.TLSEnabled,
				DBDSN:           cfg.DB.DSN,
				DBMaxOpenConns:  cfg.DB.MaxOpenConns,
				DBMaxIdleConns:  cfg.DB.MaxIdleConns,
				DBConnMaxLifetime: cfg.DB.ConnMaxLifetime,
				StorageBackend:  cfg.Storage.Backend,
				StorageBucket:   cfg.Storage.Bucket,
				StorageEndpoint: cfg.Storage.Endpoint,
				StorageAccessKey: cfg.Storage.AccessKey,
				StorageSecretKey: cfg.Storage.SecretKey,
				StorageUseSSL:   cfg.Storage.UseSSL,
				JWTSecret:       cfg.JWT.Secret,
			}

			// Connect to database
			db, err := timescale.NewClient(cmd.Context(), cfg.DB.DSN)
			if err != nil {
				logger.Error("failed to connect to database", zap.Error(err))
			}
			defer func() {
				if db != nil {
					db.Close()
				}
			}()

			// Connect to NATS
			var nc *nats.Conn
			if cfg.NATS.URL != "" {
				nc, err = connectNATS(cfg)
				if err != nil {
					logger.Error("failed to connect to NATS", zap.Error(err))
				} else {
					defer nc.Close()
				}
			}

			// Run preflight checks using the shared preflight checker
			preflightChecker := postgres.NewPreflightChecker(db.DB(), orchCfg, nc)
			result := preflightChecker.RunAll()

			// Redact sensitive data in output
			redactedResult := &postgres.PreflightResult{
				Pass:      result.Pass,
				Warnings:  result.Warnings,
				Failures:  result.Failures,
				Timestamp: result.Timestamp,
				Checks:    make([]postgres.PreflightCheck, len(result.Checks)),
			}
			for i, c := range result.Checks {
				redactedResult.Checks[i] = postgres.PreflightCheck{
					Name:    c.Name,
					Status:  c.Status,
					Message: postgres.RedactCredentials(c.Message),
					Error:   postgres.RedactCredentials(c.Error),
				}
			}

			output := map[string]interface{}{
				"results": redactedResult.Checks,
				"warnings": redactedResult.Warnings,
				"failures": redactedResult.Failures,
				"pass":     redactedResult.Pass,
				"timestamp": redactedResult.Timestamp.Format(time.RFC3339),
			}
			data, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(data))

			if !redactedResult.Pass {
				return fmt.Errorf("preflight checks failed")
			}
			return nil
		},
	}
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
		u, err := url.Parse(cfg.NATS.URL)
		if err != nil || u.Hostname() == "" {
			return nil, fmt.Errorf("NATS TLS URL: invalid server hostname")
		}
		serverName := u.Hostname()
		tlsConfig := &tls.Config{
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		}
		if cfg.NATS.TLSCAFile != "" {
			caCert, err := os.ReadFile(cfg.NATS.TLSCAFile)
			if err != nil {
				return nil, fmt.Errorf("NATS CA cert: %w", err)
			}
			caPool := x509.NewCertPool()
			if ok := caPool.AppendCertsFromPEM(caCert); !ok {
				return nil, fmt.Errorf("NATS CA cert: no valid certificates found")
			}
			tlsConfig.RootCAs = caPool
		}
		if cfg.NATS.TLSCertFile != "" && cfg.NATS.TLSKeyFile != "" {
			cert, err := tls.LoadX509KeyPair(cfg.NATS.TLSCertFile, cfg.NATS.TLSKeyFile)
			if err != nil {
				return nil, fmt.Errorf("NATS cert/key: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		natsOpts = append(natsOpts, nats.Secure(tlsConfig))
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

func newUpgradeCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var targetVersion int32

	cmd := &cobra.Command{
		Use:   "upgrade [version]",
		Short: "Upgrade the database schema to the specified version",
		Long:  `Runs the full upgrade workflow: pre-check, version validation, data migration (applies pending Up migrations), post-upgrade verification, and finalize with checksum commit. Uses PostgreSQL advisory locks for concurrency safety.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if len(args) > 0 {
				if _, err := fmt.Sscanf(args[0], "%d", &targetVersion); err != nil {
					return fmt.Errorf("invalid target version %q: %w", args[0], err)
				}
			} else {
				maxID := len(postgres.Migrations())
				if maxID == 0 {
					return fmt.Errorf("no migrations available")
				}
				targetVersion = int32(maxID) // #nosec G115 -- maxID is always < 1000 (migration count)
			}

			db, err := timescale.NewClient(ctx, cfg.DB.DSN)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer db.Close()

			sugar := logger.Sugar()
			sqlDB := &postgres.SQLDB{DB: db.DB()}
			versionStore := postgres.NewVersionStore(db.DB(), sugar)

			upgradeMgr := postgres.NewUpgradeManager(sqlDB, sugar, versionStore)

			// Register custom hooks to integrate StatePreserver snapshots
			snapshotDir := "/var/lib/strata-rmm/backups/state"
			statePreserver := postgres.NewStatePreserver(db.DB(), sugar, snapshotDir)

			upgradeMgr.RegisterHook(postgres.PreCheck, func(ctx context.Context, version int32) error {
				snapID, snapErr := statePreserver.PreDeploySnapshot(ctx)
				if snapErr != nil {
					sugar.Warnw("pre-deploy snapshot failed, continuing without it", "error", snapErr)
				} else {
					sugar.Infow("pre-deploy snapshot created", "snapshot_id", snapID)
				}
				return nil
			})

			upgradeMgr.RegisterHook(postgres.DataMigration, func(ctx context.Context, version int32) error {
				// The default data migration already applies Up migrations via lockConn.
				// No additional hook needed — the default handler handles migration execution.
				return nil
			})

			result, err := upgradeMgr.RunUpgrade(ctx, targetVersion)
			if err != nil {
				if result != nil {
					sugar.Errorw("upgrade failed", "error", err, "from_version", result.FromVersion, "to_version", targetVersion)
					if !result.Success {
						sugar.Warnw("upgrade failed — consider running rollback to revert", "target_version", targetVersion)
					}
				} else {
					sugar.Errorw("upgrade failed", "error", err)
				}
				return fmt.Errorf("upgrade failed: %w", err)
			}

			sugar.Infow("upgrade completed successfully",
				"from_version", result.FromVersion,
				"to_version", result.ToVersion,
				"duration", result.Duration.String(),
				"steps", result.StepsCompleted,
			)
			return nil
		},
	}

	cmd.Flags().Int32Var(&targetVersion, "to-version", 0, "Target schema version (default: latest migration)")
	return cmd
}

func newRollbackCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var targetVersion int32
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "rollback [version]",
		Short: "Rollback the database schema to the specified version",
		Long:  `Runs the full rollback workflow: pre-check, version downgrade validation, data rollback (applies Down migrations in reverse), post-rollback verification, and finalize with checksum commit. Uses PostgreSQL advisory locks for concurrency safety. Supports --dry-run for validation without executing changes.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrchestratorConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if len(args) > 0 {
				if _, err := fmt.Sscanf(args[0], "%d", &targetVersion); err != nil {
					return fmt.Errorf("invalid target version %q: %w", args[0], err)
				}
			} else {
				return fmt.Errorf("rollback requires a target version: orchestrator rollback <version>")
			}

			db, err := timescale.NewClient(ctx, cfg.DB.DSN)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer db.Close()

			sugar := logger.Sugar()
			sqlDB := &postgres.SQLDB{DB: db.DB()}
			versionStore := postgres.NewVersionStore(db.DB(), sugar)

			rollbackEngine := postgres.NewRollbackEngine(sqlDB, sugar, versionStore)

			// Register custom hooks to integrate StatePreserver snapshots
			snapshotDir := "/var/lib/strata-rmm/backups/state"
			statePreserver := postgres.NewStatePreserver(db.DB(), sugar, snapshotDir)

			rollbackEngine.RegisterHook(postgres.RBPreCheck, func(ctx context.Context, fromVersion, toVersion int32) error {
				snapID, snapErr := statePreserver.PreRollbackSnapshot(ctx)
				if snapErr != nil {
					sugar.Warnw("pre-rollback snapshot failed, continuing without it", "error", snapErr)
				} else {
					sugar.Infow("pre-rollback snapshot created", "snapshot_id", snapID)
				}
				return nil
			})

			if dryRun {
				rollbackEngine.SetDryRun(true)
				sugar.Info("running rollback in dry-run mode — no changes will be applied")
			}

			result, err := rollbackEngine.RunRollback(ctx, targetVersion)
			if err != nil {
				sugar.Errorw("rollback failed", "error", err, "from_version", result.FromVersion, "to_version", targetVersion)
				return fmt.Errorf("rollback failed: %w", err)
			}

			sugar.Infow("rollback completed successfully",
				"from_version", result.FromVersion,
				"to_version", result.ToVersion,
				"dry_run", result.DryRun,
				"duration", result.Duration.String(),
				"steps", result.StepsCompleted,
			)
			return nil
		},
	}

	cmd.Flags().Int32Var(&targetVersion, "to-version", 0, "Target schema version for rollback")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate rollback without applying changes")
	return cmd
}
