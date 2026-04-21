package github

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/internal/conjur"
	"github.com/cyberark/conjur-onboard/internal/core"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
	"github.com/spf13/cobra"
)

func newExpressCmd(sf *sharedFlags) *cobra.Command {
	var org           string
	var token         string
	var tenant        string
	var username      string
	var audience      string
	var createDisabled bool
	var autoApply     bool

	cmd := &cobra.Command{
		Use:   "express",
		Short: "Run discover → generate → apply end-to-end with best-practice defaults",
		Long: `Express mode runs the full onboarding flow in a single command using
CyberArk Professional Services recommended defaults:

  Identity claim  : repository  (binds workload to the specific repo)
  Enforced claims : environment (when repos have environments configured)
  Audience        : conjur-cloud

All generated artifacts are written to the working directory for review.
By default, express mode generates artifacts and prompts before applying.
Pass --apply to apply automatically without prompting.

Using recommended primary identity claim: 'repository'. Environment claims are
reported for review but not enforced by the MVP generator. To customize, re-run with:
  conjur-onboard github discover --org <org>
  conjur-onboard github generate --tenant <tenant> [custom flags]

Examples:
  conjur-onboard github express --org acme-corp --tenant myco
  CONJUR_API_KEY=xxx conjur-onboard github express --org acme-corp --tenant myco --username admin --apply`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if org == "" {
				return fmt.Errorf("--org is required")
			}
			if tenant == "" {
				return fmt.Errorf("--tenant is required")
			}

			tok, err := resolveGitHubToken(cmd.Context(), token)
			if err != nil {
				return err
			}

			wd, err := core.EnsureWorkDir(*sf.workDir)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			// ── Step 1: Discover ─────────────────────────────────────────────
			fmt.Println("==> Step 1/3: Discovering GitHub org configuration...")
			discCfg := ghdisc.DiscoverConfig{
				Org:     org,
				Token:   tok,
				Verbose: *sf.verbose,
			}
			disc, err := ghdisc.Discover(cmd.Context(), discCfg)
			if err != nil {
				return fmt.Errorf("discovery: %w", err)
			}
			if err := core.WriteJSON(wd, "discovery.json", disc); err != nil {
				return fmt.Errorf("writing discovery.json: %w", err)
			}
			fmt.Printf("    Found %d repos in org %q\n", len(disc.Repos), org)

			// ── Step 2: Generate ─────────────────────────────────────────────
			fmt.Println("==> Step 2/3: Generating Conjur API artifacts...")
			fmt.Println("    Using recommended primary identity claim: 'repository'")
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
				return fmt.Errorf("generation: %w", err)
			}
			fmt.Printf("    Authenticator : %s\n", plan.AuthenticatorName)
			fmt.Printf("    Workloads     : %d\n", plan.WorkloadCount)

			// ── Step 3: Apply ────────────────────────────────────────────────
			if !autoApply {
				fmt.Printf("\nReview the generated policy at %s/api/\n", wd)
				fmt.Printf("Then run:\n")
				fmt.Printf("  CONJUR_API_KEY=<key> conjur-onboard github apply --tenant %s --username <username> --work-dir %s\n", tenant, wd)
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

			fmt.Println("==> Step 3/3: Applying to Conjur Cloud tenant...")
			client, err := conjur.NewClient(tenant, username, apiKey, *sf.verbose)
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
				DryRun:  *sf.dryRun,
				Verbose: *sf.verbose,
			}); err != nil {
				return fmt.Errorf("validate before apply: %w", err)
			}

			acfg := core.ApplyConfig{
				WorkDir: wd,
				Plan:    loadedPlan,
				Client:  client,
				DryRun:  *sf.dryRun,
				Verbose: *sf.verbose,
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

	cmd.Flags().StringVar(&org, "org", "", "GitHub organization name (required)")
	cmd.Flags().StringVar(&token, "token", "", "GitHub personal access token (or set GITHUB_TOKEN)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Conjur Cloud tenant subdomain (required)")
	cmd.Flags().StringVar(&username, "username", "", "Conjur username (required with --apply)")
	cmd.Flags().StringVar(&audience, "audience", "conjur-cloud", "JWT audience value")
	cmd.Flags().BoolVar(&createDisabled, "create-disabled", false, "Create authenticator in disabled state")
	cmd.Flags().BoolVar(&autoApply, "apply", false, "Apply to tenant automatically without prompting")

	return cmd
}
