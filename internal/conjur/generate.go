package conjur

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cyberark/conjur-onboard/internal/core"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
)

// GenerateConfig holds all inputs for artifact generation.
type GenerateConfig struct {
	Discovery     *ghdisc.DiscoveryResult
	Tenant        string
	Audience      string
	CreateEnabled bool
	WorkDir       string
	Verbose       bool
	DryRun        bool
}

// GenerateResult summarizes what was generated.
type GenerateResult struct {
	AuthenticatorName string
	WorkloadCount     int
}

type claimAnalysis struct {
	Platform          string          `json:"platform"`
	Mode              string          `json:"mode"`
	SelectedClaims    selectedClaims  `json:"selected_claims"`
	AvailableClaims   []claimRecord   `json:"available_claims"`
	SecurityNotes      []string        `json:"security_notes"`
	ImplementationNote string          `json:"implementation_note,omitempty"`
}

type selectedClaims struct {
	TokenAppProperty string   `json:"token_app_property"`
	EnforcedClaims   []string `json:"enforced_claims"`
}

type claimRecord struct {
	Name           string `json:"name"`
	Classification string `json:"classification"`
	Recommended    bool   `json:"recommended"`
	Explanation    string `json:"explanation"`
}

// Generate writes the GitHub JWT onboarding artifact set described by the PRD.
func Generate(cfg GenerateConfig) (*GenerateResult, error) {
	if cfg.Discovery == nil {
		return nil, fmt.Errorf("discovery is required")
	}
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("work directory is required")
	}
	if cfg.Tenant == "" {
		return nil, fmt.Errorf("tenant is required")
	}
	if cfg.Audience == "" {
		cfg.Audience = "conjur-cloud"
	}

	authnBody, err := writeAuthenticatorArtifact(cfg.Discovery, cfg)
	if err != nil {
		return nil, err
	}

	hosts, err := writeWorkloadPolicyArtifact(cfg.Discovery, cfg)
	if err != nil {
		return nil, err
	}

	groupID, err := writeGroupMembersArtifact(authnBody.Name, hosts, cfg)
	if err != nil {
		return nil, err
	}

	plan := buildPlan(cfg, authnBody.Name, groupID, hosts)
	if err := core.WriteJSON(filepath.Join(cfg.WorkDir, "api"), "plan.json", plan); err != nil {
		return nil, fmt.Errorf("writing plan: %w", err)
	}

	if err := writeClaimsAnalysis(cfg, authnBody); err != nil {
		return nil, err
	}
	if err := writeIntegrationArtifacts(cfg, authnBody.Name, hosts); err != nil {
		return nil, err
	}
	if err := writeNextSteps(cfg, authnBody.Name, groupID, len(hosts)); err != nil {
		return nil, err
	}
	if err := writeConfig(cfg, authnBody.Name, len(hosts)); err != nil {
		return nil, err
	}

	return &GenerateResult{
		AuthenticatorName: authnBody.Name,
		WorkloadCount:     len(hosts),
	}, nil
}

func buildPlan(cfg GenerateConfig, authnName string, groupID string, hosts []WorkloadHost) *core.Plan {
	ops := []core.Operation{
		{
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
		},
		{
			ID:             "load-workload-policy",
			Description:    "Create GitHub workload identities under the authenticator identity path",
			Method:         "POST",
			Path:           "/policies/conjur/policy/root",
			BodyFile:       "api/02-workloads.yml",
			ContentType:    "application/x-yaml",
			ExpectedStatus: []int{200, 201},
		},
	}

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

	return &core.Plan{
		Version:           "v1alpha1",
		Platform:          "github",
		Tenant:            cfg.Tenant,
		AuthenticatorType: "jwt",
		AuthenticatorName: authnName,
		AppsGroupID:       groupID,
		IdentityPath:      identityPath(cfg.Discovery.Org),
		WorkloadCount:     len(hosts),
		Operations:        ops,
	}
}

func writeClaimsAnalysis(cfg GenerateConfig, authnBody AuthenticatorBody) error {
	enforced := authnBody.Data.Identity.EnforcedClaims
	if enforced == nil {
		enforced = []string{}
	}

	analysis := claimAnalysis{
		Platform: "github",
		Mode:     "synthetic",
		SelectedClaims: selectedClaims{
			TokenAppProperty: authnBody.Data.Identity.TokenAppProperty,
			EnforcedClaims:   enforced,
		},
		AvailableClaims: []claimRecord{
			{
				Name:           "repository",
				Classification: "identity-strong",
				Recommended:    true,
				Explanation:    "Binds authentication to a single repository such as acme/api-service.",
			},
			{
				Name:           "environment",
				Classification: "scope",
				Recommended:    hasEnvironments(cfg.Discovery.Repos),
				Explanation:    "Scopes deployments to a named GitHub environment when the workflow requests one.",
			},
			{
				Name:           "repository_owner",
				Classification: "identity-weak",
				Recommended:    false,
				Explanation:    "On its own, this grants every repository in the organization the same identity.",
			},
			{
				Name:           "workflow_ref",
				Classification: "scope",
				Recommended:    false,
				Explanation:    "Ties access to a specific workflow file and ref, which is precise but can be brittle during refactoring.",
			},
			{
				Name:           "run_id",
				Classification: "ephemeral",
				Recommended:    false,
				Explanation:    "Changes on every workflow run and is not suitable for stable workload identity.",
			},
		},
		SecurityNotes: []string{
			"Use repository as the default identity claim for GitHub Actions.",
			"Do not use repository_owner by itself unless every repository in the organization should share the same workload identity.",
			"Environment scoping can improve separation, but it requires a compatible identity strategy before being enforced.",
		},
		ImplementationNote: "Live token inspection and interactive claim selection are planned follow-up work; this generator uses the GitHub synthetic defaults from the PRD.",
	}

	if err := core.WriteJSON(cfg.WorkDir, "claims-analysis.json", analysis); err != nil {
		return fmt.Errorf("writing claims analysis: %w", err)
	}
	return nil
}

func writeConfig(cfg GenerateConfig, authnName string, workloadCount int) error {
	config := fmt.Sprintf(`platform: github
org: %s
tenant: %s
workload_auth: jwt
authenticator_name: %s
audience: %s
workload_count: %d
`, cfg.Discovery.Org, cfg.Tenant, authnName, cfg.Audience, workloadCount)

	if err := core.WriteText(cfg.WorkDir, "config.yml", config); err != nil {
		return fmt.Errorf("writing config.yml: %w", err)
	}
	return nil
}

func writeIntegrationArtifacts(cfg GenerateConfig, authnName string, hosts []WorkloadHost) error {
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

      - name: Fetch secrets from Conjur Cloud
        uses: cyberark/conjur-action@v2
        with:
          url: https://%s.secretsmgr.cyberark.cloud
          authn_id: %s
          host_id: %s
          secrets: |
            data/vault/example/safe/test-secret|TEST_SECRET

      - name: Use fetched secret
        run: ./deploy.sh
        env:
          TEST_SECRET: ${{ env.TEST_SECRET }}
`, cfg.Tenant, authnName, hostID)

	readme := fmt.Sprintf(`# GitHub Actions Integration

This directory contains a starter workflow for a GitHub Actions workload using the generated Conjur Cloud JWT authenticator.

Generated values:

- Tenant URL: `+"`https://%s.secretsmgr.cyberark.cloud`"+`
- Authenticator service ID: `+"`%s`"+`
- Example workload host ID: `+"`%s`"+`
- Example repository: `+"`%s`"+`

Before using the workflow, replace `+"`data/vault/example/safe/test-secret`"+` with a real variable path and grant the authenticator apps group access to the required safe.

The workflow must keep:

- `+"`permissions: id-token: write`"+`
- `+"`permissions: contents: read`"+`
- `+"`cyberark/conjur-action@v2`"+`
`, cfg.Tenant, authnName, hostID, repoName)

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
	next := fmt.Sprintf(`# Next Steps: GitHub Actions Onboarding

## Generated Summary

Platform: GitHub Actions

Conjur Cloud tenant: `+"`%s`"+`

Authenticator type: `+"`jwt`"+`

Authenticator name: `+"`%s`"+`

Workload count: `+"`%d`"+`

Identity claim: `+"`repository`"+`

Enforced claims: none in the MVP generator

Apps group to grant to safes: `+"`conjur/authn-jwt/%s/apps`"+`

## 1. Review the Generated Plan

Command:

`+"```sh"+`
conjur-onboard github generate --tenant %s --work-dir %s
`+"```"+`

Expected outcome: `+"`api/plan.json`"+`, `+"`api/01-create-authenticator.json`"+`, `+"`api/02-workloads.yml`"+`, `+"`api/03-add-group-members.jsonl`"+`, and `+"`integration/example-deploy.yml`"+` are present and reviewable.

## 2. Validate Against the Tenant

Command:

`+"```sh"+`
CONJUR_API_KEY=<api-key> conjur-onboard github validate --tenant %s --username <username> --work-dir %s
`+"```"+`

Expected outcome: validation can read all generated bodies and reach the tenant API.

## 3. Apply the Plan

Command:

`+"```sh"+`
CONJUR_API_KEY=<api-key> conjur-onboard github apply --tenant %s --username <username> --work-dir %s
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
`, cfg.Tenant, authnName, workloadCount, authnName, cfg.Tenant, cfg.WorkDir, cfg.Tenant, cfg.WorkDir, cfg.Tenant, cfg.WorkDir, workloadCount, groupID, authnName, identityPath(cfg.Discovery.Org))

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

// workloadID returns the workload host ID for a given repo (and optionally environment).
func workloadID(identPath, repoFullName string, env string) string {
	if env != "" {
		return identPath + "/" + repoFullName + "/" + env
	}
	return identPath + "/" + repoFullName
}

// hasEnvironments returns true if any repo in the discovery result has environments configured.
func hasEnvironments(repos []ghdisc.RepoInfo) bool {
	for _, r := range repos {
		if len(r.Environments) > 0 {
			return true
		}
	}
	return false
}

// enforcedClaims returns the generated JWT enforced claims.
func enforcedClaims(repos []ghdisc.RepoInfo) []string {
	// Environment scoping is intentionally not emitted in the MVP because the
	// repository token_app_property maps to one workload per repo. Per-environment
	// enforcement needs a compatible identity strategy and live token validation.
	return nil
}
