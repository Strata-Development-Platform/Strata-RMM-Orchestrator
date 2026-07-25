package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/cmd/agent"
	"github.com/strata-rmm/strata-rmm-orchestrator/cmd/orchestrator"
	"github.com/strata-rmm/strata-rmm-orchestrator/cmd/probe"
	"github.com/spf13/cobra"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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
		agent.NewCommand(ctx, logger),
		orchestrator.NewCommand(ctx, logger),
		probe.NewCommand(ctx, logger),
	)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logger.Fatal("Command failed", zap.Error(err))
	}
}