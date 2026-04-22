package github

import (
	"fmt"

	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/cyberark/conjur-onboard/internal/conjur"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
	"github.com/spf13/cobra"
)

func newGenerateCmd(flags shared.GlobalFlags) *cobra.Command {
	var tenant string
	var conjurURL string
	var conjurTarget string
	var audience string
	var provisioningMode string
	var authenticatorName string
	var createDisabled bool

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate Conjur API call artifacts from discovery output",
		Long: `Generate reads discovery.json from the working directory and produces:

  api/01-create-authenticator.json   Body for POST /api/authenticators (bootstrap mode only)
  api/02-workloads.yml               Policy YAML for workload creation
  api/03-add-group-members.jsonl     Bodies for group membership additions
  api/04-grant-authenticator-access.yml Self-hosted policy grant fallback
  api/plan.json                      Ordered manifest of all API calls
  integration/example-deploy.yml     GitHub Actions snippet
  NEXT_STEPS.md                      Human-readable walkthrough

Examples:
  conjur-onboard github generate --tenant myco
  conjur-onboard github generate --conjur-url https://conjur.example.com --conjur-target self-hosted
  conjur-onboard github generate --tenant myco --provisioning-mode workloads-only
  conjur-onboard github generate --tenant myco --audience my-audience`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if tenant == "" && conjurURL == "" {
				return fmt.Errorf("--tenant or --conjur-url is required")
			}
			if tenant != "" && conjurURL != "" {
				return fmt.Errorf("use only one of --tenant or --conjur-url")
			}
			if conjurTarget == "" && conjurURL != "" {
				conjurTarget = "self-hosted"
			}

			wd, err := flags.EnsureWorkDir(platformID)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			disc, err := ghdisc.LoadDiscovery(wd)
			if err != nil {
				return fmt.Errorf("loading discovery.json: %w (run 'discover' first)", err)
			}

			gcfg, err := newGitHubGenerateConfig(disc, githubGenerateOptions{
				Tenant:            tenant,
				ConjurURL:         conjurURL,
				ConjurTarget:      conjurTarget,
				Audience:          audience,
				CreateEnabled:     !createDisabled,
				WorkDir:           wd,
				ProvisioningMode:  provisioningMode,
				AuthenticatorName: authenticatorName,
				Verbose:           flags.IsVerbose(),
				DryRun:            flags.IsDryRun(),
			})
			if err != nil {
				return err
			}

			plan, err := conjur.Generate(gcfg)
			if err != nil {
				return fmt.Errorf("generation failed: %w", err)
			}

			fmt.Printf("Generation complete\n")
			fmt.Printf("  Authenticator : %s\n", plan.AuthenticatorName)
			fmt.Printf("  Mode          : %s\n", provisioningMode)
			fmt.Printf("  Target        : %s\n", conjurTarget)
			fmt.Printf("  Workloads     : %d\n", plan.WorkloadCount)
			fmt.Printf("  Artifacts in  : %s/api/\n", wd)
			fmt.Printf("\nReview the generated policy, then run:\n")
			if conjurURL != "" {
				fmt.Printf("  conjur-onboard github apply --conjur-url %s\n", conjurURL)
			} else {
				fmt.Printf("  conjur-onboard github apply --tenant %s\n", tenant)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Secrets Manager SaaS tenant subdomain")
	cmd.Flags().StringVar(&conjurURL, "conjur-url", "", "Full Conjur appliance URL for Enterprise or self-hosted")
	cmd.Flags().StringVar(&conjurTarget, "conjur-target", "", "Conjur target: saas or self-hosted")
	cmd.Flags().StringVar(&audience, "audience", "conjur-cloud", "JWT audience value")
	cmd.Flags().StringVar(&provisioningMode, "provisioning-mode", "bootstrap", "Provisioning mode: bootstrap or workloads-only")
	cmd.Flags().StringVar(&authenticatorName, "authenticator-name", "", "Existing authenticator name override for workloads-only mode")
	cmd.Flags().BoolVar(&createDisabled, "create-disabled", false, "Create authenticator in disabled state")

	return cmd
}
