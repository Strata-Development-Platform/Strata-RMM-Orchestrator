package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/strata-rmm/strata-rmm-orchestrator/cmd/orchestrator"
	"github.com/strata-rmm/strata-rmm-orchestrator/cmd/agent"
	"github.com/strata-rmm/strata-rmm-orchestrator/cmd/probe"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/platform"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app := platform.NewApp(logger)
	
	// Register subcommands
	app.AddCommand(orchestrator.NewCommand(ctx, logger))
	app.AddCommand(agent.NewCommand(ctx, logger))
	app.AddCommand(probe.NewCommand(ctx, logger))

	if err := app.Execute(); err != nil {
		logger.Fatal("Application failed", zap.Error(err))
	}
}