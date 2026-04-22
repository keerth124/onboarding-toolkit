package shared

import (
	"strings"
	"testing"
)

func TestGlobalFlagsWorkDirForUsesExplicitValue(t *testing.T) {
	workDir := "manual-dir"
	flags := GlobalFlags{WorkDir: &workDir}

	if got := flags.WorkDirFor("github"); got != "manual-dir" {
		t.Fatalf("WorkDirFor() = %q, want manual-dir", got)
	}
}

func TestGlobalFlagsWorkDirForUsesPlatformDefault(t *testing.T) {
	flags := GlobalFlags{}

	got := flags.WorkDirFor("github")
	if !strings.HasPrefix(got, "conjur-onboard-github-") {
		t.Fatalf("WorkDirFor() = %q, want github default prefix", got)
	}
}

func TestConjurConnectionFlagsValidateEndpointRequired(t *testing.T) {
	if err := (ConjurConnectionFlags{}).ValidateEndpointRequired(); err == nil {
		t.Fatal("expected missing endpoint error")
	}
	if err := (ConjurConnectionFlags{Tenant: "myco", ConjurURL: "https://conjur.example.com"}).ValidateEndpointRequired(); err == nil {
		t.Fatal("expected mutually exclusive endpoint error")
	}
	if err := (ConjurConnectionFlags{Tenant: "myco"}).ValidateEndpointRequired(); err != nil {
		t.Fatalf("ValidateEndpointRequired() error = %v", err)
	}
}
