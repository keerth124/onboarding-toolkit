package jenkins

import (
	"fmt"

	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/cyberark/conjur-onboard/internal/conjur"
	jenkinsdisc "github.com/cyberark/conjur-onboard/internal/jenkins"
	"github.com/spf13/cobra"
)

func newGenerateCmd(flags shared.GlobalFlags) *cobra.Command {
	var tenant string
	var conjurURL string
	var conjurTarget string
	var audience string
	var provisioningMode string
	var authenticatorName string
	var createDisabled bool
	var include []string
	var exclude []string
	var includeTypes []string
	var all bool

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate Conjur API call artifacts from Jenkins discovery output",
		Long: `Generate reads discovery.json from the working directory and produces
Conjur API artifacts plus Jenkins integration guidance.

For large Jenkins controllers, generation requires an explicit workload
selection unless discovery used --jobs-from-file. Use --include, --exclude,
--include-type, or --all.

Examples:
  conjur-onboard jenkins generate --tenant myco
  conjur-onboard jenkins generate --tenant myco --include "Payments/**" --exclude "Payments/sandbox/**"
  conjur-onboard jenkins generate --tenant myco --include-type folder,multibranch
  conjur-onboard jenkins generate --conjur-url https://conjur.example.com --conjur-target self-hosted`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if tenant == "" && conjurURL == "" {
				return fmt.Errorf("--tenant or --conjur-url is required")
			}
			if tenant != "" && conjurURL != "" {
				return fmt.Errorf("use only one of --tenant or --conjur-url")
			}
			if conjurTarget == "" && conjurURL != "" {
				conjurTarget = "self-hosted"
			}
			wd, err := flags.EnsureWorkDir(platformID)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}
			disc, err := jenkinsdisc.LoadDiscovery(wd)
			if err != nil {
				return fmt.Errorf("loading discovery.json: %w (run 'discover' first)", err)
			}
			if audience == "" {
				audience = jenkinsdisc.DefaultAudience
			}

			gcfg, err := newJenkinsGenerateConfig(disc, jenkinsGenerateOptions{
				Tenant:            tenant,
				ConjurURL:         conjurURL,
				ConjurTarget:      conjurTarget,
				Audience:          audience,
				CreateEnabled:     !createDisabled,
				WorkDir:           wd,
				ProvisioningMode:  provisioningMode,
				AuthenticatorName: authenticatorName,
				Selection: jenkinsdisc.Selection{
					IncludePatterns: include,
					ExcludePatterns: exclude,
					IncludeTypes:    includeTypes,
					All:             all,
				},
				Verbose: flags.IsVerbose(),
				DryRun:  flags.IsDryRun(),
			})
			if err != nil {
				return err
			}

			plan, err := conjur.Generate(gcfg)
			if err != nil {
				return fmt.Errorf("generation failed: %w", err)
			}

			fmt.Printf("Generation complete\n")
			fmt.Printf("  Authenticator : %s\n", plan.AuthenticatorName)
			fmt.Printf("  Mode          : %s\n", provisioningMode)
			fmt.Printf("  Target        : %s\n", conjurTarget)
			fmt.Printf("  Workloads     : %d\n", plan.WorkloadCount)
			fmt.Printf("  Artifacts in  : %s/api/\n", wd)
			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Secrets Manager SaaS tenant subdomain")
	cmd.Flags().StringVar(&conjurURL, "conjur-url", "", "Full Conjur appliance URL for Enterprise or self-hosted")
	cmd.Flags().StringVar(&conjurTarget, "conjur-target", "", "Conjur target: saas or self-hosted")
	cmd.Flags().StringVar(&audience, "audience", jenkinsdisc.DefaultAudience, "JWT audience value")
	cmd.Flags().StringVar(&provisioningMode, "provisioning-mode", "bootstrap", "Provisioning mode: bootstrap or workloads-only")
	cmd.Flags().StringVar(&authenticatorName, "authenticator-name", "", "Existing authenticator name override for workloads-only mode")
	cmd.Flags().BoolVar(&createDisabled, "create-disabled", false, "Create authenticator in disabled state")
	cmd.Flags().StringSliceVar(&include, "include", nil, "Jenkins full-name glob to include; supports Folder/**")
	cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "Jenkins full-name glob to exclude; supports Folder/**")
	cmd.Flags().StringSliceVar(&includeTypes, "include-type", nil, "Only include resource types such as global,folder,multibranch,pipeline,job,scope")
	cmd.Flags().BoolVar(&all, "all", false, "Generate workloads for every discovered Jenkins resource")

	return cmd
}
