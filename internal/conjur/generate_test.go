package conjur

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ghdisc "github.com/cyberark/conjur-onboard/internal/github"
)

func TestGenerateUsesClaimAnalysisSelection(t *testing.T) {
	workDir := t.TempDir()
	disc := testDiscovery()
	analysis := ghdisc.BuildSyntheticClaimAnalysis("acme/api", "", ghdisc.ClaimSelection{
		TokenAppProperty: "repository",
	})
	writeJSONForTest(t, workDir, "claims-analysis.json", analysis)

	_, err := Generate(GenerateConfig{
		Discovery:     disc,
		Tenant:        "myco",
		Audience:      "conjur-cloud",
		CreateEnabled: true,
		WorkDir:       workDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	var body AuthenticatorBody
	readJSONForTest(t, filepath.Join(workDir, "api", "01-create-authenticator.json"), &body)
	if body.Data.Identity.TokenAppProperty != "repository" {
		t.Fatalf("token_app_property = %q, want repository", body.Data.Identity.TokenAppProperty)
	}
	if len(body.Data.Identity.EnforcedClaims) != 0 {
		t.Fatalf("enforced_claims = %#v, want empty", body.Data.Identity.EnforcedClaims)
	}
}

func TestGenerateRejectsUnsupportedClaimSelection(t *testing.T) {
	workDir := t.TempDir()
	disc := testDiscovery()
	analysis := ghdisc.BuildSyntheticClaimAnalysis("acme/api", "", ghdisc.ClaimSelection{
		TokenAppProperty: "repository_owner",
	})
	writeJSONForTest(t, workDir, "claims-analysis.json", analysis)

	_, err := Generate(GenerateConfig{
		Discovery:     disc,
		Tenant:        "myco",
		Audience:      "conjur-cloud",
		CreateEnabled: true,
		WorkDir:       workDir,
	})
	if err == nil {
		t.Fatal("expected unsupported claim selection error")
	}
}

func TestGenerateWorkloadsOnlyOmitsAuthenticatorOperation(t *testing.T) {
	workDir := t.TempDir()
	disc := testDiscovery()

	result, err := Generate(GenerateConfig{
		Discovery:        disc,
		Tenant:           "myco",
		Audience:         "conjur-cloud",
		CreateEnabled:    true,
		WorkDir:          workDir,
		ProvisioningMode: "workloads-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AuthenticatorName != "github-acme" {
		t.Fatalf("AuthenticatorName = %q, want github-acme", result.AuthenticatorName)
	}
	if _, err := os.Stat(filepath.Join(workDir, "api", "01-create-authenticator.json")); !os.IsNotExist(err) {
		t.Fatalf("authenticator artifact exists or stat failed unexpectedly: %v", err)
	}

	var plan struct {
		ProvisioningMode string `json:"provisioning_mode"`
		Operations       []struct {
			ID string `json:"id"`
		} `json:"operations"`
	}
	readJSONForTest(t, filepath.Join(workDir, "api", "plan.json"), &plan)
	if plan.ProvisioningMode != "workloads-only" {
		t.Fatalf("ProvisioningMode = %q, want workloads-only", plan.ProvisioningMode)
	}
	for _, op := range plan.Operations {
		if op.ID == "create-authenticator" {
			t.Fatal("workloads-only plan included create-authenticator")
		}
	}
}

func TestGenerateAuthenticatorNameOverride(t *testing.T) {
	workDir := t.TempDir()
	disc := testDiscovery()

	result, err := Generate(GenerateConfig{
		Discovery:         disc,
		Tenant:            "myco",
		Audience:          "conjur-cloud",
		CreateEnabled:     true,
		WorkDir:           workDir,
		ProvisioningMode:  "workloads-only",
		AuthenticatorName: "github-shared",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AuthenticatorName != "github-shared" {
		t.Fatalf("AuthenticatorName = %q, want github-shared", result.AuthenticatorName)
	}

	var plan struct {
		AuthenticatorName string `json:"authenticator_name"`
		Operations        []struct {
			ID   string            `json:"id"`
			Path string            `json:"path"`
			Meta map[string]string `json:"metadata"`
		} `json:"operations"`
	}
	readJSONForTest(t, filepath.Join(workDir, "api", "plan.json"), &plan)
	if plan.AuthenticatorName != "github-shared" {
		t.Fatalf("plan AuthenticatorName = %q, want github-shared", plan.AuthenticatorName)
	}
	foundMemberPath := false
	for _, op := range plan.Operations {
		if op.ID == "add-group-member-001" {
			foundMemberPath = strings.Contains(op.Path, "github-shared")
		}
	}
	if !foundMemberPath {
		t.Fatal("group membership path did not use authenticator override")
	}
}

func testDiscovery() *ghdisc.DiscoveryResult {
	return &ghdisc.DiscoveryResult{
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
}

func writeJSONForTest(t *testing.T, dir string, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSONForTest(t *testing.T, path string, dest any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatal(err)
	}
}
