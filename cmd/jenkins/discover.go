package jenkins

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/cyberark/conjur-onboard/internal/core"
	jenkinsdisc "github.com/cyberark/conjur-onboard/internal/jenkins"
	"github.com/spf13/cobra"
)

func newDiscoverCmd(flags shared.GlobalFlags) *cobra.Command {
	var jenkinsURL string
	var username string
	var token string
	var jobsFromFile string
	var maxDepth int

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover Jenkins credential scopes and JWT configuration",
		Long: `Discover derives Jenkins JWT issuer and JWKS URI, then inventories Jenkins
jobs and folders either from Jenkins Remote API or a jobs file.

The jobs file format is one Jenkins full name per line. An optional type can be
provided after a pipe:

  Folder/Team|folder
  Folder/Team/deploy|pipeline
  GlobalCredentials|global

Examples:
  conjur-onboard jenkins discover --url https://jenkins.example.com --jobs-from-file jobs.txt
  JENKINS_API_TOKEN=xxx conjur-onboard jenkins discover --url https://jenkins.example.com --username alice --max-depth 4`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jenkinsURL == "" {
				return fmt.Errorf("--url is required")
			}
			if token == "" {
				token = os.Getenv("JENKINS_API_TOKEN")
			}
			if username == "" {
				username = os.Getenv("JENKINS_USERNAME")
			}

			wd, err := flags.EnsureWorkDir(platformID)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			result, err := jenkinsdisc.Discover(cmd.Context(), jenkinsdisc.DiscoverConfig{
				JenkinsURL:   jenkinsURL,
				Username:     username,
				Token:        token,
				JobsFromFile: jobsFromFile,
				MaxDepth:     maxDepth,
				Verbose:      flags.IsVerbose(),
			})
			if err != nil {
				return fmt.Errorf("discovery failed: %w", err)
			}

			if err := core.WriteJSON(wd, "discovery.json", result); err != nil {
				return fmt.Errorf("writing discovery.json: %w", err)
			}
			fmt.Printf("Discovery complete: %d Jenkins resources found for %q\n", len(result.Jobs), result.JenkinsURL)
			fmt.Printf("JWKS URI: %s\n", result.JWKSURI)
			for _, warning := range result.Warnings {
				fmt.Printf("Warning: %s\n", warning)
			}
			fmt.Printf("Artifacts written to: %s/discovery.json\n", wd)
			return nil
		},
	}

	cmd.Flags().StringVar(&jenkinsURL, "url", "", "Jenkins controller URL (required)")
	cmd.Flags().StringVar(&username, "username", "", "Jenkins username (or set JENKINS_USERNAME)")
	cmd.Flags().StringVar(&token, "token", "", "Jenkins API token (or set JENKINS_API_TOKEN)")
	cmd.Flags().StringVar(&jobsFromFile, "jobs-from-file", "", "Optional file of Jenkins credential scope full names to onboard")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 6, "Maximum Jenkins folder/job depth to request from Remote API")

	return cmd
}
