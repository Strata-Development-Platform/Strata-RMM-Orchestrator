package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/monitoring"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/timescale"
)

func NewCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var (
		natsURL    string
		timescaleDSN string
	)

	cmd := &cobra.Command{
		Use:   "orchestrator",
		Short: "Start the RMM orchestrator platform services",
		Long:  `Starts the core platform services: NATS consumer, TimescaleDB ingestion, alerting, and API`,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.Info("starting Strata RMM Orchestrator")

			nc, err := nats.Connect(natsURL,
				nats.Name("StrataRMM-Orchestrator"),
				nats.ReconnectWait(5*time.Second),
				nats.MaxReconnects(-1),
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

			ingest := monitoring.NewIngestService(nc, tsdb, logger)
			if err := ingest.Start(ctx); err != nil {
				return fmt.Errorf("starting ingestion: %w", err)
			}
			defer ingest.Stop()
			logger.Info("metrics ingestion started")

			logger.Info("orchestrator running, waiting for signals")
			<-ctx.Done()
			logger.Info("shutting down orchestrator")
			return nil
		},
	}

	cmd.Flags().StringVar(&natsURL, "nats-url", "nats://localhost:4222", "NATS server URL")
	cmd.Flags().StringVar(&timescaleDSN, "timescale-dsn", "postgres://localhost:5432/strata_rmm?sslmode=disable", "TimescaleDB DSN")

	return cmd
}
