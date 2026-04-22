package github

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/internal/core"
	"github.com/spf13/cobra"
)

func newRollbackCmd(sf *sharedFlags) *cobra.Command {
	var conn conjurConnectionFlags
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
  CONJUR_API_KEY=xxx conjur-onboard github rollback --conjur-url https://conjur.example.com --username admin --account myaccount --confirm
  conjur-onboard github rollback --tenant myco --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := conn.validateEndpointRequired(); err != nil {
				return err
			}
			if conn.username == "" && !*sf.dryRun {
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
				client, err = newConjurClient(conn, apiKey, *sf.verbose)
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

	cmd.Flags().StringVar(&conn.tenant, "tenant", "", "Secrets Manager SaaS tenant subdomain")
	cmd.Flags().StringVar(&conn.conjurURL, "conjur-url", "", "Full Conjur appliance URL for Enterprise or self-hosted")
	cmd.Flags().StringVar(&conn.account, "account", "conjur", "Conjur account name")
	cmd.Flags().StringVar(&conn.username, "username", "", "Conjur username for authentication (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm destructive rollback operations")

	return cmd
}
