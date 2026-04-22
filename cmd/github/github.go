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
  conjur-onboard github express --org <owner> --tenant <subdomain>
  conjur-onboard github express --org <owner> --conjur-url <appliance-url>

Step-by-step:
  conjur-onboard github discover --org <owner>
  conjur-onboard github inspect  --repo <owner>/<repo>
  conjur-onboard github generate --tenant <subdomain>
  conjur-onboard github generate --conjur-url <appliance-url> --conjur-target self-hosted
  conjur-onboard github validate --tenant <subdomain>
  conjur-onboard github validate --conjur-url <appliance-url>
  conjur-onboard github apply    --tenant <subdomain>
  conjur-onboard github apply    --conjur-url <appliance-url>
  conjur-onboard github rollback --tenant <subdomain> --confirm
  conjur-onboard github rollback --conjur-url <appliance-url> --confirm`,
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
  CONJUR_API_KEY=xxx conjur-onboard github apply --tenant myco --username admin
  CONJUR_API_KEY=xxx conjur-onboard github apply --conjur-url https://conjur.example.com --username admin --account myaccount`,
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
  CONJUR_API_KEY=xxx conjur-onboard github validate --tenant myco --username admin
  CONJUR_API_KEY=xxx conjur-onboard github validate --conjur-url https://conjur.example.com --username admin --account myaccount`,
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
  CONJUR_API_KEY=xxx conjur-onboard github rollback --tenant myco --username admin --confirm
  CONJUR_API_KEY=xxx conjur-onboard github rollback --conjur-url https://conjur.example.com --username admin --account myaccount --confirm
  conjur-onboard github rollback --tenant myco --dry-run`,
	})
}
