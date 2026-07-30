package platform

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type App struct {
	rootCmd *cobra.Command
	logger  *zap.Logger
}

func NewApp(logger *zap.Logger) *App {
	app := &App{logger: logger}
	app.rootCmd = &cobra.Command{
		Use:   "strata-rmm",
		Short: "Strata RMM Orchestrator - Unified monitoring and management platform",
		Long: `Strata RMM Orchestrator is a horizontally-scalable, multi-tenant
Remote Monitoring & Management platform with cross-platform agents.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return app
}

func (a *App) AddCommand(cmd *cobra.Command) {
	a.rootCmd.AddCommand(cmd)
}

func (a *App) Execute() error {
	return a.rootCmd.Execute()
}
