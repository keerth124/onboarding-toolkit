package github

import (
	"fmt"

	"github.com/cyberark/conjur-onboard/internal/conjur"
	"github.com/cyberark/conjur-onboard/internal/core"
)

type conjurConnectionFlags struct {
	tenant    string
	conjurURL string
	account   string
	username  string
}

func (f conjurConnectionFlags) validateEndpointRequired() error {
	if f.tenant == "" && f.conjurURL == "" {
		return fmt.Errorf("--tenant or --conjur-url is required")
	}
	if f.tenant != "" && f.conjurURL != "" {
		return fmt.Errorf("use only one of --tenant or --conjur-url")
	}
	return nil
}

func newConjurClient(f conjurConnectionFlags, apiKey string, verbose bool) (core.APIClient, error) {
	if err := f.validateEndpointRequired(); err != nil {
		return nil, err
	}
	if f.username == "" {
		return nil, fmt.Errorf("--username is required")
	}
	return conjur.NewClientFromConfig(conjur.ClientConfig{
		Tenant:    f.tenant,
		ConjurURL: f.conjurURL,
		Account:   f.account,
		Username:  f.username,
		APIKey:    apiKey,
		Verbose:   verbose,
	})
}
