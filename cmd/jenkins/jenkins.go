// Package jenkins implements the conjur-onboard jenkins subcommands.
package jenkins

import (
	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/spf13/cobra"
)

const platformID = "jenkins"

func NewJenkinsCmd(flags shared.GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jenkins",
		Short: "Onboard Jenkins workloads via the CyberArk Conjur Jenkins plugin",
		Long: `Onboard Jenkins credential scopes to Conjur using JWTs issued by the
CyberArk Conjur Jenkins plugin.

Recommended flow:
  conjur-onboard init
  conjur-onboard jenkins discover --url <jenkins-url> --jobs-from-file jobs.txt
  conjur-onboard jenkins generate

Step-by-step:
  conjur-onboard jenkins discover --url <jenkins-url> --jobs-from-file jobs.txt
  conjur-onboard jenkins inspect --job <folder/job>
  conjur-onboard jenkins generate
  conjur-onboard jenkins validate
  conjur-onboard jenkins apply
  conjur-onboard jenkins rollback --confirm`,
	}

	cmd.AddCommand(newDiscoverCmd(flags))
	cmd.AddCommand(newInspectCmd(flags))
	cmd.AddCommand(newGenerateCmd(flags))
	cmd.AddCommand(newValidateCmd(flags))
	cmd.AddCommand(newApplyCmd(flags))
	cmd.AddCommand(newRollbackCmd(flags))

	return cmd
}

func newApplyCmd(flags shared.GlobalFlags) *cobra.Command {
	return shared.NewApplyCmd(flags, shared.ApplyCommandOptions{
		PlatformID:      platformID,
		RollbackCommand: "conjur-onboard jenkins rollback",
		Long: `Apply reads plan.json from the working directory and executes the API calls
against the target Conjur endpoint in order.

Authentication uses the CONJUR_API_KEY environment variable.

Examples:
  CONJUR_API_KEY=xxx conjur-onboard jenkins apply
  CONJUR_API_KEY=xxx conjur-onboard jenkins apply --username host/data/jenkins-apps/tooling`,
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
  CONJUR_API_KEY=xxx conjur-onboard jenkins validate
  CONJUR_API_KEY=xxx conjur-onboard jenkins validate --username host/data/jenkins-apps/tooling`,
	})
}

func newRollbackCmd(flags shared.GlobalFlags) *cobra.Command {
	return shared.NewRollbackCmd(flags, shared.RollbackCommandOptions{
		PlatformID: platformID,
		Long: `Rollback reads apply-log.json and api/plan.json from the working directory,
then runs inverse operations in reverse order.

Rollback requires --confirm unless --dry-run is set.

Examples:
  CONJUR_API_KEY=xxx conjur-onboard jenkins rollback --confirm
  conjur-onboard jenkins rollback --dry-run`,
	})
}
