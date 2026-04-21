package github

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/internal/core"
	"github.com/spf13/cobra"
)

func newApplyCmd(sf *sharedFlags) *cobra.Command {
	var conn conjurConnectionFlags
	var skipValidate bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Execute the generated API plan against a Conjur tenant",
		Long: `Apply reads plan.json from the working directory and executes the API calls
against the target Conjur tenant in order:
  1. Create authenticator
  2. Load workload policy
  3. Add workloads to the authenticator's apps group, either by REST endpoint
     for SaaS or policy grant load for self-hosted targets

Authentication uses the CONJUR_API_KEY environment variable (never a CLI flag).

On partial failure, apply stops at the first error and prints the rollback command.
Re-running apply against an already-applied state is safe (idempotent).

Examples:
  CONJUR_API_KEY=xxx conjur-onboard github apply --tenant myco --username admin
  CONJUR_API_KEY=xxx conjur-onboard github apply --conjur-url https://conjur.example.com --username admin --account myaccount`,
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

			acfg := core.ApplyConfig{
				WorkDir:      wd,
				Plan:         plan,
				Client:       client,
				DryRun:       *sf.dryRun,
				Verbose:      *sf.verbose,
				SkipValidate: skipValidate,
			}

			result, err := core.Apply(cmd.Context(), acfg)
			if err != nil {
				return fmt.Errorf("apply failed: %w\n\nTo roll back: conjur-onboard github rollback", err)
			}

			fmt.Printf("\nApply complete\n")
			fmt.Printf("  Authenticator created : %s\n", result.AuthenticatorName)
			fmt.Printf("  Workloads created     : %d\n", result.WorkloadsCreated)
			fmt.Printf("  Group memberships     : %d\n", result.MembershipsAdded)
			fmt.Printf("  Log written to        : %s/apply-log.json\n", wd)
			return nil
		},
	}

	cmd.Flags().StringVar(&conn.tenant, "tenant", "", "Conjur Cloud tenant subdomain")
	cmd.Flags().StringVar(&conn.conjurURL, "conjur-url", "", "Full Conjur API/appliance URL for Enterprise or self-hosted")
	cmd.Flags().StringVar(&conn.account, "account", "conjur", "Conjur account name")
	cmd.Flags().StringVar(&conn.username, "username", "", "Conjur username for authentication (required)")
	cmd.Flags().BoolVar(&skipValidate, "skip-validate", false, "Skip pre-flight validation (not recommended)")

	return cmd
}
