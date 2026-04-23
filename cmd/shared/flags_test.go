package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

func TestGlobalFlagsWorkDirForUsesConfigValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conjur-onboard.json")
	if err := os.WriteFile(path, []byte(`{"version":"v1alpha1","work_dir":"configured-work","conjur":{"target":"saas","tenant":"myco"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	flags := GlobalFlags{ConfigPath: &path}

	if got := flags.WorkDirFor("github"); got != "configured-work" {
		t.Fatalf("WorkDirFor() = %q, want configured-work", got)
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

func TestAddConjurConnectionFlagsIncludesInsecureTLSFlag(t *testing.T) {
	var conn ConjurConnectionFlags
	cmd := &cobra.Command{Use: "test"}

	AddConjurConnectionFlags(cmd, &conn)
	if cmd.Flags().Lookup("insecure-skip-tls-verify") == nil {
		t.Fatal("missing insecure-skip-tls-verify flag")
	}
	if err := cmd.Flags().Set("insecure-skip-tls-verify", "true"); err != nil {
		t.Fatal(err)
	}
	if !conn.InsecureSkipTLSVerify {
		t.Fatal("InsecureSkipTLSVerify was not set")
	}
}

func TestResolveConjurConnectionUsesConfigDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conjur-onboard.json")
	if err := os.WriteFile(path, []byte(`{"version":"v1alpha1","conjur":{"target":"saas","tenant":"myco","account":"conjur","username":"host/data/tooling","insecure_skip_tls_verify":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	flags := GlobalFlags{ConfigPath: &path}
	cmd := &cobra.Command{Use: "test"}
	var conn ConjurConnectionFlags
	AddConjurConnectionFlags(cmd, &conn)

	if err := ResolveConjurConnection(cmd, flags, &conn); err != nil {
		t.Fatal(err)
	}
	if conn.Tenant != "myco" || conn.Username != "host/data/tooling" || !conn.InsecureSkipTLSVerify {
		t.Fatalf("resolved conn = %#v", conn)
	}
}
