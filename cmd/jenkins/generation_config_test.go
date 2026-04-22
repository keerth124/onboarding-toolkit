package jenkins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyberark/conjur-onboard/internal/conjur"
	jenkinsdisc "github.com/cyberark/conjur-onboard/internal/jenkins"
)

func TestJenkinsGenerateConfigProducesArtifacts(t *testing.T) {
	workDir := t.TempDir()
	disc := testJenkinsDiscoveryResultForCommand()

	cfg, err := newJenkinsGenerateConfig(disc, jenkinsGenerateOptions{
		Tenant:           "myco",
		Audience:         jenkinsdisc.DefaultAudience,
		CreateEnabled:    true,
		WorkDir:          workDir,
		ProvisioningMode: "bootstrap",
		Selection: jenkinsdisc.Selection{
			IncludePatterns: []string{"Payments/**"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conjur.Generate(cfg); err != nil {
		t.Fatal(err)
	}

	var body struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Name    string `json:"name"`
		Data    struct {
			Audience string `json:"audience"`
			Identity struct {
				TokenAppProperty string `json:"token_app_property"`
				IdentityPath     string `json:"identity_path"`
			} `json:"identity"`
		} `json:"data"`
	}
	readJSONForJenkinsCommandTest(t, filepath.Join(workDir, "api", "01-create-authenticator.json"), &body)
	if body.Type != "jwt" || body.Subtype != "jenkins" {
		t.Fatalf("authenticator type/subtype = %s/%s", body.Type, body.Subtype)
	}
	if body.Data.Audience != jenkinsdisc.DefaultAudience {
		t.Fatalf("audience = %q", body.Data.Audience)
	}
	if body.Data.Identity.TokenAppProperty != jenkinsdisc.DefaultTokenAppProperty {
		t.Fatalf("token_app_property = %q", body.Data.Identity.TokenAppProperty)
	}
	if body.Data.Identity.IdentityPath != "data/jenkins-apps/jenkins-example-com" {
		t.Fatalf("identity_path = %q", body.Data.Identity.IdentityPath)
	}

	workloadPolicy, err := os.ReadFile(filepath.Join(workDir, "api", "02-workloads.yml"))
	if err != nil {
		t.Fatal(err)
	}
	policy := string(workloadPolicy)
	if !strings.Contains(policy, "id: Payments") || !strings.Contains(policy, "id: Payments/API/deploy") {
		t.Fatalf("workload policy missing selected Jenkins scopes:\n%s", policy)
	}
	if strings.Contains(policy, "Platform/build") {
		t.Fatalf("workload policy included excluded scope:\n%s", policy)
	}
	if _, err := os.Stat(filepath.Join(workDir, "integration", "Jenkinsfile")); err != nil {
		t.Fatalf("Jenkinsfile missing: %v", err)
	}
}

func testJenkinsDiscoveryResultForCommand() *jenkinsdisc.DiscoveryResult {
	return &jenkinsdisc.DiscoveryResult{
		Platform:       "jenkins",
		JenkinsURL:     "https://jenkins.example.com",
		Controller:     "jenkins.example.com",
		ControllerSlug: "jenkins-example-com",
		OIDCIssuer:     "https://jenkins.example.com",
		JWKSURI:        "https://jenkins.example.com/jwtauth/conjur-jwk-set",
		Source:         "api",
		Jobs: []jenkinsdisc.JobInfo{
			{Name: "Payments", FullName: "Payments", Type: "folder"},
			{Name: "deploy", FullName: "Payments/API/deploy", Type: "pipeline", Parent: "Payments/API"},
			{Name: "build", FullName: "Platform/build", Type: "pipeline", Parent: "Platform"},
		},
	}
}

func readJSONForJenkinsCommandTest(t *testing.T, path string, dest any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatal(err)
	}
}
