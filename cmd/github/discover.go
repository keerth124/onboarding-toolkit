package github

import (
	"fmt"

	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/cyberark/conjur-onboard/internal/core"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
	"github.com/spf13/cobra"
)

func newDiscoverCmd(flags shared.GlobalFlags) *cobra.Command {
	var org string
	var token string
	var reposFromFile string

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Enumerate GitHub repos and detect OIDC configuration",
		Long: `Discover fetches non-archived repositories from the target GitHub owner,
lists environments per repo, and detects org-level sub-claim customization for
organization owners.

Output is written to discovery.json in the working directory.

Examples:
  conjur-onboard github discover --org acme-corp
  conjur-onboard github discover --org keerth124 --repos-from-file repos.txt
  conjur-onboard github discover --org acme-corp --repos-from-file repos.txt
  conjur-onboard github discover --org acme-corp --token ghp_xxx`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if org == "" {
				return fmt.Errorf("--org is required")
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

			cfg := ghdisc.DiscoverConfig{
				Org:       org,
				Token:     tok,
				RepoNames: repoNames,
				Verbose:   flags.IsVerbose(),
			}

			result, err := ghdisc.Discover(cmd.Context(), cfg)
			if err != nil {
				return fmt.Errorf("discovery failed: %w", err)
			}

			if err := core.WriteJSON(wd, "discovery.json", result); err != nil {
				return fmt.Errorf("writing discovery.json: %w", err)
			}

			fmt.Printf("Discovery complete: %d repos found for GitHub owner %q\n", len(result.Repos), org)
			fmt.Printf("Artifacts written to: %s/discovery.json\n", wd)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "GitHub organization or user owner name (required)")
	cmd.Flags().StringVar(&token, "token", "", "GitHub personal access token (or set GITHUB_TOKEN)")
	cmd.Flags().StringVar(&reposFromFile, "repos-from-file", "", "Optional file with one repo name or owner/name per line")

	return cmd
}
