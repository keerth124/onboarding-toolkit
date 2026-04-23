package appconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultPath = "conjur-onboard.json"
	Version     = "v1alpha1"
)

// Config contains global settings shared by platform commands.
type Config struct {
	Version string       `json:"version"`
	WorkDir string       `json:"work_dir,omitempty"`
	Conjur  ConjurConfig `json:"conjur"`
}

// ConjurConfig contains endpoint and tool-auth defaults. It intentionally does
// not include secrets; CONJUR_API_KEY remains environment-only.
type ConjurConfig struct {
	Target                string `json:"target,omitempty"`
	Tenant                string `json:"tenant,omitempty"`
	ConjurURL             string `json:"conjur_url,omitempty"`
	Account               string `json:"account,omitempty"`
	Username              string `json:"username,omitempty"`
	InsecureSkipTLSVerify bool   `json:"insecure_skip_tls_verify,omitempty"`
}

// Load reads a config file. Missing default config files are not an error;
// missing explicitly requested config files are.
func Load(path string, explicit bool) (Config, bool, error) {
	if path == "" {
		path = DefaultPath
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}

	var cfg Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, false, fmt.Errorf("validating %s: %w", path, err)
	}
	return cfg, true, nil
}

// Save writes a formatted config file.
func Save(path string, cfg Config, force bool) error {
	if path == "" {
		path = DefaultPath
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; pass --force to overwrite", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func (c Config) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}
	if c.Version != Version {
		return fmt.Errorf("unsupported version %q", c.Version)
	}
	return c.Conjur.Validate()
}

func (c ConjurConfig) Validate() error {
	target := strings.TrimSpace(c.Target)
	if target != "" && target != "saas" && target != "self-hosted" {
		return fmt.Errorf("conjur.target must be saas or self-hosted")
	}
	if c.Tenant != "" && c.ConjurURL != "" {
		return fmt.Errorf("set only one of conjur.tenant or conjur.conjur_url")
	}
	if target == "saas" && c.ConjurURL != "" {
		return fmt.Errorf("saas target uses conjur.tenant, not conjur.conjur_url")
	}
	if target == "self-hosted" && c.Tenant != "" {
		return fmt.Errorf("self-hosted target uses conjur.conjur_url, not conjur.tenant")
	}
	return nil
}
