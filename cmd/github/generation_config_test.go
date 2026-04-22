package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cyberark/conjur-onboard/internal/conjur"
	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
)

func TestGitHubGenerateConfigPreservesLegacyArtifacts(t *testing.T) {
	workDir := t.TempDir()
	disc := &ghdisc.DiscoveryResult{
		Platform:   "github",
		Org:        "acme",
		OIDCIssuer: "https://token.actions.githubusercontent.com",
		JWKSUri:    "https://token.actions.githubusercontent.com/.well-known/jwks",
		Repos: []ghdisc.RepoInfo{
			{
				Name:          "api",
				FullName:      "acme/api",
				DefaultBranch: "main",
				Visibility:    "private",
			},
		},
	}

	cfg, err := newGitHubGenerateConfig(disc, githubGenerateOptions{
		Tenant:           "myco",
		Audience:         "conjur-cloud",
		CreateEnabled:    true,
		WorkDir:          workDir,
		ProvisioningMode: "bootstrap",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conjur.Generate(cfg); err != nil {
		t.Fatal(err)
	}

	var plan struct {
		Platform             string `json:"platform"`
		AuthenticatorType    string `json:"authenticator_type"`
		AuthenticatorSubtype string `json:"authenticator_subtype"`
		AuthenticatorName    string `json:"authenticator_name"`
		IdentityPath         string `json:"identity_path"`
	}
	readJSONFileForTest(t, filepath.Join(workDir, "api", "plan.json"), &plan)
	if plan.Platform != "github" {
		t.Fatalf("plan Platform = %q, want github", plan.Platform)
	}
	if plan.AuthenticatorType != "jwt" {
		t.Fatalf("plan AuthenticatorType = %q, want jwt", plan.AuthenticatorType)
	}
	if plan.AuthenticatorSubtype != "github_actions" {
		t.Fatalf("plan AuthenticatorSubtype = %q, want github_actions", plan.AuthenticatorSubtype)
	}
	if plan.AuthenticatorName != "github-acme" {
		t.Fatalf("plan AuthenticatorName = %q, want github-acme", plan.AuthenticatorName)
	}
	if plan.IdentityPath != "data/github-apps/acme" {
		t.Fatalf("plan IdentityPath = %q, want data/github-apps/acme", plan.IdentityPath)
	}

	var claims struct {
		Platform string `json:"platform"`
		Selected struct {
			TokenAppProperty string `json:"token_app_property"`
		} `json:"selected_claims"`
	}
	readJSONFileForTest(t, filepath.Join(workDir, "claims-analysis.json"), &claims)
	if claims.Platform != "github" {
		t.Fatalf("claims Platform = %q, want github", claims.Platform)
	}
	if claims.Selected.TokenAppProperty != "repository" {
		t.Fatalf("claims token_app_property = %q, want repository", claims.Selected.TokenAppProperty)
	}

	if _, err := os.Stat(filepath.Join(workDir, "integration", "example-deploy.yml")); err != nil {
		t.Fatalf("integration workflow missing: %v", err)
	}
}

func readJSONFileForTest(t *testing.T, path string, dest any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatal(err)
	}
}
