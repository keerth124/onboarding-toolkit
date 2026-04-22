package conjur

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyberark/conjur-onboard/internal/platform"
)

func TestGenerateUsesClaimAnalysisSelection(t *testing.T) {
	workDir := t.TempDir()

	_, err := Generate(testGenerateConfig(workDir))
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

	workloadPolicy, err := os.ReadFile(filepath.Join(workDir, "api", "02-workloads.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(workloadPolicy), "authn-jwt/github-acme/repository: acme/api") {
		t.Fatalf("workload policy missing repository JWT annotation:\n%s", string(workloadPolicy))
	}
}

func TestGenerateRejectsMissingJWTClaimSelection(t *testing.T) {
	workDir := t.TempDir()
	cfg := testGenerateConfig(workDir)
	cfg.Claims.SelectedClaims.TokenAppProperty = ""
	cfg.Authenticator.TokenAppProperty = ""

	_, err := Generate(cfg)
	if err == nil {
		t.Fatal("expected missing token_app_property error")
	}
}

func TestGenerateWorkloadsOnlyOmitsAuthenticatorOperation(t *testing.T) {
	workDir := t.TempDir()
	cfg := testGenerateConfig(workDir)
	cfg.ProvisioningMode = "workloads-only"

	result, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.AuthenticatorName != "github-acme" {
		t.Fatalf("AuthenticatorName = %q, want github-acme", result.AuthenticatorName)
	}
	if _, err := os.Stat(filepath.Join(workDir, "api", "01-create-authenticator.json")); !os.IsNotExist(err) {
		t.Fatalf("authenticator artifact exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "api", "00-authenticator-branch.yml")); !os.IsNotExist(err) {
		t.Fatalf("authenticator branch artifact exists or stat failed unexpectedly: %v", err)
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

func TestGenerateSelfHostedUsesPolicyGrantInsteadOfGroupMembershipAPI(t *testing.T) {
	workDir := t.TempDir()
	cfg := testGenerateConfig(workDir)
	cfg.Tenant = ""
	cfg.ConjurURL = "https://conjur.example.com"
	cfg.ConjurTarget = "self-hosted"

	_, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "api", "03-add-group-members.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("self-hosted group members artifact exists or stat failed unexpectedly: %v", err)
	}
	branchPolicy, err := os.ReadFile(filepath.Join(workDir, "api", "00-authenticator-branch.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(branchPolicy), "id: conjur/authn-jwt") {
		t.Fatalf("authenticator branch policy missing authn-jwt branch:\n%s", string(branchPolicy))
	}

	grantPolicy, err := os.ReadFile(filepath.Join(workDir, "api", "04-grant-authenticator-access.yml"))
	if err != nil {
		t.Fatal(err)
	}
	grant := string(grantPolicy)
	if !strings.Contains(grant, "role: !group conjur/authn-jwt/github-acme/apps") {
		t.Fatalf("grant policy missing apps group role:\n%s", grant)
	}
	if !strings.Contains(grant, "- !host data/github-apps/acme/acme/api") {
		t.Fatalf("grant policy missing workload host:\n%s", grant)
	}

	var plan struct {
		ConjurURL            string `json:"conjur_url"`
		ConjurTarget         string `json:"conjur_target"`
		AuthenticatorSubtype string `json:"authenticator_subtype"`
		Operations           []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"operations"`
	}
	readJSONForTest(t, filepath.Join(workDir, "api", "plan.json"), &plan)
	if plan.ConjurTarget != "self-hosted" {
		t.Fatalf("ConjurTarget = %q, want self-hosted", plan.ConjurTarget)
	}
	if plan.ConjurURL != "https://conjur.example.com" {
		t.Fatalf("ConjurURL = %q, want https://conjur.example.com", plan.ConjurURL)
	}
	if plan.AuthenticatorSubtype != "" {
		t.Fatalf("AuthenticatorSubtype = %q, want empty for self-hosted", plan.AuthenticatorSubtype)
	}
	first := plan.Operations[0]
	if first.ID != "load-authenticator-branch" || first.Path != "/policies/{account}/policy/root" {
		t.Fatalf("first operation = %#v, want self-hosted branch policy load", first)
	}
	second := plan.Operations[1]
	if second.ID != "create-authenticator" || second.Path != "/authenticators/{account}" {
		t.Fatalf("second operation = %#v, want self-hosted create authenticator endpoint", second)
	}
	for _, op := range plan.Operations {
		if strings.HasPrefix(op.ID, "add-group-member-") {
			t.Fatal("self-hosted plan included group membership REST operation")
		}
	}
	last := plan.Operations[len(plan.Operations)-1]
	if last.ID != "load-authenticator-grants" || last.Path != "/policies/{account}/policy/root" {
		t.Fatalf("last operation = %#v, want load-authenticator-grants policy load", last)
	}

	var body map[string]any
	readJSONForTest(t, filepath.Join(workDir, "api", "01-create-authenticator.json"), &body)
	if _, ok := body["subtype"]; ok {
		t.Fatalf("self-hosted authenticator body included subtype: %#v", body["subtype"])
	}
}

func TestGenerateAuthenticatorNameOverride(t *testing.T) {
	workDir := t.TempDir()
	cfg := testGenerateConfig(workDir)
	cfg.ProvisioningMode = "workloads-only"
	cfg.AuthenticatorName = "github-shared"

	result, err := Generate(cfg)
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

func testGenerateConfig(workDir string) GenerateConfig {
	disc := testDiscovery()
	return GenerateConfig{
		Platform:  disc.Platform,
		Discovery: disc,
		Claims: platform.ClaimAnalysis{
			Platform: disc.Platform,
			Mode:     "synthetic",
			SelectedClaims: platform.ClaimSelection{
				TokenAppProperty: "repository",
			},
		},
		Authenticator: platform.Authenticator{
			Type:             "jwt",
			Subtype:          "github_actions",
			Name:             "github-acme",
			Enabled:          true,
			Issuer:           disc.OIDCProvider.Issuer,
			JWKSURI:          disc.OIDCProvider.JWKSURI,
			Audience:         "conjur-cloud",
			IdentityPath:     "data/github-apps/acme",
			TokenAppProperty: "repository",
		},
		Workloads: []platform.Workload{
			{
				FullPath: "data/github-apps/acme/acme/api",
				HostID:   "acme/api",
				Annotations: map[string]string{
					"authn-jwt/github-acme/repository": "acme/api",
				},
			},
		},
		IntegrationArtifacts: []platform.IntegrationArtifact{
			{Path: "integration/example-deploy.yml", Content: "name: Deploy with Conjur\n"},
		},
		NextSteps:        "# Next Steps\n",
		ConfigYAML:       "platform: github\n",
		Tenant:           "myco",
		Audience:         "conjur-cloud",
		CreateEnabled:    true,
		WorkDir:          workDir,
		ProvisioningMode: "bootstrap",
	}
}

func testDiscovery() *platform.Discovery {
	return &platform.Discovery{
		Platform: platform.Descriptor{
			ID:          "github",
			DisplayName: "GitHub Actions",
		},
		Scope: platform.Scope{
			ID:   "acme",
			Name: "acme",
			Type: "organization",
		},
		OIDCProvider: platform.OIDCProvider{
			Issuer:  "https://token.actions.githubusercontent.com",
			JWKSURI: "https://token.actions.githubusercontent.com/.well-known/jwks",
		},
		Resources: []platform.Resource{
			{
				ID:            "acme/api",
				Name:          "api",
				FullName:      "acme/api",
				DefaultBranch: "main",
				Visibility:    "private",
				Type:          "repository",
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
