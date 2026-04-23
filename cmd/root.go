package cmd

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/cmd/github"
	"github.com/cyberark/conjur-onboard/cmd/jenkins"
	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/cyberark/conjur-onboard/internal/appconfig"
	"github.com/spf13/cobra"
)

var (
	workDir        string
	configPath     string
	configExplicit bool
	nonInteractive bool
	dryRun         bool
	verbose        bool
)

var rootCmd = &cobra.Command{
	Use:   "conjur-onboard",
	Short: "Conjur Onboarding Toolkit - onboard CI/CD workloads to Conjur",
	Long: `Conjur Onboarding Toolkit (COT) helps you onboard CI/CD workloads to
CyberArk Secrets Manager SaaS or self-hosted Conjur by discovering platform
configuration, generating the required API calls, and applying them to your
Conjur endpoint.

Examples:
  conjur-onboard init
  conjur-onboard platforms
  conjur-onboard github discover --org acme-corp
  conjur-onboard github generate
  CONJUR_API_KEY=xxx conjur-onboard github apply`,
	Version: "0.1.0",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		configExplicit = cmd.Flags().Changed("config") || cmd.InheritedFlags().Changed("config")
	}

	rootCmd.PersistentFlags().StringVar(&workDir, "work-dir", "", "Directory for generated artifacts (overrides config)")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Global config file (default: "+appconfig.DefaultPath+" when present)")
	rootCmd.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false, "Suppress prompts; fail on missing values")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Print actions without executing")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose logging")

	flags := shared.GlobalFlags{
		WorkDir:        &workDir,
		ConfigPath:     &configPath,
		ConfigExplicit: &configExplicit,
		NonInteractive: &nonInteractive,
		DryRun:         &dryRun,
		Verbose:        &verbose,
	}

	rootCmd.AddCommand(newInitCmd(flags))
	rootCmd.AddCommand(newPlatformsCmd())

	githubCmd := github.NewGithubCmd(flags)
	githubCmd.Hidden = true
	rootCmd.AddCommand(githubCmd)

	jenkinsCmd := jenkins.NewJenkinsCmd(flags)
	jenkinsCmd.Hidden = true
	rootCmd.AddCommand(jenkinsCmd)
}

func newPlatformsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "platforms",
		Short: "List supported onboarding platforms",
		Long: `List supported platform commands.

Platform commands are kept out of the top-level help so the initial screen stays
focused as more integrations are added.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Supported platforms:")
			fmt.Println("  github    GitHub Actions via GitHub OIDC")
			fmt.Println("  jenkins   Jenkins via CyberArk Conjur Jenkins plugin")
			fmt.Println()
			fmt.Println("Example:")
			fmt.Println("  conjur-onboard github discover --org <owner>")
		},
	}
}
