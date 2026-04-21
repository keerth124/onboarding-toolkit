package github

import (
	"fmt"

	"github.com/cyberark/conjur-onboard/internal/conjur"
	"github.com/cyberark/conjur-onboard/internal/core"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
	"github.com/spf13/cobra"
)

func newGenerateCmd(sf *sharedFlags) *cobra.Command {
	var tenant         string
	var audience       string
	var createDisabled bool

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate Conjur API call artifacts from discovery output",
		Long: `Generate reads discovery.json from the working directory and produces:

  api/01-create-authenticator.json   Body for POST /api/authenticators
  api/02-workloads.yml               Policy YAML for workload creation
  api/03-add-group-members.jsonl     Bodies for group membership additions
  api/plan.json                      Ordered manifest of all API calls
  integration/example-deploy.yml     GitHub Actions snippet
  NEXT_STEPS.md                      Human-readable walkthrough

Examples:
  conjur-onboard github generate --tenant myco
  conjur-onboard github generate --tenant myco --audience my-audience`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if tenant == "" {
				return fmt.Errorf("--tenant is required")
			}

			wd, err := core.EnsureWorkDir(*sf.workDir)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			disc, err := ghdisc.LoadDiscovery(wd)
			if err != nil {
				return fmt.Errorf("loading discovery.json: %w (run 'discover' first)", err)
			}

			gcfg := conjur.GenerateConfig{
				Discovery:     disc,
				Tenant:        tenant,
				Audience:      audience,
				CreateEnabled: !createDisabled,
				WorkDir:       wd,
				Verbose:       *sf.verbose,
				DryRun:        *sf.dryRun,
			}

			plan, err := conjur.Generate(gcfg)
			if err != nil {
				return fmt.Errorf("generation failed: %w", err)
			}

			fmt.Printf("Generation complete\n")
			fmt.Printf("  Authenticator : %s\n", plan.AuthenticatorName)
			fmt.Printf("  Workloads     : %d\n", plan.WorkloadCount)
			fmt.Printf("  Artifacts in  : %s/api/\n", wd)
			fmt.Printf("\nReview the generated policy, then run:\n")
			fmt.Printf("  conjur-onboard github apply --tenant %s\n", tenant)
			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Conjur Cloud tenant subdomain (required)")
	cmd.Flags().StringVar(&audience, "audience", "conjur-cloud", "JWT audience value")
	cmd.Flags().BoolVar(&createDisabled, "create-disabled", false, "Create authenticator in disabled state")

	return cmd
}
