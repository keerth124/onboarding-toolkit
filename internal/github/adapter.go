package github

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/cyberark/conjur-onboard/internal/platform"
)

// Adapter implements the platform-neutral onboarding contract for GitHub
// Actions using GitHub OIDC.
type Adapter struct{}

var _ platform.Adapter = Adapter{}

// NewAdapter returns the GitHub platform adapter.
func NewAdapter() Adapter {
	return Adapter{}
}

func (Adapter) Descriptor() platform.Descriptor {
	return platform.Descriptor{
		ID:          "github",
		DisplayName: "GitHub Actions",
		Description: "GitHub Actions via GitHub OIDC",
	}
}

func (a Adapter) Discover(ctx context.Context, input platform.DiscoveryInput) (*platform.Discovery, error) {
	org := strings.TrimSpace(input.Options["org"])
	if org == "" {
		return nil, fmt.Errorf("org option is required")
	}
	result, err := Discover(ctx, DiscoverConfig{
		Org:       org,
		Token:     input.Token,
		RepoNames: splitOptionList(input.Options["repos"]),
		Verbose:   input.Verbose,
	})
	if err != nil {
		return nil, err
	}
	return a.DiscoveryFromResult(result), nil
}

func (a Adapter) InspectClaims(ctx context.Context, input platform.ClaimInspectionInput) (platform.ClaimAnalysis, error) {
	_ = ctx
	mode := input.Mode
	if mode == "" {
		mode = "synthetic"
	}
	if mode != "synthetic" {
		return platform.ClaimAnalysis{}, fmt.Errorf("only synthetic inspection is implemented for GitHub")
	}

	repo := input.Resource.FullName
	if repo == "" {
		repo = input.Resource.ID
	}
	if repo == "" {
		repo = input.PlatformOptions["repo"]
	}
	if !strings.Contains(repo, "/") {
		return platform.ClaimAnalysis{}, fmt.Errorf("repo must be in owner/name form")
	}

	analysis := BuildSyntheticClaimAnalysis(repo, input.Environment, ClaimSelection{
		TokenAppProperty: input.ClaimSelection.TokenAppProperty,
		EnforcedClaims:   append([]string(nil), input.ClaimSelection.EnforcedClaims...),
	})
	return a.ClaimAnalysisFromGitHub(analysis), nil
}

func (Adapter) DefaultClaimSelection(discovery *platform.Discovery) platform.ClaimSelection {
	_ = discovery
	return platform.ClaimSelection{
		TokenAppProperty: DefaultTokenAppProperty,
	}
}

func (a Adapter) Authenticator(input platform.GenerationInput) (platform.Authenticator, error) {
	if input.Discovery == nil {
		return platform.Authenticator{}, fmt.Errorf("discovery is required")
	}
	audience := input.Audience
	if audience == "" {
		audience = "conjur-cloud"
	}
	authnName := a.AuthenticatorName(input.Discovery.Scope.Name, input.AuthenticatorName)
	if authnName == "" {
		return platform.Authenticator{}, fmt.Errorf("authenticator name is required")
	}

	selection := input.Claims.SelectedClaims
	if selection.TokenAppProperty == "" {
		selection = a.DefaultClaimSelection(input.Discovery)
	}

	return platform.Authenticator{
		Type:             "jwt",
		Subtype:          "github_actions",
		Name:             authnName,
		Enabled:          input.CreateEnabled,
		Issuer:           input.Discovery.OIDCProvider.Issuer,
		JWKSURI:          input.Discovery.OIDCProvider.JWKSURI,
		Audience:         audience,
		IdentityPath:     a.IdentityPath(input.Discovery.Scope.Name),
		TokenAppProperty: selection.TokenAppProperty,
		EnforcedClaims:   append([]string(nil), selection.EnforcedClaims...),
	}, nil
}

func (a Adapter) Workloads(input platform.GenerationInput, authenticator platform.Authenticator) ([]platform.Workload, error) {
	if input.Discovery == nil {
		return nil, fmt.Errorf("discovery is required")
	}
	selection := input.Claims.SelectedClaims
	if selection.TokenAppProperty == "" {
		selection = a.DefaultClaimSelection(input.Discovery)
	}

	workloads := make([]platform.Workload, 0, len(input.Discovery.Resources))
	for _, resource := range input.Discovery.Resources {
		if resource.Archived {
			continue
		}
		fullName := resource.FullName
		if fullName == "" {
			fullName = resource.ID
		}
		if fullName == "" {
			continue
		}

		annotations := map[string]string{}
		if selection.TokenAppProperty == DefaultTokenAppProperty {
			annotations[JWTAnnotationKey(authenticator.Name, DefaultTokenAppProperty)] = fullName
		}

		workloads = append(workloads, platform.Workload{
			FullPath:    WorkloadID(authenticator.IdentityPath, fullName, ""),
			HostID:      fullName,
			DisplayName: fullName,
			SourceID:    resource.ID,
			Annotations: annotations,
		})
	}
	return workloads, nil
}

func (a Adapter) IntegrationArtifacts(input platform.IntegrationInput) ([]platform.IntegrationArtifact, error) {
	if input.GenerationInput.Discovery == nil {
		return nil, fmt.Errorf("discovery is required")
	}
	org := input.GenerationInput.Discovery.Scope.Name
	conjurURL := EffectiveConjurURL(input.GenerationInput.Tenant, input.GenerationInput.ConjurURL)

	hostID := "data/github-apps/" + SafeName(org) + "/OWNER/REPO"
	repoName := "owner/repo"
	if len(input.Workloads) > 0 {
		hostID = input.Workloads[0].FullPath
		repoName = strings.TrimPrefix(input.Workloads[0].HostID, org+"/")
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
`, conjurURL, input.Authenticator.Name, hostID)

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
`, conjurURL, input.Authenticator.Name, hostID, repoName)

	return []platform.IntegrationArtifact{
		{Path: "integration/example-deploy.yml", Content: workflow, Description: "GitHub Actions workflow example"},
		{Path: "integration/README.md", Content: readme, Description: "GitHub Actions integration README"},
	}, nil
}

func (a Adapter) NextSteps(input platform.NextStepsInput) (string, error) {
	if input.GenerationInput.Discovery == nil {
		return "", fmt.Errorf("discovery is required")
	}
	target := EffectiveConjurTarget(input.GenerationInput.ConjurTarget, input.GenerationInput.ConjurURL)
	conjurURL := EffectiveConjurURL(input.GenerationInput.Tenant, input.GenerationInput.ConjurURL)
	mode := input.GenerationInput.ProvisioningMode
	if mode == "" {
		mode = "bootstrap"
	}
	modeNote := "This plan creates the GitHub authenticator, workloads, and group memberships."
	if mode == "workloads-only" {
		modeNote = "This plan assumes the GitHub authenticator already exists and creates only workloads plus group memberships."
	}
	if target == "self-hosted" {
		modeNote += " Because the self-hosted target has no group membership REST endpoint, group access is granted by loading api/04-grant-authenticator-access.yml."
	}

	generateEndpoint := fmt.Sprintf("--tenant %s", input.GenerationInput.Tenant)
	validateEndpoint := fmt.Sprintf("--tenant %s", input.GenerationInput.Tenant)
	if target == "self-hosted" {
		generateEndpoint = fmt.Sprintf("--conjur-url %s --conjur-target self-hosted", conjurURL)
		validateEndpoint = fmt.Sprintf("--conjur-url %s", conjurURL)
	}
	appsGroup := input.AppsGroupID
	if appsGroup == "" {
		appsGroup = AuthenticatorAppsGroupID(input.Authenticator.Name)
	}

	return fmt.Sprintf(`# Next Steps: GitHub Actions Onboarding

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
`, target, conjurURL, input.Authenticator.Name, mode, modeNote, len(input.Workloads), input.Authenticator.Name, generateEndpoint, input.GenerationInput.WorkDir, validateEndpoint, input.GenerationInput.WorkDir, validateEndpoint, input.GenerationInput.WorkDir, len(input.Workloads), appsGroup, input.Authenticator.Name, input.Authenticator.IdentityPath), nil
}

func (a Adapter) DiscoveryFromResult(disc *DiscoveryResult) *platform.Discovery {
	if disc == nil {
		return nil
	}
	resources := make([]platform.Resource, 0, len(disc.Repos))
	for _, repo := range disc.Repos {
		resources = append(resources, platform.Resource{
			ID:            repo.FullName,
			Name:          repo.Name,
			FullName:      repo.FullName,
			Type:          "repository",
			Parent:        disc.Org,
			DefaultBranch: repo.DefaultBranch,
			Visibility:    repo.Visibility,
			Environments:  repo.Environments,
			Archived:      repo.Archived,
		})
	}

	scopeType := strings.ToLower(disc.OrgInfo.AccountType)
	if scopeType == "" {
		scopeType = "organization"
	}
	return &platform.Discovery{
		Platform: a.Descriptor(),
		Scope: platform.Scope{
			ID:          disc.Org,
			Name:        disc.Org,
			DisplayName: disc.OrgInfo.Name,
			Type:        scopeType,
		},
		OIDCProvider: platform.OIDCProvider{
			Issuer:  disc.OIDCIssuer,
			JWKSURI: disc.JWKSUri,
		},
		Resources:    resources,
		Warnings:     append([]string(nil), disc.Warnings...),
		DiscoveredAt: disc.DiscoveredAt,
	}
}

func (a Adapter) ClaimAnalysisFromGitHub(analysis ClaimAnalysis) platform.ClaimAnalysis {
	available := make([]platform.ClaimRecord, 0, len(analysis.AvailableClaims))
	for _, claim := range analysis.AvailableClaims {
		available = append(available, platform.ClaimRecord{
			Name:           claim.Name,
			ExampleValue:   claim.ExampleValue,
			Classification: claim.Classification,
			Recommended:    claim.Recommended,
			Explanation:    claim.Explanation,
		})
	}
	return platform.ClaimAnalysis{
		Platform:    a.Descriptor(),
		Mode:        analysis.Mode,
		Subject:     analysis.Repository,
		Recommended: append([]string(nil), analysis.Recommended...),
		SelectedClaims: platform.ClaimSelection{
			TokenAppProperty: analysis.SelectedClaims.TokenAppProperty,
			EnforcedClaims:   append([]string(nil), analysis.SelectedClaims.EnforcedClaims...),
		},
		AvailableClaims:    available,
		SecurityWarnings:   append([]string(nil), analysis.SecurityWarnings...),
		SecurityNotes:      append([]string(nil), analysis.SecurityNotes...),
		ImplementationNote: analysis.ImplementationNote,
	}
}

func (a Adapter) ConfigYAML(input platform.GenerationInput, authenticator platform.Authenticator, workloadCount int) string {
	target := EffectiveConjurTarget(input.ConjurTarget, input.ConjurURL)
	conjurURL := EffectiveConjurURL(input.Tenant, input.ConjurURL)
	mode := input.ProvisioningMode
	if mode == "" {
		mode = "bootstrap"
	}
	org := ""
	if input.Discovery != nil {
		org = input.Discovery.Scope.Name
	}
	return fmt.Sprintf(`platform: github
org: %s
tenant: %s
conjur_url: %s
conjur_target: %s
workload_auth: jwt
provisioning_mode: %s
authenticator_name: %s
audience: %s
workload_count: %d
`, org, input.Tenant, conjurURL, target, mode, authenticator.Name, authenticator.Audience, workloadCount)
}

func (Adapter) AuthenticatorName(org string, override string) string {
	if override != "" {
		return SafeName(override)
	}
	return "github-" + SafeName(org)
}

func (Adapter) IdentityPath(org string) string {
	return "data/github-apps/" + SafeName(org)
}

func WorkloadID(identityPath string, repoFullName string, env string) string {
	if env != "" {
		return identityPath + "/" + repoFullName + "/" + env
	}
	return identityPath + "/" + repoFullName
}

func JWTAnnotationKey(authnName string, claim string) string {
	return fmt.Sprintf("authn-jwt/%s/%s", authnName, claim)
}

func AuthenticatorAppsGroupID(authnName string) string {
	return fmt.Sprintf("conjur/authn-jwt/%s/apps", authnName)
}

func EffectiveConjurTarget(target string, conjurURL string) string {
	if target != "" {
		return target
	}
	if conjurURL != "" {
		return "self-hosted"
	}
	return "saas"
}

func EffectiveConjurURL(tenant string, rawURL string) string {
	if rawURL == "" {
		return fmt.Sprintf("https://%s.secretsmgr.cyberark.cloud/api", strings.TrimSuffix(tenant, "/"))
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(rawURL, "/")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}

var nonSafeNameRE = regexp.MustCompile(`[^A-Za-z0-9\-_]`)

func SafeName(s string) string {
	return nonSafeNameRE.ReplaceAllString(strings.ToLower(s), "-")
}

func splitOptionList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var values []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
