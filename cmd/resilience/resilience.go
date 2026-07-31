package resilience

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/resilience"
)

func NewCommand(ctx context.Context) *cobra.Command {
	var baseURL, path, label string
	var duration, requestTimeout, maxP95 time.Duration
	var rate, concurrency int
	var maxErrorRate float64
	cmd := &cobra.Command{
		Use:   "resilience",
		Short: "Run a bounded HTTP resilience exercise with enforced thresholds",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runner, err := resilience.NewLoadRunner(resilience.LoadConfig{
				BaseURL: baseURL, Path: path, Label: label,
				BearerToken: os.Getenv("STRATA_RESILIENCE_BEARER_TOKEN"),
				Duration:    duration, Rate: rate, Concurrency: concurrency,
				RequestTimeout: requestTimeout, MaxErrorRate: maxErrorRate, MaxP95: maxP95,
			})
			if err != nil {
				return err
			}
			report := runner.Run(ctx)
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report); err != nil {
				return fmt.Errorf("write resilience report: %w", err)
			}
			if !report.ThresholdPassed {
				return fmt.Errorf("resilience thresholds failed")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "", "HTTPS base URL to exercise")
	cmd.Flags().StringVar(&path, "path", "/health/ready", "request path")
	cmd.Flags().StringVar(&label, "label", "readiness", "bounded result label")
	cmd.Flags().DurationVar(&duration, "duration", time.Minute, "exercise duration")
	cmd.Flags().IntVar(&rate, "rate", 100, "requests per second")
	cmd.Flags().IntVar(&concurrency, "concurrency", 20, "maximum concurrent requests")
	cmd.Flags().DurationVar(&requestTimeout, "request-timeout", 10*time.Second, "per-request timeout")
	cmd.Flags().Float64Var(&maxErrorRate, "max-error-rate", 0.01, "maximum allowed error ratio")
	cmd.Flags().DurationVar(&maxP95, "max-p95", 500*time.Millisecond, "maximum allowed p95 latency")
	_ = cmd.MarkFlagRequired("base-url")
	return cmd
}
