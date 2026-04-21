package github

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/internal/conjur"
	"github.com/cyberark/conjur-onboard/internal/core"
	"github.com/spf13/cobra"
)

func newRollbackCmd(sf *sharedFlags) *cobra.Command {
	var tenant string
	var username string
	var confirm bool

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Reverse successful operations from apply-log.json",
		Long: `Rollback reads apply-log.json and api/plan.json from the working directory,
then runs inverse operations in reverse order.

Rollback removes workloads from the authenticator apps group, deletes generated
workloads, and deletes the authenticator only when this plan created it.

Rollback requires --confirm unless --dry-run is set.

Examples:
  CONJUR_API_KEY=xxx conjur-onboard github rollback --tenant myco --username admin --confirm
  conjur-onboard github rollback --tenant myco --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if tenant == "" {
				return fmt.Errorf("--tenant is required")
			}
			if username == "" && !*sf.dryRun {
				return fmt.Errorf("--username is required")
			}

			apiKey := os.Getenv("CONJUR_API_KEY")
			if apiKey == "" && !*sf.dryRun {
				return fmt.Errorf("CONJUR_API_KEY environment variable is required")
			}

			wd, err := core.EnsureWorkDir(*sf.workDir)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			plan, err := core.LoadPlan(wd)
			if err != nil {
				return fmt.Errorf("loading plan.json: %w", err)
			}

			var client core.APIClient
			if !*sf.dryRun {
				client, err = conjur.NewClient(tenant, username, apiKey, *sf.verbose)
				if err != nil {
					return fmt.Errorf("conjur client: %w", err)
				}
			}

			result, err := core.Rollback(cmd.Context(), core.RollbackConfig{
				WorkDir: wd,
				Plan:    plan,
				Client:  client,
				DryRun:  *sf.dryRun,
				Confirm: confirm,
				Verbose: *sf.verbose,
			})
			if err != nil {
				return fmt.Errorf("rollback failed: %w", err)
			}

			fmt.Printf("\nRollback complete\n")
			fmt.Printf("  Operations run : %d\n", result.OperationsRun)
			fmt.Printf("  Skipped        : %d\n", result.Skipped)
			fmt.Printf("  Log written to : %s/rollback-log.json\n", wd)
			if !*sf.dryRun {
				fmt.Printf("  Apply log moved: %s/apply-log.rolled-back.json\n", wd)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Conjur Cloud tenant subdomain (required)")
	cmd.Flags().StringVar(&username, "username", "", "Conjur username for authentication (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm destructive rollback operations")

	return cmd
}
