// Package github implements the conjur-onboard github subcommands.
package github

import (
	"github.com/spf13/cobra"
)

// Shared flag values threaded through all subcommands.
type sharedFlags struct {
	workDir        *string
	nonInteractive *bool
	dryRun         *bool
	verbose        *bool
}

// NewGithubCmd constructs the `github` parent command and registers all subcommands.
func NewGithubCmd(workDir *string, nonInteractive *bool, dryRun *bool, verbose *bool) *cobra.Command {
	sf := &sharedFlags{
		workDir:        workDir,
		nonInteractive: nonInteractive,
		dryRun:         dryRun,
		verbose:        verbose,
	}

	cmd := &cobra.Command{
		Use:   "github",
		Short: "Onboard GitHub Actions workloads via GitHub OIDC",
		Long: `Onboard GitHub Actions workloads to Secrets Manager SaaS using GitHub's
built-in OIDC identity tokens.

Recommended flow:
  conjur-onboard github express --org <owner> --tenant <subdomain>

Step-by-step:
  conjur-onboard github discover --org <owner>
  conjur-onboard github inspect  --repo <owner>/<repo>
  conjur-onboard github generate --tenant <subdomain>
  conjur-onboard github validate --tenant <subdomain>
  conjur-onboard github apply    --tenant <subdomain>
  conjur-onboard github rollback --tenant <subdomain> --confirm`,
	}

	cmd.AddCommand(newDiscoverCmd(sf))
	cmd.AddCommand(newInspectCmd(sf))
	cmd.AddCommand(newGenerateCmd(sf))
	cmd.AddCommand(newValidateCmd(sf))
	cmd.AddCommand(newApplyCmd(sf))
	cmd.AddCommand(newRollbackCmd(sf))
	cmd.AddCommand(newExpressCmd(sf))

	return cmd
}
