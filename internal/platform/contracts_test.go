package platform

import "testing"

func TestDescriptorValidateRequiresID(t *testing.T) {
	err := Descriptor{DisplayName: "GitHub Actions"}.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDescriptorValidateRequiresDisplayName(t *testing.T) {
	err := Descriptor{ID: "github"}.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDescriptorValidateAcceptsCompleteDescriptor(t *testing.T) {
	err := Descriptor{ID: "github", DisplayName: "GitHub Actions"}.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestContractTypesCanDescribeJWTPlatform(t *testing.T) {
	disc := Discovery{
		Platform: Descriptor{ID: "github", DisplayName: "GitHub Actions"},
		Scope:    Scope{ID: "acme", Name: "acme", Type: "organization"},
		OIDCProvider: OIDCProvider{
			Issuer:  "https://token.actions.githubusercontent.com",
			JWKSURI: "https://token.actions.githubusercontent.com/.well-known/jwks",
		},
		Resources: []Resource{
			{ID: "acme/api", Name: "api", FullName: "acme/api", Type: "repository"},
		},
	}

	input := GenerationInput{
		Discovery:    &disc,
		Audience:     "conjur-cloud",
		ConjurTarget: "saas",
		Claims: ClaimAnalysis{
			Platform: disc.Platform,
			Mode:     "synthetic",
			SelectedClaims: ClaimSelection{
				TokenAppProperty: "repository",
			},
		},
	}

	authn := Authenticator{
		Type:             "jwt",
		Subtype:          "github_actions",
		Name:             "github-acme",
		Enabled:          true,
		Issuer:           disc.OIDCProvider.Issuer,
		JWKSURI:          disc.OIDCProvider.JWKSURI,
		Audience:         input.Audience,
		IdentityPath:     "data/github-apps/acme",
		TokenAppProperty: input.Claims.SelectedClaims.TokenAppProperty,
	}

	workload := Workload{
		FullPath: "data/github-apps/acme/acme/api",
		HostID:   "acme/api",
		SourceID: disc.Resources[0].ID,
		Annotations: map[string]string{
			"authn-jwt/github-acme/repository": "acme/api",
		},
	}

	if authn.TokenAppProperty != "repository" {
		t.Fatalf("TokenAppProperty = %q, want repository", authn.TokenAppProperty)
	}
	if workload.SourceID != disc.Resources[0].ID {
		t.Fatalf("SourceID = %q, want %q", workload.SourceID, disc.Resources[0].ID)
	}
}
