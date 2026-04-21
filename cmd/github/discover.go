package github

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/internal/core"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
	"github.com/spf13/cobra"
)

func newDiscoverCmd(sf *sharedFlags) *cobra.Command {
	var org   string
	var token string

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Enumerate GitHub repos and detect OIDC configuration",
		Long: `Discover fetches all non-archived repos from the target GitHub org,
lists environments per repo, and detects org-level sub-claim customization.

Output is written to discovery.json in the working directory.

Examples:
  conjur-onboard github discover --org acme-corp
  conjur-onboard github discover --org acme-corp --token ghp_xxx`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if org == "" {
				return fmt.Errorf("--org is required")
			}

			tok := token
			if tok == "" {
				tok = os.Getenv("GITHUB_TOKEN")
			}
			if tok == "" {
				return fmt.Errorf("GitHub token required: pass --token or set GITHUB_TOKEN")
			}

			wd, err := core.EnsureWorkDir(*sf.workDir)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			cfg := ghdisc.DiscoverConfig{
				Org:     org,
				Token:   tok,
				Verbose: *sf.verbose,
			}

			result, err := ghdisc.Discover(cmd.Context(), cfg)
			if err != nil {
				return fmt.Errorf("discovery failed: %w", err)
			}

			if err := core.WriteJSON(wd, "discovery.json", result); err != nil {
				return fmt.Errorf("writing discovery.json: %w", err)
			}

			fmt.Printf("Discovery complete: %d repos found in org %q\n", len(result.Repos), org)
			fmt.Printf("Artifacts written to: %s/discovery.json\n", wd)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "GitHub organization name (required)")
	cmd.Flags().StringVar(&token, "token", "", "GitHub personal access token (or set GITHUB_TOKEN)")

	return cmd
}
