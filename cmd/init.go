package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/cyberark/conjur-onboard/cmd/shared"
	"github.com/cyberark/conjur-onboard/internal/appconfig"
	"github.com/spf13/cobra"
)

func newInitCmd(flags shared.GlobalFlags) *cobra.Command {
	var output string
	var target string
	var tenant string
	var conjurURL string
	var account string
	var username string
	var insecureSkipTLSVerify bool
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a reusable conjur-onboard config file",
		Long: `Create a reusable config file for global Conjur settings.

After init, platform commands can use the configured endpoint, account,
username, and work directory without repeating those flags. Command-line flags
still override config values when provided.

Examples:
  conjur-onboard init
  conjur-onboard init --target saas --tenant my-tenant --work-dir ./cot-work
  conjur-onboard init --target self-hosted --conjur-url https://conjur.example.com --account myaccount`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)
			nonInteractive := flags.IsNonInteractive()

			if target == "" {
				switch {
				case tenant != "":
					target = "saas"
				case conjurURL != "":
					target = "self-hosted"
				case !nonInteractive:
					value, err := prompt(reader, "Conjur target [saas/self-hosted]", "saas", true)
					if err != nil {
						return err
					}
					target = value
				}
			}
			target = strings.ToLower(strings.TrimSpace(target))
			if target != "saas" && target != "self-hosted" {
				return fmt.Errorf("--target must be saas or self-hosted")
			}

			switch target {
			case "saas":
				if tenant == "" && !nonInteractive {
					value, err := prompt(reader, "Secrets Manager SaaS tenant subdomain", "", true)
					if err != nil {
						return err
					}
					tenant = value
				}
				if tenant == "" {
					return fmt.Errorf("--tenant is required for SaaS config")
				}
				conjurURL = ""
			case "self-hosted":
				if conjurURL == "" && !nonInteractive {
					value, err := prompt(reader, "Conjur URL", "", true)
					if err != nil {
						return err
					}
					conjurURL = value
				}
				if conjurURL == "" {
					return fmt.Errorf("--conjur-url is required for self-hosted config")
				}
				tenant = ""
			}

			if account == "" {
				account = "conjur"
			}
			if !cmd.Flags().Changed("account") && !nonInteractive {
				value, err := prompt(reader, "Conjur account", account, true)
				if err != nil {
					return err
				}
				account = value
			}

			if username == "" && !nonInteractive {
				value, err := prompt(reader, "Conjur username for validate/apply (optional)", "", false)
				if err != nil {
					return err
				}
				username = value
			}

			wd := ""
			if flags.WorkDir != nil {
				wd = strings.TrimSpace(*flags.WorkDir)
			}
			if wd == "" && !nonInteractive {
				value, err := prompt(reader, "Working directory", "conjur-onboard-work", true)
				if err != nil {
					return err
				}
				wd = value
			}
			if wd == "" {
				return fmt.Errorf("--work-dir is required in --non-interactive init")
			}

			path := output
			if path == "" {
				path = flags.ConfigPathValue()
			}
			cfg := appconfig.Config{
				Version: appconfig.Version,
				WorkDir: wd,
				Conjur: appconfig.ConjurConfig{
					Target:                target,
					Tenant:                strings.TrimSpace(tenant),
					ConjurURL:             strings.TrimSpace(conjurURL),
					Account:               strings.TrimSpace(account),
					Username:              strings.TrimSpace(username),
					InsecureSkipTLSVerify: insecureSkipTLSVerify,
				},
			}
			if err := appconfig.Save(path, cfg, force); err != nil {
				return err
			}

			fmt.Printf("Config written to %s\n", path)
			fmt.Printf("  Target   : %s\n", cfg.Conjur.Target)
			if cfg.Conjur.Target == "saas" {
				fmt.Printf("  Tenant   : %s\n", cfg.Conjur.Tenant)
			} else {
				fmt.Printf("  URL      : %s\n", cfg.Conjur.ConjurURL)
			}
			fmt.Printf("  Work dir : %s\n", cfg.WorkDir)
			fmt.Printf("\nNext: run a platform discover command, for example:\n")
			fmt.Printf("  conjur-onboard github discover --org <owner>\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&output, "output", "", "Config file path (default: --config or conjur-onboard.json)")
	cmd.Flags().StringVar(&target, "target", "", "Conjur target: saas or self-hosted")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Secrets Manager SaaS tenant subdomain")
	cmd.Flags().StringVar(&conjurURL, "conjur-url", "", "Full Conjur appliance URL for self-hosted")
	cmd.Flags().StringVar(&account, "account", "", "Conjur account name")
	cmd.Flags().StringVar(&username, "username", "", "Conjur username for validate/apply")
	cmd.Flags().BoolVar(&insecureSkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS certificate verification for Conjur connections")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing config file")

	return cmd
}

func prompt(reader *bufio.Reader, label string, defaultValue string, required bool) (string, error) {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", label, defaultValue)
	} else {
		fmt.Printf("%s: ", label)
	}
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = defaultValue
	}
	if required && value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
}
