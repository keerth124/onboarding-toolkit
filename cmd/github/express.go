package github

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/cyberark/conjur-onboard/internal/conjur"
	"github.com/cyberark/conjur-onboard/internal/core"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
	"github.com/spf13/cobra"
)

func newExpressCmd(flags shared.GlobalFlags) *cobra.Command {
	var org string
	var token string
	var tenant string
	var conjurURL string
	var conjurTarget string
	var username string
	var account string
	var audience string
	var reposFromFile string
	var provisioningMode string
	var authenticatorName string
	var createDisabled bool
	var autoApply bool

	cmd := &cobra.Command{
		Use:   "express",
		Short: "Run discover, generate, and optionally apply end-to-end with best-practice defaults",
		Long: `Express mode runs the full onboarding flow in a single command using
CyberArk Professional Services recommended defaults:

  Identity claim  : repository  (binds workload to the specific repo)
  Enforced claims : none in the MVP generator
  Audience        : conjur-cloud

All generated artifacts are written to the working directory for review.
By default, express mode generates artifacts and prompts before applying.
Pass --apply to apply automatically without prompting.

Using recommended primary identity claim: 'repository'. Environment claims are
reported for review but not enforced by the MVP generator. To customize, re-run with:
  conjur-onboard github discover --org <owner>
  conjur-onboard github generate --tenant <tenant> [custom flags]
  conjur-onboard github generate --conjur-url <appliance-url> --conjur-target self-hosted [custom flags]

Examples:
  conjur-onboard github express --org acme-corp --tenant myco
  conjur-onboard github express --org acme-corp --conjur-url https://conjur.example.com
  conjur-onboard github express --org keerth124 --tenant myco --repos-from-file repos.txt
  conjur-onboard github express --org keerth124 --conjur-url https://conjur.example.com --repos-from-file repos.txt
  conjur-onboard github express --org acme-corp --tenant myco --provisioning-mode workloads-only
  conjur-onboard github express --org acme-corp --tenant myco --repos-from-file repos.txt
  CONJUR_API_KEY=xxx conjur-onboard github express --org acme-corp --tenant myco --username admin --apply
  CONJUR_API_KEY=xxx conjur-onboard github express --org acme-corp --conjur-url https://conjur.example.com --username admin --apply`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if org == "" {
				return fmt.Errorf("--org is required")
			}
			if tenant == "" {
				if conjurURL == "" {
					return fmt.Errorf("--tenant or --conjur-url is required")
				}
			}
			if tenant != "" && conjurURL != "" {
				return fmt.Errorf("use only one of --tenant or --conjur-url")
			}
			if conjurTarget == "" && conjurURL != "" {
				conjurTarget = "self-hosted"
			}

			tok, err := resolveGitHubToken(cmd.Context(), token)
			if err != nil {
				return err
			}

			repoNames, err := loadRepoNames(reposFromFile)
			if err != nil {
				return err
			}

			wd, err := flags.EnsureWorkDir(platformID)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			fmt.Println("==> Step 1/3: Discovering GitHub owner configuration...")
			discCfg := ghdisc.DiscoverConfig{
				Org:       org,
				Token:     tok,
				RepoNames: repoNames,
				Verbose:   flags.IsVerbose(),
			}
			disc, err := ghdisc.Discover(cmd.Context(), discCfg)
			if err != nil {
				return fmt.Errorf("discovery: %w", err)
			}
			if err := core.WriteJSON(wd, "discovery.json", disc); err != nil {
				return fmt.Errorf("writing discovery.json: %w", err)
			}
			fmt.Printf("    Found %d repos for GitHub owner %q\n", len(disc.Repos), org)

			fmt.Println("==> Step 2/3: Generating Conjur API artifacts...")
			fmt.Println("    Using recommended primary identity claim: 'repository'")
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
				return fmt.Errorf("generation: %w", err)
			}
			plan, err := conjur.Generate(gcfg)
			if err != nil {
				return fmt.Errorf("generation: %w", err)
			}
			fmt.Printf("    Authenticator : %s\n", plan.AuthenticatorName)
			fmt.Printf("    Mode          : %s\n", provisioningMode)
			fmt.Printf("    Target        : %s\n", conjurTarget)
			fmt.Printf("    Workloads     : %d\n", plan.WorkloadCount)

			if !autoApply {
				fmt.Printf("\nReview the generated policy at %s/api/\n", wd)
				fmt.Printf("Then run:\n")
				if conjurURL != "" {
					fmt.Printf("  CONJUR_API_KEY=<key> conjur-onboard github apply --conjur-url %s --username <username> --work-dir %s\n", conjurURL, wd)
				} else {
					fmt.Printf("  CONJUR_API_KEY=<key> conjur-onboard github apply --tenant %s --username <username> --work-dir %s\n", tenant, wd)
				}
				fmt.Printf("\nOr re-run express with --apply to apply automatically.\n")
				return nil
			}

			if username == "" {
				return fmt.Errorf("--username is required when --apply is set")
			}
			apiKey := os.Getenv("CONJUR_API_KEY")
			if apiKey == "" {
				return fmt.Errorf("CONJUR_API_KEY environment variable is required when --apply is set")
			}

			fmt.Println("==> Step 3/3: Applying to Conjur endpoint...")
			client, err := (shared.ConjurConnectionFlags{
				Tenant:    tenant,
				ConjurURL: conjurURL,
				Account:   account,
				Username:  username,
			}).NewClient(apiKey, flags.IsVerbose())
			if err != nil {
				return fmt.Errorf("conjur client: %w", err)
			}

			loadedPlan, err := core.LoadPlan(wd)
			if err != nil {
				return fmt.Errorf("loading plan: %w", err)
			}

			if _, err := core.Validate(cmd.Context(), core.ValidateConfig{
				WorkDir: wd,
				Plan:    loadedPlan,
				Client:  client,
				DryRun:  flags.IsDryRun(),
				Verbose: flags.IsVerbose(),
			}); err != nil {
				return fmt.Errorf("validate before apply: %w", err)
			}

			acfg := core.ApplyConfig{
				WorkDir: wd,
				Plan:    loadedPlan,
				Client:  client,
				DryRun:  flags.IsDryRun(),
				Verbose: flags.IsVerbose(),
			}
			result, err := core.Apply(cmd.Context(), acfg)
			if err != nil {
				return fmt.Errorf("apply: %w\n\nRollback: conjur-onboard github rollback --work-dir %s", err, wd)
			}

			fmt.Printf("\nOnboarding complete!\n")
			fmt.Printf("  Authenticator : %s\n", result.AuthenticatorName)
			fmt.Printf("  Workloads     : %d created\n", result.WorkloadsCreated)
			fmt.Printf("  Memberships   : %d added\n", result.MembershipsAdded)
			fmt.Printf("\nNext: grant the apps group access to your safes.\n")
			fmt.Printf("See %s/NEXT_STEPS.md for details.\n", wd)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "GitHub organization or user owner name (required)")
	cmd.Flags().StringVar(&token, "token", "", "GitHub personal access token (or set GITHUB_TOKEN)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Secrets Manager SaaS tenant subdomain")
	cmd.Flags().StringVar(&conjurURL, "conjur-url", "", "Full Conjur appliance URL for Enterprise or self-hosted")
	cmd.Flags().StringVar(&conjurTarget, "conjur-target", "", "Conjur target: saas or self-hosted")
	cmd.Flags().StringVar(&username, "username", "", "Conjur username (required with --apply)")
	cmd.Flags().StringVar(&account, "account", "conjur", "Conjur account name")
	cmd.Flags().StringVar(&audience, "audience", "conjur-cloud", "JWT audience value")
	cmd.Flags().StringVar(&reposFromFile, "repos-from-file", "", "Optional file with one repo name or owner/name per line")
	cmd.Flags().StringVar(&provisioningMode, "provisioning-mode", "bootstrap", "Provisioning mode: bootstrap or workloads-only")
	cmd.Flags().StringVar(&authenticatorName, "authenticator-name", "", "Existing authenticator name override for workloads-only mode")
	cmd.Flags().BoolVar(&createDisabled, "create-disabled", false, "Create authenticator in disabled state")
	cmd.Flags().BoolVar(&autoApply, "apply", false, "Apply to Conjur automatically without prompting")

	return cmd
}
