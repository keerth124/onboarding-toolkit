// Package github implements the conjur-onboard github subcommands.
package github

import (
	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/spf13/cobra"
)

const platformID = "github"

// NewGithubCmd constructs the `github` parent command and registers all subcommands.
func NewGithubCmd(flags shared.GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "github",
		Short: "Onboard GitHub Actions workloads via GitHub OIDC",
		Long: `Onboard GitHub Actions workloads to Conjur using GitHub's built-in OIDC
identity tokens.

Recommended flow:
  conjur-onboard init
  conjur-onboard github express --org <owner>

Step-by-step:
  conjur-onboard github discover --org <owner>
  conjur-onboard github inspect  --repo <owner>/<repo>
  conjur-onboard github generate
  conjur-onboard github validate
  conjur-onboard github apply
  conjur-onboard github rollback --confirm`,
	}

	cmd.AddCommand(newDiscoverCmd(flags))
	cmd.AddCommand(newInspectCmd(flags))
	cmd.AddCommand(newGenerateCmd(flags))
	cmd.AddCommand(newValidateCmd(flags))
	cmd.AddCommand(newApplyCmd(flags))
	cmd.AddCommand(newRollbackCmd(flags))
	cmd.AddCommand(newExpressCmd(flags))

	return cmd
}

func newApplyCmd(flags shared.GlobalFlags) *cobra.Command {
	return shared.NewApplyCmd(flags, shared.ApplyCommandOptions{
		PlatformID:      platformID,
		RollbackCommand: "conjur-onboard github rollback",
		Long: `Apply reads plan.json from the working directory and executes the API calls
against the target Conjur endpoint in order:
  1. Create authenticator
  2. Load workload policy
  3. Add workloads to the authenticator's apps group, either by REST endpoint
     for SaaS or policy grant load for self-hosted targets

Authentication uses the CONJUR_API_KEY environment variable (never a CLI flag).

On partial failure, apply stops at the first error and prints the rollback command.
Re-running apply against an already-applied state is safe (idempotent).

Examples:
  CONJUR_API_KEY=xxx conjur-onboard github apply
  CONJUR_API_KEY=xxx conjur-onboard github apply --username host/data/github-apps/acme/tooling`,
	})
}

func newValidateCmd(flags shared.GlobalFlags) *cobra.Command {
	return shared.NewValidateCmd(flags, shared.ValidateCommandOptions{
		PlatformID: platformID,
		Long: `Validate reads api/plan.json from the working directory, verifies that all
referenced request bodies are readable, and checks that the target Conjur
endpoint is reachable with the provided tool-auth credentials.

Authentication uses the CONJUR_API_KEY environment variable.

Examples:
  CONJUR_API_KEY=xxx conjur-onboard github validate
  CONJUR_API_KEY=xxx conjur-onboard github validate --username host/data/github-apps/acme/tooling`,
	})
}

func newRollbackCmd(flags shared.GlobalFlags) *cobra.Command {
	return shared.NewRollbackCmd(flags, shared.RollbackCommandOptions{
		PlatformID: platformID,
		Long: `Rollback reads apply-log.json and api/plan.json from the working directory,
then runs inverse operations in reverse order.

Rollback removes workloads from the authenticator apps group, deletes generated
workloads, and deletes the authenticator only when this plan created it.

Rollback requires --confirm unless --dry-run is set.

Examples:
  CONJUR_API_KEY=xxx conjur-onboard github rollback --confirm
  conjur-onboard github rollback --dry-run`,
	})
}
