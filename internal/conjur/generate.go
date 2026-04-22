package conjur

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cyberark/conjur-onboard/internal/core"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
)

// GenerateConfig holds all inputs for artifact generation.
type GenerateConfig struct {
	Discovery         *ghdisc.DiscoveryResult
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

// Generate writes the GitHub JWT onboarding artifact set described by the PRD.
func Generate(cfg GenerateConfig) (*GenerateResult, error) {
	if cfg.Discovery == nil {
		return nil, fmt.Errorf("discovery is required")
	}
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
		cfg.Audience = "conjur-cloud"
	}
	if cfg.ProvisioningMode == "" {
		cfg.ProvisioningMode = "bootstrap"
	}
	if cfg.ProvisioningMode != "bootstrap" && cfg.ProvisioningMode != "workloads-only" {
		return nil, fmt.Errorf("unsupported provisioning mode %q", cfg.ProvisioningMode)
	}

	analysis, err := loadOrCreateClaimAnalysis(cfg)
	if err != nil {
		return nil, err
	}
	if err := ghdisc.ValidateGeneratorSupportedSelection(analysis.SelectedClaims); err != nil {
		return nil, fmt.Errorf("claims analysis selection is not supported by generation: %w", err)
	}

	authnName := resolvedAuthenticatorName(cfg)
	if cfg.ProvisioningMode == "bootstrap" {
		if _, err := writeAuthenticatorArtifact(cfg.Discovery, cfg, analysis.SelectedClaims, authnName); err != nil {
			return nil, err
		}
	} else if err := removeAuthenticatorArtifact(cfg.WorkDir); err != nil {
		return nil, err
	}

	hosts, err := writeWorkloadPolicyArtifact(cfg.Discovery, cfg, authnName, analysis.SelectedClaims)
	if err != nil {
		return nil, err
	}

	groupID := appsGroupID(authnName)
	if cfg.ConjurTarget == "self-hosted" {
		if err := removeGroupMembersArtifact(cfg.WorkDir); err != nil {
			return nil, err
		}
		if err := writeAuthenticatorGrantPolicyArtifact(authnName, groupID, hosts, cfg); err != nil {
			return nil, err
		}
	} else {
		var err error
		groupID, err = writeGroupMembersArtifact(authnName, hosts, cfg)
		if err != nil {
			return nil, err
		}
		if err := removeAuthenticatorGrantPolicyArtifact(cfg.WorkDir); err != nil {
			return nil, err
		}
	}

	plan := buildPlan(cfg, authnName, groupID, hosts)
	if err := core.WriteJSON(filepath.Join(cfg.WorkDir, "api"), "plan.json", plan); err != nil {
		return nil, fmt.Errorf("writing plan: %w", err)
	}

	if err := writeClaimsAnalysis(cfg, analysis); err != nil {
		return nil, err
	}
	if err := writeIntegrationArtifacts(cfg, authnName, hosts); err != nil {
		return nil, err
	}
	if err := writeNextSteps(cfg, authnName, groupID, len(hosts)); err != nil {
		return nil, err
	}
	if err := writeConfig(cfg, authnName, len(hosts)); err != nil {
		return nil, err
	}

	return &GenerateResult{
		AuthenticatorName: authnName,
		WorkloadCount:     len(hosts),
	}, nil
}

func buildPlan(cfg GenerateConfig, authnName string, groupID string, hosts []WorkloadHost) *core.Plan {
	ops := []core.Operation{}
	if cfg.ProvisioningMode == "bootstrap" {
		ops = append(ops, core.Operation{
			ID:             "create-authenticator",
			Description:    "Create GitHub Actions JWT authenticator",
			Method:         "POST",
			Path:           "/api/authenticators",
			BodyFile:       "api/01-create-authenticator.json",
			ContentType:    "application/json",
			ExpectedStatus: []int{200, 201},
			IdempotentOn:   []int{409},
			Metadata: map[string]string{
				"authenticator_name": authnName,
			},
		})
	}

	workloadIDs := make([]string, 0, len(hosts))
	for _, host := range hosts {
		workloadIDs = append(workloadIDs, host.FullPath)
	}
	ops = append(ops, core.Operation{
		ID:             "load-workload-policy",
		Description:    "Create GitHub workload identities under the authenticator identity path",
		Method:         "POST",
		Path:           "/policies/conjur/policy/root",
		BodyFile:       "api/02-workloads.yml",
		ContentType:    "application/x-yaml",
		ExpectedStatus: []int{200, 201},
		Metadata: map[string]string{
			"workload_ids": strings.Join(workloadIDs, ","),
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
				"membership_count":  fmt.Sprintf("%d", len(hosts)),
				"rollback_behavior": "manual-policy-review",
			},
		})
	} else {
		for i, h := range hosts {
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
					"workload_id": h.FullPath,
					"group_id":    groupID,
				},
			})
		}
	}

	return &core.Plan{
		Version:           "v1alpha1",
		Platform:          "github",
		Tenant:            cfg.Tenant,
		ConjurURL:         cfg.ConjurURL,
		ConjurTarget:      cfg.ConjurTarget,
		AuthenticatorType: "jwt",
		AuthenticatorName: authnName,
		ProvisioningMode:  cfg.ProvisioningMode,
		AppsGroupID:       groupID,
		IdentityPath:      identityPath(cfg.Discovery.Org),
		WorkloadCount:     len(hosts),
		Operations:        ops,
	}
}

func removeAuthenticatorArtifact(workDir string) error {
	path := filepath.Join(workDir, "api", "01-create-authenticator.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale authenticator artifact: %w", err)
	}
	return nil
}

func writeClaimsAnalysis(cfg GenerateConfig, analysis ghdisc.ClaimAnalysis) error {
	if err := core.WriteJSON(cfg.WorkDir, "claims-analysis.json", analysis); err != nil {
		return fmt.Errorf("writing claims analysis: %w", err)
	}
	return nil
}

func loadOrCreateClaimAnalysis(cfg GenerateConfig) (ghdisc.ClaimAnalysis, error) {
	analysis, found, err := ghdisc.LoadClaimAnalysis(cfg.WorkDir)
	if err != nil {
		return ghdisc.ClaimAnalysis{}, err
	}
	if !found {
		return ghdisc.BuildDefaultClaimAnalysis(cfg.Discovery), nil
	}
	if analysis.Platform != "" && analysis.Platform != "github" {
		return ghdisc.ClaimAnalysis{}, fmt.Errorf("claims-analysis.json is for platform %q, not github", analysis.Platform)
	}
	if analysis.SelectedClaims.TokenAppProperty == "" {
		analysis.SelectedClaims.TokenAppProperty = ghdisc.DefaultTokenAppProperty
	}
	return analysis, nil
}

func writeConfig(cfg GenerateConfig, authnName string, workloadCount int) error {
	config := fmt.Sprintf(`platform: github
org: %s
tenant: %s
conjur_url: %s
conjur_target: %s
workload_auth: jwt
provisioning_mode: %s
authenticator_name: %s
audience: %s
workload_count: %d
`, cfg.Discovery.Org, cfg.Tenant, cfg.ConjurURL, cfg.ConjurTarget, cfg.ProvisioningMode, authnName, cfg.Audience, workloadCount)

	if err := core.WriteText(cfg.WorkDir, "config.yml", config); err != nil {
		return fmt.Errorf("writing config.yml: %w", err)
	}
	return nil
}

func writeIntegrationArtifacts(cfg GenerateConfig, authnName string, hosts []WorkloadHost) error {
	conjurURL := cfg.ConjurURL
	if conjurURL == "" {
		conjurURL = tenantAPIURL(cfg.Tenant)
	}
	hostID := "data/github-apps/" + sanitizeName(cfg.Discovery.Org) + "/OWNER/REPO"
	repoName := "owner/repo"
	if len(hosts) > 0 {
		hostID = hosts[0].FullPath
		repoName = strings.TrimPrefix(hosts[0].HostID, cfg.Discovery.Org+"/")
	}

	workflow := fmt.Sprintf(`name: Deploy with Conjur

on:
  workflow_dispatch:
  push:
    branches:
      - main

permissions:
  contents: read
  id-token: write

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Fetch secrets from Conjur
        uses: cyberark/conjur-action@v2
        with:
          url: %s
          authn_id: %s
          host_id: %s
          secrets: |
            data/vault/example/safe/test-secret|TEST_SECRET

      - name: Use fetched secret
        run: ./deploy.sh
        env:
          TEST_SECRET: ${{ env.TEST_SECRET }}
`, conjurURL, authnName, hostID)

	readme := fmt.Sprintf(`# GitHub Actions Integration

This directory contains a starter workflow for a GitHub Actions workload using the generated Conjur JWT authenticator.

Generated values:

- Conjur API URL: `+"`%s`"+`
- Authenticator service ID: `+"`%s`"+`
- Example workload host ID: `+"`%s`"+`
- Example repository: `+"`%s`"+`

Before using the workflow, replace `+"`data/vault/example/safe/test-secret`"+` with a real variable path and grant the authenticator apps group access to the required safe.

The workflow must keep:

- `+"`permissions: id-token: write`"+`
- `+"`permissions: contents: read`"+`
- `+"`cyberark/conjur-action@v2`"+`
`, conjurURL, authnName, hostID, repoName)

	destDir := filepath.Join(cfg.WorkDir, "integration")
	if err := core.WriteText(destDir, "example-deploy.yml", workflow); err != nil {
		return fmt.Errorf("writing integration workflow: %w", err)
	}
	if err := core.WriteText(destDir, "README.md", readme); err != nil {
		return fmt.Errorf("writing integration README: %w", err)
	}
	return nil
}

func writeNextSteps(cfg GenerateConfig, authnName string, groupID string, workloadCount int) error {
	modeNote := "This plan creates the GitHub authenticator, workloads, and group memberships."
	if cfg.ProvisioningMode == "workloads-only" {
		modeNote = "This plan assumes the GitHub authenticator already exists and creates only workloads plus group memberships."
	}
	if cfg.ConjurTarget == "self-hosted" {
		modeNote += " Because the self-hosted target has no group membership REST endpoint, group access is granted by loading api/04-grant-authenticator-access.yml."
	}
	generateEndpoint := fmt.Sprintf("--tenant %s", cfg.Tenant)
	validateEndpoint := fmt.Sprintf("--tenant %s", cfg.Tenant)
	if cfg.ConjurTarget == "self-hosted" {
		generateEndpoint = fmt.Sprintf("--conjur-url %s --conjur-target self-hosted", cfg.ConjurURL)
		validateEndpoint = fmt.Sprintf("--conjur-url %s", cfg.ConjurURL)
	}
	next := fmt.Sprintf(`# Next Steps: GitHub Actions Onboarding

## Generated Summary

Platform: GitHub Actions

Conjur target: `+"`%s`"+`

Conjur API URL: `+"`%s`"+`

Authenticator type: `+"`jwt`"+`

Authenticator name: `+"`%s`"+`

Provisioning mode: `+"`%s`"+`

%s

Workload count: `+"`%d`"+`

Identity claim: `+"`repository`"+`

Enforced claims: none in the MVP generator

Apps group to grant to safes: `+"`conjur/authn-jwt/%s/apps`"+`

## 1. Review the Generated Plan

Command:

`+"```sh"+`
conjur-onboard github generate %s --work-dir %s
`+"```"+`

Expected outcome: `+"`api/plan.json`"+`, `+"`api/01-create-authenticator.json`"+`, `+"`api/02-workloads.yml`"+`, grant or membership artifacts, and `+"`integration/example-deploy.yml`"+` are present and reviewable.

## 2. Validate Against Conjur

Command:

`+"```sh"+`
CONJUR_API_KEY=<api-key> conjur-onboard github validate %s --username <username> --work-dir %s
`+"```"+`

Expected outcome: validation can read all generated bodies and reach the Conjur endpoint.

## 3. Apply the Plan

Command:

`+"```sh"+`
CONJUR_API_KEY=<api-key> conjur-onboard github apply %s --username <username> --work-dir %s
`+"```"+`

Expected outcome: the authenticator is created, workload policy is loaded, and `+"`%d`"+` workload memberships are added to `+"`%s`"+`.

## 4. Grant Safe Access

COT does not grant access to safes. Grant the generated apps group to the safe or policy that contains the secrets this workflow should read:

`+"```text"+`
conjur/authn-jwt/%s/apps
`+"```"+`

Expected outcome: workloads in the apps group can read only the secrets that security approves.

## 5. Verify End to End

Add the sample workflow from `+"`integration/example-deploy.yml`"+` to a test repository and keep the `+"`permissions: id-token: write`"+` block. Replace the example secret path with a known test secret.

Expected outcome: the workflow fetches the test secret and the deployment step receives it through the configured environment variable.

## Troubleshooting

- HTTP 401 during validate or apply: check `+"`CONJUR_API_KEY`"+` and the `+"`--username`"+` value.
- HTTP 403 during authenticator creation: the tool identity likely needs create privileges on the authenticator policy branch, typically through `+"`Authn_Admins`"+`.
- GitHub workflow cannot obtain an OIDC token: confirm `+"`permissions: id-token: write`"+` is present at workflow or job level.
- Host not found during secret fetch: confirm the workflow repository matches one of the generated workload IDs under `+"`%s`"+`.
- Secret not found or permission denied: grant the apps group access to the safe; COT intentionally does not generate safe grants.

## Known MVP Limitation

Synthetic claim analysis is generated from the documented GitHub OIDC schema. Live inspection and interactive claim selection are not implemented in this first GitHub slice.

Environment claims are recorded for review but not enforced by the MVP generator. Enforcing `+"`environment`"+` safely requires a compatible GitHub identity strategy so Conjur can map each token to the correct workload.
`, cfg.ConjurTarget, cfg.ConjurURL, authnName, cfg.ProvisioningMode, modeNote, workloadCount, authnName, generateEndpoint, cfg.WorkDir, validateEndpoint, cfg.WorkDir, validateEndpoint, cfg.WorkDir, workloadCount, groupID, authnName, identityPath(cfg.Discovery.Org))

	if err := core.WriteText(cfg.WorkDir, "NEXT_STEPS.md", next); err != nil {
		return fmt.Errorf("writing NEXT_STEPS.md: %w", err)
	}
	return nil
}

// sanitizeName returns a string safe for use as a Conjur resource name.
// Allowed characters: A-Z a-z 0-9 - _
var nonSafeRE = regexp.MustCompile(`[^A-Za-z0-9\-_]`)

func sanitizeName(s string) string {
	return nonSafeRE.ReplaceAllString(strings.ToLower(s), "-")
}

// authenticatorName builds the deterministic authenticator name from the org.
func authenticatorName(org string) string {
	return "github-" + sanitizeName(org)
}

// identityPath returns the policy branch where workloads live.
func identityPath(org string) string {
	return "data/github-apps/" + sanitizeName(org)
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

func resolvedAuthenticatorName(cfg GenerateConfig) string {
	if cfg.AuthenticatorName != "" {
		return sanitizeName(cfg.AuthenticatorName)
	}
	return authenticatorName(cfg.Discovery.Org)
}

// workloadID returns the workload host ID for a given repo (and optionally environment).
func workloadID(identPath, repoFullName string, env string) string {
	if env != "" {
		return identPath + "/" + repoFullName + "/" + env
	}
	return identPath + "/" + repoFullName
}
