package jenkins

import (
	"fmt"
	"strings"

	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/cyberark/conjur-onboard/internal/core"
	jenkinsdisc "github.com/cyberark/conjur-onboard/internal/jenkins"
	"github.com/spf13/cobra"
)

func newInspectCmd(flags shared.GlobalFlags) *cobra.Command {
	var mode string
	var job string
	var tokenAppProperty string
	var enforcedClaims string

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect Jenkins plugin JWT claims with security annotations",
		Long: `Inspect shows the Jenkins plugin JWT claims relevant to Conjur workload
identity selection. The first implementation supports synthetic inspection
from the CyberArk Conjur Jenkins plugin claim shape.

Examples:
  conjur-onboard jenkins inspect --job Folder/Team/deploy
  conjur-onboard jenkins inspect --job GlobalCredentials
  conjur-onboard jenkins inspect --job Folder/Team --token-app-property jenkins_full_name`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode != "synthetic" {
				return fmt.Errorf("only --mode synthetic is implemented for Jenkins")
			}
			if strings.TrimSpace(job) == "" {
				return fmt.Errorf("--job is required")
			}
			selection, err := jenkinsdisc.ParseClaimSelection(tokenAppProperty, enforcedClaims)
			if err != nil {
				return err
			}
			analysis := jenkinsdisc.BuildSyntheticClaimAnalysis(job, selection)
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
	cmd.Flags().StringVar(&job, "job", "", "Jenkins full name such as Folder/Team/deploy or GlobalCredentials (required)")
	cmd.Flags().StringVar(&tokenAppProperty, "token-app-property", jenkinsdisc.DefaultTokenAppProperty, "JWT claim used to map the token to a Conjur workload")
	cmd.Flags().StringVar(&enforcedClaims, "enforced-claims", "", "Comma-separated JWT claims to require in addition to token-app-property")

	return cmd
}

func printInspection(output jenkinsdisc.ClaimAnalysis) {
	fmt.Printf("Jenkins JWT claim inspection (%s)\n", output.Mode)
	fmt.Printf("Job full name: %s\n\n", output.JobFullName)
	fmt.Printf("Selected token_app_property: %s\n", output.SelectedClaims.TokenAppProperty)
	if len(output.SelectedClaims.EnforcedClaims) == 0 {
		fmt.Println("Selected enforced_claims: none")
	} else {
		fmt.Printf("Selected enforced_claims: %s\n", strings.Join(output.SelectedClaims.EnforcedClaims, ","))
	}
	fmt.Printf("\n%-26s %-17s %-12s %s\n", "Claim", "Classification", "Recommended", "Explanation")
	for _, claim := range output.AvailableClaims {
		recommended := "no"
		if claim.Recommended {
			recommended = "yes"
		}
		fmt.Printf("%-26s %-17s %-12s %s\n", claim.Name, claim.Classification, recommended, claim.Explanation)
	}
	fmt.Println("\nRecommended default: jenkins_full_name")
	for _, warning := range output.SecurityWarnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}
