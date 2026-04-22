package jenkins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadJobsFileSupportsScopeTypesAndSpaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.txt")
	content := `# selected Jenkins scopes
GlobalCredentials|global
Payments Team|folder
Payments Team/API Deploy|pipeline
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	jobs, err := LoadJobsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	if jobs[0].FullName != "GlobalCredentials" || jobs[0].Type != "global" {
		t.Fatalf("jobs[0] = %#v, want GlobalCredentials global", jobs[0])
	}
	if jobs[1].FullName != "Payments Team" || jobs[1].Type != "folder" {
		t.Fatalf("jobs[1] = %#v, want Payments Team folder", jobs[1])
	}
	if jobs[2].Parent != "Payments Team" {
		t.Fatalf("jobs[2].Parent = %q, want Payments Team", jobs[2].Parent)
	}
}

func TestDiscoverFromJobsFileDerivesIssuerAndJWKS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.txt")
	if err := os.WriteFile(path, []byte("Payments/API|pipeline\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Discover(nil, DiscoverConfig{
		JenkinsURL:   "https://jenkins.example.com/",
		JobsFromFile: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OIDCIssuer != "https://jenkins.example.com" {
		t.Fatalf("OIDCIssuer = %q, want normalized Jenkins URL", result.OIDCIssuer)
	}
	if result.JWKSURI != "https://jenkins.example.com/jwtauth/conjur-jwk-set" {
		t.Fatalf("JWKSURI = %q", result.JWKSURI)
	}
	if result.Source != "jobs-from-file" {
		t.Fatalf("Source = %q, want jobs-from-file", result.Source)
	}
}
