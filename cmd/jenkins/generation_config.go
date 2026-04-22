package jenkins

import (
	"fmt"

	"github.com/cyberark/conjur-onboard/internal/conjur"
	jenkinsdisc "github.com/cyberark/conjur-onboard/internal/jenkins"
	"github.com/cyberark/conjur-onboard/internal/platform"
)

type jenkinsGenerateOptions struct {
	Tenant            string
	ConjurURL         string
	ConjurTarget      string
	Audience          string
	CreateEnabled     bool
	WorkDir           string
	ProvisioningMode  string
	AuthenticatorName string
	Selection         jenkinsdisc.Selection
	Verbose           bool
	DryRun            bool
}

func newJenkinsGenerateConfig(disc *jenkinsdisc.DiscoveryResult, opts jenkinsGenerateOptions) (conjur.GenerateConfig, error) {
	if disc == nil {
		return conjur.GenerateConfig{}, fmt.Errorf("discovery is required")
	}
	adapter := jenkinsdisc.NewAdapter()
	pdisc := adapter.DiscoveryFromResult(disc)
	pdisc, err := jenkinsdisc.FilterDiscoveryResources(pdisc, opts.Selection)
	if err != nil {
		return conjur.GenerateConfig{}, err
	}

	analysis, err := loadOrCreateJenkinsClaimAnalysis(disc, opts.WorkDir)
	if err != nil {
		return conjur.GenerateConfig{}, err
	}
	if err := jenkinsdisc.ValidateGeneratorSupportedSelection(analysis.SelectedClaims); err != nil {
		return conjur.GenerateConfig{}, fmt.Errorf("claims analysis selection is not supported by generation: %w", err)
	}
	genInput := platform.GenerationInput{
		Discovery:         pdisc,
		Claims:            adapter.ClaimAnalysisFromJenkins(analysis),
		Tenant:            opts.Tenant,
		ConjurURL:         opts.ConjurURL,
		ConjurTarget:      opts.ConjurTarget,
		Audience:          opts.Audience,
		CreateEnabled:     opts.CreateEnabled,
		WorkDir:           opts.WorkDir,
		ProvisioningMode:  opts.ProvisioningMode,
		AuthenticatorName: opts.AuthenticatorName,
		Verbose:           opts.Verbose,
		DryRun:            opts.DryRun,
	}
	authn, err := adapter.Authenticator(genInput)
	if err != nil {
		return conjur.GenerateConfig{}, err
	}
	if genInput.Audience == "" {
		genInput.Audience = authn.Audience
	}
	workloads, err := adapter.Workloads(genInput, authn)
	if err != nil {
		return conjur.GenerateConfig{}, err
	}
	integrationArtifacts, err := adapter.IntegrationArtifacts(platform.IntegrationInput{
		GenerationInput: genInput,
		Authenticator:   authn,
		Workloads:       workloads,
		AppsGroupID:     jenkinsdisc.AuthenticatorAppsGroupID(authn.Name),
	})
	if err != nil {
		return conjur.GenerateConfig{}, err
	}
	nextSteps, err := adapter.NextSteps(platform.NextStepsInput{
		GenerationInput: genInput,
		Authenticator:   authn,
		Workloads:       workloads,
		AppsGroupID:     jenkinsdisc.AuthenticatorAppsGroupID(authn.Name),
	})
	if err != nil {
		return conjur.GenerateConfig{}, err
	}

	return conjur.GenerateConfig{
		Platform:              adapter.Descriptor(),
		Discovery:             pdisc,
		Claims:                genInput.Claims,
		ClaimAnalysisArtifact: analysis,
		Authenticator:         authn,
		Workloads:             workloads,
		IntegrationArtifacts:  integrationArtifacts,
		NextSteps:             nextSteps,
		ConfigYAML:            adapter.ConfigYAML(genInput, authn, len(workloads)),
		Tenant:                opts.Tenant,
		ConjurURL:             opts.ConjurURL,
		ConjurTarget:          opts.ConjurTarget,
		Audience:              genInput.Audience,
		CreateEnabled:         opts.CreateEnabled,
		WorkDir:               opts.WorkDir,
		ProvisioningMode:      opts.ProvisioningMode,
		AuthenticatorName:     authn.Name,
		Verbose:               opts.Verbose,
		DryRun:                opts.DryRun,
	}, nil
}

func loadOrCreateJenkinsClaimAnalysis(disc *jenkinsdisc.DiscoveryResult, workDir string) (jenkinsdisc.ClaimAnalysis, error) {
	analysis, found, err := jenkinsdisc.LoadClaimAnalysis(workDir)
	if err != nil {
		return jenkinsdisc.ClaimAnalysis{}, err
	}
	if !found {
		return jenkinsdisc.BuildDefaultClaimAnalysis(disc), nil
	}
	if analysis.Platform != "" && analysis.Platform != "jenkins" {
		return jenkinsdisc.ClaimAnalysis{}, fmt.Errorf("claims-analysis.json is for platform %q, not jenkins", analysis.Platform)
	}
	if analysis.SelectedClaims.TokenAppProperty == "" {
		analysis.SelectedClaims.TokenAppProperty = jenkinsdisc.DefaultTokenAppProperty
	}
	return analysis, nil
}
