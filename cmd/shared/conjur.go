package shared

import (
	"fmt"

	"github.com/cyberark/conjur-onboard/internal/appconfig"
	"github.com/cyberark/conjur-onboard/internal/conjur"
	"github.com/cyberark/conjur-onboard/internal/core"
	"github.com/spf13/cobra"
)

// ConjurConnectionFlags are shared Conjur endpoint and authentication flags.
type ConjurConnectionFlags struct {
	Tenant                string
	ConjurURL             string
	Account               string
	Username              string
	InsecureSkipTLSVerify bool
}

func (f ConjurConnectionFlags) ValidateEndpointRequired() error {
	if f.Tenant == "" && f.ConjurURL == "" {
		return fmt.Errorf("--tenant or --conjur-url is required")
	}
	if f.Tenant != "" && f.ConjurURL != "" {
		return fmt.Errorf("use only one of --tenant or --conjur-url")
	}
	return nil
}

func (f ConjurConnectionFlags) NewClient(apiKey string, verbose bool) (core.APIClient, error) {
	if err := f.ValidateEndpointRequired(); err != nil {
		return nil, err
	}
	if f.Username == "" {
		return nil, fmt.Errorf("--username is required")
	}
	return conjur.NewClientFromConfig(conjur.ClientConfig{
		Tenant:                f.Tenant,
		ConjurURL:             f.ConjurURL,
		Account:               f.Account,
		Username:              f.Username,
		APIKey:                apiKey,
		Verbose:               verbose,
		InsecureSkipTLSVerify: f.InsecureSkipTLSVerify,
	})
}

func ResolveConjurConnection(cmd *cobra.Command, flags GlobalFlags, conn *ConjurConnectionFlags) error {
	cfg, found, err := flags.LoadConfig()
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	resolveConjurValues(cmd, &conn.Tenant, &conn.ConjurURL, nil, &conn.Account, &conn.Username, &conn.InsecureSkipTLSVerify, cfg.Conjur)
	return nil
}

func ResolveConjurGenerate(cmd *cobra.Command, flags GlobalFlags, tenant *string, conjurURL *string, conjurTarget *string) error {
	cfg, found, err := flags.LoadConfig()
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	resolveConjurValues(cmd, tenant, conjurURL, conjurTarget, nil, nil, nil, cfg.Conjur)
	return nil
}

func ResolveConjurValues(cmd *cobra.Command, flags GlobalFlags, tenant *string, conjurURL *string, conjurTarget *string, account *string, username *string, insecureSkipTLSVerify *bool) error {
	cfg, found, err := flags.LoadConfig()
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	resolveConjurValues(cmd, tenant, conjurURL, conjurTarget, account, username, insecureSkipTLSVerify, cfg.Conjur)
	return nil
}

func resolveConjurValues(cmd *cobra.Command, tenant *string, conjurURL *string, conjurTarget *string, account *string, username *string, insecureSkipTLSVerify *bool, cfgConjur appconfig.ConjurConfig) {
	tenantChanged := cmd.Flags().Changed("tenant")
	urlChanged := cmd.Flags().Changed("conjur-url")
	targetChanged := cmd.Flags().Changed("conjur-target")

	if tenant != nil && tenantChanged && *tenant != "" {
		if conjurURL != nil && !urlChanged {
			*conjurURL = ""
		}
		if conjurTarget != nil && !targetChanged {
			*conjurTarget = "saas"
		}
	}
	if conjurURL != nil && urlChanged && *conjurURL != "" {
		if tenant != nil && !tenantChanged {
			*tenant = ""
		}
		if conjurTarget != nil && !targetChanged {
			*conjurTarget = "self-hosted"
		}
	}

	if tenant != nil && !tenantChanged && !urlChanged && *tenant == "" {
		*tenant = cfgConjur.Tenant
	}
	if conjurURL != nil && !urlChanged && !tenantChanged && *conjurURL == "" {
		*conjurURL = cfgConjur.ConjurURL
	}
	if conjurTarget != nil && !targetChanged && *conjurTarget == "" {
		*conjurTarget = cfgConjur.Target
	}
	if account != nil && !cmd.Flags().Changed("account") && cfgConjur.Account != "" {
		*account = cfgConjur.Account
	}
	if username != nil && !cmd.Flags().Changed("username") && *username == "" {
		*username = cfgConjur.Username
	}
	if insecureSkipTLSVerify != nil && !cmd.Flags().Changed("insecure-skip-tls-verify") && cfgConjur.InsecureSkipTLSVerify {
		*insecureSkipTLSVerify = true
	}
}

// AddConjurConnectionFlags registers the common Conjur connection flags.
func AddConjurConnectionFlags(cmd *cobra.Command, conn *ConjurConnectionFlags) {
	cmd.Flags().StringVar(&conn.Tenant, "tenant", "", "Override SaaS tenant subdomain from config")
	cmd.Flags().StringVar(&conn.ConjurURL, "conjur-url", "", "Override self-hosted Conjur URL from config")
	cmd.Flags().StringVar(&conn.Account, "account", "conjur", "Override Conjur account name from config")
	cmd.Flags().StringVar(&conn.Username, "username", "", "Override Conjur username from config")
	cmd.Flags().BoolVar(&conn.InsecureSkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS certificate verification for Conjur connections")
}
