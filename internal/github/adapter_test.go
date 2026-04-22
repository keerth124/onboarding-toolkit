package github

import (
	"testing"

	"github.com/cyberark/conjur-onboard/internal/platform"
)

func TestAdapterAuthenticatorMetadata(t *testing.T) {
	adapter := NewAdapter()
	disc := adapter.DiscoveryFromResult(testAdapterDiscoveryResult())
	claims := adapter.ClaimAnalysisFromGitHub(BuildSyntheticClaimAnalysis("acme/api", "", ClaimSelection{
		TokenAppProperty: DefaultTokenAppProperty,
	}))

	authn, err := adapter.Authenticator(platform.GenerationInput{
		Discovery:     disc,
		Claims:        claims,
		Audience:      "conjur-cloud",
		CreateEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if authn.Type != "jwt" {
		t.Fatalf("Type = %q, want jwt", authn.Type)
	}
	if authn.Subtype != "github_actions" {
		t.Fatalf("Subtype = %q, want github_actions", authn.Subtype)
	}
	if authn.Name != "github-acme" {
		t.Fatalf("Name = %q, want github-acme", authn.Name)
	}
	if authn.IdentityPath != "data/github-apps/acme" {
		t.Fatalf("IdentityPath = %q, want data/github-apps/acme", authn.IdentityPath)
	}
	if authn.TokenAppProperty != DefaultTokenAppProperty {
		t.Fatalf("TokenAppProperty = %q, want %s", authn.TokenAppProperty, DefaultTokenAppProperty)
	}
}

func TestAdapterWorkloadsMapRepositories(t *testing.T) {
	adapter := NewAdapter()
	disc := adapter.DiscoveryFromResult(testAdapterDiscoveryResult())
	claims := adapter.ClaimAnalysisFromGitHub(BuildSyntheticClaimAnalysis("acme/api", "", ClaimSelection{
		TokenAppProperty: DefaultTokenAppProperty,
	}))
	authn, err := adapter.Authenticator(platform.GenerationInput{
		Discovery: disc,
		Claims:    claims,
		Audience:  "conjur-cloud",
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
	if len(workloads) != 1 {
		t.Fatalf("len(workloads) = %d, want 1", len(workloads))
	}
	if workloads[0].FullPath != "data/github-apps/acme/acme/api" {
		t.Fatalf("FullPath = %q, want data/github-apps/acme/acme/api", workloads[0].FullPath)
	}
	if workloads[0].HostID != "acme/api" {
		t.Fatalf("HostID = %q, want acme/api", workloads[0].HostID)
	}
	if workloads[0].Annotations["authn-jwt/github-acme/repository"] != "acme/api" {
		t.Fatalf("repository annotation = %q, want acme/api", workloads[0].Annotations["authn-jwt/github-acme/repository"])
	}
}

func TestAdapterDiscoveryFromResultNormalizesResources(t *testing.T) {
	adapter := NewAdapter()
	disc := adapter.DiscoveryFromResult(testAdapterDiscoveryResult())

	if disc.Platform.ID != "github" {
		t.Fatalf("Platform.ID = %q, want github", disc.Platform.ID)
	}
	if disc.Scope.Name != "acme" {
		t.Fatalf("Scope.Name = %q, want acme", disc.Scope.Name)
	}
	if disc.OIDCProvider.Issuer != "https://token.actions.githubusercontent.com" {
		t.Fatalf("Issuer = %q", disc.OIDCProvider.Issuer)
	}
	if len(disc.Resources) != 1 {
		t.Fatalf("len(Resources) = %d, want 1", len(disc.Resources))
	}
	if disc.Resources[0].FullName != "acme/api" {
		t.Fatalf("FullName = %q, want acme/api", disc.Resources[0].FullName)
	}
}

func TestAdapterDefaultClaimSelection(t *testing.T) {
	selection := NewAdapter().DefaultClaimSelection(nil)
	if selection.TokenAppProperty != DefaultTokenAppProperty {
		t.Fatalf("TokenAppProperty = %q, want %s", selection.TokenAppProperty, DefaultTokenAppProperty)
	}
	if len(selection.EnforcedClaims) != 0 {
		t.Fatalf("EnforcedClaims = %#v, want empty", selection.EnforcedClaims)
	}
}

func testAdapterDiscoveryResult() *DiscoveryResult {
	return &DiscoveryResult{
		Platform:   "github",
		Org:        "acme",
		OIDCIssuer: "https://token.actions.githubusercontent.com",
		JWKSUri:    "https://token.actions.githubusercontent.com/.well-known/jwks",
		OrgInfo: OrgInfo{
			Name:        "Acme",
			AccountType: "Organization",
		},
		Repos: []RepoInfo{
			{
				Name:          "api",
				FullName:      "acme/api",
				DefaultBranch: "main",
				Visibility:    "private",
			},
		},
	}
}
