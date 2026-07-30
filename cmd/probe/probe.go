package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/probe"
)

func NewCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var (
		configFile string
		tenantID   string
		natsURL    string
	)

	cmd := &cobra.Command{
		Use:   "probe",
		Short: "Start the network probe for agentless monitoring",
		Long:  `Lightweight agentless collector for network segments - SNMP polling, flow collection, synthetic monitoring, topology discovery`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if tenantID == "" {
				return fmt.Errorf("--tenant-id is required")
			}

			nc, err := nats.Connect(natsURL,
				nats.Name("StrataRMM-Probe"),
				nats.ReconnectWait(5*time.Second),
				nats.MaxReconnects(-1),
				nats.RetryOnFailedConnect(true),
			)
			if err != nil {
				return fmt.Errorf("connecting to NATS: %w", err)
			}
			defer nc.Close()
			logger.Info("connected to NATS", zap.String("url", nc.ConnectedUrl()))

			cfg := probe.Config{
				ProbeID:           fmt.Sprintf("probe-%s", tenantID),
				TenantID:          tenantID,
				NATSURL:           natsURL,
				DiscoveryEnabled:  true,
				DiscoverySubnets:  []string{},
				FlowEnabled:       true,
				FlowPort:          2055,
				FlowProtocols:     []string{"netflow9", "ipfix"},
				PollInterval:      5 * time.Minute,
				DiscoveryInterval: 1 * time.Hour,
			}

			if configFile != "" {
				logger.Warn("config file loading not yet implemented, using defaults")
			}

			p := probe.New(cfg.ProbeID, tenantID, nc, cfg, logger)
			if err := p.Start(ctx); err != nil {
				return fmt.Errorf("starting probe: %w", err)
			}
			defer p.Stop()
			logger.Info("network probe running")

			<-ctx.Done()
			logger.Info("shutting down network probe")
			return nil
		},
	}

	cmd.Flags().StringVar(&configFile, "config", "", "Path to probe config file")
	cmd.Flags().StringVar(&tenantID, "tenant-id", "", "Tenant ID (required)")
	cmd.Flags().StringVar(&natsURL, "nats-url", "nats://localhost:4222", "NATS server URL")

	return cmd
}
