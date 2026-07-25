package orchestrator

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/alerting"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/inventory"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/monitoring"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/platform"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/remote"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func NewCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
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
			logger.Info("starting Strata RMM Orchestrator")

			nc, err := nats.Connect(natsURL,
				nats.Name("StrataRMM-Orchestrator"),
				nats.ReconnectWait(5*time.Second),
				nats.MaxReconnects(-1),
				nats.RetryOnFailedConnect(true),
			)
			if err != nil {
				return fmt.Errorf("connecting to NATS: %w", err)
			}
			defer nc.Close()
			logger.Info("connected to NATS", zap.String("url", nc.ConnectedUrl()))

			tsdb, err := timescale.NewClient(ctx, timescaleDSN)
			if err != nil {
				return fmt.Errorf("connecting to TimescaleDB: %w", err)
			}
			defer tsdb.Close()
			logger.Info("connected to TimescaleDB")

			if err := tsdb.ApplyMigrations(ctx); err != nil {
				return fmt.Errorf("applying migrations: %w", err)
			}
			logger.Info("TimescaleDB migrations applied")

			sm := postgres.NewSchemaManager(tsdb.DB())
			if err := sm.Apply(); err != nil {
				return fmt.Errorf("applying relational schema: %w", err)
			}
			logger.Info("relational schema migrations applied")

			if err := postgres.SeedDevTenant(tsdb.DB()); err != nil {
				logger.Warn("seed dev tenant", zap.Error(err))
			} else {
				logger.Info("dev tenant seeded")
			}

			ingest := monitoring.NewIngestService(nc, tsdb, logger)
			if err := ingest.Start(ctx); err != nil {
				return fmt.Errorf("starting ingestion: %w", err)
			}
			defer ingest.Stop()
			logger.Info("metrics ingestion started")

			alertStore := alerting.NewStore(tsdb.DB())
			alertNotifier := alerting.NewNotifier()
			alertEngine := alerting.NewEngine(nc, tsdb, alertStore, alertNotifier, logger)
			if err := alertEngine.Start(ctx); err != nil {
				return fmt.Errorf("starting alerting engine: %w", err)
			}
			defer alertEngine.Stop()
			logger.Info("alerting engine started")

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

			api := platform.NewAPIServer(apiAddr, tsdb, nc, logger).
				WithAlertEngine(alertEngine).
				WithVulnEngine(vulnEngine).
				WithCVESyncEngine(cveSync)

			if storageBackend != "" && storageBackend != "none" {
				storageCfg := storage.Config{
					Type:       storageBackend,
					Bucket:     storageBucket,
					Region:     storageRegion,
					Endpoint:   storageEndpoint,
					AccessKey:  os.Getenv("STORAGE_ACCESS_KEY"),
					SecretKey:  os.Getenv("STORAGE_SECRET_KEY"),
					UseSSL:     os.Getenv("STORAGE_USE_SSL") == "true",
					KMSKeyID:   os.Getenv("STORAGE_KMS_KEY_ID"),
				}

				sb, err := storage.NewBackend(ctx, storageCfg)
				if err != nil {
					logger.Warn("storage backend init (continuing without recordings)", zap.Error(err))
				} else {
					logger.Info("storage backend initialized", zap.String("type", storageBackend), zap.String("bucket", storageBucket))

					recStore := remote.NewRecordingStore(tsdb.DB())
					recorder := remote.NewRecorder(sb, logger)

					gw := remote.NewGateway(nc, tunnelAddr, logger).
						WithRecorder(recorder).
						WithRecordingStore(recStore)
					if err := gw.Start(ctx); err != nil {
						return fmt.Errorf("starting tunnel gateway: %w", err)
					}
					logger.Info("tunnel gateway started", zap.String("addr", tunnelAddr))

					cleanup := remote.NewCleanupJob(recStore, sb, logger)
					cleanup.Start(ctx)

					api = api.WithRecordingStore(recStore).WithStorageBackend(sb)
				}
			}
			if err := api.Start(ctx); err != nil {
				return fmt.Errorf("starting API server: %w", err)
			}
			defer api.Stop(ctx)
			logger.Info("API server started", zap.String("addr", apiAddr))

			logger.Info("orchestrator running, waiting for signals")
			<-ctx.Done()
			logger.Info("shutting down orchestrator")
			return nil
		},
	}

	cmd.Flags().StringVar(&natsURL, "nats-url", "nats://localhost:4222", "NATS server URL")
	cmd.Flags().StringVar(&timescaleDSN, "timescale-dsn", "postgres://localhost:5432/strata_rmm?sslmode=disable", "TimescaleDB DSN")
	cmd.Flags().StringVar(&apiAddr, "api-addr", ":8080", "API server listen address")
	cmd.Flags().StringVar(&tunnelAddr, "tunnel-addr", ":8443", "Tunnel gateway listen address")

	cmd.Flags().StringVar(&storageBackend, "storage-backend", envOrDefault("STORAGE_BACKEND", "local"), "Storage backend (minio, s3, local, none)")
	cmd.Flags().StringVar(&storageBucket, "storage-bucket", envOrDefault("STORAGE_BUCKET", "strata-recordings"), "Storage bucket name")
	cmd.Flags().StringVar(&storageRegion, "storage-region", envOrDefault("STORAGE_REGION", ""), "Storage region")
	cmd.Flags().StringVar(&storageEndpoint, "storage-endpoint", envOrDefault("STORAGE_ENDPOINT", ""), "Storage endpoint (for MinIO/S3-compatible)")

	return cmd
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
