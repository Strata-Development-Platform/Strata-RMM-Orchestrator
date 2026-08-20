package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/cmd/agent"
	"github.com/strata-rmm/strata-rmm-orchestrator/cmd/orchestrator"
	"github.com/strata-rmm/strata-rmm-orchestrator/cmd/probe"
	resiliencecmd "github.com/strata-rmm/strata-rmm-orchestrator/cmd/resilience"
	syntheticcmd "github.com/strata-rmm/strata-rmm-orchestrator/cmd/synthetic"
)

var (
	version = "0.0.0-dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := map[string]string{
				"version": version,
				"commit":  commit,
				"date":    date,
			}
			output, _ := cmd.Flags().GetString("output")
			if output == "json" {
				data, _ := json.Marshal(info)
				fmt.Println(string(data))
			} else {
				fmt.Printf("Strata RMM v%s (commit: %s, built: %s)\n", version, commit, date)
			}
			return nil
		},
	}
	versionCmd.Flags().StringP("output", "o", "text", "Output format (text|json)")

	rootCmd := &cobra.Command{
		Use:   "strata-rmm",
		Short: "Strata RMM Platform - Unified Remote Monitoring & Management",
		Long: `Strata RMM is a horizontally-scalable, multi-tenant Remote Monitoring & Management platform
with cross-platform agents (Go) supporting both SaaS and self-hosted deployments.

Components:
  agent         - Cross-platform monitoring agent (Windows/Linux/macOS)
  orchestrator   - Platform services (NATS consumer, TimescaleDB, alerting, API)
  probe         - Network probe for agentless monitoring (SNMP, flows, synthetics)`,
	}

	rootCmd.AddCommand(
		versionCmd,
		agent.NewCommand(ctx, logger),
		orchestrator.NewProductCommand(ctx, version, commit, logger),
		orchestrator.NewDockerUpdateHostCommand(ctx, version, commit, logger),
		probe.NewCommand(ctx, logger),
		resiliencecmd.NewCommand(ctx),
		syntheticcmd.NewCommand(ctx),
	)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logger.Fatal("Command failed", zap.Error(err))
	}
}
