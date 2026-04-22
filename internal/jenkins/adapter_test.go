package jenkins

import (
	"testing"

	"github.com/cyberark/conjur-onboard/internal/platform"
)

func TestAdapterAuthenticatorMetadata(t *testing.T) {
	adapter := NewAdapter()
	disc := adapter.DiscoveryFromResult(testDiscoveryResult())
	claims := adapter.ClaimAnalysisFromJenkins(BuildDefaultClaimAnalysis(testDiscoveryResult()))

	authn, err := adapter.Authenticator(platform.GenerationInput{
		Discovery:     disc,
		Claims:        claims,
		CreateEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if authn.Type != "jwt" {
		t.Fatalf("Type = %q, want jwt", authn.Type)
	}
	if authn.Subtype != "jenkins" {
		t.Fatalf("Subtype = %q, want jenkins", authn.Subtype)
	}
	if authn.Name != "jenkins-jenkins-example-com" {
		t.Fatalf("Name = %q, want jenkins-jenkins-example-com", authn.Name)
	}
	if authn.Audience != DefaultAudience {
		t.Fatalf("Audience = %q, want %s", authn.Audience, DefaultAudience)
	}
	if authn.TokenAppProperty != DefaultTokenAppProperty {
		t.Fatalf("TokenAppProperty = %q, want %s", authn.TokenAppProperty, DefaultTokenAppProperty)
	}
}

func TestAdapterWorkloadsMapCredentialScopes(t *testing.T) {
	adapter := NewAdapter()
	disc := adapter.DiscoveryFromResult(testDiscoveryResult())
	claims := adapter.ClaimAnalysisFromJenkins(BuildDefaultClaimAnalysis(testDiscoveryResult()))
	authn, err := adapter.Authenticator(platform.GenerationInput{
		Discovery: disc,
		Claims:    claims,
	})
	if err != nil {
		t.Fatal(err)
	}

	workloads, err := adapter.Workloads(platform.GenerationInput{
		Discovery: disc,
		Claims:    claims,
	}, authn)
	if err != nil {
		t.Fatal(err)
	}
	if len(workloads) != 2 {
		t.Fatalf("len(workloads) = %d, want 2", len(workloads))
	}
	if workloads[0].FullPath != "data/jenkins-apps/jenkins-example-com/GlobalCredentials" {
		t.Fatalf("FullPath = %q", workloads[0].FullPath)
	}
	if workloads[1].FullPath != "data/jenkins-apps/jenkins-example-com/Payments/API/deploy" {
		t.Fatalf("FullPath = %q", workloads[1].FullPath)
	}
	key := "authn-jwt/jenkins-jenkins-example-com/jenkins_full_name"
	if workloads[1].Annotations[key] != "Payments/API/deploy" {
		t.Fatalf("annotation = %q, want Payments/API/deploy", workloads[1].Annotations[key])
	}
}

func testDiscoveryResult() *DiscoveryResult {
	return &DiscoveryResult{
		Platform:       "jenkins",
		JenkinsURL:     "https://jenkins.example.com",
		Controller:     "jenkins.example.com",
		ControllerSlug: "jenkins-example-com",
		OIDCIssuer:     "https://jenkins.example.com",
		JWKSURI:        "https://jenkins.example.com/jwtauth/conjur-jwk-set",
		Source:         "jobs-from-file",
		Jobs: []JobInfo{
			{Name: "GlobalCredentials", FullName: "GlobalCredentials", Type: "global"},
			{Name: "deploy", FullName: "Payments/API/deploy", Type: "pipeline", Parent: "Payments/API"},
		},
	}
}
