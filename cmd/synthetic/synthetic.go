package synthetic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/synthetic"
)

func NewCommand(ctx context.Context) *cobra.Command {
	var baseURL string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "synthetic",
		Short: "Run independent production-path synthetic checks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runner, err := synthetic.New(synthetic.Config{
				BaseURL:  baseURL,
				Email:    os.Getenv("STRATA_SYNTHETIC_EMAIL"),
				Password: os.Getenv("STRATA_SYNTHETIC_PASSWORD"),
				TenantID: os.Getenv("STRATA_SYNTHETIC_TENANT_ID"),
				Timeout:  timeout,
			})
			if err != nil {
				return err
			}
			results := runner.Run(ctx)
			encoder := json.NewEncoder(cmd.OutOrStdout())
			failed := false
			for _, result := range results {
				if err := encoder.Encode(result); err != nil {
					return fmt.Errorf("write synthetic result: %w", err)
				}
				failed = failed || !result.Success
			}
			if failed {
				return fmt.Errorf("one or more synthetic checks failed")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "", "HTTPS base URL to check")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "per-request timeout")
	_ = cmd.MarkFlagRequired("base-url")
	return cmd
}
