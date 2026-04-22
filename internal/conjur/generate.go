package conjur

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cyberark/conjur-onboard/internal/core"
	"github.com/cyberark/conjur-onboard/internal/platform"
)

// GenerateConfig holds platform-neutral inputs for Conjur artifact generation.
type GenerateConfig struct {
	Platform              platform.Descriptor
	Discovery             *platform.Discovery
	Claims                platform.ClaimAnalysis
	ClaimAnalysisArtifact any
	Authenticator         platform.Authenticator
	Workloads             []platform.Workload
	IntegrationArtifacts  []platform.IntegrationArtifact
	NextSteps             string
	ConfigYAML            string

	Tenant            string
	ConjurURL         string
	ConjurTarget      string
	Audience          string
	CreateEnabled     bool
	WorkDir           string
	ProvisioningMode  string
	AuthenticatorName string
	Verbose           bool
	DryRun            bool
}

// GenerateResult summarizes what was generated.
type GenerateResult struct {
	AuthenticatorName string
	WorkloadCount     int
}

// Generate writes a JWT onboarding artifact set from platform-neutral inputs.
func Generate(cfg GenerateConfig) (*GenerateResult, error) {
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("work directory is required")
	}
	if cfg.Tenant == "" && cfg.ConjurURL == "" {
		return nil, fmt.Errorf("tenant or conjur URL is required")
	}
	if cfg.ConjurURL == "" {
		cfg.ConjurURL = tenantAPIURL(cfg.Tenant)
	} else {
		_, apiURL, err := normalizeConjurURL(cfg.ConjurURL)
		if err != nil {
			return nil, err
		}
		cfg.ConjurURL = apiURL
	}
	if cfg.ConjurTarget == "" {
		cfg.ConjurTarget = "saas"
	}
	if cfg.ConjurTarget != "saas" && cfg.ConjurTarget != "self-hosted" {
		return nil, fmt.Errorf("unsupported Conjur target %q", cfg.ConjurTarget)
	}
	if cfg.ConjurTarget == "self-hosted" && cfg.ConjurURL == "" {
		return nil, fmt.Errorf("conjur URL is required for self-hosted target")
	}
	if cfg.Tenant == "" {
		cfg.Tenant = tenantNameFromURL(cfg.ConjurURL)
	}
	if cfg.Audience == "" {
		cfg.Audience = firstNonEmpty(cfg.Authenticator.Audience, "conjur-cloud")
	}
	if cfg.ProvisioningMode == "" {
		cfg.ProvisioningMode = "bootstrap"
	}
	if cfg.ProvisioningMode != "bootstrap" && cfg.ProvisioningMode != "workloads-only" {
		return nil, fmt.Errorf("unsupported provisioning mode %q", cfg.ProvisioningMode)
	}

	authn, err := resolvedAuthenticator(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.ConjurTarget == "self-hosted" {
		authn.Subtype = ""
	}
	if cfg.Platform.ID == "" && cfg.Discovery != nil {
		cfg.Platform = cfg.Discovery.Platform
	}
	if err := cfg.Platform.Validate(); err != nil {
		return nil, err
	}
	if len(cfg.Workloads) == 0 {
		return nil, fmt.Errorf("at least one workload is required")
	}

	if cfg.ProvisioningMode == "bootstrap" {
		if _, err := writeAuthenticatorArtifact(authn, cfg); err != nil {
			return nil, err
		}
	} else if err := removeAuthenticatorArtifact(cfg.WorkDir); err != nil {
		return nil, err
	}

	if err := writeWorkloadPolicyArtifact(authn.IdentityPath, cfg.Workloads, cfg); err != nil {
		return nil, err
	}

	groupID := appsGroupID(authn.Name)
	if cfg.ConjurTarget == "self-hosted" {
		if err := removeGroupMembersArtifact(cfg.WorkDir); err != nil {
			return nil, err
		}
		if err := writeAuthenticatorGrantPolicyArtifact(authn.Name, groupID, cfg.Workloads, cfg); err != nil {
			return nil, err
		}
	} else {
		var err error
		groupID, err = writeGroupMembersArtifact(authn.Name, cfg.Workloads, cfg)
		if err != nil {
			return nil, err
		}
		if err := removeAuthenticatorGrantPolicyArtifact(cfg.WorkDir); err != nil {
			return nil, err
		}
	}

	plan := buildPlan(cfg, authn, groupID)
	if err := core.WriteJSON(filepath.Join(cfg.WorkDir, "api"), "plan.json", plan); err != nil {
		return nil, fmt.Errorf("writing plan: %w", err)
	}

	if err := writeClaimsAnalysis(cfg); err != nil {
		return nil, err
	}
	if err := writeIntegrationArtifacts(cfg); err != nil {
		return nil, err
	}
	if cfg.NextSteps != "" {
		if err := core.WriteText(cfg.WorkDir, "NEXT_STEPS.md", cfg.NextSteps); err != nil {
			return nil, fmt.Errorf("writing NEXT_STEPS.md: %w", err)
		}
	}
	if cfg.ConfigYAML != "" {
		if err := core.WriteText(cfg.WorkDir, "config.yml", cfg.ConfigYAML); err != nil {
			return nil, fmt.Errorf("writing config.yml: %w", err)
		}
	}

	return &GenerateResult{
		AuthenticatorName: authn.Name,
		WorkloadCount:     len(cfg.Workloads),
	}, nil
}

func resolvedAuthenticator(cfg GenerateConfig) (platform.Authenticator, error) {
	authn := cfg.Authenticator
	if cfg.AuthenticatorName != "" {
		authn.Name = sanitizeName(cfg.AuthenticatorName)
	}
	if authn.Name == "" {
		return platform.Authenticator{}, fmt.Errorf("authenticator name is required")
	}
	if authn.Type == "" {
		return platform.Authenticator{}, fmt.Errorf("authenticator type is required")
	}
	authn.Enabled = cfg.CreateEnabled
	authn.Audience = cfg.Audience

	if authn.Issuer == "" && cfg.Discovery != nil {
		authn.Issuer = cfg.Discovery.OIDCProvider.Issuer
	}
	if authn.JWKSURI == "" && cfg.Discovery != nil {
		authn.JWKSURI = cfg.Discovery.OIDCProvider.JWKSURI
	}
	if authn.TokenAppProperty == "" {
		authn.TokenAppProperty = cfg.Claims.SelectedClaims.TokenAppProperty
	}
	if len(authn.EnforcedClaims) == 0 {
		authn.EnforcedClaims = cfg.Claims.SelectedClaims.EnforcedClaims
	}

	if authn.Type == "jwt" {
		if authn.Issuer == "" {
			return platform.Authenticator{}, fmt.Errorf("JWT authenticator issuer is required")
		}
		if authn.JWKSURI == "" {
			return platform.Authenticator{}, fmt.Errorf("JWT authenticator JWKS URI is required")
		}
		if authn.Audience == "" {
			return platform.Authenticator{}, fmt.Errorf("JWT authenticator audience is required")
		}
		if authn.IdentityPath == "" {
			return platform.Authenticator{}, fmt.Errorf("JWT authenticator identity path is required")
		}
		if authn.TokenAppProperty == "" {
			return platform.Authenticator{}, fmt.Errorf("JWT authenticator token_app_property is required")
		}
	}

	return authn, nil
}

func buildPlan(cfg GenerateConfig, authn platform.Authenticator, groupID string) *core.Plan {
	platformID := cfg.Platform.ID
	platformName := cfg.Platform.DisplayName
	if platformName == "" {
		platformName = platformID
	}

	ops := []core.Operation{}
	if cfg.ProvisioningMode == "bootstrap" {
		ops = append(ops, core.Operation{
			ID:             "create-authenticator",
			Description:    fmt.Sprintf("Create %s %s authenticator", platformName, strings.ToUpper(authn.Type)),
			Method:         "POST",
			Path:           createAuthenticatorPath(cfg.ConjurTarget),
			BodyFile:       "api/01-create-authenticator.json",
			ContentType:    "application/json",
			ExpectedStatus: []int{200, 201},
			IdempotentOn:   []int{409},
			Metadata: map[string]string{
				"authenticator_name": authn.Name,
				"rollback_kind":      "authenticator",
			},
		})
	}

	workloadIDs := make([]string, 0, len(cfg.Workloads))
	for _, workload := range cfg.Workloads {
		workloadIDs = append(workloadIDs, workload.FullPath)
	}
	ops = append(ops, core.Operation{
		ID:             "load-workload-policy",
		Description:    fmt.Sprintf("Create %s workload identities under the authenticator identity path", platformName),
		Method:         "POST",
		Path:           "/policies/conjur/policy/root",
		BodyFile:       "api/02-workloads.yml",
		ContentType:    "application/x-yaml",
		ExpectedStatus: []int{200, 201},
		Metadata: map[string]string{
			"rollback_kind": "workload-policy",
			"workload_ids":  strings.Join(workloadIDs, ","),
		},
	})

	if cfg.ConjurTarget == "self-hosted" {
		ops = append(ops, core.Operation{
			ID:             "load-authenticator-grants",
			Description:    "Grant generated workloads to authenticator apps group using policy load",
			Method:         "POST",
			Path:           "/policies/conjur/policy/root",
			BodyFile:       "api/04-grant-authenticator-access.yml",
			ContentType:    "application/x-yaml",
			ExpectedStatus: []int{200, 201},
			Metadata: map[string]string{
				"group_id":          groupID,
				"membership_count":  fmt.Sprintf("%d", len(cfg.Workloads)),
				"rollback_behavior": "manual-policy-review",
				"rollback_kind":     "manual-policy-review",
			},
		})
	} else {
		for i, workload := range cfg.Workloads {
			ops = append(ops, core.Operation{
				ID:             fmt.Sprintf("add-group-member-%03d", i+1),
				Description:    "Add workload to authenticator apps group",
				Method:         "POST",
				Path:           "/api/groups/" + groupID + "/members",
				BodyFile:       "api/03-add-group-members.jsonl",
				BodyLine:       i + 1,
				ContentType:    "application/json",
				ExpectedStatus: []int{200, 201, 204},
				IdempotentOn:   []int{409},
				Metadata: map[string]string{
					"rollback_kind": "group-member",
					"workload_id":   workload.FullPath,
					"group_id":      groupID,
					"member_kind":   "workload",
				},
			})
		}
	}

	return &core.Plan{
		Version:              "v1alpha1",
		Platform:             platformID,
		Tenant:               cfg.Tenant,
		ConjurURL:            cfg.ConjurURL,
		ConjurTarget:         cfg.ConjurTarget,
		AuthenticatorType:    authn.Type,
		AuthenticatorSubtype: authn.Subtype,
		AuthenticatorName:    authn.Name,
		ProvisioningMode:     cfg.ProvisioningMode,
		AppsGroupID:          groupID,
		IdentityPath:         authn.IdentityPath,
		WorkloadCount:        len(cfg.Workloads),
		Operations:           ops,
	}
}

func createAuthenticatorPath(target string) string {
	if target == "self-hosted" {
		return "/authenticators/{account}"
	}
	return "/api/authenticators"
}

func removeAuthenticatorArtifact(workDir string) error {
	path := filepath.Join(workDir, "api", "01-create-authenticator.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale authenticator artifact: %w", err)
	}
	return nil
}

func writeClaimsAnalysis(cfg GenerateConfig) error {
	artifact := cfg.ClaimAnalysisArtifact
	if artifact == nil {
		artifact = cfg.Claims
	}
	if err := core.WriteJSON(cfg.WorkDir, "claims-analysis.json", artifact); err != nil {
		return fmt.Errorf("writing claims analysis: %w", err)
	}
	return nil
}

func writeIntegrationArtifacts(cfg GenerateConfig) error {
	for _, artifact := range cfg.IntegrationArtifacts {
		if artifact.Path == "" {
			return fmt.Errorf("integration artifact path is required")
		}
		path := filepath.FromSlash(artifact.Path)
		destDir := filepath.Join(cfg.WorkDir, filepath.Dir(path))
		name := filepath.Base(path)
		if err := core.WriteText(destDir, name, artifact.Content); err != nil {
			return fmt.Errorf("writing integration artifact %s: %w", artifact.Path, err)
		}
	}
	return nil
}

// sanitizeName returns a string safe for use as a Conjur resource name.
// Allowed characters: A-Z a-z 0-9 - _
var nonSafeRE = regexp.MustCompile(`[^A-Za-z0-9\-_]`)

func sanitizeName(s string) string {
	return nonSafeRE.ReplaceAllString(strings.ToLower(s), "-")
}

// appsGroupID returns the URL-encoded apps group identifier.
func appsGroupID(authnName string) string {
	raw := fmt.Sprintf("conjur/authn-jwt/%s/apps", authnName)
	// URL-encode the slashes for use in the API path.
	return strings.ReplaceAll(raw, "/", "%2F")
}

func tenantAPIURL(tenant string) string {
	return tenantAPIBaseURL(tenant)
}

func tenantNameFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.Split(parsed.Host, ".")[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
