package github

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/internal/core"
	"github.com/spf13/cobra"
)

func newValidateCmd(sf *sharedFlags) *cobra.Command {
	var conn conjurConnectionFlags

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run non-mutating checks against a generated GitHub API plan",
		Long: `Validate reads api/plan.json from the working directory, verifies that all
referenced request bodies are readable, and checks that the target Conjur tenant
is reachable with the provided tool-auth credentials.

Authentication uses the CONJUR_API_KEY environment variable.

Examples:
  CONJUR_API_KEY=xxx conjur-onboard github validate --tenant myco --username admin
  CONJUR_API_KEY=xxx conjur-onboard github validate --conjur-url https://conjur.example.com --username admin --account myaccount`,
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
				return fmt.Errorf("loading plan.json: %w (run 'generate' first)", err)
			}

			var client core.APIClient
			if !*sf.dryRun {
				client, err = newConjurClient(conn, apiKey, *sf.verbose)
				if err != nil {
					return fmt.Errorf("conjur client: %w", err)
				}
			}

			result, err := core.Validate(cmd.Context(), core.ValidateConfig{
				WorkDir: wd,
				Plan:    plan,
				Client:  client,
				DryRun:  *sf.dryRun,
				Verbose: *sf.verbose,
			})
			if err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			fmt.Printf("Validation complete\n")
			fmt.Printf("  Operations checked : %d\n", result.Checked)
			fmt.Printf("  Log written to     : %s/validate-log.json\n", wd)
			for _, warning := range result.Warnings {
				fmt.Printf("  Warning            : %s\n", warning)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&conn.tenant, "tenant", "", "Conjur Cloud tenant subdomain")
	cmd.Flags().StringVar(&conn.conjurURL, "conjur-url", "", "Full Conjur API/appliance URL for Enterprise or self-hosted")
	cmd.Flags().StringVar(&conn.account, "account", "conjur", "Conjur account name")
	cmd.Flags().StringVar(&conn.username, "username", "", "Conjur username for authentication (required)")

	return cmd
}
