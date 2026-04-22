package jenkins

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/cyberark/conjur-onboard/internal/platform"
)

type Adapter struct{}

var _ platform.Adapter = Adapter{}

func NewAdapter() Adapter {
	return Adapter{}
}

func (Adapter) Descriptor() platform.Descriptor {
	return platform.Descriptor{
		ID:          "jenkins",
		DisplayName: "Jenkins",
		Description: "Jenkins workloads via the CyberArk Conjur Jenkins plugin",
	}
}

func (a Adapter) Discover(ctx context.Context, input platform.DiscoveryInput) (*platform.Discovery, error) {
	result, err := Discover(ctx, DiscoverConfig{
		JenkinsURL:   input.Options["url"],
		Username:     input.Options["username"],
		Token:        input.Token,
		JobsFromFile: input.Options["jobs_from_file"],
		MaxDepth:     optionInt(input.Options["max_depth"]),
		Verbose:      input.Verbose,
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
		return platform.ClaimAnalysis{}, fmt.Errorf("only synthetic inspection is implemented for Jenkins")
	}
	fullName := input.Resource.FullName
	if fullName == "" {
		fullName = input.Resource.ID
	}
	if fullName == "" {
		fullName = input.PlatformOptions["job"]
	}
	if fullName == "" {
		return platform.ClaimAnalysis{}, fmt.Errorf("job full name is required")
	}
	analysis := BuildSyntheticClaimAnalysis(fullName, ClaimSelection{
		TokenAppProperty: input.ClaimSelection.TokenAppProperty,
		EnforcedClaims:   append([]string(nil), input.ClaimSelection.EnforcedClaims...),
	})
	return a.ClaimAnalysisFromJenkins(analysis), nil
}

func (Adapter) DefaultClaimSelection(discovery *platform.Discovery) platform.ClaimSelection {
	_ = discovery
	return platform.ClaimSelection{TokenAppProperty: DefaultTokenAppProperty}
}

func (a Adapter) Authenticator(input platform.GenerationInput) (platform.Authenticator, error) {
	if input.Discovery == nil {
		return platform.Authenticator{}, fmt.Errorf("discovery is required")
	}
	audience := input.Audience
	if audience == "" {
		audience = DefaultAudience
	}
	authnName := a.AuthenticatorName(input.Discovery.Scope.Name, input.AuthenticatorName)
	selection := input.Claims.SelectedClaims
	if selection.TokenAppProperty == "" {
		selection = a.DefaultClaimSelection(input.Discovery)
	}

	return platform.Authenticator{
		Type:             "jwt",
		Subtype:          "jenkins",
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

func (Adapter) Workloads(input platform.GenerationInput, authenticator platform.Authenticator) ([]platform.Workload, error) {
	if input.Discovery == nil {
		return nil, fmt.Errorf("discovery is required")
	}
	workloads := make([]platform.Workload, 0, len(input.Discovery.Resources))
	for _, resource := range input.Discovery.Resources {
		fullName := resource.FullName
		if fullName == "" {
			fullName = resource.ID
		}
		if fullName == "" {
			continue
		}
		annotations := map[string]string{
			JWTAnnotationKey(authenticator.Name, DefaultTokenAppProperty): fullName,
		}
		if resource.Type != "" {
			annotations[JWTAnnotationKey(authenticator.Name, "jenkins_scope_type")] = resource.Type
		}
		hostID := strings.Trim(fullName, "/")
		workloads = append(workloads, platform.Workload{
			FullPath:    WorkloadID(authenticator.IdentityPath, hostID),
			HostID:      hostID,
			DisplayName: fullName,
			SourceID:    resource.ID,
			Annotations: annotations,
			Metadata: map[string]string{
				"jenkins_type": resource.Type,
			},
		})
	}
	return workloads, nil
}

func (Adapter) IntegrationArtifacts(input platform.IntegrationInput) ([]platform.IntegrationArtifact, error) {
	conjurURL := EffectiveConjurURL(input.GenerationInput.Tenant, input.GenerationInput.ConjurURL)
	hostID := "data/jenkins-apps/jenkins/Folder/Deploy"
	if len(input.Workloads) > 0 {
		hostID = input.Workloads[0].FullPath
	}
	jenkinsfile := fmt.Sprintf(`pipeline {
  agent any

  stages {
    stage('Use Conjur secret') {
      steps {
        // Configure the CyberArk Conjur Jenkins plugin for this credential scope.
        // Authenticator service ID: authn-jwt/%s
        // Host ID: %s
        withCredentials([string(credentialsId: 'data/vault/example/safe/test-secret', variable: 'TEST_SECRET')]) {
          sh './deploy.sh'
        }
      }
    }
  }
}
`, input.Authenticator.Name, hostID)

	readme := fmt.Sprintf(`# Jenkins Integration

This directory contains a starter Jenkinsfile fragment for the CyberArk Conjur Jenkins plugin.

Generated values:

- Conjur URL: `+"`%s`"+`
- Authenticator service ID: `+"`authn-jwt/%s`"+`
- Example workload host ID: `+"`%s`"+`

Configure the Conjur plugin at the matching Jenkins credential scope. The scope can be global, a folder, a multibranch parent, or a leaf job. Replace `+"`data/vault/example/safe/test-secret`"+` with a real Conjur variable path.
`, conjurURL, input.Authenticator.Name, hostID)

	return []platform.IntegrationArtifact{
		{Path: "integration/Jenkinsfile", Content: jenkinsfile, Description: "Jenkins pipeline example"},
		{Path: "integration/README.md", Content: readme, Description: "Jenkins integration README"},
	}, nil
}

func (Adapter) NextSteps(input platform.NextStepsInput) (string, error) {
	target := EffectiveConjurTarget(input.GenerationInput.ConjurTarget, input.GenerationInput.ConjurURL)
	conjurURL := EffectiveConjurURL(input.GenerationInput.Tenant, input.GenerationInput.ConjurURL)
	mode := input.GenerationInput.ProvisioningMode
	if mode == "" {
		mode = "bootstrap"
	}
	appsGroup := input.AppsGroupID
	if appsGroup == "" {
		appsGroup = AuthenticatorAppsGroupID(input.Authenticator.Name)
	}
	generateEndpoint := fmt.Sprintf("--tenant %s", input.GenerationInput.Tenant)
	validateEndpoint := fmt.Sprintf("--tenant %s", input.GenerationInput.Tenant)
	if target == "self-hosted" {
		generateEndpoint = fmt.Sprintf("--conjur-url %s --conjur-target self-hosted", conjurURL)
		validateEndpoint = fmt.Sprintf("--conjur-url %s", conjurURL)
	}
	return fmt.Sprintf(`# Next Steps: Jenkins Onboarding

## Generated Summary

Platform: Jenkins

Conjur target: `+"`%s`"+`

Conjur URL: `+"`%s`"+`

Authenticator type: `+"`jwt`"+`

Authenticator name: `+"`%s`"+`

Provisioning mode: `+"`%s`"+`

Workload count: `+"`%d`"+`

Identity claim: `+"`jenkins_full_name`"+`

Audience: `+"`%s`"+`

Apps group to grant to safes: `+"`%s`"+`

## 1. Review the Generated Plan

Command:

`+"```sh"+`
conjur-onboard jenkins generate %s --work-dir %s
`+"```"+`

Expected outcome: `+"`api/plan.json`"+`, `+"`api/01-create-authenticator.json`"+`, `+"`api/02-workloads.yml`"+`, membership artifacts, and `+"`integration/Jenkinsfile`"+` are present and reviewable.

## 2. Validate Against Conjur

Command:

`+"```sh"+`
CONJUR_API_KEY=<api-key> conjur-onboard jenkins validate %s --username <username> --work-dir %s
`+"```"+`

Expected outcome: validation can read all generated bodies and reach the Conjur endpoint.

## 3. Apply the Plan

Command:

`+"```sh"+`
CONJUR_API_KEY=<api-key> conjur-onboard jenkins apply %s --username <username> --work-dir %s
`+"```"+`

Expected outcome: the authenticator is created, workload policy is loaded, and Jenkins workloads are added to the authenticator apps group.

## 4. Configure Jenkins Credential Scope

Configure the CyberArk Conjur Jenkins plugin at the Jenkins scope represented by the generated workload host. Folder-level scopes can be inherited by descendant jobs when Jenkins credential inheritance is enabled.

Expected outcome: the Jenkins credential scope uses service ID `+"`authn-jwt/%s`"+` and a host ID from `+"`api/02-workloads.yml`"+`.

## 5. Grant Safe Access

COT does not grant access to safes. Grant the generated apps group to the safe or policy that contains the secrets Jenkins should read:

`+"```text"+`
%s
`+"```"+`

## 6. Verify End to End

Use the sample `+"`integration/Jenkinsfile`"+` in a test job under the selected credential scope and replace the example secret path with a known test secret.

Expected outcome: the Jenkins job fetches the test secret through the Conjur credentials provider.

## Troubleshooting

- HTTP 401 during validate or apply: check `+"`CONJUR_API_KEY`"+` and the `+"`--username`"+` value.
- HTTP 403 during authenticator creation: the tool identity likely needs create privileges on the authenticator policy branch, typically through `+"`Authn_Admins`"+`.
- Jenkins cannot fetch secrets: confirm the plugin scope host ID matches a generated workload and the job is inside that credential scope.
- JWT validation fails: confirm Jenkins exposes `+"`%s`"+` and the authenticator audience is `+"`%s`"+`.
- Secret not found or permission denied: grant the apps group access to the safe; COT intentionally does not generate safe grants.
`, target, conjurURL, input.Authenticator.Name, mode, len(input.Workloads), input.Authenticator.Audience, appsGroup, generateEndpoint, input.GenerationInput.WorkDir, validateEndpoint, input.GenerationInput.WorkDir, validateEndpoint, input.GenerationInput.WorkDir, input.Authenticator.Name, appsGroup, input.Authenticator.JWKSURI, input.Authenticator.Audience), nil
}

func (a Adapter) DiscoveryFromResult(disc *DiscoveryResult) *platform.Discovery {
	if disc == nil {
		return nil
	}
	resources := make([]platform.Resource, 0, len(disc.Jobs))
	for _, job := range disc.Jobs {
		resources = append(resources, platform.Resource{
			ID:       job.FullName,
			Name:     job.Name,
			FullName: job.FullName,
			Type:     job.Type,
			Parent:   job.Parent,
			Metadata: map[string]string{
				"url":   job.URL,
				"class": job.Class,
			},
		})
	}
	return &platform.Discovery{
		Platform: a.Descriptor(),
		Scope: platform.Scope{
			ID:          disc.ControllerSlug,
			Name:        disc.ControllerSlug,
			DisplayName: disc.Controller,
			Type:        "controller",
			Metadata: map[string]string{
				"jenkins_url": disc.JenkinsURL,
			},
		},
		OIDCProvider: platform.OIDCProvider{
			Issuer:  disc.OIDCIssuer,
			JWKSURI: disc.JWKSURI,
		},
		Resources:    resources,
		Warnings:     append([]string(nil), disc.Warnings...),
		Metadata:     map[string]string{"source": disc.Source, "plugin_version": disc.PluginVersion, "version": disc.Version},
		DiscoveredAt: disc.DiscoveredAt,
	}
}

func (a Adapter) ClaimAnalysisFromJenkins(analysis ClaimAnalysis) platform.ClaimAnalysis {
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
		Subject:     analysis.JobFullName,
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
	controller := ""
	jenkinsURL := ""
	if input.Discovery != nil {
		controller = input.Discovery.Scope.Name
		jenkinsURL = input.Discovery.Scope.Metadata["jenkins_url"]
	}
	return fmt.Sprintf(`platform: jenkins
controller: %s
jenkins_url: %s
tenant: %s
conjur_url: %s
conjur_target: %s
workload_auth: jwt
provisioning_mode: %s
authenticator_name: %s
audience: %s
workload_count: %d
`, controller, jenkinsURL, input.Tenant, conjurURL, target, firstNonEmpty(input.ProvisioningMode, "bootstrap"), authenticator.Name, authenticator.Audience, workloadCount)
}

func (Adapter) AuthenticatorName(controller string, override string) string {
	if override != "" {
		return SafeName(override)
	}
	return "jenkins-" + SafeName(controller)
}

func (Adapter) IdentityPath(controller string) string {
	return "data/jenkins-apps/" + SafeName(controller)
}

func WorkloadID(identityPath string, hostID string) string {
	return strings.TrimRight(identityPath, "/") + "/" + strings.Trim(hostID, "/")
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

func optionInt(value string) int {
	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
		return 0
	}
	return n
}
