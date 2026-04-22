package github

import (
	"fmt"
	"strings"

	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/cyberark/conjur-onboard/internal/core"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
	"github.com/spf13/cobra"
)

func newInspectCmd(flags shared.GlobalFlags) *cobra.Command {
	var mode string
	var repo string
	var environment string
	var tokenAppProperty string
	var enforcedClaims string

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect GitHub OIDC claims with security annotations",
		Long: `Inspect shows the GitHub Actions OIDC claims relevant to Conjur workload
identity selection. The first implementation supports synthetic inspection from
GitHub's documented claim shape.

Examples:
  conjur-onboard github inspect --repo acme-corp/api-service
  conjur-onboard github inspect --mode synthetic --repo acme-corp/api-service --environment production
  conjur-onboard github inspect --repo acme-corp/api-service --token-app-property repository`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode != "synthetic" {
				return fmt.Errorf("only --mode synthetic is implemented in this GitHub slice")
			}
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}
			if !strings.Contains(repo, "/") {
				return fmt.Errorf("--repo must be in owner/name form")
			}

			selection, err := ghdisc.ParseClaimSelection(tokenAppProperty, enforcedClaims)
			if err != nil {
				return err
			}

			analysis := ghdisc.BuildSyntheticClaimAnalysis(repo, environment, selection)
			printInspection(analysis)

			wd, err := flags.EnsureWorkDir(platformID)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}
			if err := core.WriteJSON(wd, "claims-analysis.json", analysis); err != nil {
				return fmt.Errorf("writing claims-analysis.json: %w", err)
			}
			fmt.Printf("\nClaims analysis written to: %s/claims-analysis.json\n", wd)
			return nil
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "synthetic", "Inspection mode: synthetic")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository in owner/name form (required)")
	cmd.Flags().StringVar(&environment, "environment", "", "Optional GitHub environment example")
	cmd.Flags().StringVar(&tokenAppProperty, "token-app-property", ghdisc.DefaultTokenAppProperty, "JWT claim used to map the token to a Conjur workload")
	cmd.Flags().StringVar(&enforcedClaims, "enforced-claims", "", "Comma-separated JWT claims to require in addition to token-app-property")

	return cmd
}

func printInspection(output ghdisc.ClaimAnalysis) {
	fmt.Printf("GitHub OIDC claim inspection (%s)\n", output.Mode)
	fmt.Printf("Repository: %s\n\n", output.Repository)
	fmt.Printf("Selected token_app_property: %s\n", output.SelectedClaims.TokenAppProperty)
	if len(output.SelectedClaims.EnforcedClaims) == 0 {
		fmt.Println("Selected enforced_claims: none")
	} else {
		fmt.Printf("Selected enforced_claims: %s\n", strings.Join(output.SelectedClaims.EnforcedClaims, ","))
	}
	fmt.Printf("\n%-18s %-17s %-12s %s\n", "Claim", "Classification", "Recommended", "Explanation")
	for _, claim := range output.AvailableClaims {
		recommended := "no"
		if claim.Recommended {
			recommended = "yes"
		}
		fmt.Printf("%-18s %-17s %-12s %s\n", claim.Name, claim.Classification, recommended, claim.Explanation)
	}
	fmt.Println("\nRecommended default: repository")
	for _, warning := range output.SecurityWarnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}
