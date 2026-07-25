package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/collectors"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/comms"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/core"
	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/update"
)

func NewCommand(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start the RMM monitoring agent",
		Long:  `Cross-platform monitoring agent (Windows/Linux/macOS) that connects to the Strata RMM platform via NATS JetStream`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := core.DefaultConfig()

			cfg.Agent.TenantID, _ = cmd.Flags().GetString("tenant-id")
			cfg.Agent.EnrollmentToken, _ = cmd.Flags().GetString("enrollment-token")

			natsURLs, _ := cmd.Flags().GetStringSlice("nats-url")
			if len(natsURLs) > 0 {
				cfg.NATS.URLs = natsURLs
			}

			if configPath != "" {
				if err := cfg.Load(configPath); err != nil {
					return fmt.Errorf("loading config: %w", err)
				}
			}

			cl := &zapLogger{logger: logger}

			agent := core.New(cfg, cl)
			if err := agent.Start(ctx); err != nil {
				return fmt.Errorf("starting agent: %w", err)
			}
			defer agent.Stop()

			natsClient := comms.NewClient(&cfg.NATS, agent.Identity(), cl)
			if err := natsClient.Connect(ctx); err != nil {
				return fmt.Errorf("connecting to NATS: %w", err)
			}
			defer natsClient.Close()

			commsHandler := comms.NewCommsHandler(natsClient, agent.Store(), cl)
			if err := commsHandler.Start(ctx); err != nil {
				return fmt.Errorf("starting comms: %w", err)
			}
			defer commsHandler.Stop()

			updateStore := update.NewStore(agent.Store().DB())
			if err := updateStore.Init(); err != nil {
				return fmt.Errorf("init update store: %w", err)
			}

			manifestURL := cfg.Update.ManifestURL
			if manifestURL == "" {
				manifestURL = "https://releases.strata-rmm.io"
			}

			updateClient := update.NewClient(update.ClientOptions{
				ManifestURL:   manifestURL,
				Store:         updateStore,
				DataDir:       cfg.Agent.DataDir,
				Channel:       update.Channel(cfg.Update.Channel),
				CheckInterval: cfg.Update.CheckInterval,
			})

			if cfg.Update.Enabled {
				ident := agent.Identity()
				rolloutMgr := update.NewRolloutManager(natsClient.Conn(), updateStore, updateClient, ident.AgentID, ident.TenantID)
				if err := rolloutMgr.SubscribeCommands(ctx); err != nil {
					return fmt.Errorf("subscribe rollout commands: %w", err)
				}

				go func() {
					ticker := time.NewTicker(cfg.Update.CheckInterval)
					defer ticker.Stop()

					state, _ := updateStore.GetState()
					if updateClient.ShouldCheck(state.LastCheck) {
						manifest, err := updateClient.Check(ctx)
						if err == nil && manifest != nil && rolloutMgr.ShouldApply(manifest.Version) {
							binaryPath, dlErr := updateClient.Download(ctx, manifest)
							if dlErr == nil {
								if applyErr := updateClient.Apply(binaryPath); applyErr == nil {
									updateClient.VerifyAndSwitch()
									logger.Info("agent auto-updated", zap.String("version", manifest.Version))
								}
							}
						}
						updateStore.SetLastCheck(time.Now())
					}

					for {
						select {
						case <-ticker.C:
							manifest, err := updateClient.Check(ctx)
							if err != nil || manifest == nil {
								continue
							}
							if !rolloutMgr.ShouldApply(manifest.Version) {
								continue
							}
							binaryPath, dlErr := updateClient.Download(ctx, manifest)
							if dlErr != nil {
								logger.Warn("update download failed", zap.Error(dlErr))
								continue
							}
							if applyErr := updateClient.Apply(binaryPath); applyErr != nil {
								logger.Warn("update apply failed", zap.Error(applyErr))
								continue
							}
							if verifyErr := updateClient.VerifyAndSwitch(); verifyErr != nil {
								logger.Warn("update verify failed, rolling back", zap.Error(verifyErr))
								updateClient.Rollback()
								continue
							}
							logger.Info("agent auto-updated", zap.String("version", manifest.Version))
							updateStore.SetLastCheck(time.Now())
						case <-ctx.Done():
							return
						}
					}
				}()
			}

			sysCollector := collectors.NewSystemCollector(cfg.Collect.Interval)
			sysCollector.Start(ctx)
			defer sysCollector.Stop()

			logger.Info("agent running",
				zap.String("agent_id", agent.Identity().AgentID),
				zap.String("tenant_id", agent.Identity().TenantID),
				zap.Strings("nats_urls", cfg.NATS.URLs),
			)

			<-ctx.Done()
			logger.Info("shutting down agent")

			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to agent config file (YAML)")
	cmd.Flags().String("tenant-id", "", "Tenant ID (required)")
	cmd.Flags().String("enrollment-token", "", "Agent enrollment token")
	cmd.Flags().StringSlice("nats-url", []string{"nats://localhost:4222"}, "NATS server URL(s)")
	cmd.Flags().Bool("install-service", false, "Install as system service")
	cmd.Flags().Bool("uninstall-service", false, "Uninstall system service")

	return cmd
}

type zapLogger struct {
	logger *zap.Logger
}

func (l *zapLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Info(msg, toFields(keysAndValues)...)
}

func (l *zapLogger) Error(msg string, keysAndValues ...interface{}) {
	l.logger.Error(msg, toFields(keysAndValues)...)
}

func (l *zapLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.logger.Warn(msg, toFields(keysAndValues)...)
}

func (l *zapLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.logger.Debug(msg, toFields(keysAndValues)...)
}

func toFields(keysAndValues []interface{}) []zap.Field {
	var fields []zap.Field
	for i := 0; i < len(keysAndValues)-1; i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			continue
		}
		fields = append(fields, zap.Any(key, keysAndValues[i+1]))
	}
	return fields
}
