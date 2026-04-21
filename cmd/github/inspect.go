package github

import (
	"fmt"
	"strings"

	"github.com/cyberark/conjur-onboard/internal/core"
	"github.com/spf13/cobra"
)

type inspectedClaim struct {
	Name           string `json:"name"`
	ExampleValue   string `json:"example_value"`
	Classification string `json:"classification"`
	Recommended    bool   `json:"recommended"`
	Explanation    string `json:"explanation"`
}

type inspectOutput struct {
	Platform        string           `json:"platform"`
	Mode            string           `json:"mode"`
	Repository      string           `json:"repository"`
	Recommended     []string         `json:"recommended"`
	Claims          []inspectedClaim `json:"claims"`
	SecurityWarnings []string         `json:"security_warnings"`
}

func newInspectCmd(sf *sharedFlags) *cobra.Command {
	var mode string
	var repo string
	var environment string

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect GitHub OIDC claims with security annotations",
		Long: `Inspect shows the GitHub Actions OIDC claims relevant to Conjur workload
identity selection. The first implementation supports synthetic inspection from
GitHub's documented claim shape.

Examples:
  conjur-onboard github inspect --repo acme-corp/api-service
  conjur-onboard github inspect --mode synthetic --repo acme-corp/api-service --environment production`,
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

			analysis := syntheticInspect(repo, environment)
			printInspection(analysis)

			wd, err := core.EnsureWorkDir(*sf.workDir)
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

	return cmd
}

func syntheticInspect(repo, environment string) inspectOutput {
	owner := strings.SplitN(repo, "/", 2)[0]
	envValue := environment
	if envValue == "" {
		envValue = "production"
	}

	return inspectOutput{
		Platform:    "github",
		Mode:        "synthetic",
		Repository:  repo,
		Recommended: []string{"repository", "environment"},
		Claims: []inspectedClaim{
			{
				Name:           "iss",
				ExampleValue:   "https://token.actions.githubusercontent.com",
				Classification: "metadata",
				Recommended:    false,
				Explanation:    "Issuer used by Conjur to verify the JWT source.",
			},
			{
				Name:           "aud",
				ExampleValue:   "conjur-cloud",
				Classification: "scope",
				Recommended:    false,
				Explanation:    "Audience expected by the Conjur authenticator.",
			},
			{
				Name:           "repository",
				ExampleValue:   repo,
				Classification: "identity-strong",
				Recommended:    true,
				Explanation:    "Recommended primary identity claim; binds access to one repository.",
			},
			{
				Name:           "repository_owner",
				ExampleValue:   owner,
				Classification: "identity-weak",
				Recommended:    false,
				Explanation:    "Too broad by itself because every repository in the owner scope can share it.",
			},
			{
				Name:           "environment",
				ExampleValue:   envValue,
				Classification: "scope",
				Recommended:    true,
				Explanation:    "Useful for protected deployment environments when paired with a compatible identity strategy.",
			},
			{
				Name:           "workflow_ref",
				ExampleValue:   repo + "/.github/workflows/deploy.yml@refs/heads/main",
				Classification: "scope",
				Recommended:    false,
				Explanation:    "Precise but brittle because workflow file moves and branch changes alter the claim.",
			},
			{
				Name:           "run_id",
				ExampleValue:   "1234567890",
				Classification: "ephemeral",
				Recommended:    false,
				Explanation:    "Changes every run and should not be used for stable workload identity.",
			},
		},
		SecurityWarnings: []string{
			"Selecting only repository_owner grants every repository in that owner scope the same workload identity.",
			"Environment scoping should be validated with a live token before production use.",
		},
	}
}

func printInspection(output inspectOutput) {
	fmt.Printf("GitHub OIDC claim inspection (%s)\n", output.Mode)
	fmt.Printf("Repository: %s\n\n", output.Repository)
	fmt.Printf("%-18s %-17s %-12s %s\n", "Claim", "Classification", "Recommended", "Explanation")
	for _, claim := range output.Claims {
		recommended := "no"
		if claim.Recommended {
			recommended = "yes"
		}
		fmt.Printf("%-18s %-17s %-12s %s\n", claim.Name, claim.Classification, recommended, claim.Explanation)
	}
	fmt.Println("\nRecommended defaults: repository, environment")
	for _, warning := range output.SecurityWarnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}
