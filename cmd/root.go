package cmd

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/cmd/github"
	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/spf13/cobra"
)

var (
	workDir        string
	nonInteractive bool
	dryRun         bool
	verbose        bool
)

var rootCmd = &cobra.Command{
	Use:   "conjur-onboard",
	Short: "Conjur Onboarding Toolkit - onboard CI/CD workloads to Conjur",
	Long: `Conjur Onboarding Toolkit (COT) helps you onboard CI/CD workloads to
CyberArk Secrets Manager SaaS, Conjur Enterprise, or Secrets Manager Self-Hosted
by discovering your platform configuration, generating the required API calls,
and applying them to your Conjur endpoint.

Platforms:
  github       GitHub Actions via GitHub OIDC

Examples:
  conjur-onboard github express --org acme-corp --tenant myco
  conjur-onboard github express --org acme-corp --conjur-url https://conjur.example.com

  conjur-onboard github discover --org acme-corp
  conjur-onboard github inspect --repo acme-corp/api-service
  conjur-onboard github generate --tenant myco
  conjur-onboard github generate --conjur-url https://conjur.example.com --conjur-target self-hosted
  conjur-onboard github validate --tenant myco --username admin
  conjur-onboard github apply --conjur-url https://conjur.example.com --username admin --account conjur`,
	Version: "0.1.0",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&workDir, "work-dir", "", "Directory for generated artifacts (default: conjur-onboard-<platform>-<timestamp>)")
	rootCmd.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false, "Suppress prompts; fail on missing values")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Print actions without executing")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose logging")

	githubCmd := github.NewGithubCmd(shared.GlobalFlags{
		WorkDir:        &workDir,
		NonInteractive: &nonInteractive,
		DryRun:         &dryRun,
		Verbose:        &verbose,
	})
	rootCmd.AddCommand(githubCmd)
}
