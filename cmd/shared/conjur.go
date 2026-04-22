package shared

import (
	"fmt"

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

// AddConjurConnectionFlags registers the common Conjur connection flags.
func AddConjurConnectionFlags(cmd *cobra.Command, conn *ConjurConnectionFlags) {
	cmd.Flags().StringVar(&conn.Tenant, "tenant", "", "Secrets Manager SaaS tenant subdomain")
	cmd.Flags().StringVar(&conn.ConjurURL, "conjur-url", "", "Full Conjur appliance URL for Enterprise or self-hosted")
	cmd.Flags().StringVar(&conn.Account, "account", "conjur", "Conjur account name")
	cmd.Flags().StringVar(&conn.Username, "username", "", "Conjur username for authentication (required)")
	cmd.Flags().BoolVar(&conn.InsecureSkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS certificate verification for Conjur connections (insecure; local testing only)")
}
